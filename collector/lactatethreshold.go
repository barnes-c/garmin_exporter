package collector

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("lactatethreshold", defaultEnabled, newLactateThresholdCollector)
}

type lactateThresholdCollector struct {
	runningSpeedMPS  *prometheus.Desc
	runningHeartRate *prometheus.Desc
	cyclingHeartRate *prometheus.Desc
	logger           *slog.Logger
}

func newLactateThresholdCollector(logger *slog.Logger) (Collector, error) {
	const sub = "lactatethreshold"
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, sub, name), help, nil, nil)
	}
	return &lactateThresholdCollector{
		runningSpeedMPS:  d("running_speed_mps", "Running lactate threshold speed in meters per second."),
		runningHeartRate: d("running_heart_rate_bpm", "Running lactate threshold heart rate in bpm."),
		cyclingHeartRate: d("cycling_heart_rate_bpm", "Cycling lactate threshold heart rate in bpm."),
		logger:           logger,
	}, nil
}

func (c *lactateThresholdCollector) Update(ch chan<- prometheus.Metric) error {
	if garminClient == nil {
		return ErrNoData
	}

	entries, err := garminClient.LactateThreshold()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return ErrNoData
	}

	e := entries[0]
	if e.Speed != nil && *e.Speed > 0 {
		ch <- prometheus.MustNewConstMetric(c.runningSpeedMPS, prometheus.GaugeValue, *e.Speed)
	}
	if e.HearRate != nil && *e.HearRate > 0 {
		ch <- prometheus.MustNewConstMetric(c.runningHeartRate, prometheus.GaugeValue, float64(*e.HearRate))
	}
	if e.HeartRateCycling != nil && *e.HeartRateCycling > 0 {
		ch <- prometheus.MustNewConstMetric(c.cyclingHeartRate, prometheus.GaugeValue, float64(*e.HeartRateCycling))
	}
	return nil
}
