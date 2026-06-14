package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("stress", newStressCollector)
}

type stressCollector struct {
	registrar
	log *slog.Logger
	src garmin.Source

	avgStressLevel metric.Int64ObservableGauge
	maxStressLevel metric.Int64ObservableGauge
}

func newStressCollector(log *slog.Logger) (Collector, error) {
	return &stressCollector{log: log}, nil
}

func (c *stressCollector) Name() string { return "stress" }

func (c *stressCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	c.avgStressLevel, err = meter.Int64ObservableGauge(
		"garmin.stress.avg_level",
		metric.WithDescription("Average stress level for the day (0-100)."),
		metric.WithUnit("{score}"),
	)
	if err != nil {
		return err
	}
	c.maxStressLevel, err = meter.Int64ObservableGauge(
		"garmin.stress.max_level",
		metric.WithDescription("Peak stress level for the day (0-100)."),
		metric.WithUnit("{score}"),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.avgStressLevel, c.maxStressLevel)
	return err
}

func (c *stressCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || snap.Stress == nil {
		return nil
	}
	s := snap.Stress
	if s.AvgStressLevel > 0 {
		o.ObserveInt64(c.avgStressLevel, int64(s.AvgStressLevel))
	}
	if s.MaxStressLevel > 0 {
		o.ObserveInt64(c.maxStressLevel, int64(s.MaxStressLevel))
	}
	return nil
}
