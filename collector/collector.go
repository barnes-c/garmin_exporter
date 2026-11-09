// Package collector includes all individual Garmin sub-collectors. Each one
// reads from a garmin.Source snapshot and registers OTel observable
// instruments on the supplied Meter.
package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

// Collector is the contract each sub-collector implements. Register installs
// the collector's instruments on the supplied Meter and wires their observe
// callback to read from src. Close unregisters everything so a clean
// shutdown leaves no dangling callbacks on the MeterProvider.
type Collector interface {
	Name() string
	Register(meter metric.Meter, src garmin.Source) error
	Close() error
}

// registrar carries the OTel callback handle that every collector needs to
// unregister on shutdown. Collectors embed it so they don't each have to
// reimplement the same Close().
type registrar struct {
	registration metric.Registration
}

// Close unregisters the embedded callback. Safe to call before Register.
func (r *registrar) Close() error {
	if r.registration == nil {
		return nil
	}
	return r.registration.Unregister()
}

var (
	factoriesMu sync.Mutex
	factories   = make(map[string]func(logger *slog.Logger) (Collector, error))
)

// registerCollector adds a sub-collector to the registry. Called from init()
// in each collector file.
func registerCollector(name string, factory func(logger *slog.Logger) (Collector, error)) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	factories[name] = factory
}

// Registered returns the names of every collector known to the registry,
// sorted alphabetically.
func Registered() []string {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	out := make([]string, 0, len(factories))
	for n := range factories {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Group is the live set of sub-collectors. It owns each instance and is the
// surface main.go uses to register everything against a Meter and to close
// cleanly at shutdown.
type Group struct {
	log        *slog.Logger
	collectors map[string]Collector
	src        garmin.Source
	upGauge    metric.Int64ObservableGauge
	upCallback metric.Registration
}

// hasData reports, per collector, whether the last refresh produced the data
// it reads. It backs garmin.collector.up; every collector needs an entry.
var hasData = map[string]func(s *garmin.Snapshot) bool{
	"activities":        func(s *garmin.Snapshot) bool { return s.Activities != nil },
	"blood_pressure":    func(s *garmin.Snapshot) bool { return s.BloodPressure != nil },
	"body":              func(s *garmin.Snapshot) bool { return s.Body != nil },
	"body_composition":  func(s *garmin.Snapshot) bool { return s.BodyComposition != nil },
	"cycling":           func(s *garmin.Snapshot) bool { return s.Cycling != nil },
	"devices":           func(s *garmin.Snapshot) bool { return s.Devices != nil },
	"fitness_age":       func(s *garmin.Snapshot) bool { return s.FitnessAge != nil },
	"gear":              func(s *garmin.Snapshot) bool { return s.Gear != nil },
	"goals":             func(s *garmin.Snapshot) bool { return s.Goals != nil },
	"golf":              func(s *garmin.Snapshot) bool { return s.Golf != nil },
	"heart_rate":        func(s *garmin.Snapshot) bool { return s.HeartRate != nil },
	"hydration":         func(s *garmin.Snapshot) bool { return s.Hydration != nil },
	"intensity":         func(s *garmin.Snapshot) bool { return s.Intensity != nil },
	"lactate_threshold": func(s *garmin.Snapshot) bool { return s.LactateThreshold != nil },
	"personal_records":  func(s *garmin.Snapshot) bool { return s.PersonalRecords != nil },
	"respiration":       func(s *garmin.Snapshot) bool { return s.Respiration != nil },
	"running_tolerance": func(s *garmin.Snapshot) bool { return s.RunningTolerance != nil },
	"sleep":             func(s *garmin.Snapshot) bool { return s.Sleep != nil },
	"spo2":              func(s *garmin.Snapshot) bool { return s.SpO2 != nil },
	"stress":            func(s *garmin.Snapshot) bool { return s.Stress != nil },
	"training":          func(s *garmin.Snapshot) bool { return s.Training != nil },
	"training_status":   func(s *garmin.Snapshot) bool { return s.TrainingStatus != nil },
	"wellness":          func(s *garmin.Snapshot) bool { return s.UserSummary != nil },
}

// NewGroup instantiates every registered collector. If filters is non-empty,
// the result is restricted to that named subset; an unknown name is an error.
func NewGroup(logger *slog.Logger, filters ...string) (*Group, error) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()

	for name := range factories {
		if hasData[name] == nil {
			return nil, fmt.Errorf("collector %s has no garmin.collector.up check", name)
		}
	}

	filterSet := make(map[string]bool, len(filters))
	for _, f := range filters {
		if _, ok := factories[f]; !ok {
			return nil, fmt.Errorf("unknown collector: %s", f)
		}
		filterSet[f] = true
	}

	out := make(map[string]Collector)
	for name, factory := range factories {
		if len(filterSet) > 0 && !filterSet[name] {
			continue
		}
		c, err := factory(logger.With("collector", name))
		if err != nil {
			return nil, fmt.Errorf("instantiate %s: %w", name, err)
		}
		out[name] = c
	}
	return &Group{log: logger, collectors: out}, nil
}

// RegisterAll calls Register on every collector in the group, then registers
// a shared garmin.collector.up gauge that is 1 for every registered collector.
func (g *Group) RegisterAll(meter metric.Meter, src garmin.Source) error {
	for name, c := range g.collectors {
		if err := c.Register(meter, src); err != nil {
			return fmt.Errorf("register %s: %w", name, err)
		}
	}

	if len(g.collectors) == 0 {
		return nil
	}
	g.src = src

	var err error
	g.upGauge, err = meter.Int64ObservableGauge(
		"garmin.collector.up",
		metric.WithDescription("1 if the collector has data from the last refresh, 0 if its Garmin call failed or returned nothing."),
	)
	if err != nil {
		return fmt.Errorf("create garmin.collector.up: %w", err)
	}
	g.upCallback, err = meter.RegisterCallback(g.observeUp, g.upGauge)
	if err != nil {
		return fmt.Errorf("register garmin.collector.up callback: %w", err)
	}
	return nil
}

func (g *Group) observeUp(_ context.Context, o metric.Observer) error {
	snap := g.src.Snapshot()
	for name := range g.collectors {
		var up int64
		if snap != nil && hasData[name](snap) {
			up = 1
		}
		o.ObserveInt64(g.upGauge, up, metric.WithAttributes(attribute.String("collector", name)))
	}
	return nil
}

// Close unregisters the shared up callback and closes every collector.
func (g *Group) Close() error {
	var errs []error
	if g.upCallback != nil {
		if err := g.upCallback.Unregister(); err != nil {
			errs = append(errs, fmt.Errorf("unregister garmin.collector.up: %w", err))
		}
	}
	for name, c := range g.collectors {
		if err := c.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

// Names returns the collector names in sorted order.
func (g *Group) Names() []string {
	out := make([]string, 0, len(g.collectors))
	for n := range g.collectors {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
