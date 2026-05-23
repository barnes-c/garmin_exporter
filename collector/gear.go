package collector

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("gear", defaultEnabled, newGearCollector)
}

type gearCollector struct {
	maxMeters        *prometheus.Desc
	notifiedAtMeters *prometheus.Desc
	active           *prometheus.Desc
	logger           *slog.Logger
}

func newGearCollector(logger *slog.Logger) (Collector, error) {
	const sub = "gear"
	labels := []string{"gear_name", "gear_type"}
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, sub, name), help, labels, nil)
	}
	return &gearCollector{
		maxMeters:        d("max_meters", "Retirement distance limit for the gear in meters."),
		notifiedAtMeters: d("notified_at_meters", "Distance at which a retirement notification is sent, in meters."),
		active:           d("active", "1 if the gear is active, 0 otherwise."),
		logger:           logger,
	}, nil
}

func (c *gearCollector) Update(ch chan<- prometheus.Metric) error {
	client := getClient()
	if client == nil {
		return ErrNoData
	}

	summary, err := client.UserSummary(time.Now())
	if err != nil {
		return err
	}

	gear, err := client.Gear(summary.UserProfileID)
	if err != nil {
		return err
	}
	if len(gear) == 0 {
		return ErrNoData
	}

	for _, g := range gear {
		name := g.DisplayName
		if name == "" {
			name = g.CustomMakeModel
		}
		gtype := g.GearTypeName
		labels := []string{name, gtype}

		ch <- prometheus.MustNewConstMetric(c.maxMeters, prometheus.GaugeValue, float64(g.MaxMeters), labels...)
		ch <- prometheus.MustNewConstMetric(c.notifiedAtMeters, prometheus.GaugeValue, float64(g.NotifiedMeters), labels...)

		var activeVal float64
		if g.GearStatusName == "active" {
			activeVal = 1
		}
		ch <- prometheus.MustNewConstMetric(c.active, prometheus.GaugeValue, activeVal, labels...)
	}
	return nil
}
