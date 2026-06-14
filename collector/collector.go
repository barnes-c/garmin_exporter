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

// DepCheck reports whether a collector's data dependency is currently
// available. Declared once at registerCollector time and consulted by
// Group when emitting garmin.collector.up; lets alerts distinguish "no
// data" from "actually zero" without each collector implementing the
// same predicate.
type DepCheck func(garmin.Source) bool

// SnapshotAvailable is the dep check for collectors that only need any
// snapshot to exist (i.e. scraper has run at least once).
var SnapshotAvailable DepCheck = func(s garmin.Source) bool {
	return s != nil && s.Snapshot() != nil
}

// SnapshotHas builds a DepCheck that requires a non-nil snapshot and a
// specific sub-field within it. Mirrors ovs-exporter's UnixctlHas pattern
// for the Garmin best-effort per-endpoint fetch.
func SnapshotHas(f func(*garmin.Snapshot) bool) DepCheck {
	return func(s garmin.Source) bool {
		if s == nil {
			return false
		}
		snap := s.Snapshot()
		return snap != nil && f(snap)
	}
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
	factoriesMu   sync.Mutex
	factories     = make(map[string]func(logger *slog.Logger) (Collector, error))
	collectorDeps = make(map[string]DepCheck)
)

// registerCollector adds a sub-collector to the registry. Called from init()
// in each collector file. dep is the data-availability predicate consulted by
// the garmin.collector.up gauge.
func registerCollector(name string, factory func(logger *slog.Logger) (Collector, error), dep DepCheck) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	factories[name] = factory
	collectorDeps[name] = dep
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
	deps       map[string]DepCheck
	src        garmin.Source
	upGauge    metric.Int64ObservableGauge
	upCallback metric.Registration
}

// NewGroup instantiates every registered collector. If filters is non-empty,
// the result is restricted to that named subset; an unknown name is an error.
func NewGroup(logger *slog.Logger, filters ...string) (*Group, error) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()

	filterSet := make(map[string]bool, len(filters))
	for _, f := range filters {
		if _, ok := factories[f]; !ok {
			return nil, fmt.Errorf("unknown collector: %s", f)
		}
		filterSet[f] = true
	}

	out := make(map[string]Collector)
	deps := make(map[string]DepCheck)
	for name, factory := range factories {
		if len(filterSet) > 0 && !filterSet[name] {
			continue
		}
		c, err := factory(logger.With("collector", name))
		if err != nil {
			return nil, fmt.Errorf("instantiate %s: %w", name, err)
		}
		out[name] = c
		deps[name] = collectorDeps[name]
	}
	return &Group{log: logger, collectors: out, deps: deps}, nil
}

// RegisterAll calls Register on every collector in the group, then registers
// a shared garmin.collector.up gauge whose value is driven by each
// collector's registry-declared DepCheck.
func (g *Group) RegisterAll(meter metric.Meter, src garmin.Source) error {
	g.src = src
	for name, c := range g.collectors {
		if err := c.Register(meter, src); err != nil {
			return fmt.Errorf("register %s: %w", name, err)
		}
	}

	if len(g.collectors) == 0 {
		return nil
	}

	var err error
	g.upGauge, err = meter.Int64ObservableGauge(
		"garmin.collector.up",
		metric.WithDescription("1 if the collector's data dependency is currently available; 0 otherwise."),
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
	for name := range g.collectors {
		v := int64(0)
		if dep := g.deps[name]; dep != nil && dep(g.src) {
			v = 1
		}
		o.ObserveInt64(g.upGauge, v, metric.WithAttributes(attribute.String("collector", name)))
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
