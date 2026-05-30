package scrape

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestOutcomeReadyBeforeFirstScrape(t *testing.T) {
	o := NewOutcome()
	if !o.Ready() {
		t.Fatal("expected outcome to be ready before the first scrape")
	}
}

func TestOutcomeRecordSuccess(t *testing.T) {
	o := NewOutcome()
	o.now = func() time.Time { return time.Unix(1000, 0) }
	o.Record(true)

	if !o.Ready() {
		t.Fatal("expected ready after a successful scrape")
	}
	if !o.recorded || !o.succeeded {
		t.Fatalf("unexpected state: recorded=%v succeeded=%v", o.recorded, o.succeeded)
	}
	if o.timestamp != time.Unix(1000, 0) {
		t.Fatalf("expected timestamp %v, got %v", time.Unix(1000, 0), o.timestamp)
	}
}

func TestOutcomeRecordFailure(t *testing.T) {
	o := NewOutcome()
	o.Record(false)

	if o.Ready() {
		t.Fatal("expected not ready after a failed scrape")
	}
}

func TestOutcomeRecoversAfterFailure(t *testing.T) {
	o := NewOutcome()
	o.Record(false)
	o.Record(true)
	if !o.Ready() {
		t.Fatal("expected ready after a recovery scrape")
	}
}

func TestOutcomeCollectMetricBeforeScrape(t *testing.T) {
	o := NewOutcome()
	registry := prometheus.NewRegistry()
	if err := registry.Register(o); err != nil {
		t.Fatalf("register outcome collector: %s", err)
	}
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather scrape metrics: %s", err)
	}
	assertGaugeValue(t, metricFamilies, "garmin_last_scrape_timestamp_seconds", 0)
}

func TestOutcomeCollectMetricAfterScrape(t *testing.T) {
	o := NewOutcome()
	o.now = func() time.Time { return time.Unix(1234567890, 0) }
	o.Record(true)

	registry := prometheus.NewRegistry()
	if err := registry.Register(o); err != nil {
		t.Fatalf("register outcome collector: %s", err)
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
