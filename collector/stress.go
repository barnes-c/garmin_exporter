package collector

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("stress", defaultEnabled, newStressCollector)
}

type stressCollector struct {
	avgStressLevel *prometheus.Desc
	maxStressLevel *prometheus.Desc
	logger         *slog.Logger
}

func newStressCollector(logger *slog.Logger) (Collector, error) {
	const sub = "stress"
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, sub, name), help, nil, nil)
	}
	return &stressCollector{
		avgStressLevel: d("avg_level", "Average stress level for the day (0-100)."),
		maxStressLevel: d("max_level", "Peak stress level for the day (0-100)."),
		logger:         logger,
	}, nil
}

func (c *stressCollector) Update(ctx context.Context, ch chan<- prometheus.Metric) error {
	client := getClient()
	if client == nil {
		return ErrNoData
	}

	s, err := client.AllDayStress(ctx, time.Now())
	if err != nil {
		return err
	}

	g := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}

	if s.AvgStressLevel > 0 {
		g(c.avgStressLevel, float64(s.AvgStressLevel))
	}
	if s.MaxStressLevel > 0 {
		g(c.maxStressLevel, float64(s.MaxStressLevel))
	}
	return nil
}
