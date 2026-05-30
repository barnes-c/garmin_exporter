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

// Outcome tracks the most recent scrape's success and timestamp. It is the
// source of truth for both the /readyz probe and the
// garmin_last_scrape_timestamp_seconds metric.
type Outcome struct {
	mtx       sync.RWMutex
	recorded  bool
	succeeded bool
	timestamp time.Time

	now func() time.Time
}

// NewOutcome returns an empty scrape outcome.
func NewOutcome() *Outcome {
	return &Outcome{now: time.Now}
}

// Record marks the most recent scrape as succeeded or failed and stamps it
// with the current time.
func (o *Outcome) Record(success bool) {
	o.mtx.Lock()
	defer o.mtx.Unlock()
	o.recorded = true
	o.succeeded = success
	o.timestamp = o.now()
}

// Ready reports whether the most recent scrape succeeded. Before the first
// scrape it returns true so the readiness probe doesn't deadlock the very
// first scrape that would update this state.
func (o *Outcome) Ready() bool {
	o.mtx.RLock()
	defer o.mtx.RUnlock()
	if !o.recorded {
		return true
	}
	return o.succeeded
}

// Describe implements prometheus.Collector.
func (o *Outcome) Describe(ch chan<- *prometheus.Desc) {
	ch <- lastScrapeTimestampDesc
}

// Collect implements prometheus.Collector.
func (o *Outcome) Collect(ch chan<- prometheus.Metric) {
	o.mtx.RLock()
	defer o.mtx.RUnlock()
	var ts float64
	if o.recorded {
		ts = float64(o.timestamp.Unix())
	}
	ch <- prometheus.MustNewConstMetric(lastScrapeTimestampDesc, prometheus.GaugeValue, ts)
}
