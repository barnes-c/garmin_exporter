package scrape

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestStateReadyBeforeFirstScrape(t *testing.T) {
	s := NewState()
	if !s.Ready() {
		t.Fatal("expected scrape state to be ready before the first scrape")
	}
}

func TestStateRecordSuccess(t *testing.T) {
	s := NewState()
	s.now = func() time.Time { return time.Unix(1000, 0) }
	s.Record(true)

	if !s.Ready() {
		t.Fatal("expected ready after a successful scrape")
	}
	if !s.recorded || !s.succeeded {
		t.Fatalf("unexpected state: recorded=%v succeeded=%v", s.recorded, s.succeeded)
	}
	if s.timestamp != time.Unix(1000, 0) {
		t.Fatalf("expected timestamp %v, got %v", time.Unix(1000, 0), s.timestamp)
	}
}

func TestStateRecordFailure(t *testing.T) {
	s := NewState()
	s.Record(false)

	if s.Ready() {
		t.Fatal("expected not ready after a failed scrape")
	}
}

func TestStateRecoversAfterFailure(t *testing.T) {
	s := NewState()
	s.Record(false)
	s.Record(true)
	if !s.Ready() {
		t.Fatal("expected ready after a recovery scrape")
	}
}

func TestStateCollectMetricBeforeScrape(t *testing.T) {
	s := NewState()
	registry := prometheus.NewRegistry()
	if err := registry.Register(s); err != nil {
		t.Fatalf("register scrape state collector: %s", err)
	}
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather scrape metrics: %s", err)
	}
	assertGaugeValue(t, metricFamilies, "garmin_last_scrape_timestamp_seconds", 0)
}

func TestStateCollectMetricAfterScrape(t *testing.T) {
	s := NewState()
	s.now = func() time.Time { return time.Unix(1234567890, 0) }
	s.Record(true)

	registry := prometheus.NewRegistry()
	if err := registry.Register(s); err != nil {
		t.Fatalf("register scrape state collector: %s", err)
	}
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather scrape metrics: %s", err)
	}
	assertGaugeValue(t, metricFamilies, "garmin_last_scrape_timestamp_seconds", 1234567890)
}

func assertGaugeValue(t *testing.T, metricFamilies []*dto.MetricFamily, name string, want float64) {
	t.Helper()

	for _, metricFamily := range metricFamilies {
		if metricFamily.GetName() != name {
			continue
		}
		metrics := metricFamily.GetMetric()
		if len(metrics) != 1 {
			t.Fatalf("expected metric family %q to have 1 metric, got %d", name, len(metrics))
		}
		if got := metrics[0].GetGauge().GetValue(); got != want {
			t.Fatalf("expected metric %q value %v, got %v", name, want, got)
		}
		return
	}

	t.Fatalf("missing metric family %q", name)
}
