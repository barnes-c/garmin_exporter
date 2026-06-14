package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("respiration", newRespirationCollector,
		SnapshotHas(func(s *garmin.Snapshot) bool { return s.Respiration != nil }))
}

type respirationCollector struct {
	registrar
	log *slog.Logger
	src garmin.Source

	avgWaking metric.Float64ObservableGauge
	highest   metric.Float64ObservableGauge
	lowest    metric.Float64ObservableGauge
}

func newRespirationCollector(log *slog.Logger) (Collector, error) {
	return &respirationCollector{log: log}, nil
}

func (c *respirationCollector) Name() string { return "respiration" }

func (c *respirationCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	c.avgWaking, err = meter.Float64ObservableGauge(
		"garmin.respiration.avg_waking_bpm",
		metric.WithDescription("Average waking respiration rate in breaths per minute."),
		metric.WithUnit("{breath}/min"),
	)
	if err != nil {
		return err
	}
	c.highest, err = meter.Float64ObservableGauge(
		"garmin.respiration.highest_bpm",
		metric.WithDescription("Highest respiration rate in breaths per minute."),
		metric.WithUnit("{breath}/min"),
	)
	if err != nil {
		return err
	}
	c.lowest, err = meter.Float64ObservableGauge(
		"garmin.respiration.lowest_bpm",
		metric.WithDescription("Lowest respiration rate in breaths per minute."),
		metric.WithUnit("{breath}/min"),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.avgWaking, c.highest, c.lowest)
	return err
}

func (c *respirationCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || snap.Respiration == nil {
		return nil
	}
	r := snap.Respiration
	if r.TodayAvgWakingRespirationValue > 0 {
		o.ObserveFloat64(c.avgWaking, r.TodayAvgWakingRespirationValue)
	}
	if r.HighestRespirationValue > 0 {
		o.ObserveFloat64(c.highest, r.HighestRespirationValue)
	}
	if r.LowestRespirationValue > 0 {
		o.ObserveFloat64(c.lowest, r.LowestRespirationValue)
	}
	return nil
}
