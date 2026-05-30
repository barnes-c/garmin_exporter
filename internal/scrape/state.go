// Package scrape tracks the most recent collector scrape outcome and exposes
// it as both a readiness signal and a Prometheus metric.
package scrape

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var lastScrapeTimestampDesc = prometheus.NewDesc(
	prometheus.BuildFQName("garmin", "", "last_scrape_timestamp_seconds"),
	"garmin_exporter: Unix timestamp of the most recent metrics scrape, or 0 before the first scrape.",
	nil,
	nil,
)

// State tracks the most recent scrape outcome.
type State struct {
	mtx       sync.RWMutex
	recorded  bool
	succeeded bool
	timestamp time.Time

	now func() time.Time
}

// NewState returns an empty scrape state.
func NewState() *State {
	return &State{now: time.Now}
}

// Record marks the most recent scrape as succeeded or failed and stamps it
// with the current time.
func (s *State) Record(success bool) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.recorded = true
	s.succeeded = success
	s.timestamp = s.now()
}

// Ready reports whether the most recent scrape succeeded. Before the first
// scrape it returns true so the readiness probe doesn't deadlock the very
// first scrape that would update this state.
func (s *State) Ready() bool {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	if !s.recorded {
		return true
	}
	return s.succeeded
}

// Describe implements prometheus.Collector.
func (s *State) Describe(ch chan<- *prometheus.Desc) {
	ch <- lastScrapeTimestampDesc
}

// Collect implements prometheus.Collector.
func (s *State) Collect(ch chan<- prometheus.Metric) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	var ts float64
	if s.recorded {
		ts = float64(s.timestamp.Unix())
	}
	ch <- prometheus.MustNewConstMetric(lastScrapeTimestampDesc, prometheus.GaugeValue, ts)
}
