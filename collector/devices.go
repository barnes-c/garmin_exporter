package collector

import (
	"log/slog"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("devices", defaultEnabled, newDevicesCollector)
}

type devicesCollector struct {
	info   *prometheus.Desc
	count  *prometheus.Desc
	logger *slog.Logger
}

func newDevicesCollector(logger *slog.Logger) (Collector, error) {
	const sub = "device"
	return &devicesCollector{
		info: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sub, "info"),
			"Garmin device information (value is always 1).",
			[]string{"device_id", "name", "type", "status"}, nil),
		count: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sub, "count"),
			"Number of registered Garmin devices.",
			nil, nil),
		logger: logger,
	}, nil
}

func (c *devicesCollector) Update(ch chan<- prometheus.Metric) error {
	client := getClient()
	if client == nil {
		return ErrNoData
	}

	devices, err := client.Devices()
	if err != nil {
		return err
	}

	ch <- prometheus.MustNewConstMetric(c.count, prometheus.GaugeValue, float64(len(devices)))

	for _, d := range devices {
		ch <- prometheus.MustNewConstMetric(c.info, prometheus.GaugeValue, 1,
			strconv.FormatInt(d.DeviceID, 10),
			d.ProductDisplayName,
			d.DisplayName,
			d.DeviceStatus,
		)
	}
	return nil
}
