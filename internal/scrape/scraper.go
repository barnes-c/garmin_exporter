package scrape

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var (
	scrapeDurationDesc = prometheus.NewDesc(
		prometheus.BuildFQName("garmin", "scrape", "collector_duration_seconds"),
		"garmin_exporter: Duration of a collector scrape.",
		[]string{"collector"},
		nil,
	)
	scrapeSuccessDesc = prometheus.NewDesc(
		prometheus.BuildFQName("garmin", "scrape", "collector_success"),
		"garmin_exporter: Whether a collector succeeded.",
		[]string{"collector"},
		nil,
	)

	cacheAgeDesc = prometheus.NewDesc(
		prometheus.BuildFQName("garmin", "cache", "age_seconds"),
		"garmin_exporter: Seconds since the most recent cache refresh; 0 before the first refresh.",
		nil,
		nil,
	)
	cacheInFlightDesc = prometheus.NewDesc(
		prometheus.BuildFQName("garmin", "cache", "refresh_in_flight"),
		"garmin_exporter: 1 if a cache refresh is currently running, 0 otherwise.",
		nil,
		nil,
	)
)

const (
	refreshResultSuccess = "success"
	refreshResultFailure = "failure"
	refreshResultSkipped = "skipped"
)

// Config configures a Scraper.
type Config struct {
	// TTL is how often the background loop refreshes data from Garmin.
	TTL time.Duration
	// Logger is used for refresh-loop diagnostics.
	Logger *slog.Logger
	// BuildCollectors returns a fresh map of Garmin sub-collectors keyed by
	// name. Called once per refresh. Each value should implement
	// LastErrorReporter so the scraper can distinguish real failures from
	// "no data" outcomes; if not implemented, success is inferred from
	// Gather() error alone.
	BuildCollectors func() (map[string]prometheus.Collector, error)
	// AuthReady blocks until authentication has succeeded at least once, or
	// returns ctx.Err() if ctx is cancelled. The first refresh is delayed
	// until this returns nil.
	AuthReady func(ctx context.Context) error
	// OnScrape is invoked after each refresh with whether the refresh
	// produced data from at least one collector.
	OnScrape func(success bool)
}

// LastErrorReporter exposes the most recent collector error. Optional but
// recommended; allows distinguishing real failures from "no data" responses.
type LastErrorReporter interface {
	LastError() error
}

type snapshot struct {
	all     []*dto.MetricFamily
	byColl  map[string][]*dto.MetricFamily
	builtAt time.Time
}

// Scraper owns the cached snapshot and the refresh loop.
type Scraper struct {
	cfg     Config
	current atomic.Pointer[snapshot]
	sem     chan struct{}

	refreshInFlight atomic.Int64
	refreshTotal    *prometheus.CounterVec
	refreshDuration prometheus.Histogram
}

// New constructs a Scraper. It does not start the refresh loop; call Run.
func New(cfg Config) *Scraper {
	s := &Scraper{
		cfg: cfg,
		sem: make(chan struct{}, 1),
		refreshTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "garmin",
				Subsystem: "cache",
				Name:      "refresh_total",
				Help:      "garmin_exporter: Number of cache refresh attempts, labelled by result.",
			},
			[]string{"result"},
		),
		refreshDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "garmin",
			Subsystem: "cache",
			Name:      "refresh_duration_seconds",
			Help:      "garmin_exporter: Time spent rebuilding the cache snapshot.",
			Buckets:   []float64{0.1, 0.5, 1, 2.5, 5, 10, 15, 30, 60},
		}),
	}
	// Pre-register the three label values so dashboards see a zero series
	// even before the first refresh of each kind.
	for _, result := range []string{refreshResultSuccess, refreshResultFailure, refreshResultSkipped} {
		s.refreshTotal.WithLabelValues(result)
	}
	return s
}

// Run blocks until ctx is cancelled. It waits for AuthReady to signal the
// first successful login, performs an initial refresh, then refreshes on a
// TTL ticker. In-flight refreshes are dropped (with a warning) if a new
// tick arrives while the previous refresh is still running.
func (s *Scraper) Run(ctx context.Context) {
	if err := s.cfg.AuthReady(ctx); err != nil {
		return
	}
	s.refreshOnce(ctx)

	t := time.NewTicker(s.cfg.TTL)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			go s.refreshOnce(ctx)
		}
	}
}

// Refresh runs a single refresh synchronously. Returns nil if it ran or if
// another refresh was already in flight (in which case it returns
// immediately without waiting). Useful for tests and an eventual admin
// "/refresh" endpoint.
func (s *Scraper) Refresh(ctx context.Context) error {
	s.refreshOnce(ctx)
	return nil
}

func (s *Scraper) refreshOnce(ctx context.Context) {
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	default:
		s.cfg.Logger.Warn("skipping refresh: previous still running")
		s.refreshTotal.WithLabelValues(refreshResultSkipped).Inc()
		return
	}
	s.refreshInFlight.Store(1)
	defer s.refreshInFlight.Store(0)
	start := time.Now()
	defer func() { s.refreshDuration.Observe(time.Since(start).Seconds()) }()

	tracer := otel.Tracer("garmin_exporter/scrape")
	ctx, rootSpan := tracer.Start(ctx, "garmin.refresh")
	defer rootSpan.End()

	cols, err := s.cfg.BuildCollectors()
	if err != nil {
		rootSpan.RecordError(err)
		rootSpan.SetStatus(codes.Error, "build collectors failed")
		s.cfg.Logger.Error("build collectors", "err", err)
		s.cfg.OnScrape(false)
		s.refreshTotal.WithLabelValues(refreshResultFailure).Inc()
		return
	}

	rootSpan.SetAttributes(attribute.Int("garmin.collectors.count", len(cols)))

	type result struct {
		name     string
		families []*dto.MetricFamily
		duration time.Duration
		success  bool
	}
	results := make([]result, 0, len(cols))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for name, c := range cols {
		wg.Add(1)
		go func(name string, c prometheus.Collector) {
			defer wg.Done()
			_, collSpan := tracer.Start(ctx, "garmin.collect."+name)
			defer collSpan.End()

			r := result{name: name}
			if cs, ok := c.(contextSetter); ok {
				cs.SetContext(ctx)
			}
			reg := prometheus.NewRegistry()
			if regErr := reg.Register(c); regErr != nil {
				collSpan.RecordError(regErr)
				collSpan.SetStatus(codes.Error, "register failed")
				s.cfg.Logger.Error("register collector", "name", name, "err", regErr)
				mu.Lock()
				results = append(results, r)
				mu.Unlock()
				return
			}
			start := time.Now()
			families, gErr := reg.Gather()
			r.duration = time.Since(start)
			r.families = families
			r.success = gErr == nil
			if reporter, ok := c.(LastErrorReporter); ok {
				if reporter.LastError() != nil {
					r.success = false
				}
			}
			if !r.success {
				if gErr != nil {
					collSpan.RecordError(gErr)
				}
				collSpan.SetStatus(codes.Error, "collect failed")
			}
			collSpan.SetAttributes(
				attribute.Bool("garmin.collector.success", r.success),
				attribute.Float64("garmin.collector.duration_seconds", r.duration.Seconds()),
			)
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}(name, c)
	}
	wg.Wait()

	if ctx.Err() != nil {
		return
	}

	snap := &snapshot{
		byColl:  make(map[string][]*dto.MetricFamily, len(results)),
		builtAt: time.Now(),
	}
	anySuccess := false
	for _, r := range results {
		successVal := 0.0
		if r.success {
			successVal = 1.0
			anySuccess = true
		}
		orchestration := []prometheus.Metric{
			prometheus.MustNewConstMetric(scrapeDurationDesc, prometheus.GaugeValue, r.duration.Seconds(), r.name),
			prometheus.MustNewConstMetric(scrapeSuccessDesc, prometheus.GaugeValue, successVal, r.name),
		}
		orchFamilies := gatherMetrics(orchestration)
		merged := make([]*dto.MetricFamily, 0, len(r.families)+len(orchFamilies))
		merged = append(merged, r.families...)
		merged = append(merged, orchFamilies...)
		snap.byColl[r.name] = merged
	}
	snap.all = mergeFamilies(snap.byColl)

	s.current.Store(snap)
	s.cfg.OnScrape(anySuccess)
	if anySuccess {
		s.refreshTotal.WithLabelValues(refreshResultSuccess).Inc()
	} else {
		s.refreshTotal.WithLabelValues(refreshResultFailure).Inc()
	}
	rootSpan.SetAttributes(attribute.Bool("garmin.refresh.success", anySuccess))
	if !anySuccess {
		rootSpan.SetStatus(codes.Error, "no collector succeeded")
	}
	s.cfg.Logger.Debug("refresh complete", "duration_seconds", time.Since(snap.builtAt).Seconds(), "success", anySuccess)
}

// Gatherer returns a Gatherer that serves the most recent snapshot. Before
// the first refresh completes it returns (nil, nil).
func (s *Scraper) Gatherer() prometheus.Gatherer {
	return gathererFunc(func() ([]*dto.MetricFamily, error) {
		snap := s.current.Load()
		if snap == nil {
			return nil, nil
		}
		return snap.all, nil
	})
}

// Describe implements prometheus.Collector. The Scraper exposes its own
// cache-observability metrics; the cached Garmin data is exposed via
// Gatherer instead.
func (s *Scraper) Describe(ch chan<- *prometheus.Desc) {
	ch <- cacheAgeDesc
	ch <- cacheInFlightDesc
	s.refreshTotal.Describe(ch)
	s.refreshDuration.Describe(ch)
}

// Collect implements prometheus.Collector.
func (s *Scraper) Collect(ch chan<- prometheus.Metric) {
	var age float64
	if snap := s.current.Load(); snap != nil {
		age = time.Since(snap.builtAt).Seconds()
	}
	ch <- prometheus.MustNewConstMetric(cacheAgeDesc, prometheus.GaugeValue, age)
	ch <- prometheus.MustNewConstMetric(cacheInFlightDesc, prometheus.GaugeValue, float64(s.refreshInFlight.Load()))
	s.refreshTotal.Collect(ch)
	s.refreshDuration.Collect(ch)
}

// FilteredGatherer returns a Gatherer that serves only the families
// belonging to the named collectors. Unknown names are silently ignored.
func (s *Scraper) FilteredGatherer(names []string) prometheus.Gatherer {
	wanted := make(map[string]struct{}, len(names))
	for _, n := range names {
		wanted[n] = struct{}{}
	}
	return gathererFunc(func() ([]*dto.MetricFamily, error) {
		snap := s.current.Load()
		if snap == nil {
			return nil, nil
		}
		buckets := make(map[string][]*dto.MetricFamily, len(wanted))
		for name := range wanted {
			if fams, ok := snap.byColl[name]; ok {
				buckets[name] = fams
			}
		}
		return mergeFamilies(buckets), nil
	})
}

// contextSetter is implemented by collector adapters that accept a context
// before Gather is called.
type contextSetter interface {
	SetContext(context.Context)
}

type gathererFunc func() ([]*dto.MetricFamily, error)

func (f gathererFunc) Gather() ([]*dto.MetricFamily, error) { return f() }

// gatherMetrics registers the given metrics in a throw-away registry and
// returns the resulting MetricFamilies, so they can be merged into a
// snapshot alongside families from collector Gather() calls.
func gatherMetrics(metrics []prometheus.Metric) []*dto.MetricFamily {
	reg := prometheus.NewRegistry()
	reg.MustRegister(&constCollector{metrics: metrics})
	families, _ := reg.Gather()
	return families
}

type constCollector struct {
	metrics []prometheus.Metric
}

func (c *constCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range c.metrics {
		ch <- m.Desc()
	}
}

func (c *constCollector) Collect(ch chan<- prometheus.Metric) {
	for _, m := range c.metrics {
		ch <- m
	}
}

// mergeFamilies merges per-collector MetricFamily slices into a single
// slice, combining metrics of families that share a fully-qualified name.
// Order is not guaranteed.
func mergeFamilies(byColl map[string][]*dto.MetricFamily) []*dto.MetricFamily {
	byName := make(map[string]*dto.MetricFamily)
	for _, fams := range byColl {
		for _, f := range fams {
			name := f.GetName()
			if existing, ok := byName[name]; ok {
				existing.Metric = append(existing.Metric, f.Metric...)
				continue
			}
			byName[name] = &dto.MetricFamily{
				Name:   f.Name,
				Help:   f.Help,
				Type:   f.Type,
				Unit:   f.Unit,
				Metric: append([]*dto.Metric(nil), f.Metric...),
			}
		}
	}
	out := make([]*dto.MetricFamily, 0, len(byName))
	for _, f := range byName {
		out = append(out, f)
	}
	return out
}
