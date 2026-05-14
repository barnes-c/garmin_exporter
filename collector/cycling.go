package collector

import (
	"encoding/json"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("cycling", defaultEnabled, newCyclingCollector)
}

type cyclingCollector struct {
	ftp    *prometheus.Desc
	logger *slog.Logger
}

func newCyclingCollector(logger *slog.Logger) (Collector, error) {
	const sub = "cycling"
	return &cyclingCollector{
		ftp: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sub, "ftp_watts"),
			"Cycling functional threshold power in watts.", nil, nil),
		logger: logger,
	}, nil
}

func (c *cyclingCollector) Update(ch chan<- prometheus.Metric) error {
	if garminClient == nil {
		return ErrNoData
	}

	result, err := garminClient.CyclingFTP()
	if err != nil {
		return err
	}

	raw, ok := result["mostRecentBiometric"]
	if !ok {
		return ErrNoData
	}

	var bio struct {
		Value float64 `json:"value"`
	}
	if err := json.Unmarshal(raw, &bio); err != nil || bio.Value == 0 {
		return ErrNoData
	}

	ch <- prometheus.MustNewConstMetric(c.ftp, prometheus.GaugeValue, bio.Value)
	return nil
}
