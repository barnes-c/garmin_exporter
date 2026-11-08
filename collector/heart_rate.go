package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("heart_rate", newHeartRateCollector)
}

type heartRateCollector struct {
	registrar
	log *slog.Logger
	src garmin.Source

	resting     metric.Int64ObservableGauge
	min         metric.Int64ObservableGauge
	max         metric.Int64ObservableGauge
	sevenDayAvg metric.Int64ObservableGauge
}

func newHeartRateCollector(log *slog.Logger) (Collector, error) {
	return &heartRateCollector{log: log}, nil
}

func (c *heartRateCollector) Name() string { return "heart_rate" }

func (c *heartRateCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	c.resting, err = meter.Int64ObservableGauge(
		"garmin.heart_rate.resting_bpm",
		metric.WithDescription("Resting heart rate in bpm."),
	)
	if err != nil {
		return err
	}
	c.min, err = meter.Int64ObservableGauge(
		"garmin.heart_rate.min_bpm",
		metric.WithDescription("Minimum heart rate in bpm."),
	)
	if err != nil {
		return err
	}
	c.max, err = meter.Int64ObservableGauge(
		"garmin.heart_rate.max_bpm",
		metric.WithDescription("Maximum heart rate in bpm."),
	)
	if err != nil {
		return err
	}
	c.sevenDayAvg, err = meter.Int64ObservableGauge(
		"garmin.heart_rate.seven_day_avg_resting_bpm",
		metric.WithDescription("7-day average resting heart rate in bpm."),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.resting, c.min, c.max, c.sevenDayAvg)
	return err
}

func (c *heartRateCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || snap.HeartRate == nil {
		return nil
	}
	hr := snap.HeartRate
	if hr.RestingHeartRate > 0 {
		o.ObserveInt64(c.resting, int64(hr.RestingHeartRate))
	}
	if hr.MinHeartRate > 0 {
		o.ObserveInt64(c.min, int64(hr.MinHeartRate))
	}
	if hr.MaxHeartRate > 0 {
		o.ObserveInt64(c.max, int64(hr.MaxHeartRate))
	}
	if hr.LastSevenDaysAvgRestingHeartRate > 0 {
		o.ObserveInt64(c.sevenDayAvg, int64(hr.LastSevenDaysAvgRestingHeartRate))
	}
	return nil
}
