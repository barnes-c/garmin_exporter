package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("spo2", DefaultEnabled, newSpO2Collector)
}

type spO2Collector struct {
	log *slog.Logger
	src garmin.Source

	average     metric.Float64ObservableGauge
	lowest      metric.Float64ObservableGauge
	sevenDayAvg metric.Float64ObservableGauge

	registration metric.Registration
}

func newSpO2Collector(log *slog.Logger) (Collector, error) {
	return &spO2Collector{log: log}, nil
}

func (c *spO2Collector) Name() string { return "spo2" }

func (c *spO2Collector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	c.average, err = meter.Float64ObservableGauge(
		"garmin.spo2.avg_percent",
		metric.WithDescription("Average SpO2 percentage for the day."),
		metric.WithUnit("%"),
	)
	if err != nil {
		return err
	}
	c.lowest, err = meter.Float64ObservableGauge(
		"garmin.spo2.lowest_percent",
		metric.WithDescription("Lowest SpO2 percentage for the day."),
		metric.WithUnit("%"),
	)
	if err != nil {
		return err
	}
	c.sevenDayAvg, err = meter.Float64ObservableGauge(
		"garmin.spo2.seven_day_avg_percent",
		metric.WithDescription("7-day average SpO2 percentage."),
		metric.WithUnit("%"),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.average, c.lowest, c.sevenDayAvg)
	return err
}

func (c *spO2Collector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || snap.SpO2 == nil {
		return nil
	}
	s := snap.SpO2
	if s.AverageSpO2 > 0 {
		o.ObserveFloat64(c.average, s.AverageSpO2)
	}
	if s.LowestSpO2 > 0 {
		o.ObserveFloat64(c.lowest, s.LowestSpO2)
	}
	if s.LastSevenDaysAvgSpO2 > 0 {
		o.ObserveFloat64(c.sevenDayAvg, s.LastSevenDaysAvgSpO2)
	}
	return nil
}

func (c *spO2Collector) Close() error {
	if c.registration == nil {
		return nil
	}
	return c.registration.Unregister()
}
