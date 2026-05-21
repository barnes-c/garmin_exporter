package collector

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("runningtolerance", defaultDisabled, newRunningToleranceCollector)
}

type runningToleranceCollector struct {
	score  *prometheus.Desc
	level  *prometheus.Desc
	logger *slog.Logger
}

func newRunningToleranceCollector(logger *slog.Logger) (Collector, error) {
	const sub = "runningtolerance"
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, sub, name), help, nil, nil)
	}
	return &runningToleranceCollector{
		score:  d("score", "Running tolerance score."),
		level:  d("level", "Running tolerance level."),
		logger: logger,
	}, nil
}

func (c *runningToleranceCollector) Update(ch chan<- prometheus.Metric) error {
	if garminClient == nil {
		return ErrNoData
	}

	now := time.Now()
	entries, err := garminClient.RunningTolerance(now.AddDate(0, 0, -7), now)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return ErrNoData
	}

	latest := entries[len(entries)-1]
	if latest.Score > 0 {
		ch <- prometheus.MustNewConstMetric(c.score, prometheus.GaugeValue, latest.Score)
	}
	if latest.Level > 0 {
		ch <- prometheus.MustNewConstMetric(c.level, prometheus.GaugeValue, float64(latest.Level))
	}
	return nil
}
