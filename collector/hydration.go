package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("hydration", newHydrationCollector)
}

type hydrationCollector struct {
	registrar
	log *slog.Logger
	src garmin.Source

	intakeML       metric.Float64ObservableGauge
	goalML         metric.Float64ObservableGauge
	dailyAvgML     metric.Float64ObservableGauge
	sweatLossML    metric.Float64ObservableGauge
	activityIntake metric.Float64ObservableGauge
}

func newHydrationCollector(log *slog.Logger) (Collector, error) {
	return &hydrationCollector{log: log}, nil
}

func (c *hydrationCollector) Name() string { return "hydration" }

func (c *hydrationCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	c.intakeML, err = meter.Float64ObservableGauge(
		"garmin.hydration.intake_ml",
		metric.WithDescription("Total hydration intake in millilitres."),
		metric.WithUnit("mL"),
	)
	if err != nil {
		return err
	}
	c.goalML, err = meter.Float64ObservableGauge(
		"garmin.hydration.goal_ml",
		metric.WithDescription("Daily hydration goal in millilitres."),
		metric.WithUnit("mL"),
	)
	if err != nil {
		return err
	}
	c.dailyAvgML, err = meter.Float64ObservableGauge(
		"garmin.hydration.daily_avg_ml",
		metric.WithDescription("Daily average hydration intake in millilitres."),
		metric.WithUnit("mL"),
	)
	if err != nil {
		return err
	}
	c.sweatLossML, err = meter.Float64ObservableGauge(
		"garmin.hydration.sweat_loss_ml",
		metric.WithDescription("Estimated sweat loss in millilitres."),
		metric.WithUnit("mL"),
	)
	if err != nil {
		return err
	}
	c.activityIntake, err = meter.Float64ObservableGauge(
		"garmin.hydration.activity_intake_ml",
		metric.WithDescription("Hydration intake during activities in millilitres."),
		metric.WithUnit("mL"),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.intakeML, c.goalML, c.dailyAvgML, c.sweatLossML, c.activityIntake)
	return err
}

func (c *hydrationCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || snap.Hydration == nil {
		return nil
	}
	h := snap.Hydration
	o.ObserveFloat64(c.intakeML, h.ValueInML)
	o.ObserveFloat64(c.goalML, h.GoalInML)
	if h.DailyAverageinML > 0 {
		o.ObserveFloat64(c.dailyAvgML, h.DailyAverageinML)
	}
	if h.SweatLossInML > 0 {
		o.ObserveFloat64(c.sweatLossML, h.SweatLossInML)
	}
	if h.ActivityIntakeInML > 0 {
		o.ObserveFloat64(c.activityIntake, h.ActivityIntakeInML)
	}
	return nil
}
