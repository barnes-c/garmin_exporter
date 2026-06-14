package otel

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	otelslog "go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	otelruntime "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// scopeName is the instrumentation scope reported on Meter and Tracer.
// Use the module path so the OTel `instrumentation.scope.name` attribute
// reflects this codebase.
const scopeName = "github.com/barnes-c/garmin_exporter"

// Config configures the OTel pipeline.
//
// The Prometheus reader is ALWAYS attached to the
// MeterProvider regardless of MetricsExporter — /metrics is part of the
// exporter's contract, not a configurable signal. MetricsExporter therefore
// controls only the *push* exporters layered on top of Prom.
//
// MetricsExporter accepts a comma-separated list, e.g. "otlp", "otlp,console",
// or "none". The literal "prometheus" is accepted as a no-op (already on).
// TracesExporter and LogsExporter accept a single value ("otlp", "console",
// or "none").
type Config struct {
	ServiceName     string
	ServiceVersion  string
	MetricsExporter string // push exporters; default "none"
	TracesExporter  string // default "none"
	LogsExporter    string // default "none"
	PromMaxRequests int    // promhttp MaxRequestsInFlight; 0 → 40

	// ConfigFile, when non-empty, switches Setup to the declarative YAML
	// path: the file is parsed via go.opentelemetry.io/contrib/otelconf
	// and drives the SDK end-to-end. MetricsExporter, TracesExporter, and
	// LogsExporter are ignored — the YAML is the single source of truth,
	// matching the OTel spec rule that environment variables MUST NOT
	// apply when OTEL_CONFIG_FILE is set. ServiceName and PrometheusEnabled
	// still control the Prom reader the exporter always owns.
	// See internal/otel/config_file.go.
	ConfigFile string

	// PrometheusEnabled controls whether the OTel SDK's Prometheus reader
	// is attached to the MeterProvider and a corresponding handler is
	// returned for /metrics. Default is on; callers turn it off when the
	// exporter is used purely as an OTLP pusher. When false,
	// Result.PromHandler is nil and the route is omitted from the HTTP
	// mux.
	PrometheusEnabled bool
}

// Result is what Setup returns. PromHandler is non-nil when
// Config.PrometheusEnabled is true (the default for normal callers) and
// serves /metrics; it is nil when the caller disabled the Prom reader.
// Logger is the original logger by default; when LogsExporter is not
// "none" it is tee'd to also forward records through the OTel log
// pipeline — callers should replace their logger with this one.
type Result struct {
	Meter       metric.Meter
	Tracer      trace.Tracer
	PromHandler http.Handler
	Logger      *slog.Logger
	Shutdown    func(ctx context.Context) error
}

// Setup constructs the configured OTel pipeline. The Prometheus reader is
// always installed. Push exporters whose selector is "none" are skipped.
// The returned Shutdown must be called at process exit.
//
// When cfg.ConfigFile is non-empty, Setup dispatches to setupFromYAML and
// the flag-derived selectors are ignored. ServiceName and PrometheusEnabled
// continue to govern the Prom reader we always own.
func Setup(ctx context.Context, logger *slog.Logger, cfg Config) (*Result, error) {
	setSDKErrorHandler(logger)

	if cfg.ConfigFile != "" {
		return setupFromYAML(ctx, logger, cfg)
	}

	cfg.MetricsExporter = cmp.Or(cfg.MetricsExporter, "none")
	cfg.TracesExporter = cmp.Or(cfg.TracesExporter, "none")
	cfg.LogsExporter = cmp.Or(cfg.LogsExporter, "none")
	cfg.PromMaxRequests = cmp.Or(cfg.PromMaxRequests, 40)

	if cfg.ServiceName == "" {
		return nil, fmt.Errorf("ServiceName is required")
	}

	res, err := buildResource(ctx, cfg.ServiceName, cfg.ServiceVersion)
	if err != nil {
		return nil, err
	}

	var shutdowns []func(context.Context) error

	mp, promHandler, err := buildMeterProvider(ctx, res, cfg)
	if err != nil {
		return nil, err
	}
	otel.SetMeterProvider(mp)
	shutdowns = append(shutdowns, mp.Shutdown)
	meter := otel.Meter(scopeName)

	// Auto-collect Go runtime metrics (goroutines, GC, heap)
	if err := otelruntime.Start(otelruntime.WithMeterProvider(mp)); err != nil {
		logger.Warn("Failed to start runtime instrumentation", "err", err)
	}

	// Install the global TextMapPropagator so otelhttp (and any other
	// instrumentation hanging off the global) extracts incoming W3C
	// traceparent / baggage headers. autoprop honours OTEL_PROPAGATORS
	otel.SetTextMapPropagator(autoprop.NewTextMapPropagator())

	tracer := otel.Tracer(scopeName)
	if cfg.TracesExporter != "none" {
		tp, err := buildTracerProvider(ctx, res, cfg)
		if err != nil {
			return nil, err
		}
		otel.SetTracerProvider(tp)
		shutdowns = append(shutdowns, tp.Shutdown)
		tracer = tp.Tracer(scopeName)
	}

	finalLogger := logger
	if cfg.LogsExporter != "none" {
		lp, teedLogger, err := buildLoggerProvider(ctx, res, logger, cfg)
		if err != nil {
			return nil, err
		}
		shutdowns = append(shutdowns, lp.Shutdown)
		finalLogger = teedLogger
	}

	logger.Info("OTel pipeline configured",
		"service_name", cfg.ServiceName,
		"service_version", cfg.ServiceVersion,
		"metrics_exporter", cfg.MetricsExporter,
		"traces_exporter", cfg.TracesExporter,
		"logs_exporter", cfg.LogsExporter,
	)

	shutdown := func(ctx context.Context) error {
		var errs []error
		for _, s := range shutdowns {
			if err := s(ctx); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}

	return &Result{
		Meter:       meter,
		Tracer:      tracer,
		PromHandler: promHandler,
		Logger:      finalLogger,
		Shutdown:    shutdown,
	}, nil
}

// buildMeterProvider assembles the MeterProvider. The Prometheus reader is
// always present; additional push readers come from the parsed
// MetricsExporter list (each kind resolved through autoexport, which honours
// the OTEL_EXPORTER_OTLP_* env vars). Histograms are aggregated as native
// (base-2) exponential histograms — Prom 3.0+ ingests these directly.
func buildMeterProvider(ctx context.Context, res *resource.Resource, cfg Config) (*sdkmetric.MeterProvider, http.Handler, error) {
	var (
		readers     []sdkmetric.Reader
		promHandler http.Handler
	)
	if cfg.PrometheusEnabled {
		r, h, err := newPromReader(cfg.PromMaxRequests)
		if err != nil {
			return nil, nil, err
		}
		readers = append(readers, r)
		promHandler = h
	}

	pushKinds, err := parsePushExporters(cfg.MetricsExporter)
	if err != nil {
		return nil, nil, err
	}
	for _, kind := range pushKinds {
		r, err := metricReaderForKind(ctx, kind)
		if err != nil {
			return nil, nil, fmt.Errorf("autoexport metric reader %q: %w", kind, err)
		}
		readers = append(readers, r)
	}

	opts := []sdkmetric.Option{
		sdkmetric.WithResource(res),
		sdkmetric.WithView(sdkmetric.NewView(
			sdkmetric.Instrument{Kind: sdkmetric.InstrumentKindHistogram},
			sdkmetric.Stream{
				Aggregation: sdkmetric.AggregationBase2ExponentialHistogram{
					MaxSize:  160,
					MaxScale: 20,
				},
			},
		)),
	}
	for _, r := range readers {
		opts = append(opts, sdkmetric.WithReader(r))
	}

	return sdkmetric.NewMeterProvider(opts...), promHandler, nil
}

func buildTracerProvider(ctx context.Context, res *resource.Resource, cfg Config) (*sdktrace.TracerProvider, error) {
	exp, err := exporterForKind(ctx, "OTEL_TRACES_EXPORTER", cfg.TracesExporter,
		func(c context.Context) (sdktrace.SpanExporter, error) {
			return autoexport.NewSpanExporter(c)
		})
	if err != nil {
		return nil, fmt.Errorf("autoexport span exporter: %w", err)
	}

	// No WithSampler — the SDK reads OTEL_TRACES_SAMPLER and
	// OTEL_TRACES_SAMPLER_ARG, falling back to ParentBased(AlwaysSample).
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	), nil
}

func buildLoggerProvider(ctx context.Context, res *resource.Resource, logger *slog.Logger, cfg Config) (*sdklog.LoggerProvider, *slog.Logger, error) {
	exp, err := exporterForKind(ctx, "OTEL_LOGS_EXPORTER", cfg.LogsExporter,
		func(c context.Context) (sdklog.Exporter, error) {
			return autoexport.NewLogExporter(c)
		})
	if err != nil {
		return nil, nil, fmt.Errorf("autoexport log exporter: %w", err)
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)),
	)

	otelHandler := otelslog.NewHandler(cfg.ServiceName,
		otelslog.WithLoggerProvider(lp),
		otelslog.WithSource(true),
	)
	return lp, slog.New(multiHandler{logger.Handler(), otelHandler}), nil
}

// metricReaderForKind resolves one push-metric kind via autoexport. The
// contrib package keys off OTEL_METRICS_EXPORTER, so we set the env var
// for the duration of the call to feed it our parsed kind.
func metricReaderForKind(ctx context.Context, kind string) (sdkmetric.Reader, error) {
	restore := setEnvScoped("OTEL_METRICS_EXPORTER", kind)
	defer restore()
	return autoexport.NewMetricReader(ctx)
}

// exporterForKind is the generic shim for traces and logs: scope the
// signal-selector env var to the caller's value and let autoexport build
// the exporter.
func exporterForKind[T any](ctx context.Context, envKey, kind string, factory func(context.Context) (T, error)) (T, error) {
	restore := setEnvScoped(envKey, kind)
	defer restore()
	return factory(ctx)
}

// setEnvScoped temporarily sets env[key]=val and returns a restore func
// that reinstates the previous value (or unsets if there was none).
func setEnvScoped(key, val string) func() {
	prev, had := os.LookupEnv(key)
	_ = os.Setenv(key, val)
	return func() {
		if had {
			_ = os.Setenv(key, prev)
		} else {
			_ = os.Unsetenv(key)
		}
	}
}

// parsePushExporters splits the comma-separated MetricsExporter value into
// the non-Prom kinds that need a PeriodicReader. "none" or empty yields nil.
// "prometheus" is accepted but ignored — the Prom reader is always-on, and
// autoexport's "prometheus" selector would otherwise spawn a second HTTP
// listener that clashes with /metrics. Other entries are normalized
// (trimmed, deduped).
func parsePushExporters(s string) ([]string, error) {
	if strings.TrimSpace(s) == "none" {
		return nil, nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, raw := range strings.Split(s, ",") {
		k := strings.TrimSpace(raw)
		switch k {
		case "", "prometheus":
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out, nil
}

func setSDKErrorHandler(logger *slog.Logger) {
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Error("OTel SDK error", "err", err)
	}))
}

// multiHandler fans out slog records to multiple handlers.
type multiHandler []slog.Handler

func (m multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range m {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (m multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make(multiHandler, len(m))
	for i, h := range m {
		handlers[i] = h.WithAttrs(attrs)
	}
	return handlers
}

func (m multiHandler) WithGroup(name string) slog.Handler {
	handlers := make(multiHandler, len(m))
	for i, h := range m {
		handlers[i] = h.WithGroup(name)
	}
	return handlers
}
