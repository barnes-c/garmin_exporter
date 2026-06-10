package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("cycling", DefaultEnabled, newCyclingCollector)
}

type cyclingCollector struct {
	log *slog.Logger
	src garmin.Source

	ftp metric.Float64ObservableGauge

	registration metric.Registration
}

func newCyclingCollector(log *slog.Logger) (Collector, error) {
	return &cyclingCollector{log: log}, nil
}

func (c *cyclingCollector) Name() string { return "cycling" }

func (c *cyclingCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	c.ftp, err = meter.Float64ObservableGauge(
		"garmin.cycling.ftp_watts",
		metric.WithDescription("Cycling functional threshold power in watts."),
		metric.WithUnit("W"),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe, c.ftp)
	return err
}

func (c *cyclingCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || snap.Cycling == nil || snap.Cycling.FTPWatts == 0 {
		return nil
	}
	o.ObserveFloat64(c.ftp, snap.Cycling.FTPWatts)
	return nil
}

func (c *cyclingCollector) Close() error {
	if c.registration == nil {
		return nil
	}
	return c.registration.Unregister()
}
