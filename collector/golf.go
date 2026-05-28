package collector

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("golf", defaultDisabled, newGolfCollector)
}

type golfCollector struct {
	lastScore *prometheus.Desc
	lastToPar *prometheus.Desc
	logger    *slog.Logger
}

func newGolfCollector(logger *slog.Logger) (Collector, error) {
	const sub = "golf"
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, sub, name), help, nil, nil)
	}
	return &golfCollector{
		lastScore: d("last_round_score", "Total score of the most recent golf round."),
		lastToPar: d("last_round_to_par", "Score relative to par for the most recent golf round."),
		logger:    logger,
	}, nil
}

func (c *golfCollector) Update(ch chan<- prometheus.Metric, _ time.Time) error {
	client := getClient()
	if client == nil {
		return ErrNoData
	}

	scorecards, err := client.GolfSummary(0, 1)
	if err != nil {
		return err
	}
	if len(scorecards) == 0 {
		return ErrNoData
	}

	g := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}

	latest := scorecards[0]
	g(c.lastScore, float64(latest.TotalScore))
	g(c.lastToPar, float64(latest.ToPar))
	return nil
}
