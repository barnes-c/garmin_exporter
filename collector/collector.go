// Package collector includes all individual Garmin sub-collectors. Each one
// reads from a garmin.Source snapshot and registers OTel observable
// instruments on the supplied Meter.
package collector

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/alecthomas/kingpin/v2"
	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

const (
	DefaultEnabled  = true
	DefaultDisabled = false
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

var (
	factoriesMu      sync.Mutex
	factories        = make(map[string]func(logger *slog.Logger) (Collector, error))
	collectorState   = make(map[string]*bool)
	forcedCollectors = make(map[string]bool)
)

// registerCollector adds a sub-collector to the registry and declares its
// --collector.<name> flag. Called from init() in each collector file. The
// flag's Action records the collector as "forced" so DisableDefaultCollectors
// knows to leave operator-set values alone.
func registerCollector(name string, isDefaultEnabled bool, factory func(logger *slog.Logger) (Collector, error)) {
	helpDefaultState := "disabled"
	if isDefaultEnabled {
		helpDefaultState = "enabled"
	}
	flagName := fmt.Sprintf("collector.%s", name)
	flagHelp := fmt.Sprintf("Enable the %s collector (default: %s).", name, helpDefaultState)
	defaultValue := fmt.Sprintf("%v", isDefaultEnabled)

	flag := kingpin.Flag(flagName, flagHelp).
		Default(defaultValue).
		Action(collectorFlagAction(name)).
		Bool()

	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	collectorState[name] = flag
	factories[name] = factory
}

// collectorFlagAction tags a collector as explicitly set by the operator so
// DisableDefaultCollectors does not override it.
func collectorFlagAction(name string) func(*kingpin.ParseContext) error {
	return func(*kingpin.ParseContext) error {
		factoriesMu.Lock()
		forcedCollectors[name] = true
		factoriesMu.Unlock()
		return nil
	}
}

// DisableDefaultCollectors flips every non-explicitly-set collector to
// disabled. Used by --collector.disable-defaults to switch into opt-in mode.
func DisableDefaultCollectors() {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	for name, state := range collectorState {
		if !forcedCollectors[name] {
			*state = false
		}
	}
}

// Registered returns the names of every collector known to the registry,
// regardless of enable state. Sorted alphabetically.
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

// Group is the live set of enabled sub-collectors. It owns each instance and
// is the surface main.go uses to register everything against a Meter and to
// close cleanly at shutdown.
type Group struct {
	log        *slog.Logger
	collectors map[string]Collector
}

// NewGroup instantiates every enabled collector. If filters is non-empty,
// the result is restricted to that subset; filtering an unknown or disabled
// collector is an error.
func NewGroup(logger *slog.Logger, filters ...string) (*Group, error) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()

	filterSet := make(map[string]bool, len(filters))
	for _, f := range filters {
		state, ok := collectorState[f]
		if !ok {
			return nil, fmt.Errorf("unknown collector: %s", f)
		}
		if !*state {
			return nil, fmt.Errorf("disabled collector: %s", f)
		}
		filterSet[f] = true
	}

	out := make(map[string]Collector)
	for name, state := range collectorState {
		if !*state {
			continue
		}
		if len(filterSet) > 0 && !filterSet[name] {
			continue
		}
		c, err := factories[name](logger.With("collector", name))
		if err != nil {
			return nil, fmt.Errorf("instantiate %s: %w", name, err)
		}
		out[name] = c
	}
	return &Group{log: logger, collectors: out}, nil
}

// RegisterAll calls Register on every collector in the group, passing the
// supplied Meter and Source. Stops and returns on the first error.
func (g *Group) RegisterAll(meter metric.Meter, src garmin.Source) error {
	for name, c := range g.collectors {
		if err := c.Register(meter, src); err != nil {
			return fmt.Errorf("register %s: %w", name, err)
		}
	}
	return nil
}

// Close calls Close on every collector and joins their errors.
func (g *Group) Close() error {
	var errs []error
	for name, c := range g.collectors {
		if err := c.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

// Names returns the enabled collector names in sorted order. Used for filter
// validation and landing-page logging.
func (g *Group) Names() []string {
	out := make([]string, 0, len(g.collectors))
	for n := range g.collectors {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
