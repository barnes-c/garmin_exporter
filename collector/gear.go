package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("gear", DefaultEnabled, newGearCollector,
		SnapshotHas(func(s *garmin.Snapshot) bool { return s.Gear != nil }))
}

type gearCollector struct {
	registrar
	log *slog.Logger
	src garmin.Source

	maxMeters        metric.Int64ObservableGauge
	notifiedAtMeters metric.Int64ObservableGauge
	active           metric.Int64ObservableGauge
}

func newGearCollector(log *slog.Logger) (Collector, error) {
	return &gearCollector{log: log}, nil
}

func (c *gearCollector) Name() string { return "gear" }

func (c *gearCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	c.maxMeters, err = meter.Int64ObservableGauge(
		"garmin.gear.max_meters",
		metric.WithDescription("Retirement distance limit for the gear in meters."),
		metric.WithUnit("m"),
	)
	if err != nil {
		return err
	}
	c.notifiedAtMeters, err = meter.Int64ObservableGauge(
		"garmin.gear.notified_at_meters",
		metric.WithDescription("Distance at which a retirement notification is sent, in meters."),
		metric.WithUnit("m"),
	)
	if err != nil {
		return err
	}
	c.active, err = meter.Int64ObservableGauge(
		"garmin.gear.active",
		metric.WithDescription("1 if the gear is active, 0 otherwise."),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.maxMeters, c.notifiedAtMeters, c.active)
	return err
}

func (c *gearCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil {
		return nil
	}
	for _, g := range snap.Gear {
		name := g.DisplayName
		if name == "" {
			name = g.CustomMakeModel
		}
		attrs := metric.WithAttributes(
			attribute.String("gear_name", name),
			attribute.String("gear_type", g.GearTypeName),
		)
		o.ObserveInt64(c.maxMeters, int64(g.MaxMeters), attrs)
		o.ObserveInt64(c.notifiedAtMeters, int64(g.NotifiedMeters), attrs)
		var active int64
		if g.GearStatusName == "active" {
			active = 1
		}
		o.ObserveInt64(c.active, active, attrs)
	}
	return nil
}
