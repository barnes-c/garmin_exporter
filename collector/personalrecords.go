package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("personalrecords", DefaultEnabled, newPersonalRecordsCollector)
}

type personalRecordsCollector struct {
	log *slog.Logger
	src garmin.Source

	value metric.Float64ObservableGauge

	registration metric.Registration
}

func newPersonalRecordsCollector(log *slog.Logger) (Collector, error) {
	return &personalRecordsCollector{log: log}, nil
}

func (c *personalRecordsCollector) Name() string { return "personalrecords" }

func (c *personalRecordsCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	c.value, err = meter.Float64ObservableGauge(
		"garmin.personalrecords.value",
		metric.WithDescription("Personal record value in raw Garmin units (varies by pr_type)."),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe, c.value)
	return err
}

func (c *personalRecordsCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil {
		return nil
	}
	for _, r := range snap.PersonalRecords {
		if r.PrTypeLabelKey == "" || r.Value == 0 {
			continue
		}
		o.ObserveFloat64(c.value, r.Value, metric.WithAttributes(
			attribute.String("pr_type", r.PrTypeLabelKey),
		))
	}
	return nil
}

func (c *personalRecordsCollector) Close() error {
	if c.registration == nil {
		return nil
	}
	return c.registration.Unregister()
}
