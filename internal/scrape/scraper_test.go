package scrape

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// counterCollector exposes a single gauge with a configurable value.
type counterCollector struct {
	name string
	desc *prometheus.Desc
	val  atomic.Int64
}

func newCounterCollector(name string) *counterCollector {
	return &counterCollector{
		name: name,
		desc: prometheus.NewDesc("test_"+name+"_value", "value", nil, nil),
	}
}

func (c *counterCollector) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }
func (c *counterCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(c.val.Load()))
}

func TestGatherBeforeFirstRefresh(t *testing.T) {
	s := New(Config{Logger: discardLogger()})
	families, err := s.Gatherer().Gather()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(families) != 0 {
		t.Fatalf("expected empty families, got %d", len(families))
	}
}

func TestRefreshAtomicSwap(t *testing.T) {
	c := newCounterCollector("a")
	c.val.Store(1)
	s := New(Config{
		Logger: discardLogger(),
		BuildCollectors: func() (map[string]prometheus.Collector, error) {
			return map[string]prometheus.Collector{"a": c}, nil
		},
		OnScrape: func(bool) {},
	})

	// Stress: read while refreshing.
	stop := make(chan struct{})
	var readErr atomic.Pointer[error]
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				if _, err := s.Gatherer().Gather(); err != nil {
					readErr.Store(&err)
					return
				}
			}
		}
	}()

	ctx := context.Background()
	for i := int64(1); i <= 50; i++ {
		c.val.Store(i)
		s.Refresh(ctx)
	}
	close(stop)
	if errPtr := readErr.Load(); errPtr != nil {
		t.Fatalf("reader saw error during refresh: %v", *errPtr)
	}
	families, _ := s.Gatherer().Gather()
	if len(families) == 0 {
		t.Fatal("expected non-empty families after refresh")
	}
}

func TestRunWaitsForAuth(t *testing.T) {
	buildCalls := atomic.Int32{}
	authReady := make(chan struct{})
	s := New(Config{
		TTL:    time.Hour,
		Logger: discardLogger(),
		BuildCollectors: func() (map[string]prometheus.Collector, error) {
			buildCalls.Add(1)
			return map[string]prometheus.Collector{}, nil
		},
		AuthReady: func(ctx context.Context) error {
			select {
			case <-authReady:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		OnScrape: func(bool) {},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	// Give the loop time to start; assert no build call before auth.
	time.Sleep(20 * time.Millisecond)
	if buildCalls.Load() != 0 {
		t.Fatalf("expected no refresh before auth ready, got %d", buildCalls.Load())
	}

	close(authReady)
	// Expect exactly one initial refresh.
	deadline := time.After(time.Second)
	for buildCalls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("no refresh after auth ready")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	cancel()
	<-done
	if got := buildCalls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 build call, got %d", got)
	}
}

func TestOverlapProtection(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	buildCalls := atomic.Int32{}
	s := New(Config{
		Logger: discardLogger(),
		BuildCollectors: func() (map[string]prometheus.Collector, error) {
			buildCalls.Add(1)
			started <- struct{}{}
			<-release
			return map[string]prometheus.Collector{}, nil
		},
		OnScrape: func(bool) {},
	})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); s.Refresh(context.Background()) }()
	// Wait for the first refresh to actually be inside BuildCollectors.
	<-started
	go func() { defer wg.Done(); s.Refresh(context.Background()) }()
	// The second call should return immediately due to semaphore.
	time.Sleep(20 * time.Millisecond)
	if got := buildCalls.Load(); got != 1 {
		t.Fatalf("expected only 1 build during overlap, got %d", got)
	}
	close(release)
	wg.Wait()
}

func TestOnScrapeWired(t *testing.T) {
	tests := []struct {
		name        string
		collectors  map[string]prometheus.Collector
		wantSuccess bool
	}{
		{
			name:        "no data emits failure",
			collectors:  map[string]prometheus.Collector{},
			wantSuccess: false,
		},
		{
			name:        "data emits success",
			collectors:  map[string]prometheus.Collector{"a": newCounterCollector("a")},
			wantSuccess: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got atomic.Bool
			var called atomic.Bool
			s := New(Config{
				Logger:          discardLogger(),
				BuildCollectors: func() (map[string]prometheus.Collector, error) { return tc.collectors, nil },
				OnScrape:        func(b bool) { got.Store(b); called.Store(true) },
			})
			s.Refresh(context.Background())
			if !called.Load() {
				t.Fatal("OnScrape never called")
			}
			if got.Load() != tc.wantSuccess {
				t.Fatalf("OnScrape: want %v, got %v", tc.wantSuccess, got.Load())
			}
		})
	}
}

func TestRunCtxCancel(t *testing.T) {
	s := New(Config{
		TTL:             time.Hour,
		Logger:          discardLogger(),
		BuildCollectors: func() (map[string]prometheus.Collector, error) { return nil, nil },
		AuthReady:       func(context.Context) error { return nil },
		OnScrape:        func(bool) {},
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return within 1s of cancel")
	}
}

func TestFilteredGathererIntersect(t *testing.T) {
	a := newCounterCollector("a")
	a.val.Store(11)
	b := newCounterCollector("b")
	b.val.Store(22)
	s := New(Config{
		Logger: discardLogger(),
		BuildCollectors: func() (map[string]prometheus.Collector, error) {
			return map[string]prometheus.Collector{"alpha": a, "beta": b}, nil
		},
		OnScrape: func(bool) {},
	})
	s.Refresh(context.Background())

	families, err := s.FilteredGatherer([]string{"alpha"}).Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	gotNames := map[string]struct{}{}
	for _, f := range families {
		gotNames[f.GetName()] = struct{}{}
	}
	if _, ok := gotNames["test_a_value"]; !ok {
		t.Errorf("expected alpha's metric, missing")
	}
	if _, ok := gotNames["test_b_value"]; ok {
		t.Errorf("did not expect beta's metric when filter excludes it")
	}
	// Scrape orchestration metrics should be present for alpha only.
	if _, ok := gotNames["garmin_scrape_collector_success"]; !ok {
		t.Error("expected scrape_collector_success in filtered output")
	}
}

func TestBuildCollectorsError(t *testing.T) {
	wantErr := errors.New("build failed")
	var success atomic.Bool
	success.Store(true)
	var called atomic.Bool
	s := New(Config{
		Logger:          discardLogger(),
		BuildCollectors: func() (map[string]prometheus.Collector, error) { return nil, wantErr },
		OnScrape:        func(b bool) { success.Store(b); called.Store(true) },
	})
	s.Refresh(context.Background())
	if !called.Load() {
		t.Fatal("OnScrape never called on build error")
	}
	if success.Load() {
		t.Fatal("OnScrape should report failure on build error")
	}
}

func TestRefreshTotalSuccess(t *testing.T) {
	c := newCounterCollector("a")
	s := New(Config{
		Logger: discardLogger(),
		BuildCollectors: func() (map[string]prometheus.Collector, error) {
			return map[string]prometheus.Collector{"a": c}, nil
		},
		OnScrape: func(bool) {},
	})
	s.Refresh(context.Background())

	if got := testutil.ToFloat64(s.refreshTotal.WithLabelValues(refreshResultSuccess)); got != 1 {
		t.Fatalf("expected 1 success refresh, got %v", got)
	}
	if got := testutil.ToFloat64(s.refreshTotal.WithLabelValues(refreshResultFailure)); got != 0 {
		t.Fatalf("expected 0 failure refreshes, got %v", got)
	}
}

func TestRefreshTotalFailureOnEmpty(t *testing.T) {
	s := New(Config{
		Logger:          discardLogger(),
		BuildCollectors: func() (map[string]prometheus.Collector, error) { return map[string]prometheus.Collector{}, nil },
		OnScrape:        func(bool) {},
	})
	s.Refresh(context.Background())

	if got := testutil.ToFloat64(s.refreshTotal.WithLabelValues(refreshResultFailure)); got != 1 {
		t.Fatalf("expected 1 failure refresh, got %v", got)
	}
}

func TestRefreshTotalFailureOnBuildError(t *testing.T) {
	s := New(Config{
		Logger:          discardLogger(),
		BuildCollectors: func() (map[string]prometheus.Collector, error) { return nil, errors.New("boom") },
		OnScrape:        func(bool) {},
	})
	s.Refresh(context.Background())

	if got := testutil.ToFloat64(s.refreshTotal.WithLabelValues(refreshResultFailure)); got != 1 {
		t.Fatalf("expected 1 failure refresh, got %v", got)
	}
}

func TestRefreshTotalSkipped(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	s := New(Config{
		Logger: discardLogger(),
		BuildCollectors: func() (map[string]prometheus.Collector, error) {
			started <- struct{}{}
			<-release
			return map[string]prometheus.Collector{}, nil
		},
		OnScrape: func(bool) {},
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); s.Refresh(context.Background()) }()
	<-started
	// Second concurrent refresh hits the semaphore and is skipped.
	ran, _ := s.Refresh(context.Background())
	if ran {
		t.Fatal("expected second Refresh to report skipped (false), got true")
	}
	if got := testutil.ToFloat64(s.refreshTotal.WithLabelValues(refreshResultSkipped)); got != 1 {
		t.Fatalf("expected 1 skipped refresh, got %v", got)
	}
	close(release)
	wg.Wait()
}

func TestRefreshDurationObserved(t *testing.T) {
	c := newCounterCollector("a")
	s := New(Config{
		Logger: discardLogger(),
		BuildCollectors: func() (map[string]prometheus.Collector, error) {
			return map[string]prometheus.Collector{"a": c}, nil
		},
		OnScrape: func(bool) {},
	})
	s.Refresh(context.Background())

	families := gatherScraperFamilies(t, s)
	hist := findFamily(families, "garmin_cache_refresh_duration_seconds")
	if hist == nil {
		t.Fatal("histogram family missing")
	}
	if got := hist.GetMetric()[0].GetHistogram().GetSampleCount(); got != 1 {
		t.Fatalf("expected 1 histogram sample, got %d", got)
	}
}

func TestCacheAgeBeforeFirstRefresh(t *testing.T) {
	s := New(Config{Logger: discardLogger()})
	families := gatherScraperFamilies(t, s)
	assertGaugeValue(t, families, "garmin_cache_age_seconds", 0)
	assertGaugeValue(t, families, "garmin_cache_refresh_in_flight", 0)
}

func TestCacheAgeAfterRefresh(t *testing.T) {
	c := newCounterCollector("a")
	s := New(Config{
		Logger: discardLogger(),
		BuildCollectors: func() (map[string]prometheus.Collector, error) {
			return map[string]prometheus.Collector{"a": c}, nil
		},
		OnScrape: func(bool) {},
	})
	s.Refresh(context.Background())

	families := gatherScraperFamilies(t, s)
	age := findFamily(families, "garmin_cache_age_seconds")
	if age == nil {
		t.Fatal("cache_age family missing")
	}
	val := age.GetMetric()[0].GetGauge().GetValue()
	if val < 0 || val > 1 {
		t.Fatalf("expected cache age between 0 and 1 seconds right after refresh, got %v", val)
	}
}

func gatherScraperFamilies(t *testing.T, s *Scraper) []*dto.MetricFamily {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(s); err != nil {
		t.Fatalf("register scraper: %v", err)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	return families
}

func findFamily(families []*dto.MetricFamily, name string) *dto.MetricFamily {
	for _, f := range families {
		if f.GetName() == name {
			return f
		}
	}
	return nil
}
