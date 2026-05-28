package collector

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("intensity", defaultEnabled, newIntensityCollector)
}

type intensityCollector struct {
	weeklyGoal   *prometheus.Desc
	moderateMins *prometheus.Desc
	vigorousMins *prometheus.Desc
	logger       *slog.Logger
}

func newIntensityCollector(logger *slog.Logger) (Collector, error) {
	const sub = "intensity"
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, sub, name), help, nil, nil)
	}
	return &intensityCollector{
		weeklyGoal:   d("weekly_goal_minutes", "Weekly intensity minutes goal."),
		moderateMins: d("moderate_minutes_total", "Cumulative moderate intensity minutes this week."),
		vigorousMins: d("vigorous_minutes_total", "Cumulative vigorous intensity minutes this week."),
		logger:       logger,
	}, nil
}

func (c *intensityCollector) Update(ch chan<- prometheus.Metric, date time.Time) error {
	client := getClient()
	if client == nil {
		return ErrNoData
	}

	im, err := client.IntensityMinutes(date)
	if err != nil {
		return err
	}

	g := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}

	g(c.weeklyGoal, float64(im.WeeklyGoal))
	g(c.moderateMins, float64(im.ModerateIntensityMinutes))
	g(c.vigorousMins, float64(im.VigorousIntensityMinutes))
	return nil
}
