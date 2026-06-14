package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("runningtolerance", DefaultEnabled, newRunningToleranceCollector,
		SnapshotHas(func(s *garmin.Snapshot) bool { return s.RunningTolerance != nil }))
}

type runningToleranceCollector struct {
	registrar
	log *slog.Logger
	src garmin.Source

	score metric.Float64ObservableGauge
	level metric.Int64ObservableGauge
}

func newRunningToleranceCollector(log *slog.Logger) (Collector, error) {
	return &runningToleranceCollector{log: log}, nil
}

func (c *runningToleranceCollector) Name() string { return "runningtolerance" }

func (c *runningToleranceCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	c.score, err = meter.Float64ObservableGauge(
		"garmin.runningtolerance.score",
		metric.WithDescription("Running tolerance score."),
	)
	if err != nil {
		return err
	}
	c.level, err = meter.Int64ObservableGauge(
		"garmin.runningtolerance.level",
		metric.WithDescription("Running tolerance level."),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe, c.score, c.level)
	return err
}

func (c *runningToleranceCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || len(snap.RunningTolerance) == 0 {
		return nil
	}
	latest := snap.RunningTolerance[len(snap.RunningTolerance)-1]
	if latest.Score > 0 {
		o.ObserveFloat64(c.score, latest.Score)
	}
	if latest.Level > 0 {
		o.ObserveInt64(c.level, int64(latest.Level))
	}
	return nil
}
