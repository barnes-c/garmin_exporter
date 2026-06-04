package otlp

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// errorHandler always returns an error from Handle.
type errorHandler struct{ err error }

func (e *errorHandler) Enabled(_ context.Context, _ slog.Level) bool  { return true }
func (e *errorHandler) Handle(_ context.Context, _ slog.Record) error { return e.err }
func (e *errorHandler) WithAttrs(_ []slog.Attr) slog.Handler          { return e }
func (e *errorHandler) WithGroup(_ string) slog.Handler               { return e }

// captureHandler records Handle calls and propagates WithAttrs/WithGroup immutably.
type captureHandler struct {
	enabled bool
	records []slog.Record
	attrs   []slog.Attr
	group   string
}

func (c *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return c.enabled }

func (c *captureHandler) Handle(_ context.Context, r slog.Record) error {
	c.records = append(c.records, r)
	return nil
}

func (c *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	n := &captureHandler{enabled: c.enabled, group: c.group}
	n.attrs = append(append([]slog.Attr{}, c.attrs...), attrs...)
	return n
}

func (c *captureHandler) WithGroup(name string) slog.Handler {
	return &captureHandler{enabled: c.enabled, attrs: c.attrs, group: name}
}

func TestMultiHandler_Enabled(t *testing.T) {
	ctx := context.Background()

	allOff := multiHandler{&captureHandler{}, &captureHandler{}}
	if allOff.Enabled(ctx, slog.LevelInfo) {
		t.Error("expected false when all handlers disabled")
	}

	oneOn := multiHandler{&captureHandler{}, &captureHandler{enabled: true}}
	if !oneOn.Enabled(ctx, slog.LevelInfo) {
		t.Error("expected true when at least one handler is enabled")
	}
}

func TestMultiHandler_Handle_DispatchesToAll(t *testing.T) {
	h1 := &captureHandler{enabled: true}
	h2 := &captureHandler{enabled: true}
	m := multiHandler{h1, h2}

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)
	if err := m.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	if len(h1.records) != 1 || len(h2.records) != 1 {
		t.Errorf("expected 1 record each, got h1=%d h2=%d", len(h1.records), len(h2.records))
	}
}

func TestMultiHandler_Handle_SkipsDisabledHandlers(t *testing.T) {
	h1 := &captureHandler{enabled: false}
	h2 := &captureHandler{enabled: true}
	m := multiHandler{h1, h2}

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)
	_ = m.Handle(context.Background(), r)

	if len(h1.records) != 0 {
		t.Error("disabled handler should not receive records")
	}
	if len(h2.records) != 1 {
		t.Error("enabled handler should receive record")
	}
}

func TestMultiHandler_WithAttrs_IsImmutable(t *testing.T) {
	h1 := &captureHandler{enabled: true}
	h2 := &captureHandler{enabled: true}
	m := multiHandler{h1, h2}

	m2 := m.WithAttrs([]slog.Attr{slog.String("k", "v")})

	mh, ok := m2.(multiHandler)
	if !ok {
		t.Fatal("WithAttrs should return a multiHandler")
	}
	if len(mh) != len(m) {
		t.Errorf("expected %d handlers, got %d", len(m), len(mh))
	}
	if len(h1.attrs) != 0 || len(h2.attrs) != 0 {
		t.Error("WithAttrs must not mutate original handlers")
	}
	for i, h := range mh {
		if len(h.(*captureHandler).attrs) != 1 {
			t.Errorf("handler %d: expected 1 attr, got %d", i, len(h.(*captureHandler).attrs))
		}
	}
}

func TestMultiHandler_WithGroup_IsImmutable(t *testing.T) {
	h1 := &captureHandler{enabled: true}
	h2 := &captureHandler{enabled: true}
	m := multiHandler{h1, h2}

	m2 := m.WithGroup("http")

	mh, ok := m2.(multiHandler)
	if !ok {
		t.Fatal("WithGroup should return a multiHandler")
	}
	if h1.group != "" || h2.group != "" {
		t.Error("WithGroup must not mutate original handlers")
	}
	for i, h := range mh {
		if h.(*captureHandler).group != "http" {
			t.Errorf("handler %d: expected group \"http\", got %q", i, h.(*captureHandler).group)
		}
	}
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}

func TestSetup_AllNone_NoError(t *testing.T) {
	shutdown, updatedLogger, err := Setup(context.Background(), nil, newTestLogger(), Config{
		Protocol:        "grpc",
		Interval:        15 * time.Second,
		MetricsExporter: "none",
		TracesExporter:  "none",
		LogsExporter:    "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updatedLogger != nil {
		t.Error("expected nil logger when logs are disabled")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		t.Error("shutdown should not error when nothing was started:", err)
	}
}

func TestSetup_LogsEnabled_ReturnsLogger(t *testing.T) {
	shutdown, updatedLogger, err := Setup(context.Background(), nil, newTestLogger(), Config{
		Protocol:        "grpc",
		Interval:        15 * time.Second,
		MetricsExporter: "none",
		TracesExporter:  "none",
		LogsExporter:    "otlp",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updatedLogger == nil {
		t.Error("expected non-nil logger when logs are enabled")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = shutdown(ctx)
}

func TestSetup_EmptyExporters_DefaultToOTLP(t *testing.T) {
	// Empty strings should default to "otlp"; with logs defaulting on we expect
	// a non-nil updated logger.
	shutdown, updatedLogger, err := Setup(context.Background(), nil, newTestLogger(), Config{
		Protocol:        "grpc",
		Interval:        15 * time.Second,
		MetricsExporter: "none",
		TracesExporter:  "none",
		// LogsExporter intentionally empty
	})
	if err != nil {
		t.Fatal(err)
	}
	if updatedLogger == nil {
		t.Error("empty LogsExporter should default to \"otlp\" and return an updated logger")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = shutdown(ctx)
}

func TestMultiHandler_Handle_PropagatesError(t *testing.T) {
	want := errors.New("handler error")
	m := multiHandler{&errorHandler{err: want}}

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)
	if got := m.Handle(context.Background(), r); !errors.Is(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestSetup_MetricsAndTracesEnabled(t *testing.T) {
	for _, protocol := range []string{"grpc", "http/protobuf"} {
		t.Run(protocol, func(t *testing.T) {
			shutdown, _, err := Setup(context.Background(), prometheus.NewRegistry(), newTestLogger(), Config{
				Protocol:        protocol,
				Interval:        15 * time.Second,
				MetricsExporter: "otlp",
				TracesExporter:  "otlp",
				LogsExporter:    "none",
			})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = shutdown(ctx)
		})
	}
}

func TestSetup_HTTP_LogsEnabled(t *testing.T) {
	shutdown, updatedLogger, err := Setup(context.Background(), nil, newTestLogger(), Config{
		Protocol:        "http/protobuf",
		Interval:        15 * time.Second,
		MetricsExporter: "none",
		TracesExporter:  "none",
		LogsExporter:    "otlp",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updatedLogger == nil {
		t.Error("expected non-nil logger when logs are enabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = shutdown(ctx)
}

func TestSetup_InvalidProtocol_ReturnsError(t *testing.T) {
	for _, exporter := range []string{"MetricsExporter", "TracesExporter", "LogsExporter"} {
		cfg := Config{
			Protocol:        "invalid",
			Interval:        15 * time.Second,
			MetricsExporter: "none",
			TracesExporter:  "none",
			LogsExporter:    "none",
		}
		switch exporter {
		case "MetricsExporter":
			cfg.MetricsExporter = "otlp"
		case "TracesExporter":
			cfg.TracesExporter = "otlp"
		case "LogsExporter":
			cfg.LogsExporter = "otlp"
		}

		_, _, err := Setup(context.Background(), nil, newTestLogger(), cfg)
		if err == nil {
			t.Errorf("%s: expected error for invalid protocol", exporter)
		}
	}
}
