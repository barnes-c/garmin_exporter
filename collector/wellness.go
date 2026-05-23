package collector

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	registerCollector("wellness", defaultEnabled, newWellnessCollector)
}

type wellnessCollector struct {
	totalKilocalories       *prometheus.Desc
	activeKilocalories      *prometheus.Desc
	bmrKilocalories         *prometheus.Desc
	totalSteps              *prometheus.Desc
	stepGoal                *prometheus.Desc
	totalDistanceMeters     *prometheus.Desc
	highlyActiveSeconds     *prometheus.Desc
	activeSeconds           *prometheus.Desc
	sedentarySeconds        *prometheus.Desc
	sleepingSeconds         *prometheus.Desc
	moderateIntensityMins   *prometheus.Desc
	vigorousIntensityMins   *prometheus.Desc
	floorsAscended          *prometheus.Desc
	floorsDescended         *prometheus.Desc
	floorsAscendedGoal      *prometheus.Desc
	minHeartRate            *prometheus.Desc
	maxHeartRate            *prometheus.Desc
	restingHeartRate        *prometheus.Desc
	avgRestingHeartRate7Day *prometheus.Desc
	avgWakingRespiration    *prometheus.Desc
	highestRespiration      *prometheus.Desc
	lowestRespiration       *prometheus.Desc
	avgStressDuration       *prometheus.Desc
	highStressDuration      *prometheus.Desc
	lowStressDuration       *prometheus.Desc
	restStressDuration      *prometheus.Desc
	bodyBatteryCharged      *prometheus.Desc
	bodyBatteryDrained      *prometheus.Desc
	bodyBatteryHighest      *prometheus.Desc
	bodyBatteryLowest       *prometheus.Desc
	bodyBatteryLatest       *prometheus.Desc
	logger                  *slog.Logger
}

func newWellnessCollector(logger *slog.Logger) (Collector, error) {
	const sub = "wellness"
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, sub, name), help, nil, nil)
	}
	return &wellnessCollector{
		totalKilocalories:       d("total_kilocalories", "Total kilocalories burned."),
		activeKilocalories:      d("active_kilocalories", "Active kilocalories burned."),
		bmrKilocalories:         d("bmr_kilocalories", "BMR kilocalories."),
		totalSteps:              d("total_steps", "Total steps for the day."),
		stepGoal:                d("step_goal", "Daily step goal."),
		totalDistanceMeters:     d("total_distance_meters", "Total distance in meters."),
		highlyActiveSeconds:     d("highly_active_seconds", "Seconds of highly active time."),
		activeSeconds:           d("active_seconds", "Seconds of active time."),
		sedentarySeconds:        d("sedentary_seconds", "Seconds of sedentary time."),
		sleepingSeconds:         d("sleeping_seconds", "Seconds of sleeping time."),
		moderateIntensityMins:   d("moderate_intensity_minutes", "Moderate intensity minutes."),
		vigorousIntensityMins:   d("vigorous_intensity_minutes", "Vigorous intensity minutes."),
		floorsAscended:          d("floors_ascended", "Floors ascended."),
		floorsDescended:         d("floors_descended", "Floors descended."),
		floorsAscendedGoal:      d("floors_ascended_goal", "Daily floors ascended goal."),
		minHeartRate:            d("min_heart_rate_bpm", "Minimum heart rate in bpm."),
		maxHeartRate:            d("max_heart_rate_bpm", "Maximum heart rate in bpm."),
		restingHeartRate:        d("resting_heart_rate_bpm", "Resting heart rate in bpm."),
		avgRestingHeartRate7Day: d("avg_resting_heart_rate_7day_bpm", "7-day average resting heart rate in bpm."),
		avgWakingRespiration:    d("avg_waking_respiration_bpm", "Average waking respiration rate."),
		highestRespiration:      d("highest_respiration_bpm", "Highest respiration rate."),
		lowestRespiration:       d("lowest_respiration_bpm", "Lowest respiration rate."),
		avgStressDuration:       d("avg_stress_duration_seconds", "Average stress event duration in seconds."),
		highStressDuration:      d("high_stress_duration_seconds", "High stress duration in seconds."),
		lowStressDuration:       d("low_stress_duration_seconds", "Low stress duration in seconds."),
		restStressDuration:      d("rest_stress_duration_seconds", "Rest/recovery stress duration in seconds."),
		bodyBatteryCharged:      d("body_battery_charged", "Body battery charged today."),
		bodyBatteryDrained:      d("body_battery_drained", "Body battery drained today."),
		bodyBatteryHighest:      d("body_battery_highest", "Highest body battery value today."),
		bodyBatteryLowest:       d("body_battery_lowest", "Lowest body battery value today."),
		bodyBatteryLatest:       d("body_battery_latest", "Most recent body battery value."),
		logger:                  logger,
	}, nil
}

func (c *wellnessCollector) Update(ch chan<- prometheus.Metric) error {
	client := getClient()
	if client == nil {
		return ErrNoData
	}
	s, err := client.UserSummary(time.Now())
	if err != nil {
		return err
	}

	g := func(desc *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v)
	}
	g(c.totalKilocalories, s.TotalKilocalories)
	g(c.activeKilocalories, s.ActiveKilocalories)
	g(c.bmrKilocalories, s.BmrKilocalories)
	g(c.totalSteps, float64(s.TotalSteps))
	g(c.stepGoal, float64(s.DailyStepGoal))
	g(c.totalDistanceMeters, s.TotalDistanceMeters)
	g(c.highlyActiveSeconds, float64(s.HighlyActiveSeconds))
	g(c.activeSeconds, float64(s.ActiveSeconds))
	g(c.sedentarySeconds, float64(s.SedentarySeconds))
	g(c.sleepingSeconds, float64(s.SleepingSeconds))
	g(c.moderateIntensityMins, float64(s.ModerateDurationMinutes))
	g(c.vigorousIntensityMins, float64(s.VigorousDurationMinutes))
	g(c.floorsAscended, s.FloorsAscended)
	g(c.floorsDescended, s.FloorsDescended)
	g(c.floorsAscendedGoal, s.FloorsAscendedGoal)
	if s.MinHeartRate > 0 {
		g(c.minHeartRate, float64(s.MinHeartRate))
	}
	if s.MaxHeartRate > 0 {
		g(c.maxHeartRate, float64(s.MaxHeartRate))
	}
	if s.RestingHeartRate > 0 {
		g(c.restingHeartRate, float64(s.RestingHeartRate))
	}
	if s.LastSevenDaysAvgRestingHeartRate > 0 {
		g(c.avgRestingHeartRate7Day, float64(s.LastSevenDaysAvgRestingHeartRate))
	}
	if s.AvgWakingRespirationValue > 0 {
		g(c.avgWakingRespiration, s.AvgWakingRespirationValue)
	}
	if s.HighestRespirationValue > 0 {
		g(c.highestRespiration, s.HighestRespirationValue)
	}
	if s.LowestRespirationValue > 0 {
		g(c.lowestRespiration, s.LowestRespirationValue)
	}
	g(c.avgStressDuration, float64(s.AvgStressDuration))
	g(c.highStressDuration, float64(s.HighStressDuration))
	g(c.lowStressDuration, float64(s.LowStressDuration))
	g(c.restStressDuration, float64(s.RestStressDuration))
	g(c.bodyBatteryCharged, float64(s.BodyBatteryChargedValue))
	g(c.bodyBatteryDrained, float64(s.BodyBatteryDrainedValue))
	if s.BodyBatteryHighestValue > 0 {
		g(c.bodyBatteryHighest, float64(s.BodyBatteryHighestValue))
	}
	if s.BodyBatteryLowestValue > 0 {
		g(c.bodyBatteryLowest, float64(s.BodyBatteryLowestValue))
	}
	if s.BodyBatteryMostRecentValue > 0 {
		g(c.bodyBatteryLatest, float64(s.BodyBatteryMostRecentValue))
	}
	return nil
}
