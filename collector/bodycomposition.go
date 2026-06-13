package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("bodycomposition", DefaultEnabled, newBodyCompositionCollector)
}

type bodyCompositionCollector struct {
	log *slog.Logger
	src garmin.Source

	weightGrams  metric.Float64ObservableGauge
	bmi          metric.Float64ObservableGauge
	bodyFat      metric.Float64ObservableGauge
	bodyWater    metric.Float64ObservableGauge
	boneMass     metric.Float64ObservableGauge
	muscleMass   metric.Float64ObservableGauge
	visceralFat  metric.Float64ObservableGauge
	metabolicAge metric.Float64ObservableGauge

	registration metric.Registration
}

func newBodyCompositionCollector(log *slog.Logger) (Collector, error) {
	return &bodyCompositionCollector{log: log}, nil
}

func (c *bodyCompositionCollector) Name() string { return "bodycomposition" }

func (c *bodyCompositionCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	c.weightGrams, err = meter.Float64ObservableGauge(
		"garmin.body_composition.weight_grams_avg",
		metric.WithDescription("30-day average body weight in grams."),
		metric.WithUnit("g"),
	)
	if err != nil {
		return err
	}
	c.bmi, err = meter.Float64ObservableGauge(
		"garmin.body_composition.bmi_avg",
		metric.WithDescription("30-day average BMI."),
		metric.WithUnit("{ratio}"),
	)
	if err != nil {
		return err
	}
	c.bodyFat, err = meter.Float64ObservableGauge(
		"garmin.body_composition.fat_percent_avg",
		metric.WithDescription("30-day average body fat percentage."),
		metric.WithUnit("%"),
	)
	if err != nil {
		return err
	}
	c.bodyWater, err = meter.Float64ObservableGauge(
		"garmin.body_composition.water_percent_avg",
		metric.WithDescription("30-day average body water percentage."),
		metric.WithUnit("%"),
	)
	if err != nil {
		return err
	}
	c.boneMass, err = meter.Float64ObservableGauge(
		"garmin.body_composition.bone_mass_grams_avg",
		metric.WithDescription("30-day average bone mass in grams."),
		metric.WithUnit("g"),
	)
	if err != nil {
		return err
	}
	c.muscleMass, err = meter.Float64ObservableGauge(
		"garmin.body_composition.muscle_mass_grams_avg",
		metric.WithDescription("30-day average muscle mass in grams."),
		metric.WithUnit("g"),
	)
	if err != nil {
		return err
	}
	c.visceralFat, err = meter.Float64ObservableGauge(
		"garmin.body_composition.visceral_fat_avg",
		metric.WithDescription("30-day average visceral fat rating."),
		metric.WithUnit("{rating}"),
	)
	if err != nil {
		return err
	}
	c.metabolicAge, err = meter.Float64ObservableGauge(
		"garmin.body_composition.metabolic_age_years_avg",
		metric.WithDescription("30-day average metabolic age in years."),
		metric.WithUnit("a"),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.weightGrams, c.bmi, c.bodyFat, c.bodyWater,
		c.boneMass, c.muscleMass, c.visceralFat, c.metabolicAge)
	return err
}

func (c *bodyCompositionCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || snap.BodyComposition == nil {
		return nil
	}
	avg := snap.BodyComposition.TotalAverage
	if avg.Weight == 0 && avg.Bmi == 0 {
		return nil
	}
	if avg.Weight > 0 {
		o.ObserveFloat64(c.weightGrams, avg.Weight)
	}
	if avg.Bmi > 0 {
		o.ObserveFloat64(c.bmi, avg.Bmi)
	}
	if avg.BodyFat > 0 {
		o.ObserveFloat64(c.bodyFat, avg.BodyFat)
	}
	if avg.BodyWater > 0 {
		o.ObserveFloat64(c.bodyWater, avg.BodyWater)
	}
	if avg.BoneMass > 0 {
		o.ObserveFloat64(c.boneMass, avg.BoneMass)
	}
	if avg.MuscleMass > 0 {
		o.ObserveFloat64(c.muscleMass, avg.MuscleMass)
	}
	if avg.VisceralFat > 0 {
		o.ObserveFloat64(c.visceralFat, avg.VisceralFat)
	}
	if avg.MetabolicAge > 0 {
		o.ObserveFloat64(c.metabolicAge, avg.MetabolicAge)
	}
	return nil
}

func (c *bodyCompositionCollector) Close() error {
	if c.registration == nil {
		return nil
	}
	return c.registration.Unregister()
}
