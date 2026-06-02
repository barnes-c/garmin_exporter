// Package collector includes all individual collectors to gather and export system metrics.
package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/alecthomas/kingpin/v2"
	"github.com/barnes-c/go-garminconnect/garminconnect"
	"github.com/prometheus/client_golang/prometheus"
)

// Namespace defines the common namespace to be used by all metrics.
const namespace = "garmin"

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
)

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

// GarminCollector holds the set of enabled Garmin sub-collectors.
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

// Each returned value also implements LastErrorReporter.
func (n *GarminCollector) PromCollectors(opts ...AdapterOption) map[string]prometheus.Collector {
	out := make(map[string]prometheus.Collector, len(n.Collectors))
	for name, c := range n.Collectors {
		out[name] = newAdapter(c, opts...)
	}
	return out
}

// AdapterOption configures an individual sub-collector adapter.
type AdapterOption func(*adapter)

// WithUnauthorizedHandler installs a callback that is invoked when a
// collector reports an ErrUnauthorized error during Update.
func WithUnauthorizedHandler(fn func()) AdapterOption {
	return func(a *adapter) { a.onUnauthorized = fn }
}

// LastErrorReporter exposes the most recent Update error from a Garmin
// sub-collector wrapped via PromCollectors.
type LastErrorReporter interface {
	LastError() error
}

type adapter struct {
	inner          Collector
	mu             sync.Mutex
	err            error
	onUnauthorized func()
	ctx            context.Context
}

// SetContext stores ctx so it is available during the next Collect call.
// Called by the scraper before reg.Gather() to thread request context through.
func (a *adapter) SetContext(ctx context.Context) {
	a.ctx = ctx
}

func newAdapter(c Collector, opts ...AdapterOption) *adapter {
	a := &adapter{inner: c}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func (a *adapter) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(a, ch)
}

func (a *adapter) Collect(ch chan<- prometheus.Metric) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	err := a.inner.Update(ctx, ch)
	a.mu.Lock()
	if err != nil && !IsNoDataError(err) {
		a.err = err
		if errors.Is(err, garminconnect.ErrUnauthorized) && a.onUnauthorized != nil {
			a.onUnauthorized()
		}
	} else {
		a.err = nil
	}
	a.mu.Unlock()
}

// LastError returns the most recent non-nil error reported by Update.
func (a *adapter) LastError() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.err
}

// Collector is the interface a collector has to implement.
type Collector interface {
	// Get new metrics and expose them via prometheus registry.
	Update(ctx context.Context, ch chan<- prometheus.Metric) error
}

// ErrNoData indicates the collector found no data to collect, but had no other error.
var ErrNoData = errors.New("collector returned no data")

func IsNoDataError(err error) bool {
	return err == ErrNoData
}
