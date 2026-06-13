package collector

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("activities", DefaultEnabled, newActivitiesCollector)
}

type activitiesCollector struct {
	log *slog.Logger
	src garmin.Source

	count         metric.Int64ObservableGauge
	durationTotal metric.Float64ObservableGauge
	distanceTotal metric.Float64ObservableGauge
	caloriesTotal metric.Float64ObservableGauge
	lastTimestamp metric.Int64ObservableGauge
	lifetimeCount metric.Int64ObservableGauge

	registration metric.Registration
}

func newActivitiesCollector(log *slog.Logger) (Collector, error) {
	return &activitiesCollector{log: log}, nil
}

func (c *activitiesCollector) Name() string { return "activities" }

func (c *activitiesCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	if c.count, err = meter.Int64ObservableGauge(
		"garmin.activity.count",
		metric.WithDescription("Number of activities fetched."),
		metric.WithUnit("{activity}"),
	); err != nil {
		return err
	}
	if c.durationTotal, err = meter.Float64ObservableGauge(
		"garmin.activity.duration_seconds_total",
		metric.WithDescription("Sum of activity durations in seconds."),
		metric.WithUnit("s"),
	); err != nil {
		return err
	}
	if c.distanceTotal, err = meter.Float64ObservableGauge(
		"garmin.activity.distance_meters_total",
		metric.WithDescription("Sum of activity distances in meters."),
		metric.WithUnit("m"),
	); err != nil {
		return err
	}
	if c.caloriesTotal, err = meter.Float64ObservableGauge(
		"garmin.activity.calories_total",
		metric.WithDescription("Sum of activity calories."),
		metric.WithUnit("kcal"),
	); err != nil {
		return err
	}
	if c.lastTimestamp, err = meter.Int64ObservableGauge(
		"garmin.activity.last_timestamp_seconds",
		metric.WithDescription("Unix timestamp of the most recent activity of this type."),
		metric.WithUnit("s"),
	); err != nil {
		return err
	}
	if c.lifetimeCount, err = meter.Int64ObservableGauge(
		"garmin.activity.lifetime_count",
		metric.WithDescription("Total number of activities ever recorded."),
		metric.WithUnit("{activity}"),
	); err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.count, c.durationTotal, c.distanceTotal, c.caloriesTotal,
		c.lastTimestamp, c.lifetimeCount,
	)
	return err
}

func (c *activitiesCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || snap.Activities == nil {
		return nil
	}
	a := snap.Activities

	if a.Lifetime > 0 {
		o.ObserveInt64(c.lifetimeCount, int64(a.Lifetime))
	}
	if len(a.Recent) == 0 {
		return nil
	}

	type stats struct {
		count    int64
		duration float64
		distance float64
		calories float64
		lastTS   int64
	}
	byType := make(map[string]*stats)

	for _, act := range a.Recent {
		typeKey := act.ActivityType.TypeKey
		if typeKey == "" {
			typeKey = "unknown"
		}
		s := byType[typeKey]
		if s == nil {
			s = &stats{}
			byType[typeKey] = s
		}
		s.count++
		s.duration += act.Duration
		s.distance += act.Distance
		s.calories += act.Calories

		if act.StartTimeGMT != "" {
			t, err := time.Parse("2006-01-02 15:04:05", act.StartTimeGMT)
			if err != nil {
				c.log.Warn("failed to parse activity timestamp", "value", act.StartTimeGMT, "err", err)
			} else if ts := t.Unix(); ts > s.lastTS {
				s.lastTS = ts
			}
		}
	}

	for typeKey, s := range byType {
		attrs := metric.WithAttributes(attribute.String("type", typeKey))
		o.ObserveInt64(c.count, s.count, attrs)
		o.ObserveFloat64(c.durationTotal, s.duration, attrs)
		o.ObserveFloat64(c.distanceTotal, s.distance, attrs)
		o.ObserveFloat64(c.caloriesTotal, s.calories, attrs)
		if s.lastTS > 0 {
			o.ObserveInt64(c.lastTimestamp, s.lastTS, attrs)
		}
	}
	return nil
}

func (c *activitiesCollector) Close() error {
	if c.registration == nil {
		return nil
	}
	return c.registration.Unregister()
}
