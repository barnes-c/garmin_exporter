// Copyright Christopher Barnes
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package collector

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("activities", defaultEnabled, newActivitiesCollector)
}

type activitiesCollector struct {
	count         *prometheus.Desc
	durationTotal *prometheus.Desc
	distanceTotal *prometheus.Desc
	caloriesTotal *prometheus.Desc
	lastTimestamp *prometheus.Desc
	logger        *slog.Logger
}

func newActivitiesCollector(logger *slog.Logger) (Collector, error) {
	const sub = "activity"
	labels := []string{"type"}
	return &activitiesCollector{
		count: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sub, "count"),
			"Number of activities fetched.", labels, nil),
		durationTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sub, "duration_seconds_total"),
			"Sum of activity durations in seconds.", labels, nil),
		distanceTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sub, "distance_meters_total"),
			"Sum of activity distances in meters.", labels, nil),
		caloriesTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sub, "calories_total"),
			"Sum of activity calories.", labels, nil),
		lastTimestamp: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sub, "last_timestamp_seconds"),
			"Unix timestamp of the most recent activity of this type.", labels, nil),
		logger: logger,
	}, nil
}

func (c *activitiesCollector) Update(ch chan<- prometheus.Metric) error {
	if garminClient == nil {
		return ErrNoData
	}

	activities, err := garminClient.Activities(activityLimit)
	if err != nil {
		return err
	}
	if len(activities) == 0 {
		return ErrNoData
	}

	type stats struct {
		count    float64
		duration float64
		distance float64
		calories float64
		lastTS   float64
	}
	byType := make(map[string]*stats)

	for _, a := range activities {
		typeKey := a.ActivityType.TypeKey
		if typeKey == "" {
			typeKey = "unknown"
		}
		s := byType[typeKey]
		if s == nil {
			s = &stats{}
			byType[typeKey] = s
		}
		s.count++
		s.duration += a.Duration
		s.distance += a.Distance
		s.calories += a.Calories

		if a.StartTimeGMT != "" {
			t, err := time.Parse("2006-01-02 15:04:05", a.StartTimeGMT)
			if err == nil {
				ts := float64(t.Unix())
				if ts > s.lastTS {
					s.lastTS = ts
				}
			}
		}
	}

	for typeKey, s := range byType {
		ch <- prometheus.MustNewConstMetric(c.count, prometheus.GaugeValue, s.count, typeKey)
		ch <- prometheus.MustNewConstMetric(c.durationTotal, prometheus.GaugeValue, s.duration, typeKey)
		ch <- prometheus.MustNewConstMetric(c.distanceTotal, prometheus.GaugeValue, s.distance, typeKey)
		ch <- prometheus.MustNewConstMetric(c.caloriesTotal, prometheus.GaugeValue, s.calories, typeKey)
		if s.lastTS > 0 {
			ch <- prometheus.MustNewConstMetric(c.lastTimestamp, prometheus.GaugeValue, s.lastTS, typeKey)
		}
	}
	return nil
}
