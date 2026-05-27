package collector

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("heartrate", defaultEnabled, newHeartRateCollector)
}

type heartRateCollector struct {
	resting     *prometheus.Desc
	min         *prometheus.Desc
	max         *prometheus.Desc
	sevenDayAvg *prometheus.Desc
	logger      *slog.Logger
}

func newHeartRateCollector(logger *slog.Logger) (Collector, error) {
	const sub = "heartrate"
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, sub, name), help, nil, nil)
	}
	return &heartRateCollector{
		resting:     d("resting_bpm", "Resting heart rate in bpm."),
		min:         d("min_bpm", "Minimum heart rate in bpm."),
		max:         d("max_bpm", "Maximum heart rate in bpm."),
		sevenDayAvg: d("seven_day_avg_resting_bpm", "7-day average resting heart rate in bpm."),
		logger:      logger,
	}, nil
}

func (c *heartRateCollector) Update(ch chan<- prometheus.Metric) error {
	client := getClient()
	if client == nil {
		return ErrNoData
	}

	hr, err := client.HeartRates(time.Now())
	if err != nil {
		return err
	}

	g := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}

	if hr.RestingHeartRate > 0 {
		g(c.resting, float64(hr.RestingHeartRate))
	}
	if hr.MinHeartRate > 0 {
		g(c.min, float64(hr.MinHeartRate))
	}

	var maxBPM int
	if acts, err := client.Activities(activityLimit); err == nil {
		for _, a := range acts {
			if int(a.MaxHR) > maxBPM {
				maxBPM = int(a.MaxHR)
			}
		}
	}
	if maxBPM == 0 {
		maxBPM = hr.MaxHeartRate
	}
	if maxBPM > 0 {
		g(c.max, float64(maxBPM))
	}

	if hr.LastSevenDaysAvgRestingHeartRate > 0 {
		g(c.sevenDayAvg, float64(hr.LastSevenDaysAvgRestingHeartRate))
	}
	return nil
}
