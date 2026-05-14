package collector

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("spo2", defaultEnabled, newSpO2Collector)
}

type spO2Collector struct {
	average     *prometheus.Desc
	lowest      *prometheus.Desc
	sevenDayAvg *prometheus.Desc
	logger      *slog.Logger
}

func newSpO2Collector(logger *slog.Logger) (Collector, error) {
	const sub = "spo2"
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, sub, name), help, nil, nil)
	}
	return &spO2Collector{
		average:     d("avg_percent", "Average SpO2 percentage for the day."),
		lowest:      d("lowest_percent", "Lowest SpO2 percentage for the day."),
		sevenDayAvg: d("seven_day_avg_percent", "7-day average SpO2 percentage."),
		logger:      logger,
	}, nil
}

func (c *spO2Collector) Update(ch chan<- prometheus.Metric) error {
	if garminClient == nil {
		return ErrNoData
	}

	s, err := garminClient.SpO2(time.Now())
	if err != nil {
		return err
	}

	g := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}

	if s.AverageSpO2 > 0 {
		g(c.average, s.AverageSpO2)
	}
	if s.LowestSpO2 > 0 {
		g(c.lowest, s.LowestSpO2)
	}
	if s.LastSevenDaysAvgSpO2 > 0 {
		g(c.sevenDayAvg, s.LastSevenDaysAvgSpO2)
	}
	return nil
}
