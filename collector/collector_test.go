package collector_test

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"testing"

	"github.com/barnes-c/go-garminconnect/garminconnect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/barnes-c/garmin_exporter/collector"
	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

// staticSource implements garmin.Source with a fixed snapshot.
type staticSource struct{ snap *garmin.Snapshot }

func (s *staticSource) Snapshot() *garmin.Snapshot { return s.snap }

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func setupMeter(t *testing.T) (metric.Meter, *sdkmetric.ManualReader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	return mp.Meter("test"), reader
}

func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) []metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	var out []metricdata.Metrics
	for _, sm := range rm.ScopeMetrics {
		out = append(out, sm.Metrics...)
	}
	return out
}

func findGauge[N int64 | float64](t *testing.T, metrics []metricdata.Metrics, name string) metricdata.Gauge[N] {
	t.Helper()
	for _, m := range metrics {
		if m.Name == name {
			g, ok := m.Data.(metricdata.Gauge[N])
			if !ok {
				t.Fatalf("metric %q has unexpected type %T", name, m.Data)
			}
			return g
		}
	}
	t.Fatalf("metric %q not found", name)
	return metricdata.Gauge[N]{}
}

func gaugeValue[N int64 | float64](t *testing.T, metrics []metricdata.Metrics, name string) N {
	t.Helper()
	g := findGauge[N](t, metrics, name)
	if len(g.DataPoints) == 0 {
		t.Fatalf("metric %q has no data points", name)
	}
	return g.DataPoints[0].Value
}

// --- Registry tests ---

func TestRegistered_IsSorted(t *testing.T) {
	names := collector.Registered()
	if len(names) == 0 {
		t.Fatal("no collectors registered")
	}
	if !slices.IsSorted(names) {
		t.Errorf("Registered() not sorted: %v", names)
	}
}

func TestNewGroup_NoFilter_AllCollectors(t *testing.T) {
	g, err := collector.NewGroup(discardLogger())
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if got, want := g.Names(), collector.Registered(); !slices.Equal(got, want) {
		t.Errorf("Names() = %v, want %v", got, want)
	}
}

func TestNewGroup_WithFilter_SubsetOnly(t *testing.T) {
	g, err := collector.NewGroup(discardLogger(), "heart_rate", "sleep")
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if got := g.Names(); !slices.Equal(got, []string{"heart_rate", "sleep"}) {
		t.Errorf("Names() = %v, want [heartrate sleep]", got)
	}
}

func TestNewGroup_UnknownCollector_ReturnsError(t *testing.T) {
	if _, err := collector.NewGroup(discardLogger(), "nonexistent"); err == nil {
		t.Fatal("expected error for unknown collector name")
	}
}

// --- garmin.collector.up ---

func TestRegisterAll_UpGaugeReflectsData(t *testing.T) {
	meter, reader := setupMeter(t)
	src := &staticSource{snap: &garmin.Snapshot{HeartRate: &garminconnect.HeartRates{}}}

	g, err := collector.NewGroup(discardLogger(), "heart_rate", "stress")
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if err := g.RegisterAll(meter, src); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	metrics := collectMetrics(t, reader)
	gauge := findGauge[int64](t, metrics, "garmin.collector.up")

	seen := make(map[string]int64)
	for _, dp := range gauge.DataPoints {
		for _, a := range dp.Attributes.ToSlice() {
			if a.Key == "collector" {
				seen[a.Value.AsString()] = dp.Value
			}
		}
	}
	for name, want := range map[string]int64{"heart_rate": 1, "stress": 0} {
		if v, ok := seen[name]; !ok || v != want {
			t.Errorf("garmin.collector.up{collector=%q} = %v, want %d", name, v, want)
		}
	}
}

// --- Per-collector observe tests ---

func TestHeartRateCollector_EmitsFromSnapshot(t *testing.T) {
	meter, reader := setupMeter(t)
	src := &staticSource{snap: &garmin.Snapshot{
		HeartRate: &garminconnect.HeartRates{
			RestingHeartRate:                 58,
			MinHeartRate:                     45,
			MaxHeartRate:                     175,
			LastSevenDaysAvgRestingHeartRate: 60,
		},
	}}

	g, err := collector.NewGroup(discardLogger(), "heart_rate")
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if err := g.RegisterAll(meter, src); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	metrics := collectMetrics(t, reader)
	if got := gaugeValue[int64](t, metrics, "garmin.heart_rate.resting_bpm"); got != 58 {
		t.Errorf("resting_bpm = %d, want 58", got)
	}
	if got := gaugeValue[int64](t, metrics, "garmin.heart_rate.min_bpm"); got != 45 {
		t.Errorf("min_bpm = %d, want 45", got)
	}
	if got := gaugeValue[int64](t, metrics, "garmin.heart_rate.max_bpm"); got != 175 {
		t.Errorf("max_bpm = %d, want 175", got)
	}
	if got := gaugeValue[int64](t, metrics, "garmin.heart_rate.seven_day_avg_resting_bpm"); got != 60 {
		t.Errorf("seven_day_avg = %d, want 60", got)
	}
}

func TestHeartRateCollector_NilSnapshot_EmitsNothing(t *testing.T) {
	meter, reader := setupMeter(t)
	src := &staticSource{}

	g, err := collector.NewGroup(discardLogger(), "heart_rate")
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if err := g.RegisterAll(meter, src); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	metrics := collectMetrics(t, reader)
	for _, m := range metrics {
		if m.Name == "garmin.heart_rate.resting_bpm" {
			if g, ok := m.Data.(metricdata.Gauge[int64]); ok && len(g.DataPoints) > 0 {
				t.Errorf("expected no data points with nil snapshot, got %d", len(g.DataPoints))
			}
		}
	}
}

func TestStressCollector_EmitsFromSnapshot(t *testing.T) {
	meter, reader := setupMeter(t)
	src := &staticSource{snap: &garmin.Snapshot{
		Stress: &garminconnect.StressData{
			AvgStressLevel: 42,
			MaxStressLevel: 88,
		},
	}}

	g, err := collector.NewGroup(discardLogger(), "stress")
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if err := g.RegisterAll(meter, src); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	metrics := collectMetrics(t, reader)
	if got := gaugeValue[int64](t, metrics, "garmin.stress.avg_level"); got != 42 {
		t.Errorf("avg_level = %d, want 42", got)
	}
	if got := gaugeValue[int64](t, metrics, "garmin.stress.max_level"); got != 88 {
		t.Errorf("max_level = %d, want 88", got)
	}
}

func TestCyclingCollector_EmitsFromSnapshot(t *testing.T) {
	meter, reader := setupMeter(t)
	ftp := 285.0
	src := &staticSource{snap: &garmin.Snapshot{
		Cycling: &garminconnect.CyclingFTP{FunctionalThresholdPower: &ftp},
	}}

	g, err := collector.NewGroup(discardLogger(), "cycling")
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if err := g.RegisterAll(meter, src); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	metrics := collectMetrics(t, reader)
	if got := gaugeValue[float64](t, metrics, "garmin.cycling.ftp_watts"); got != 285 {
		t.Errorf("ftp_watts = %v, want 285", got)
	}
}

func TestFitnessAgeCollector_EmitsFromSnapshot(t *testing.T) {
	meter, reader := setupMeter(t)
	bmiAge := 34.0
	src := &staticSource{snap: &garmin.Snapshot{
		FitnessAge: &garminconnect.FitnessAge{
			ChronologicalAge:     40,
			FitnessAge:           35.4,
			AchievableFitnessAge: 32,
			PreviousFitnessAge:   36,
			Components: map[string]garminconnect.FitnessAgeComponent{
				"bmi": {Value: 23.5, PotentialAge: &bmiAge},
				"rhr": {Value: 52},
			},
		},
	}}

	g, err := collector.NewGroup(discardLogger(), "fitness_age")
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if err := g.RegisterAll(meter, src); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	metrics := collectMetrics(t, reader)
	if got := gaugeValue[float64](t, metrics, "garmin.fitness_age.years"); got != 35.4 {
		t.Errorf("years = %v, want 35.4", got)
	}
	if got := gaugeValue[int64](t, metrics, "garmin.fitness_age.chronological_years"); got != 40 {
		t.Errorf("chronological_years = %d, want 40", got)
	}
	if got := gaugeValue[float64](t, metrics, "garmin.fitness_age.achievable_years"); got != 32 {
		t.Errorf("achievable_years = %v, want 32", got)
	}

	value := findGauge[float64](t, metrics, "garmin.fitness_age.component_value")
	byComp := make(map[string]float64)
	for _, dp := range value.DataPoints {
		for _, a := range dp.Attributes.ToSlice() {
			if a.Key == "component" {
				byComp[a.Value.AsString()] = dp.Value
			}
		}
	}
	if byComp["bmi"] != 23.5 {
		t.Errorf("component_value{bmi} = %v, want 23.5", byComp["bmi"])
	}
	if byComp["rhr"] != 52 {
		t.Errorf("component_value{rhr} = %v, want 52", byComp["rhr"])
	}

	// rhr has no potential age, so only bmi should appear.
	potential := findGauge[float64](t, metrics, "garmin.fitness_age.component_potential_years")
	if len(potential.DataPoints) != 1 {
		t.Fatalf("component_potential_years data points = %d, want 1", len(potential.DataPoints))
	}
	if potential.DataPoints[0].Value != 34 {
		t.Errorf("component_potential_years{bmi} = %v, want 34", potential.DataPoints[0].Value)
	}
}

func TestGearCollector_EmitsStatsByUUID(t *testing.T) {
	meter, reader := setupMeter(t)
	src := &staticSource{snap: &garmin.Snapshot{
		Gear: []garminconnect.Gear{
			{UUID: "abc", DisplayName: "Trail Shoes", GearTypeName: "shoes", GearStatusName: "active", MaxMeters: 800000},
			{UUID: "def", DisplayName: "Road Bike", GearTypeName: "bike", GearStatusName: "active"},
		},
		GearStats: map[string]*garminconnect.GearStats{
			"abc": {TotalDistance: 123456, TotalActivities: 42},
			// "def" has no stats; it should emit no stats data points.
		},
	}}

	g, err := collector.NewGroup(discardLogger(), "gear")
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if err := g.RegisterAll(meter, src); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	metrics := collectMetrics(t, reader)

	dist := findGauge[float64](t, metrics, "garmin.gear.total_distance_meters")
	if len(dist.DataPoints) != 1 {
		t.Fatalf("total_distance_meters data points = %d, want 1", len(dist.DataPoints))
	}
	if dist.DataPoints[0].Value != 123456 {
		t.Errorf("total_distance_meters = %v, want 123456", dist.DataPoints[0].Value)
	}
	name, _ := dist.DataPoints[0].Attributes.Value("gear_name")
	if name.AsString() != "Trail Shoes" {
		t.Errorf("gear_name = %q, want Trail Shoes", name.AsString())
	}

	acts := findGauge[int64](t, metrics, "garmin.gear.total_activities")
	if len(acts.DataPoints) != 1 || acts.DataPoints[0].Value != 42 {
		t.Errorf("total_activities = %v, want one point of 42", acts.DataPoints)
	}
}

func TestClose_NoError(t *testing.T) {
	meter, _ := setupMeter(t)
	src := &staticSource{}

	g, err := collector.NewGroup(discardLogger(), "heart_rate")
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if err := g.RegisterAll(meter, src); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}
	if err := g.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func attrValue[N int64 | float64](t *testing.T, metrics []metricdata.Metrics, name, key, val string) N {
	t.Helper()
	for _, dp := range findGauge[N](t, metrics, name).DataPoints {
		if v, ok := dp.Attributes.Value(attribute.Key(key)); ok && v.AsString() == val {
			return dp.Value
		}
	}
	t.Fatalf("metric %q has no data point with %s=%q", name, key, val)
	return 0
}

func TestSleepCollector_EmitsScoresAndNeed(t *testing.T) {
	meter, reader := setupMeter(t)
	src := &staticSource{snap: &garmin.Snapshot{Sleep: &garmin.Sleep{Data: &garminconnect.SleepData{
		DailySleepDTO: garminconnect.DailySleepDTO{
			SleepTimeSeconds: 27000,
			HighestSpO2Value: 99,
			AwakeCount:       2,
			AvgHeartRate:     52.5,
			SleepScores: map[string]garminconnect.SleepScore{
				"overall":        {Value: 81},
				"deepPercentage": {Value: 90},
				"stress":         {QualifierKey: "GOOD"},
			},
			SleepNeed: &garminconnect.SleepNeed{Baseline: 480, Actual: 500},
		},
		BodyBatteryChange: 63,
	}}}}

	g, err := collector.NewGroup(discardLogger(), "sleep")
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if err := g.RegisterAll(meter, src); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	m := collectMetrics(t, reader)
	if got := attrValue[int64](t, m, "garmin.sleep.score", "component", "overall"); got != 81 {
		t.Errorf("score overall = %d, want 81", got)
	}
	if got := attrValue[int64](t, m, "garmin.sleep.score", "component", "deepPercentage"); got != 90 {
		t.Errorf("score deepPercentage = %d, want 90", got)
	}
	if n := len(findGauge[int64](t, m, "garmin.sleep.score").DataPoints); n != 2 {
		t.Errorf("score has %d data points, want 2 (components without a value are skipped)", n)
	}
	for name, want := range map[string]int64{
		"garmin.sleep.need_baseline_seconds": 28800,
		"garmin.sleep.need_actual_seconds":   30000,
		"garmin.sleep.body_battery_change":   63,
		"garmin.sleep.awakenings":            2,
	} {
		if got := gaugeValue[int64](t, m, name); got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
	}
	if got := gaugeValue[float64](t, m, "garmin.sleep.spo2_high_percent"); got != 99 {
		t.Errorf("spo2_high_percent = %v, want 99", got)
	}
	if got := gaugeValue[float64](t, m, "garmin.sleep.avg_heart_rate_bpm"); got != 52.5 {
		t.Errorf("avg_heart_rate_bpm = %v, want 52.5", got)
	}
}

func TestWellnessCollector_EmitsStressBreakdown(t *testing.T) {
	meter, reader := setupMeter(t)
	src := &staticSource{snap: &garmin.Snapshot{UserSummary: &garminconnect.UserSummary{
		TotalStressDuration:    50000,
		MediumStressDuration:   3000,
		ActivityStressDuration: 4000,
		HighStressPercentage:   12.5,
		RestStressPercentage:   40,
		BodyBatteryAtWakeTime:  88,
		BodyBatteryDuringSleep: 55,
	}}}

	g, err := collector.NewGroup(discardLogger(), "wellness")
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if err := g.RegisterAll(meter, src); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	m := collectMetrics(t, reader)
	for name, want := range map[string]int64{
		"garmin.wellness.total_stress_duration_seconds":    50000,
		"garmin.wellness.medium_stress_duration_seconds":   3000,
		"garmin.wellness.activity_stress_duration_seconds": 4000,
		"garmin.wellness.body_battery_at_wake":             88,
		"garmin.wellness.body_battery_during_sleep":        55,
	} {
		if got := gaugeValue[int64](t, m, name); got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
	}
	if got := attrValue[float64](t, m, "garmin.wellness.stress_time_percent", "level", "high"); got != 12.5 {
		t.Errorf("stress_time_percent high = %v, want 12.5", got)
	}
	if got := attrValue[float64](t, m, "garmin.wellness.stress_time_percent", "level", "rest"); got != 40 {
		t.Errorf("stress_time_percent rest = %v, want 40", got)
	}
}

func TestTrainingCollector_EmitsReadinessFactors(t *testing.T) {
	meter, reader := setupMeter(t)
	src := &staticSource{snap: &garmin.Snapshot{Training: &garmin.Training{
		Readiness: []garminconnect.TrainingReadiness{{
			Score:                     70,
			HRVFactorPercent:          95,
			SleepHistoryScore:         60,
			RecoveryTimeFactorPercent: 100,
		}},
	}}}

	g, err := collector.NewGroup(discardLogger(), "training")
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if err := g.RegisterAll(meter, src); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	m := collectMetrics(t, reader)
	for factor, want := range map[string]int64{"hrv": 95, "sleep_history": 60, "recovery_time": 100} {
		if got := attrValue[int64](t, m, "garmin.training.readiness_factor_percent", "factor", factor); got != want {
			t.Errorf("readiness_factor_percent %s = %d, want %d", factor, got, want)
		}
	}
}

func TestIntensityCollector_EmitsWeekToDate(t *testing.T) {
	meter, reader := setupMeter(t)
	src := &staticSource{snap: &garmin.Snapshot{Intensity: &garminconnect.IntensityMinutesData{
		WeeklyGoal:               150,
		ModerateIntensityMinutes: 5, // today only; must not be reported as weekly
		VigorousIntensityMinutes: 2,
		WeeklyModerate:           60,
		WeeklyVigorous:           20,
		WeeklyTotal:              100,
	}}}

	g, err := collector.NewGroup(discardLogger(), "intensity")
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if err := g.RegisterAll(meter, src); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	m := collectMetrics(t, reader)
	for name, want := range map[string]int64{
		"garmin.intensity.weekly_goal_minutes":     150,
		"garmin.intensity.weekly_moderate_minutes": 60,
		"garmin.intensity.weekly_vigorous_minutes": 20,
		"garmin.intensity.weekly_total_minutes":    100,
	} {
		if got := gaugeValue[int64](t, m, name); got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
	}
}

func TestLactateThresholdCollector_ReadsSeparateEntries(t *testing.T) {
	meter, reader := setupMeter(t)
	speed := 0.33 // Garmin's unit is 10 m/s
	hr := 172
	// Garmin returns the speed and the heart rate in separate entries.
	src := &staticSource{snap: &garmin.Snapshot{
		LactateThreshold: []garminconnect.LactateThresholdEntry{{Speed: &speed}, {HearRate: &hr}},
	}}

	g, err := collector.NewGroup(discardLogger(), "lactate_threshold")
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if err := g.RegisterAll(meter, src); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	m := collectMetrics(t, reader)
	if got := gaugeValue[float64](t, m, "garmin.lactate_threshold.running_speed_mps"); got < 3.29 || got > 3.31 {
		t.Errorf("running_speed_mps = %v, want 3.3", got)
	}
	if got := gaugeValue[int64](t, m, "garmin.lactate_threshold.running_heart_rate_bpm"); got != 172 {
		t.Errorf("running_heart_rate_bpm = %d, want 172", got)
	}
}
