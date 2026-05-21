package collector

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("personalrecords", defaultEnabled, newPersonalRecordsCollector)
}

type personalRecordsCollector struct {
	value  *prometheus.Desc
	logger *slog.Logger
}

func newPersonalRecordsCollector(logger *slog.Logger) (Collector, error) {
	return &personalRecordsCollector{
		value: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "personalrecords", "value"),
			"Personal record value in raw Garmin units (varies by pr_type).",
			[]string{"pr_type"}, nil),
		logger: logger,
	}, nil
}

func (c *personalRecordsCollector) Update(ch chan<- prometheus.Metric) error {
	if garminClient == nil {
		return ErrNoData
	}

	records, err := garminClient.PersonalRecords()
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return ErrNoData
	}

	for _, r := range records {
		if r.PrTypeLabelKey == "" || r.Value == 0 {
			continue
		}
		ch <- prometheus.MustNewConstMetric(c.value, prometheus.GaugeValue, r.Value, r.PrTypeLabelKey)
	}
	return nil
}
