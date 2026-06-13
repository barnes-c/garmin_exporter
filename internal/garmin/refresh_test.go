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
