package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("bloodpressure", newBloodPressureCollector,
		SnapshotHas(func(s *garmin.Snapshot) bool { return s.BloodPressure != nil }))
}

type bloodPressureCollector struct {
	registrar
	log *slog.Logger
	src garmin.Source

	systolic  metric.Int64ObservableGauge
	diastolic metric.Int64ObservableGauge
	pulse     metric.Int64ObservableGauge
}

func newBloodPressureCollector(log *slog.Logger) (Collector, error) {
	return &bloodPressureCollector{log: log}, nil
}

func (c *bloodPressureCollector) Name() string { return "bloodpressure" }

func (c *bloodPressureCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	c.systolic, err = meter.Int64ObservableGauge(
		"garmin.blood_pressure.systolic_mmhg",
		metric.WithDescription("Most recent systolic blood pressure in mmHg."),
		metric.WithUnit("mm[Hg]"),
	)
	if err != nil {
		return err
	}
	c.diastolic, err = meter.Int64ObservableGauge(
		"garmin.blood_pressure.diastolic_mmhg",
		metric.WithDescription("Most recent diastolic blood pressure in mmHg."),
		metric.WithUnit("mm[Hg]"),
	)
	if err != nil {
		return err
	}
	c.pulse, err = meter.Int64ObservableGauge(
		"garmin.blood_pressure.pulse_bpm",
		metric.WithDescription("Most recent pulse from blood pressure reading in bpm."),
		metric.WithUnit("{beat}/min"),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.systolic, c.diastolic, c.pulse)
	return err
}

func (c *bloodPressureCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || snap.BloodPressure == nil || len(snap.BloodPressure.Measurements) == 0 {
		return nil
	}
	latest := snap.BloodPressure.Measurements[len(snap.BloodPressure.Measurements)-1]
	if latest.Systolic > 0 {
		o.ObserveInt64(c.systolic, int64(latest.Systolic))
	}
	if latest.Diastolic > 0 {
		o.ObserveInt64(c.diastolic, int64(latest.Diastolic))
	}
	if latest.Pulse > 0 {
		o.ObserveInt64(c.pulse, int64(latest.Pulse))
	}
	return nil
}
