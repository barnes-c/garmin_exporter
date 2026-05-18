package collector

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("respiration", defaultEnabled, newRespirationCollector)
}

type respirationCollector struct {
	avgWaking *prometheus.Desc
	highest   *prometheus.Desc
	lowest    *prometheus.Desc
	logger    *slog.Logger
}

func newRespirationCollector(logger *slog.Logger) (Collector, error) {
	const sub = "respiration"
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, sub, name), help, nil, nil)
	}
	return &respirationCollector{
		avgWaking: d("avg_waking_bpm", "Average waking respiration rate in breaths per minute."),
		highest:   d("highest_bpm", "Highest respiration rate in breaths per minute."),
		lowest:    d("lowest_bpm", "Lowest respiration rate in breaths per minute."),
		logger:    logger,
	}, nil
}

func (c *respirationCollector) Update(ch chan<- prometheus.Metric) error {
	if garminClient == nil {
		return ErrNoData
	}

	r, err := garminClient.Respiration(time.Now())
	if err != nil {
		return err
	}

	g := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}

	if r.TodayAvgWakingRespirationValue > 0 {
		g(c.avgWaking, r.TodayAvgWakingRespirationValue)
	}
	if r.HighestRespirationValue > 0 {
		g(c.highest, r.HighestRespirationValue)
	}
	if r.LowestRespirationValue > 0 {
		g(c.lowest, r.LowestRespirationValue)
	}
	return nil
}
