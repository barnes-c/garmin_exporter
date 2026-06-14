package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("intensity", DefaultEnabled, newIntensityCollector,
		SnapshotHas(func(s *garmin.Snapshot) bool { return s.Intensity != nil }))
}

type intensityCollector struct {
	registrar
	log *slog.Logger
	src garmin.Source

	weeklyGoal   metric.Int64ObservableGauge
	moderateMins metric.Int64ObservableGauge
	vigorousMins metric.Int64ObservableGauge
}

func newIntensityCollector(log *slog.Logger) (Collector, error) {
	return &intensityCollector{log: log}, nil
}

func (c *intensityCollector) Name() string { return "intensity" }

func (c *intensityCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	c.weeklyGoal, err = meter.Int64ObservableGauge(
		"garmin.intensity.weekly_goal_minutes",
		metric.WithDescription("Weekly intensity minutes goal."),
		metric.WithUnit("min"),
	)
	if err != nil {
		return err
	}
	c.moderateMins, err = meter.Int64ObservableGauge(
		"garmin.intensity.moderate_minutes_total",
		metric.WithDescription("Cumulative moderate intensity minutes this week."),
		metric.WithUnit("min"),
	)
	if err != nil {
		return err
	}
	c.vigorousMins, err = meter.Int64ObservableGauge(
		"garmin.intensity.vigorous_minutes_total",
		metric.WithDescription("Cumulative vigorous intensity minutes this week."),
		metric.WithUnit("min"),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.weeklyGoal, c.moderateMins, c.vigorousMins)
	return err
}

func (c *intensityCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || snap.Intensity == nil {
		return nil
	}
	im := snap.Intensity
	o.ObserveInt64(c.weeklyGoal, int64(im.WeeklyGoal))
	o.ObserveInt64(c.moderateMins, int64(im.ModerateIntensityMinutes))
	o.ObserveInt64(c.vigorousMins, int64(im.VigorousIntensityMinutes))
	return nil
}
