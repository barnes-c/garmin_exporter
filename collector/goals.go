package collector

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("goals", defaultEnabled, newGoalsCollector)
}

type goalsCollector struct {
	activeGoals  *prometheus.Desc
	earnedBadges *prometheus.Desc
	logger       *slog.Logger
}

func newGoalsCollector(logger *slog.Logger) (Collector, error) {
	const sub = "goals"
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, sub, name), help, nil, nil)
	}
	return &goalsCollector{
		activeGoals:  d("active_total", "Number of active fitness goals."),
		earnedBadges: d("earned_badges_total", "Total number of earned achievement badges."),
		logger:       logger,
	}, nil
}

func (c *goalsCollector) Update(ch chan<- prometheus.Metric, _ time.Time) error {
	client := getClient()
	if client == nil {
		return ErrNoData
	}

	goals, err := client.Goals("active", 0, 100)
	if err != nil {
		c.logger.Debug("goals unavailable", "err", err)
	} else {
		ch <- prometheus.MustNewConstMetric(c.activeGoals, prometheus.GaugeValue, float64(len(goals)))
	}

	badges, err := client.EarnedBadges()
	if err != nil {
		c.logger.Debug("badges unavailable", "err", err)
	} else {
		ch <- prometheus.MustNewConstMetric(c.earnedBadges, prometheus.GaugeValue, float64(len(badges)))
	}

	return nil
}
