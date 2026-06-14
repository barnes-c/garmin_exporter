package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

func init() {
	registerCollector("wellness", newWellnessCollector,
		SnapshotHas(func(s *garmin.Snapshot) bool { return s.UserSummary != nil }))
}

type wellnessCollector struct {
	registrar
	log *slog.Logger
	src garmin.Source

	totalKilocalories       metric.Float64ObservableGauge
	activeKilocalories      metric.Float64ObservableGauge
	bmrKilocalories         metric.Float64ObservableGauge
	totalSteps              metric.Int64ObservableGauge
	stepGoal                metric.Int64ObservableGauge
	totalDistanceMeters     metric.Float64ObservableGauge
	highlyActiveSeconds     metric.Int64ObservableGauge
	activeSeconds           metric.Int64ObservableGauge
	sedentarySeconds        metric.Int64ObservableGauge
	sleepingSeconds         metric.Int64ObservableGauge
	moderateIntensityMins   metric.Int64ObservableGauge
	vigorousIntensityMins   metric.Int64ObservableGauge
	floorsAscended          metric.Float64ObservableGauge
	floorsDescended         metric.Float64ObservableGauge
	floorsAscendedGoal      metric.Float64ObservableGauge
	minHeartRate            metric.Int64ObservableGauge
	maxHeartRate            metric.Int64ObservableGauge
	restingHeartRate        metric.Int64ObservableGauge
	avgRestingHeartRate7Day metric.Int64ObservableGauge
	avgWakingRespiration    metric.Float64ObservableGauge
	highestRespiration      metric.Float64ObservableGauge
	lowestRespiration       metric.Float64ObservableGauge
	avgStressDuration       metric.Int64ObservableGauge
	highStressDuration      metric.Int64ObservableGauge
	lowStressDuration       metric.Int64ObservableGauge
	restStressDuration      metric.Int64ObservableGauge
	bodyBatteryCharged      metric.Int64ObservableGauge
	bodyBatteryDrained      metric.Int64ObservableGauge
	bodyBatteryHighest      metric.Int64ObservableGauge
	bodyBatteryLowest       metric.Int64ObservableGauge
	bodyBatteryLatest       metric.Int64ObservableGauge
}

func newWellnessCollector(log *slog.Logger) (Collector, error) {
	return &wellnessCollector{log: log}, nil
}

func (c *wellnessCollector) Name() string { return "wellness" }

func (c *wellnessCollector) Register(meter metric.Meter, src garmin.Source) error {
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
	if c.totalKilocalories, err = floatGauge("garmin.wellness.total_kilocalories", "Total kilocalories burned.", "kcal"); err != nil {
		return err
	}
	if c.activeKilocalories, err = floatGauge("garmin.wellness.active_kilocalories", "Active kilocalories burned.", "kcal"); err != nil {
		return err
	}
	if c.bmrKilocalories, err = floatGauge("garmin.wellness.bmr_kilocalories", "BMR kilocalories.", "kcal"); err != nil {
		return err
	}
	if c.totalSteps, err = intGauge("garmin.wellness.total_steps", "Total steps for the day.", "{step}"); err != nil {
		return err
	}
	if c.stepGoal, err = intGauge("garmin.wellness.step_goal", "Daily step goal.", "{step}"); err != nil {
		return err
	}
	if c.totalDistanceMeters, err = floatGauge("garmin.wellness.total_distance_meters", "Total distance in meters.", "m"); err != nil {
		return err
	}
	if c.highlyActiveSeconds, err = intGauge("garmin.wellness.highly_active_seconds", "Seconds of highly active time.", "s"); err != nil {
		return err
	}
	if c.activeSeconds, err = intGauge("garmin.wellness.active_seconds", "Seconds of active time.", "s"); err != nil {
		return err
	}
	if c.sedentarySeconds, err = intGauge("garmin.wellness.sedentary_seconds", "Seconds of sedentary time.", "s"); err != nil {
		return err
	}
	if c.sleepingSeconds, err = intGauge("garmin.wellness.sleeping_seconds", "Seconds of sleeping time.", "s"); err != nil {
		return err
	}
	if c.moderateIntensityMins, err = intGauge("garmin.wellness.moderate_intensity_minutes", "Moderate intensity minutes.", "min"); err != nil {
		return err
	}
	if c.vigorousIntensityMins, err = intGauge("garmin.wellness.vigorous_intensity_minutes", "Vigorous intensity minutes.", "min"); err != nil {
		return err
	}
	if c.floorsAscended, err = floatGauge("garmin.wellness.floors_ascended", "Floors ascended.", "{floor}"); err != nil {
		return err
	}
	if c.floorsDescended, err = floatGauge("garmin.wellness.floors_descended", "Floors descended.", "{floor}"); err != nil {
		return err
	}
	if c.floorsAscendedGoal, err = floatGauge("garmin.wellness.floors_ascended_goal", "Daily floors ascended goal.", "{floor}"); err != nil {
		return err
	}
	if c.minHeartRate, err = intGauge("garmin.wellness.min_heart_rate_bpm", "Minimum heart rate in bpm.", "{beat}/min"); err != nil {
		return err
	}
	if c.maxHeartRate, err = intGauge("garmin.wellness.max_heart_rate_bpm", "Maximum heart rate in bpm.", "{beat}/min"); err != nil {
		return err
	}
	if c.restingHeartRate, err = intGauge("garmin.wellness.resting_heart_rate_bpm", "Resting heart rate in bpm.", "{beat}/min"); err != nil {
		return err
	}
	if c.avgRestingHeartRate7Day, err = intGauge("garmin.wellness.avg_resting_heart_rate_7day_bpm", "7-day average resting heart rate in bpm.", "{beat}/min"); err != nil {
		return err
	}
	if c.avgWakingRespiration, err = floatGauge("garmin.wellness.avg_waking_respiration_bpm", "Average waking respiration rate.", "{breath}/min"); err != nil {
		return err
	}
	if c.highestRespiration, err = floatGauge("garmin.wellness.highest_respiration_bpm", "Highest respiration rate.", "{breath}/min"); err != nil {
		return err
	}
	if c.lowestRespiration, err = floatGauge("garmin.wellness.lowest_respiration_bpm", "Lowest respiration rate.", "{breath}/min"); err != nil {
		return err
	}
	if c.avgStressDuration, err = intGauge("garmin.wellness.avg_stress_duration_seconds", "Average stress event duration in seconds.", "s"); err != nil {
		return err
	}
	if c.highStressDuration, err = intGauge("garmin.wellness.high_stress_duration_seconds", "High stress duration in seconds.", "s"); err != nil {
		return err
	}
	if c.lowStressDuration, err = intGauge("garmin.wellness.low_stress_duration_seconds", "Low stress duration in seconds.", "s"); err != nil {
		return err
	}
	if c.restStressDuration, err = intGauge("garmin.wellness.rest_stress_duration_seconds", "Rest/recovery stress duration in seconds.", "s"); err != nil {
		return err
	}
	if c.bodyBatteryCharged, err = intGauge("garmin.wellness.body_battery_charged", "Body battery charged today.", "{level}"); err != nil {
		return err
	}
	if c.bodyBatteryDrained, err = intGauge("garmin.wellness.body_battery_drained", "Body battery drained today.", "{level}"); err != nil {
		return err
	}
	if c.bodyBatteryHighest, err = intGauge("garmin.wellness.body_battery_highest", "Highest body battery value today.", "{level}"); err != nil {
		return err
	}
	if c.bodyBatteryLowest, err = intGauge("garmin.wellness.body_battery_lowest", "Lowest body battery value today.", "{level}"); err != nil {
		return err
	}
	if c.bodyBatteryLatest, err = intGauge("garmin.wellness.body_battery_latest", "Most recent body battery value.", "{level}"); err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.totalKilocalories, c.activeKilocalories, c.bmrKilocalories,
		c.totalSteps, c.stepGoal, c.totalDistanceMeters,
		c.highlyActiveSeconds, c.activeSeconds, c.sedentarySeconds, c.sleepingSeconds,
		c.moderateIntensityMins, c.vigorousIntensityMins,
		c.floorsAscended, c.floorsDescended, c.floorsAscendedGoal,
		c.minHeartRate, c.maxHeartRate, c.restingHeartRate, c.avgRestingHeartRate7Day,
		c.avgWakingRespiration, c.highestRespiration, c.lowestRespiration,
		c.avgStressDuration, c.highStressDuration, c.lowStressDuration, c.restStressDuration,
		c.bodyBatteryCharged, c.bodyBatteryDrained, c.bodyBatteryHighest, c.bodyBatteryLowest, c.bodyBatteryLatest,
	)
	return err
}

func (c *wellnessCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.Snapshot()
	if snap == nil || snap.UserSummary == nil {
		return nil
	}
	s := snap.UserSummary
	o.ObserveFloat64(c.totalKilocalories, s.TotalKilocalories)
	o.ObserveFloat64(c.activeKilocalories, s.ActiveKilocalories)
	o.ObserveFloat64(c.bmrKilocalories, s.BmrKilocalories)
	o.ObserveInt64(c.totalSteps, int64(s.TotalSteps))
	o.ObserveInt64(c.stepGoal, int64(s.DailyStepGoal))
	o.ObserveFloat64(c.totalDistanceMeters, s.TotalDistanceMeters)
	o.ObserveInt64(c.highlyActiveSeconds, int64(s.HighlyActiveSeconds))
	o.ObserveInt64(c.activeSeconds, int64(s.ActiveSeconds))
	o.ObserveInt64(c.sedentarySeconds, int64(s.SedentarySeconds))
	o.ObserveInt64(c.sleepingSeconds, int64(s.SleepingSeconds))
	o.ObserveInt64(c.moderateIntensityMins, int64(s.ModerateDurationMinutes))
	o.ObserveInt64(c.vigorousIntensityMins, int64(s.VigorousDurationMinutes))
	o.ObserveFloat64(c.floorsAscended, s.FloorsAscended)
	o.ObserveFloat64(c.floorsDescended, s.FloorsDescended)
	o.ObserveFloat64(c.floorsAscendedGoal, s.FloorsAscendedGoal)
	if s.MinHeartRate > 0 {
		o.ObserveInt64(c.minHeartRate, int64(s.MinHeartRate))
	}
	if s.MaxHeartRate > 0 {
		o.ObserveInt64(c.maxHeartRate, int64(s.MaxHeartRate))
	}
	if s.RestingHeartRate > 0 {
		o.ObserveInt64(c.restingHeartRate, int64(s.RestingHeartRate))
	}
	if s.LastSevenDaysAvgRestingHeartRate > 0 {
		o.ObserveInt64(c.avgRestingHeartRate7Day, int64(s.LastSevenDaysAvgRestingHeartRate))
	}
	if s.AvgWakingRespirationValue > 0 {
		o.ObserveFloat64(c.avgWakingRespiration, s.AvgWakingRespirationValue)
	}
	if s.HighestRespirationValue > 0 {
		o.ObserveFloat64(c.highestRespiration, s.HighestRespirationValue)
	}
	if s.LowestRespirationValue > 0 {
		o.ObserveFloat64(c.lowestRespiration, s.LowestRespirationValue)
	}
	o.ObserveInt64(c.avgStressDuration, int64(s.AvgStressDuration))
	o.ObserveInt64(c.highStressDuration, int64(s.HighStressDuration))
	o.ObserveInt64(c.lowStressDuration, int64(s.LowStressDuration))
	o.ObserveInt64(c.restStressDuration, int64(s.RestStressDuration))
	o.ObserveInt64(c.bodyBatteryCharged, int64(s.BodyBatteryChargedValue))
	o.ObserveInt64(c.bodyBatteryDrained, int64(s.BodyBatteryDrainedValue))
	if s.BodyBatteryHighestValue > 0 {
		o.ObserveInt64(c.bodyBatteryHighest, int64(s.BodyBatteryHighestValue))
	}
	if s.BodyBatteryLowestValue > 0 {
		o.ObserveInt64(c.bodyBatteryLowest, int64(s.BodyBatteryLowestValue))
	}
	if s.BodyBatteryMostRecentValue > 0 {
		o.ObserveInt64(c.bodyBatteryLatest, int64(s.BodyBatteryMostRecentValue))
	}
	return nil
}
