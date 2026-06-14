package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("body", newBodyCollector)
}

type bodyCollector struct {
	registrar
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
}

func newBodyCollector(log *slog.Logger) (Collector, error) {
	return &bodyCollector{log: log}, nil
}

func (c *bodyCollector) Name() string { return "body" }

func (c *bodyCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	c.weightGrams, err = meter.Float64ObservableGauge(
		"garmin.body.weight_grams",
		metric.WithDescription("Body weight in grams."),
		metric.WithUnit("g"),
	)
	if err != nil {
		return err
	}
	c.bmi, err = meter.Float64ObservableGauge(
		"garmin.body.bmi",
		metric.WithDescription("Body mass index."),
		metric.WithUnit("{ratio}"),
	)
	if err != nil {
		return err
	}
	c.bodyFat, err = meter.Float64ObservableGauge(
		"garmin.body.fat_percent",
		metric.WithDescription("Body fat percentage."),
		metric.WithUnit("%"),
	)
	if err != nil {
		return err
	}
	c.bodyWater, err = meter.Float64ObservableGauge(
		"garmin.body.water_percent",
		metric.WithDescription("Body water percentage."),
		metric.WithUnit("%"),
	)
	if err != nil {
		return err
	}
	c.boneMass, err = meter.Float64ObservableGauge(
		"garmin.body.bone_mass_grams",
		metric.WithDescription("Bone mass in grams."),
		metric.WithUnit("g"),
	)
	if err != nil {
		return err
	}
	c.muscleMass, err = meter.Float64ObservableGauge(
		"garmin.body.muscle_mass_grams",
		metric.WithDescription("Muscle mass in grams."),
		metric.WithUnit("g"),
	)
	if err != nil {
		return err
	}
	c.visceralFat, err = meter.Float64ObservableGauge(
		"garmin.body.visceral_fat",
		metric.WithDescription("Visceral fat rating."),
		metric.WithUnit("{rating}"),
	)
	if err != nil {
		return err
	}
	c.metabolicAge, err = meter.Float64ObservableGauge(
		"garmin.body.metabolic_age_years",
		metric.WithDescription("Metabolic age in years."),
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

func (c *bodyCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || snap.Body == nil || len(snap.Body.DateWeightList) == 0 {
		return nil
	}
	w := snap.Body.DateWeightList[0]
	if w.Weight > 0 {
		o.ObserveFloat64(c.weightGrams, w.Weight)
	}
	if w.Bmi > 0 {
		o.ObserveFloat64(c.bmi, w.Bmi)
	}
	if w.BodyFat > 0 {
		o.ObserveFloat64(c.bodyFat, w.BodyFat)
	}
	if w.BodyWater > 0 {
		o.ObserveFloat64(c.bodyWater, w.BodyWater)
	}
	if w.BoneMass > 0 {
		o.ObserveFloat64(c.boneMass, w.BoneMass)
	}
	if w.MuscleMass > 0 {
		o.ObserveFloat64(c.muscleMass, w.MuscleMass)
	}
	if w.VisceralFat > 0 {
		o.ObserveFloat64(c.visceralFat, w.VisceralFat)
	}
	if w.MetabolicAge > 0 {
		o.ObserveFloat64(c.metabolicAge, w.MetabolicAge)
	}
	return nil
}
