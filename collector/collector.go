// Copyright Christopher Barnes
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package collector includes all individual collectors to gather and export garmin metrics.
package collector

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
)

const namespace = "garmin"

var (
	scrapeDurationDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "scrape", "collector_duration_seconds"),
		"garmin_exporter: Duration of a collector scrape.",
		[]string{"collector"},
		nil,
	)
	scrapeSuccessDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "scrape", "collector_success"),
		"garmin_exporter: Whether a collector succeeded.",
		[]string{"collector"},
		nil,
	)
)

var (
	factories              = make(map[string]func(logger *slog.Logger) (Collector, error))
	initiatedCollectorsMtx = sync.Mutex{}
	initiatedCollectors    = make(map[string]Collector)
	collectorState         = make(map[string]*bool)
	forcedCollectors       = map[string]bool{}
)

// Collector is the interface a collector has to implement.
type Collector interface {
	Update(ch chan<- prometheus.Metric) error
}

// ErrNoData indicates the collector found no data to collect, but had no other error.
var ErrNoData = errors.New("collector returned no data")

// IsNoDataError returns true if err is ErrNoData.
func IsNoDataError(err error) bool {
	return err == ErrNoData
}

// registerCollector registers a new collector with a --collector.<name> CLI flag.
// Call this from an init() function in each collector file.
func registerCollector(name string, isDefaultEnabled bool, factory func(logger *slog.Logger) (Collector, error)) {
	helpDefaultState := "disabled"
	if isDefaultEnabled {
		helpDefaultState = "enabled"
	}
	flagName := fmt.Sprintf("collector.%s", name)
	flagHelp := fmt.Sprintf("Enable the %s collector (default: %s).", name, helpDefaultState)
	defaultValue := fmt.Sprintf("%v", isDefaultEnabled)

	flag := kingpin.Flag(flagName, flagHelp).Default(defaultValue).Action(collectorFlagAction(name)).Bool()
	collectorState[name] = flag
	factories[name] = factory
}

func collectorFlagAction(name string) func(ctx *kingpin.ParseContext) error {
	return func(ctx *kingpin.ParseContext) error {
		forcedCollectors[name] = true
		return nil
	}
}

// GarminCollector implements the prometheus.Collector interface.
type GarminCollector struct {
	Collectors map[string]Collector
	logger     *slog.Logger
}

// DisableDefaultCollectors sets all collectors that have not been explicitly
// enabled via CLI to disabled.
func DisableDefaultCollectors() {
	for c := range collectorState {
		if _, ok := forcedCollectors[c]; !ok {
			*collectorState[c] = false
		}
	}
}

// NewGarminCollector creates a new GarminCollector.
func NewGarminCollector(logger *slog.Logger, filters ...string) (*GarminCollector, error) {
	f := make(map[string]bool)
	for _, filter := range filters {
		enabled, exist := collectorState[filter]
		if !exist {
			return nil, fmt.Errorf("missing collector: %s", filter)
		}
		if !*enabled {
			return nil, fmt.Errorf("disabled collector: %s", filter)
		}
		f[filter] = true
	}

	collectors := make(map[string]Collector)
	initiatedCollectorsMtx.Lock()
	defer initiatedCollectorsMtx.Unlock()
	for key, enabled := range collectorState {
		if !*enabled || (len(f) > 0 && !f[key]) {
			continue
		}
		if c, ok := initiatedCollectors[key]; ok {
			collectors[key] = c
		} else {
			c, err := factories[key](logger.With("collector", key))
			if err != nil {
				return nil, err
			}
			collectors[key] = c
			initiatedCollectors[key] = c
		}
	}
	return &GarminCollector{Collectors: collectors, logger: logger}, nil
}

func (gc GarminCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- scrapeDurationDesc
	ch <- scrapeSuccessDesc
}

func (gc GarminCollector) Collect(ch chan<- prometheus.Metric) {
	var wg sync.WaitGroup
	wg.Add(len(gc.Collectors))
	for name, c := range gc.Collectors {
		go func(name string, c Collector) {
			execute(name, c, ch, gc.logger)
			wg.Done()
		}(name, c)
	}
	wg.Wait()
}

func execute(name string, c Collector, ch chan<- prometheus.Metric, logger *slog.Logger) {
	begin := time.Now()
	err := c.Update(ch)
	duration := time.Since(begin)

	var success float64
	if err != nil {
		if IsNoDataError(err) {
			logger.Debug("collector returned no data", "name", name, "duration_seconds", duration.Seconds(), "err", err)
		} else {
			logger.Error("collector failed", "name", name, "duration_seconds", duration.Seconds(), "err", err)
		}
	} else {
		logger.Debug("collector succeeded", "name", name, "duration_seconds", duration.Seconds())
		success = 1
	}
	ch <- prometheus.MustNewConstMetric(scrapeDurationDesc, prometheus.GaugeValue, duration.Seconds(), name)
	ch <- prometheus.MustNewConstMetric(scrapeSuccessDesc, prometheus.GaugeValue, success, name)
}
