package collector_test

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"testing"

	"github.com/barnes-c/go-garminconnect/garminconnect"
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
	g, err := collector.NewGroup(discardLogger(), "heartrate", "sleep")
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if got := g.Names(); !slices.Equal(got, []string{"heartrate", "sleep"}) {
		t.Errorf("Names() = %v, want [heartrate sleep]", got)
	}
}

func TestNewGroup_UnknownCollector_ReturnsError(t *testing.T) {
	if _, err := collector.NewGroup(discardLogger(), "nonexistent"); err == nil {
		t.Fatal("expected error for unknown collector name")
	}
}

// --- garmin.collector.up ---

func TestRegisterAll_UpGaugeEmitsOnePerCollector(t *testing.T) {
	meter, reader := setupMeter(t)
	src := &staticSource{}

	g, err := collector.NewGroup(discardLogger(), "heartrate", "stress")
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
	for _, name := range []string{"heartrate", "stress"} {
		if v, ok := seen[name]; !ok || v != 1 {
			t.Errorf("garmin.collector.up{collector=%q} = %v, want 1", name, v)
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

	g, err := collector.NewGroup(discardLogger(), "heartrate")
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if err := g.RegisterAll(meter, src); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	metrics := collectMetrics(t, reader)
	if got := gaugeValue[int64](t, metrics, "garmin.heartrate.resting_bpm"); got != 58 {
		t.Errorf("resting_bpm = %d, want 58", got)
	}
	if got := gaugeValue[int64](t, metrics, "garmin.heartrate.min_bpm"); got != 45 {
		t.Errorf("min_bpm = %d, want 45", got)
	}
	if got := gaugeValue[int64](t, metrics, "garmin.heartrate.max_bpm"); got != 175 {
		t.Errorf("max_bpm = %d, want 175", got)
	}
	if got := gaugeValue[int64](t, metrics, "garmin.heartrate.seven_day_avg_resting_bpm"); got != 60 {
		t.Errorf("seven_day_avg = %d, want 60", got)
	}
}

func TestHeartRateCollector_NilSnapshot_EmitsNothing(t *testing.T) {
	meter, reader := setupMeter(t)
	src := &staticSource{}

	g, err := collector.NewGroup(discardLogger(), "heartrate")
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if err := g.RegisterAll(meter, src); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	metrics := collectMetrics(t, reader)
	for _, m := range metrics {
		if m.Name == "garmin.heartrate.resting_bpm" {
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
	src := &staticSource{snap: &garmin.Snapshot{
		Cycling: &garmin.Cycling{FTPWatts: 285},
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
	src := &staticSource{snap: &garmin.Snapshot{
		FitnessAge: &garmin.FitnessAge{
			ChronologicalAge:     40,
			FitnessAge:           35,
			AchievableFitnessAge: 32,
			PreviousFitnessAge:   36,
			Components: []garmin.FitnessAgeComponent{
				{Name: "bmi", Value: 23.5, PotentialAge: 34, HasPotential: true},
				{Name: "rhr", Value: 52},
			},
		},
	}}

	g, err := collector.NewGroup(discardLogger(), "fitnessage")
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	if err := g.RegisterAll(meter, src); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}

	metrics := collectMetrics(t, reader)
	if got := gaugeValue[int64](t, metrics, "garmin.fitness_age.years"); got != 35 {
		t.Errorf("years = %d, want 35", got)
	}
	if got := gaugeValue[int64](t, metrics, "garmin.fitness_age.chronological_years"); got != 40 {
		t.Errorf("chronological_years = %d, want 40", got)
	}
	if got := gaugeValue[int64](t, metrics, "garmin.fitness_age.achievable_years"); got != 32 {
		t.Errorf("achievable_years = %d, want 32", got)
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

func TestClose_NoError(t *testing.T) {
	meter, _ := setupMeter(t)
	src := &staticSource{}

	g, err := collector.NewGroup(discardLogger(), "heartrate")
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
