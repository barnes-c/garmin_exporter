package main

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestScrapeStateReadyBeforeFirstScrape(t *testing.T) {
	s := newScrapeState()
	if !s.ready() {
		t.Fatal("expected scrape state to be ready before the first scrape")
	}
}

func TestScrapeStateRecordSuccess(t *testing.T) {
	s := newScrapeState()
	s.now = func() time.Time { return time.Unix(1000, 0) }
	s.record(true)

	if !s.ready() {
		t.Fatal("expected ready after a successful scrape")
	}
	if !s.recorded || !s.succeeded {
		t.Fatalf("unexpected state: recorded=%v succeeded=%v", s.recorded, s.succeeded)
	}
	if s.timestamp != time.Unix(1000, 0) {
		t.Fatalf("expected timestamp %v, got %v", time.Unix(1000, 0), s.timestamp)
	}
}

func TestScrapeStateRecordFailure(t *testing.T) {
	s := newScrapeState()
	s.record(false)

	if s.ready() {
		t.Fatal("expected not ready after a failed scrape")
	}
}

func TestScrapeStateRecoversAfterFailure(t *testing.T) {
	s := newScrapeState()
	s.record(false)
	s.record(true)
	if !s.ready() {
		t.Fatal("expected ready after a recovery scrape")
	}
}

func TestScrapeStateCollectMetricBeforeScrape(t *testing.T) {
	s := newScrapeState()
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

func TestScrapeStateCollectMetricAfterScrape(t *testing.T) {
	s := newScrapeState()
	s.now = func() time.Time { return time.Unix(1234567890, 0) }
	s.record(true)

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
