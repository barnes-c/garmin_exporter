package collector

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("sleep", defaultEnabled, newSleepCollector)
}

type sleepCollector struct {
	sleepSeconds             *prometheus.Desc
	napSeconds               *prometheus.Desc
	deepSleepSeconds         *prometheus.Desc
	lightSleepSeconds        *prometheus.Desc
	remSleepSeconds          *prometheus.Desc
	awakeSeconds             *prometheus.Desc
	restlessMoments          *prometheus.Desc
	avgRespiration           *prometheus.Desc
	lowestRespiration        *prometheus.Desc
	highestRespiration       *prometheus.Desc
	avgStress                *prometheus.Desc
	spO2Avg                  *prometheus.Desc
	spO2Low                  *prometheus.Desc
	hrvWeeklyAvg             *prometheus.Desc
	hrvLastNight             *prometheus.Desc
	hrvLastNight5MinHigh     *prometheus.Desc
	hrvBaselineLowUpper      *prometheus.Desc
	hrvBaselineBalancedLow   *prometheus.Desc
	hrvBaselineBalancedUpper *prometheus.Desc
	logger                   *slog.Logger
}

func newSleepCollector(logger *slog.Logger) (Collector, error) {
	const sub = "sleep"
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, sub, name), help, nil, nil)
	}
	return &sleepCollector{
		sleepSeconds:             d("total_seconds", "Total sleep duration in seconds."),
		napSeconds:               d("nap_seconds", "Nap duration in seconds."),
		deepSleepSeconds:         d("deep_seconds", "Deep sleep duration in seconds."),
		lightSleepSeconds:        d("light_seconds", "Light sleep duration in seconds."),
		remSleepSeconds:          d("rem_seconds", "REM sleep duration in seconds."),
		awakeSeconds:             d("awake_seconds", "Awake time during sleep period in seconds."),
		restlessMoments:          d("restless_moments_total", "Number of restless moments during sleep."),
		avgRespiration:           d("avg_respiration_bpm", "Average respiration rate during sleep."),
		lowestRespiration:        d("lowest_respiration_bpm", "Lowest respiration rate during sleep."),
		highestRespiration:       d("highest_respiration_bpm", "Highest respiration rate during sleep."),
		avgStress:                d("avg_stress", "Average stress level during sleep."),
		spO2Avg:                  d("spo2_avg_percent", "Average SpO2 reading during sleep."),
		spO2Low:                  d("spo2_low_percent", "Lowest SpO2 reading during sleep."),
		hrvWeeklyAvg:             d("hrv_weekly_avg_ms", "7-day average HRV in ms."),
		hrvLastNight:             d("hrv_last_night_ms", "Last night average HRV in ms."),
		hrvLastNight5MinHigh:     d("hrv_last_night_5min_high_ms", "Last night 5-minute high HRV in ms."),
		hrvBaselineLowUpper:      d("hrv_baseline_low_upper_ms", "HRV baseline low zone upper bound in ms."),
		hrvBaselineBalancedLow:   d("hrv_baseline_balanced_low_ms", "HRV baseline balanced zone lower bound in ms."),
		hrvBaselineBalancedUpper: d("hrv_baseline_balanced_upper_ms", "HRV baseline balanced zone upper bound in ms."),
		logger:                   logger,
	}, nil
}

func (c *sleepCollector) Update(ch chan<- prometheus.Metric) error {
	client := getClient()
	if client == nil {
		return ErrNoData
	}
	now := time.Now()

	s, err := client.SleepData(now)
	if err != nil {
		return err
	}

	g := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}

	dto := s.DailySleepDTO
	g(c.sleepSeconds, float64(dto.SleepTimeSeconds))
	g(c.napSeconds, float64(dto.NapTimeSeconds))
	g(c.deepSleepSeconds, float64(dto.DeepSleepSeconds))
	g(c.lightSleepSeconds, float64(dto.LightSleepSeconds))
	g(c.remSleepSeconds, float64(dto.REMSleepSeconds))
	g(c.awakeSeconds, float64(dto.AwakeSeconds))
	g(c.restlessMoments, float64(s.RestlessMomentsCount))
	if dto.AverageRespirationValue > 0 {
		g(c.avgRespiration, dto.AverageRespirationValue)
	}
	if dto.LowestRespirationValue > 0 {
		g(c.lowestRespiration, dto.LowestRespirationValue)
	}
	if dto.HighestRespirationValue > 0 {
		g(c.highestRespiration, dto.HighestRespirationValue)
	}
	if dto.AvgSleepStress > 0 {
		g(c.avgStress, dto.AvgSleepStress)
	}
	if dto.SpO2AvgReadingPercent > 0 {
		g(c.spO2Avg, dto.SpO2AvgReadingPercent)
	}
	if dto.SpO2LowReadingPercent > 0 {
		g(c.spO2Low, dto.SpO2LowReadingPercent)
	}

	c.collectHRV(ch, now)
	return nil
}

func (c *sleepCollector) collectHRV(ch chan<- prometheus.Metric, now time.Time) {
	client := getClient()
	h, err := client.HRVData(now)
	if err != nil {
		c.logger.Debug("HRV data unavailable", "err", err)
		return
	}
	g := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}
	hrv := h.HRVSummary
	if hrv.WeeklyAvg > 0 {
		g(c.hrvWeeklyAvg, float64(hrv.WeeklyAvg))
	}
	if hrv.LastNight > 0 {
		g(c.hrvLastNight, float64(hrv.LastNight))
	}
	if hrv.LastNight5MinHigh > 0 {
		g(c.hrvLastNight5MinHigh, float64(hrv.LastNight5MinHigh))
	}
	if hrv.Baseline.LowUpper > 0 {
		g(c.hrvBaselineLowUpper, float64(hrv.Baseline.LowUpper))
	}
	if hrv.Baseline.BalancedLow > 0 {
		g(c.hrvBaselineBalancedLow, float64(hrv.Baseline.BalancedLow))
	}
	if hrv.Baseline.BalancedUpper > 0 {
		g(c.hrvBaselineBalancedUpper, float64(hrv.Baseline.BalancedUpper))
	}
}
