package collector

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("bloodpressure", defaultEnabled, newBloodPressureCollector)
}

type bloodPressureCollector struct {
	systolic  *prometheus.Desc
	diastolic *prometheus.Desc
	pulse     *prometheus.Desc
	logger    *slog.Logger
}

func newBloodPressureCollector(logger *slog.Logger) (Collector, error) {
	const sub = "blood_pressure"
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, sub, name), help, nil, nil)
	}
	return &bloodPressureCollector{
		systolic:  d("systolic_mmhg", "Most recent systolic blood pressure in mmHg."),
		diastolic: d("diastolic_mmhg", "Most recent diastolic blood pressure in mmHg."),
		pulse:     d("pulse_bpm", "Most recent pulse from blood pressure reading in bpm."),
		logger:    logger,
	}, nil
}

func (c *bloodPressureCollector) Update(ch chan<- prometheus.Metric) error {
	if garminClient == nil {
		return ErrNoData
	}

	now := time.Now()
	bp, err := garminClient.BloodPressure(now.AddDate(0, -1, 0), now)
	if err != nil {
		return err
	}
	if len(bp.Measurements) == 0 {
		return ErrNoData
	}

	latest := bp.Measurements[len(bp.Measurements)-1]
	g := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}

	if latest.Systolic > 0 {
		g(c.systolic, float64(latest.Systolic))
	}
	if latest.Diastolic > 0 {
		g(c.diastolic, float64(latest.Diastolic))
	}
	if latest.Pulse > 0 {
		g(c.pulse, float64(latest.Pulse))
	}
	return nil
}
