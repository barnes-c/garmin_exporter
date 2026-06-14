package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("golf", newGolfCollector)
}

type golfCollector struct {
	registrar
	log *slog.Logger
	src garmin.Source

	lastScore metric.Int64ObservableGauge
	lastToPar metric.Int64ObservableGauge
}

func newGolfCollector(log *slog.Logger) (Collector, error) {
	return &golfCollector{log: log}, nil
}

func (c *golfCollector) Name() string { return "golf" }

func (c *golfCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	var err error
	c.lastScore, err = meter.Int64ObservableGauge(
		"garmin.golf.last_round_score",
		metric.WithDescription("Total score of the most recent golf round."),
		metric.WithUnit("{stroke}"),
	)
	if err != nil {
		return err
	}
	c.lastToPar, err = meter.Int64ObservableGauge(
		"garmin.golf.last_round_to_par",
		metric.WithDescription("Score relative to par for the most recent golf round."),
		metric.WithUnit("{stroke}"),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.lastScore, c.lastToPar)
	return err
}

func (c *golfCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || len(snap.Golf) == 0 {
		return nil
	}
	latest := snap.Golf[0]
	o.ObserveInt64(c.lastScore, int64(latest.TotalScore))
	o.ObserveInt64(c.lastToPar, int64(latest.ToPar))
	return nil
}
