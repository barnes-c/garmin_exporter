package otel

import (
	"bytes"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
)

func TestParsePushExporters(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{"otlp only", "otlp", []string{"otlp"}, false},
		{"otlp and console", "otlp,console", []string{"otlp", "console"}, false},
		{"whitespace tolerated", "otlp, console", []string{"otlp", "console"}, false},
		{"dedup", "otlp,otlp", []string{"otlp"}, false},
		{"empty entries skipped", "otlp,,console", []string{"otlp", "console"}, false},
		{"prometheus ignored (always-on)", "prometheus,otlp", []string{"otlp"}, false},
		{"prometheus only yields no push", "prometheus", nil, false},
		{"none disables all push", "none", nil, false},
		{"none with whitespace", "  none  ", nil, false},
		{"empty input yields no push", "", nil, false},
		{"only commas yields no push", ",,,", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePushExporters(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetSDKErrorHandler_RoutesThroughLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	setSDKErrorHandler(logger)

	otel.Handle(errors.New("synthetic SDK failure"))

	out := buf.String()
	if !strings.Contains(out, "OTel SDK error") {
		t.Fatalf("log missing handler tag; got:\n%s", out)
	}
	if !strings.Contains(out, "synthetic SDK failure") {
		t.Fatalf("log missing wrapped error; got:\n%s", out)
	}
	if !strings.Contains(out, "level=ERROR") {
		t.Fatalf("expected ERROR level; got:\n%s", out)
	}
}

// TestSetSDKErrorHandler_ContextHandlerStays_NoLeakBetweenCalls guards
// against accidental leakage across Setup() invocations in the same
// process — the global handler should be cleanly replaced, not
// concatenated.
func TestSetSDKErrorHandler_LastCallWins(t *testing.T) {
	var first, second bytes.Buffer
	setSDKErrorHandler(slog.New(slog.NewTextHandler(&first, nil)))
	setSDKErrorHandler(slog.New(slog.NewTextHandler(&second, nil)))

	otel.Handle(errors.New("after-second"))

	if first.Len() != 0 {
		t.Fatalf("first logger should not receive; got: %s", first.String())
	}
	if !strings.Contains(second.String(), "after-second") {
		t.Fatalf("second logger should receive; got: %s", second.String())
	}
}
