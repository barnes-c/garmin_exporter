// Package collector includes all individual collectors to gather and export system metrics.
package collector

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/barnes-c/go-garminconnect/garminconnect"
	"github.com/prometheus/client_golang/prometheus"
)

// Namespace defines the common namespace to be used by all metrics.
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

const (
	defaultEnabled  = true
	defaultDisabled = false
)

var (
	factories              = make(map[string]func(logger *slog.Logger) (Collector, error))
	initiatedCollectorsMtx = sync.Mutex{}
	initiatedCollectors    = make(map[string]Collector)
	collectorState         = make(map[string]*bool)
	forcedCollectors       = map[string]bool{} // collectors which have been explicitly enabled or disabled

	garminClientMtx sync.RWMutex
	garminClient    *garminconnect.Client
	activityLimit   = 30

	reauthMu sync.Mutex
	reauthCh chan<- struct{}

	scrapeRecorderMu sync.RWMutex
	scrapeRecorder   func(success bool)
)

// SetScrapeRecorder registers a callback invoked after each scrape with
// whether the scrape produced data from at least one collector.
func SetScrapeRecorder(fn func(success bool)) {
	scrapeRecorderMu.Lock()
	defer scrapeRecorderMu.Unlock()
	scrapeRecorder = fn
}

func recordScrape(success bool) {
	scrapeRecorderMu.RLock()
	fn := scrapeRecorder
	scrapeRecorderMu.RUnlock()
	if fn != nil {
		fn(success)
	}
}

// SetReauthChannel registers a buffered channel that receives a signal
// whenever a collector encounters ErrUnauthorized, triggering a re-login.
func SetReauthChannel(ch chan<- struct{}) {
	reauthMu.Lock()
	defer reauthMu.Unlock()
	reauthCh = ch
}

func signalReauth() {
	reauthMu.Lock()
	ch := reauthCh
	reauthMu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default: // already signalled, don't block
	}
}

// SetClient sets the Garmin API client used by all collectors.
func SetClient(c *garminconnect.Client) {
	garminClientMtx.Lock()
	defer garminClientMtx.Unlock()

	garminClient = c
}

func getClient() *garminconnect.Client {
	garminClientMtx.RLock()
	defer garminClientMtx.RUnlock()

	return garminClient
}

// SetActivityLimit sets how many recent activities collectors should fetch.
func SetActivityLimit(n int) { activityLimit = n }

func registerCollector(collector string, isDefaultEnabled bool, factory func(logger *slog.Logger) (Collector, error)) {
	var helpDefaultState string
	if isDefaultEnabled {
		helpDefaultState = "enabled"
	} else {
		helpDefaultState = "disabled"
	}

	flagName := fmt.Sprintf("collector.%s", collector)
	flagHelp := fmt.Sprintf("Enable the %s collector (default: %s).", collector, helpDefaultState)
	defaultValue := fmt.Sprintf("%v", isDefaultEnabled)

	flag := kingpin.Flag(flagName, flagHelp).Default(defaultValue).Action(collectorFlagAction(collector)).Bool()
	collectorState[collector] = flag

	factories[collector] = factory
}

// GarminCollector implements the prometheus.Collector interface.
type GarminCollector struct {
	Collectors map[string]Collector
	logger     *slog.Logger
}

// DisableDefaultCollectors sets the collector state to false for all collectors which
// have not been explicitly enabled on the command line.
func DisableDefaultCollectors() {
	for c := range collectorState {
		if _, ok := forcedCollectors[c]; !ok {
			*collectorState[c] = false
		}
	}
}

// collectorFlagAction generates a new action function for the given collector
// to track whether it has been explicitly enabled or disabled from the command line.
// A new action function is needed for each collector flag because the ParseContext
// does not contain information about which flag called the action.
// See: https://github.com/alecthomas/kingpin/issues/294
func collectorFlagAction(collector string) func(ctx *kingpin.ParseContext) error {
	return func(ctx *kingpin.ParseContext) error {
		forcedCollectors[collector] = true
		return nil
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
		if collector, ok := initiatedCollectors[key]; ok {
			collectors[key] = collector
		} else {
			collector, err := factories[key](logger.With("collector", key))
			if err != nil {
				return nil, err
			}
			collectors[key] = collector
			initiatedCollectors[key] = collector
		}
	}
	return &GarminCollector{Collectors: collectors, logger: logger}, nil
}

// Describe implements the prometheus.Collector interface.
func (n GarminCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- scrapeDurationDesc
	ch <- scrapeSuccessDesc
}

// Collect implements the prometheus.Collector interface.
func (n GarminCollector) Collect(ch chan<- prometheus.Metric) {
	wg := sync.WaitGroup{}
	wg.Add(len(n.Collectors))
	var ok atomic.Int32
	for name, c := range n.Collectors {
		go func(name string, c Collector) {
			defer wg.Done()
			if execute(name, c, ch, n.logger) {
				ok.Add(1)
			}
		}(name, c)
	}
	wg.Wait()
	recordScrape(ok.Load() > 0)
}

// execute runs a single collector and returns true if it ran without a real
// error. ErrNoData counts as "ran without error" — the collector worked,
// there was simply nothing to expose.
func execute(name string, c Collector, ch chan<- prometheus.Metric, logger *slog.Logger) bool {
	begin := time.Now()
	err := c.Update(ch)
	duration := time.Since(begin)
	var success float64
	ok := true

	if err != nil {
		if IsNoDataError(err) {
			logger.Debug("collector returned no data", "name", name, "duration_seconds", duration.Seconds(), "err", err)
		} else {
			logger.Error("collector failed", "name", name, "duration_seconds", duration.Seconds(), "err", err)
			if errors.Is(err, garminconnect.ErrUnauthorized) {
				signalReauth()
			}
			ok = false
		}
		success = 0
	} else {
		logger.Debug("collector succeeded", "name", name, "duration_seconds", duration.Seconds())
		success = 1
	}
	ch <- prometheus.MustNewConstMetric(scrapeDurationDesc, prometheus.GaugeValue, duration.Seconds(), name)
	ch <- prometheus.MustNewConstMetric(scrapeSuccessDesc, prometheus.GaugeValue, success, name)
	return ok
}

// Collector is the interface a collector has to implement.
type Collector interface {
	// Get new metrics and expose them via prometheus registry.
	Update(ch chan<- prometheus.Metric) error
}

// ErrNoData indicates the collector found no data to collect, but had no other error.
var ErrNoData = errors.New("collector returned no data")

func IsNoDataError(err error) bool {
	return err == ErrNoData
}
