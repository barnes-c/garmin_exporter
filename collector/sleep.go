package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("sleep", DefaultEnabled, newSleepCollector)
}

type sleepCollector struct {
	log *slog.Logger
	src garmin.Source

	sleepSeconds             metric.Int64ObservableGauge
	napSeconds               metric.Int64ObservableGauge
	deepSleepSeconds         metric.Int64ObservableGauge
	lightSleepSeconds        metric.Int64ObservableGauge
	remSleepSeconds          metric.Int64ObservableGauge
	awakeSeconds             metric.Int64ObservableGauge
	restlessMoments          metric.Int64ObservableGauge
	avgRespiration           metric.Float64ObservableGauge
	lowestRespiration        metric.Float64ObservableGauge
	highestRespiration       metric.Float64ObservableGauge
	avgStress                metric.Float64ObservableGauge
	spO2Avg                  metric.Float64ObservableGauge
	spO2Low                  metric.Float64ObservableGauge
	hrvWeeklyAvg             metric.Int64ObservableGauge
	hrvLastNight             metric.Int64ObservableGauge
	hrvLastNight5MinHigh     metric.Int64ObservableGauge
	hrvBaselineLowUpper      metric.Int64ObservableGauge
	hrvBaselineBalancedLow   metric.Int64ObservableGauge
	hrvBaselineBalancedUpper metric.Int64ObservableGauge

	registration metric.Registration
}

func newSleepCollector(log *slog.Logger) (Collector, error) {
	return &sleepCollector{log: log}, nil
}

func (c *sleepCollector) Name() string { return "sleep" }

func (c *sleepCollector) Register(meter metric.Meter, src garmin.Source) error {
	c.src = src

	intGauge := func(name, desc, unit string) (metric.Int64ObservableGauge, error) {
		return meter.Int64ObservableGauge(name,
			metric.WithDescription(desc), metric.WithUnit(unit))
	}
	floatGauge := func(name, desc, unit string) (metric.Float64ObservableGauge, error) {
		return meter.Float64ObservableGauge(name,
			metric.WithDescription(desc), metric.WithUnit(unit))
	}

	var err error
	if c.sleepSeconds, err = intGauge("garmin.sleep.total_seconds", "Total sleep duration in seconds.", "s"); err != nil {
		return err
	}
	if c.napSeconds, err = intGauge("garmin.sleep.nap_seconds", "Nap duration in seconds.", "s"); err != nil {
		return err
	}
	if c.deepSleepSeconds, err = intGauge("garmin.sleep.deep_seconds", "Deep sleep duration in seconds.", "s"); err != nil {
		return err
	}
	if c.lightSleepSeconds, err = intGauge("garmin.sleep.light_seconds", "Light sleep duration in seconds.", "s"); err != nil {
		return err
	}
	if c.remSleepSeconds, err = intGauge("garmin.sleep.rem_seconds", "REM sleep duration in seconds.", "s"); err != nil {
		return err
	}
	if c.awakeSeconds, err = intGauge("garmin.sleep.awake_seconds", "Awake time during sleep period in seconds.", "s"); err != nil {
		return err
	}
	if c.restlessMoments, err = intGauge("garmin.sleep.restless_moments_total", "Number of restless moments during sleep.", "{moment}"); err != nil {
		return err
	}
	if c.avgRespiration, err = floatGauge("garmin.sleep.avg_respiration_bpm", "Average respiration rate during sleep.", "{breath}/min"); err != nil {
		return err
	}
	if c.lowestRespiration, err = floatGauge("garmin.sleep.lowest_respiration_bpm", "Lowest respiration rate during sleep.", "{breath}/min"); err != nil {
		return err
	}
	if c.highestRespiration, err = floatGauge("garmin.sleep.highest_respiration_bpm", "Highest respiration rate during sleep.", "{breath}/min"); err != nil {
		return err
	}
	if c.avgStress, err = floatGauge("garmin.sleep.avg_stress", "Average stress level during sleep.", "{score}"); err != nil {
		return err
	}
	if c.spO2Avg, err = floatGauge("garmin.sleep.spo2_avg_percent", "Average SpO2 reading during sleep.", "%"); err != nil {
		return err
	}
	if c.spO2Low, err = floatGauge("garmin.sleep.spo2_low_percent", "Lowest SpO2 reading during sleep.", "%"); err != nil {
		return err
	}
	if c.hrvWeeklyAvg, err = intGauge("garmin.sleep.hrv_weekly_avg_ms", "7-day average HRV in ms.", "ms"); err != nil {
		return err
	}
	if c.hrvLastNight, err = intGauge("garmin.sleep.hrv_last_night_ms", "Last night average HRV in ms.", "ms"); err != nil {
		return err
	}
	if c.hrvLastNight5MinHigh, err = intGauge("garmin.sleep.hrv_last_night_5min_high_ms", "Last night 5-minute high HRV in ms.", "ms"); err != nil {
		return err
	}
	if c.hrvBaselineLowUpper, err = intGauge("garmin.sleep.hrv_baseline_low_upper_ms", "HRV baseline low zone upper bound in ms.", "ms"); err != nil {
		return err
	}
	if c.hrvBaselineBalancedLow, err = intGauge("garmin.sleep.hrv_baseline_balanced_low_ms", "HRV baseline balanced zone lower bound in ms.", "ms"); err != nil {
		return err
	}
	if c.hrvBaselineBalancedUpper, err = intGauge("garmin.sleep.hrv_baseline_balanced_upper_ms", "HRV baseline balanced zone upper bound in ms.", "ms"); err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.sleepSeconds, c.napSeconds, c.deepSleepSeconds, c.lightSleepSeconds, c.remSleepSeconds, c.awakeSeconds, c.restlessMoments,
		c.avgRespiration, c.lowestRespiration, c.highestRespiration, c.avgStress,
		c.spO2Avg, c.spO2Low,
		c.hrvWeeklyAvg, c.hrvLastNight, c.hrvLastNight5MinHigh,
		c.hrvBaselineLowUpper, c.hrvBaselineBalancedLow, c.hrvBaselineBalancedUpper,
	)
	return err
}

func (c *sleepCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || snap.Sleep == nil {
		return nil
	}
	if snap.Sleep.Data != nil {
		s := snap.Sleep.Data
		dto := s.DailySleepDTO
		o.ObserveInt64(c.sleepSeconds, int64(dto.SleepTimeSeconds))
		o.ObserveInt64(c.napSeconds, int64(dto.NapTimeSeconds))
		o.ObserveInt64(c.deepSleepSeconds, int64(dto.DeepSleepSeconds))
		o.ObserveInt64(c.lightSleepSeconds, int64(dto.LightSleepSeconds))
		o.ObserveInt64(c.remSleepSeconds, int64(dto.REMSleepSeconds))
		o.ObserveInt64(c.awakeSeconds, int64(dto.AwakeSeconds))
		o.ObserveInt64(c.restlessMoments, int64(s.RestlessMomentsCount))
		if dto.AverageRespirationValue > 0 {
			o.ObserveFloat64(c.avgRespiration, dto.AverageRespirationValue)
		}
		if dto.LowestRespirationValue > 0 {
			o.ObserveFloat64(c.lowestRespiration, dto.LowestRespirationValue)
		}
		if dto.HighestRespirationValue > 0 {
			o.ObserveFloat64(c.highestRespiration, dto.HighestRespirationValue)
		}
		if dto.AvgSleepStress > 0 {
			o.ObserveFloat64(c.avgStress, dto.AvgSleepStress)
		}
		if dto.SpO2AvgReadingPercent > 0 {
			o.ObserveFloat64(c.spO2Avg, dto.SpO2AvgReadingPercent)
		}
		if dto.SpO2LowReadingPercent > 0 {
			o.ObserveFloat64(c.spO2Low, dto.SpO2LowReadingPercent)
		}
	}
	if snap.Sleep.HRV != nil {
		hrv := snap.Sleep.HRV.HRVSummary
		if hrv.WeeklyAvg > 0 {
			o.ObserveInt64(c.hrvWeeklyAvg, int64(hrv.WeeklyAvg))
		}
		if hrv.LastNight > 0 {
			o.ObserveInt64(c.hrvLastNight, int64(hrv.LastNight))
		}
		if hrv.LastNight5MinHigh > 0 {
			o.ObserveInt64(c.hrvLastNight5MinHigh, int64(hrv.LastNight5MinHigh))
		}
		if hrv.Baseline.LowUpper > 0 {
			o.ObserveInt64(c.hrvBaselineLowUpper, int64(hrv.Baseline.LowUpper))
		}
		if hrv.Baseline.BalancedLow > 0 {
			o.ObserveInt64(c.hrvBaselineBalancedLow, int64(hrv.Baseline.BalancedLow))
		}
		if hrv.Baseline.BalancedUpper > 0 {
			o.ObserveInt64(c.hrvBaselineBalancedUpper, int64(hrv.Baseline.BalancedUpper))
		}
	}
	return nil
}

func (c *sleepCollector) Close() error {
	if c.registration == nil {
		return nil
	}
	return c.registration.Unregister()
}
