package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("fitnessage", newFitnessAgeCollector)
}

type fitnessAgeCollector struct {
	registrar
	log *slog.Logger
	src garmin.Source

	years          metric.Int64ObservableGauge
	chronological  metric.Int64ObservableGauge
	achievable     metric.Int64ObservableGauge
	previous       metric.Int64ObservableGauge
	componentValue metric.Float64ObservableGauge
	componentAge   metric.Float64ObservableGauge
}

func newFitnessAgeCollector(log *slog.Logger) (Collector, error) {
	return &fitnessAgeCollector{log: log}, nil
}

func (c *fitnessAgeCollector) Name() string { return "fitnessage" }

func (c *fitnessAgeCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	if c.years, err = meter.Int64ObservableGauge(
		"garmin.fitness_age.years",
		metric.WithDescription("Estimated fitness age in years."),
		metric.WithUnit("a"),
	); err != nil {
		return err
	}
	if c.chronological, err = meter.Int64ObservableGauge(
		"garmin.fitness_age.chronological_years",
		metric.WithDescription("Chronological (actual) age in years."),
		metric.WithUnit("a"),
	); err != nil {
		return err
	}
	if c.achievable, err = meter.Int64ObservableGauge(
		"garmin.fitness_age.achievable_years",
		metric.WithDescription("Best achievable fitness age in years."),
		metric.WithUnit("a"),
	); err != nil {
		return err
	}
	if c.previous, err = meter.Int64ObservableGauge(
		"garmin.fitness_age.previous_years",
		metric.WithDescription("Previous fitness age in years."),
		metric.WithUnit("a"),
	); err != nil {
		return err
	}
	if c.componentValue, err = meter.Float64ObservableGauge(
		"garmin.fitness_age.component_value",
		metric.WithDescription("Current value of a fitness-age contributing factor."),
	); err != nil {
		return err
	}
	if c.componentAge, err = meter.Float64ObservableGauge(
		"garmin.fitness_age.component_potential_years",
		metric.WithDescription("Potential fitness age if this factor were at target."),
		metric.WithUnit("a"),
	); err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.years, c.chronological, c.achievable, c.previous,
		c.componentValue, c.componentAge)
	return err
}

func (c *fitnessAgeCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || snap.FitnessAge == nil {
		return nil
	}
	fa := snap.FitnessAge
	if fa.FitnessAge > 0 {
		o.ObserveInt64(c.years, int64(fa.FitnessAge))
	}
	if fa.ChronologicalAge > 0 {
		o.ObserveInt64(c.chronological, int64(fa.ChronologicalAge))
	}
	if fa.AchievableFitnessAge > 0 {
		o.ObserveInt64(c.achievable, int64(fa.AchievableFitnessAge))
	}
	if fa.PreviousFitnessAge > 0 {
		o.ObserveInt64(c.previous, int64(fa.PreviousFitnessAge))
	}
	for _, comp := range fa.Components {
		attrs := metric.WithAttributes(attribute.String("component", comp.Name))
		o.ObserveFloat64(c.componentValue, comp.Value, attrs)
		if comp.HasPotential {
			o.ObserveFloat64(c.componentAge, comp.PotentialAge, attrs)
		}
	}
	return nil
}
