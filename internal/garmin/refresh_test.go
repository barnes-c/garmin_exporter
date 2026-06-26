package garmin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewRefresh_ReturnsErrNotLoggedInWhenClientNil(t *testing.T) {
	r := NewRefresh(NewClient(), discardLogger(), RefreshConfig{})
	snap, err := r(context.Background())
	if !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("err = %v, want ErrNotLoggedIn", err)
	}
	if snap != nil {
		t.Errorf("snap = %v, want nil", snap)
	}
}

func TestParseCyclingFTP_PullsValue(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"mostRecentBiometric": map[string]any{"value": 247.5},
	})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	v, ok := parseCyclingFTP(m)
	if !ok || v != 247.5 {
		t.Errorf("parseCyclingFTP = (%v, %v), want (247.5, true)", v, ok)
	}
}

func TestParseCyclingFTP_MissingField(t *testing.T) {
	if v, ok := parseCyclingFTP(map[string]json.RawMessage{}); ok || v != 0 {
		t.Errorf("parseCyclingFTP empty = (%v, %v), want (0, false)", v, ok)
	}
}

func TestParseCyclingFTP_ZeroValueDropped(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"mostRecentBiometric": map[string]any{"value": 0},
	})
	var m map[string]json.RawMessage
	_ = json.Unmarshal(raw, &m)
	if v, ok := parseCyclingFTP(m); ok || v != 0 {
		t.Errorf("parseCyclingFTP zero = (%v, %v), want (0, false)", v, ok)
	}
}

func TestParseFitnessAge_PullsAgesAndComponents(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"chronologicalAge":     40,
		"fitnessAge":           35,
		"achievableFitnessAge": 32,
		"previousFitnessAge":   36,
		"components": map[string]any{
			"bmi": map[string]any{"value": 23.5, "potentialAge": 34},
			"rhr": map[string]any{"value": 52},
		},
	})
	var m map[string]json.RawMessage
	_ = json.Unmarshal(raw, &m)

	fa := parseFitnessAge(m)
	if fa == nil {
		t.Fatal("parseFitnessAge = nil, want value")
	}
	if fa.FitnessAge != 35 || fa.ChronologicalAge != 40 || fa.AchievableFitnessAge != 32 || fa.PreviousFitnessAge != 36 {
		t.Errorf("ages = %+v", fa)
	}
	if len(fa.Components) != 2 {
		t.Fatalf("components = %d, want 2 (sorted)", len(fa.Components))
	}
	// Components are sorted by name: bmi, rhr.
	if fa.Components[0].Name != "bmi" || fa.Components[0].Value != 23.5 || !fa.Components[0].HasPotential || fa.Components[0].PotentialAge != 34 {
		t.Errorf("bmi component = %+v", fa.Components[0])
	}
	if fa.Components[1].Name != "rhr" || fa.Components[1].HasPotential {
		t.Errorf("rhr component = %+v, want no potential age", fa.Components[1])
	}
}

func TestParseFitnessAge_EmptyMap(t *testing.T) {
	if fa := parseFitnessAge(map[string]json.RawMessage{}); fa != nil {
		t.Errorf("parseFitnessAge empty = %+v, want nil", fa)
	}
}
