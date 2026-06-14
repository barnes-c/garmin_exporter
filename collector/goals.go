package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("goals", DefaultEnabled, newGoalsCollector,
		SnapshotHas(func(s *garmin.Snapshot) bool { return s.Goals != nil }))
}

type goalsCollector struct {
	registrar
	log *slog.Logger
	src garmin.Source

	activeGoals  metric.Int64ObservableGauge
	earnedBadges metric.Int64ObservableGauge
}

func newGoalsCollector(log *slog.Logger) (Collector, error) {
	return &goalsCollector{log: log}, nil
}

func (c *goalsCollector) Name() string { return "goals" }

func (c *goalsCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	c.activeGoals, err = meter.Int64ObservableGauge(
		"garmin.goals.active_total",
		metric.WithDescription("Number of active fitness goals."),
		metric.WithUnit("{goal}"),
	)
	if err != nil {
		return err
	}
	c.earnedBadges, err = meter.Int64ObservableGauge(
		"garmin.goals.earned_badges_total",
		metric.WithDescription("Total number of earned achievement badges."),
		metric.WithUnit("{badge}"),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.activeGoals, c.earnedBadges)
	return err
}

func (c *goalsCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || snap.Goals == nil {
		return nil
	}
	g := snap.Goals
	if g.Active != nil {
		o.ObserveInt64(c.activeGoals, int64(len(g.Active)))
	}
	if g.Badges != nil {
		o.ObserveInt64(c.earnedBadges, int64(len(g.Badges)))
	}
	return nil
}
