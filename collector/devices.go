package collector

import (
	"context"
	"log/slog"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("devices", newDevicesCollector)
}

type devicesCollector struct {
	registrar
	log *slog.Logger
	src garmin.Source

	info  metric.Int64ObservableGauge
	count metric.Int64ObservableGauge
}

func newDevicesCollector(log *slog.Logger) (Collector, error) {
	return &devicesCollector{log: log}, nil
}

func (c *devicesCollector) Name() string { return "devices" }

func (c *devicesCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	c.info, err = meter.Int64ObservableGauge(
		"garmin.device.info",
		metric.WithDescription("Garmin device information (value is always 1)."),
		metric.WithUnit("{device}"),
	)
	if err != nil {
		return err
	}
	c.count, err = meter.Int64ObservableGauge(
		"garmin.device.count",
		metric.WithDescription("Number of registered Garmin devices."),
		metric.WithUnit("{device}"),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe, c.info, c.count)
	return err
}

func (c *devicesCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil {
		return nil
	}
	devices := snap.Devices
	o.ObserveInt64(c.count, int64(len(devices)))
	for _, d := range devices {
		o.ObserveInt64(c.info, 1, metric.WithAttributes(
			attribute.String("device_id", strconv.FormatInt(d.DeviceID, 10)),
			attribute.String("name", d.ProductDisplayName),
			attribute.String("type", d.DisplayName),
			attribute.String("status", d.DeviceStatus),
		))
	}
	return nil
}
