package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("lactate_threshold", newLactateThresholdCollector)
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

func (c *lactateThresholdCollector) Name() string { return "lactate_threshold" }

func (c *lactateThresholdCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	c.runningSpeedMPS, err = meter.Float64ObservableGauge(
		"garmin.lactate_threshold.running_speed_mps",
		metric.WithDescription("Running lactate threshold speed in meters per second."),
	)
	if err != nil {
		return err
	}
	c.runningHeartRate, err = meter.Int64ObservableGauge(
		"garmin.lactate_threshold.running_heart_rate_bpm",
		metric.WithDescription("Running lactate threshold heart rate in bpm."),
	)
	if err != nil {
		return err
	}
	c.cyclingHeartRate, err = meter.Int64ObservableGauge(
		"garmin.lactate_threshold.cycling_heart_rate_bpm",
		metric.WithDescription("Cycling lactate threshold heart rate in bpm."),
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
	if snap == nil {
		return nil
	}
	// Garmin returns one entry per measurement (one with the speed, another
	// with the heart rate), so take each value from the entry that has it.
	var speedSeen, hrSeen, cyclingSeen bool
	for _, e := range snap.LactateThreshold {
		if !speedSeen && e.Speed != nil && *e.Speed > 0 {
			// Garmin reports this speed in units of 10 m/s (e.g. 0.33 for 3.3 m/s).
			o.ObserveFloat64(c.runningSpeedMPS, *e.Speed*10)
			speedSeen = true
		}
		if !hrSeen && e.HearRate != nil && *e.HearRate > 0 {
			o.ObserveInt64(c.runningHeartRate, int64(*e.HearRate))
			hrSeen = true
		}
		if !cyclingSeen && e.HeartRateCycling != nil && *e.HeartRateCycling > 0 {
			o.ObserveInt64(c.cyclingHeartRate, int64(*e.HeartRateCycling))
			cyclingSeen = true
		}
	}
	return nil
}
