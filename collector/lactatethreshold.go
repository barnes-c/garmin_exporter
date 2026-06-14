package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("lactatethreshold", newLactateThresholdCollector,
		SnapshotHas(func(s *garmin.Snapshot) bool { return s.LactateThreshold != nil }))
}

type lactateThresholdCollector struct {
	registrar
	log *slog.Logger
	src garmin.Source

	runningSpeedMPS  metric.Float64ObservableGauge
	runningHeartRate metric.Int64ObservableGauge
	cyclingHeartRate metric.Int64ObservableGauge
}

func newLactateThresholdCollector(log *slog.Logger) (Collector, error) {
	return &lactateThresholdCollector{log: log}, nil
}

func (c *lactateThresholdCollector) Name() string { return "lactatethreshold" }

func (c *lactateThresholdCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	c.runningSpeedMPS, err = meter.Float64ObservableGauge(
		"garmin.lactatethreshold.running_speed_mps",
		metric.WithDescription("Running lactate threshold speed in meters per second."),
		metric.WithUnit("m/s"),
	)
	if err != nil {
		return err
	}
	c.runningHeartRate, err = meter.Int64ObservableGauge(
		"garmin.lactatethreshold.running_heart_rate_bpm",
		metric.WithDescription("Running lactate threshold heart rate in bpm."),
		metric.WithUnit("{beat}/min"),
	)
	if err != nil {
		return err
	}
	c.cyclingHeartRate, err = meter.Int64ObservableGauge(
		"garmin.lactatethreshold.cycling_heart_rate_bpm",
		metric.WithDescription("Cycling lactate threshold heart rate in bpm."),
		metric.WithUnit("{beat}/min"),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.runningSpeedMPS, c.runningHeartRate, c.cyclingHeartRate)
	return err
}

func (c *lactateThresholdCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || len(snap.LactateThreshold) == 0 {
		return nil
	}
	e := snap.LactateThreshold[0]
	if e.Speed != nil && *e.Speed > 0 {
		o.ObserveFloat64(c.runningSpeedMPS, *e.Speed)
	}
	if e.HearRate != nil && *e.HearRate > 0 {
		o.ObserveInt64(c.runningHeartRate, int64(*e.HearRate))
	}
	if e.HeartRateCycling != nil && *e.HeartRateCycling > 0 {
		o.ObserveInt64(c.cyclingHeartRate, int64(*e.HeartRateCycling))
	}
	return nil
}
