package collector

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("hydration", defaultEnabled, newHydrationCollector)
}

type hydrationCollector struct {
	intakeML       *prometheus.Desc
	goalML         *prometheus.Desc
	dailyAvgML     *prometheus.Desc
	sweatLossML    *prometheus.Desc
	activityIntake *prometheus.Desc
	logger         *slog.Logger
}

func newHydrationCollector(logger *slog.Logger) (Collector, error) {
	const sub = "hydration"
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, sub, name), help, nil, nil)
	}
	return &hydrationCollector{
		intakeML:       d("intake_ml", "Total hydration intake in millilitres."),
		goalML:         d("goal_ml", "Daily hydration goal in millilitres."),
		dailyAvgML:     d("daily_avg_ml", "Daily average hydration intake in millilitres."),
		sweatLossML:    d("sweat_loss_ml", "Estimated sweat loss in millilitres."),
		activityIntake: d("activity_intake_ml", "Hydration intake during activities in millilitres."),
		logger:         logger,
	}, nil
}

func (c *hydrationCollector) Update(ctx context.Context, ch chan<- prometheus.Metric) error {
	client := getClient()
	if client == nil {
		return ErrNoData
	}

	h, err := client.Hydration(ctx, time.Now())
	if err != nil {
		return err
	}

	g := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}

	g(c.intakeML, h.ValueInML)
	g(c.goalML, h.GoalInML)
	if h.DailyAverageinML > 0 {
		g(c.dailyAvgML, h.DailyAverageinML)
	}
	if h.SweatLossInML > 0 {
		g(c.sweatLossML, h.SweatLossInML)
	}
	if h.ActivityIntakeInML > 0 {
		g(c.activityIntake, h.ActivityIntakeInML)
	}
	return nil
}
