// Package otlp wires the existing Prometheus registries into an OpenTelemetry
// metrics push pipeline and configures distributed tracing and log export so
// the exporter can emit OTLP in addition to serving /metrics.
package otlp

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	otelslog "go.opentelemetry.io/contrib/bridges/otelslog"
	prombridge "go.opentelemetry.io/contrib/bridges/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/version"
)

// Config configures the OTLP push pipeline.
type Config struct {
	Protocol        string
	Interval        time.Duration
	MetricsExporter string // "otlp" or "none"
	TracesExporter  string // "otlp" or "none"
	LogsExporter    string // "otlp" or "none"
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

// Setup constructs the enabled OTLP pipelines and returns a shutdown function
// and, when log export is enabled, an updated *slog.Logger that tees records to
// both the original handler and the OTLP backend. The caller should replace
// their logger with the returned one when it is non-nil.
//
// Which pipelines are active is controlled by cfg.MetricsExporter,
// cfg.TracesExporter, and cfg.LogsExporter (each defaults to "otlp" when
// empty; set to "none" to disable).
func Setup(ctx context.Context, gatherer prometheus.Gatherer, logger *slog.Logger, cfg Config) (func(context.Context) error, *slog.Logger, error) {
	cfg.MetricsExporter = cmp.Or(cfg.MetricsExporter, "otlp")
	cfg.TracesExporter = cmp.Or(cfg.TracesExporter, "otlp")
	cfg.LogsExporter = cmp.Or(cfg.LogsExporter, "otlp")

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("garmin_exporter"),
			semconv.ServiceVersion(version.Version),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("creating OTel resource: %w", err)
	}

	var shutdowns []func(context.Context) error

	if cfg.MetricsExporter != "none" {
		var metricExporter sdkmetric.Exporter
		switch cfg.Protocol {
		case "grpc":
			metricExporter, err = otlpmetricgrpc.New(ctx)
		case "http/protobuf", "http":
			metricExporter, err = otlpmetrichttp.New(ctx)
		default:
			return nil, nil, fmt.Errorf("unsupported OTLP protocol %q, must be \"grpc\" or \"http/protobuf\"", cfg.Protocol)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("creating OTLP %s metric exporter: %w", cfg.Protocol, err)
		}

		bridge := prombridge.NewMetricProducer(prombridge.WithGatherer(gatherer))
		reader := sdkmetric.NewPeriodicReader(
			metricExporter,
			sdkmetric.WithProducer(bridge),
			sdkmetric.WithInterval(cfg.Interval),
		)
		meterProvider := sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(reader),
			sdkmetric.WithResource(res),
		)
		shutdowns = append(shutdowns, meterProvider.Shutdown)
	}

	if cfg.TracesExporter != "none" {
		var traceExporter sdktrace.SpanExporter
		switch cfg.Protocol {
		case "grpc":
			traceExporter, err = otlptracegrpc.New(ctx)
		case "http/protobuf", "http":
			traceExporter, err = otlptracehttp.New(ctx)
		default:
			return nil, nil, fmt.Errorf("unsupported OTLP protocol %q, must be \"grpc\" or \"http/protobuf\"", cfg.Protocol)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("creating OTLP %s trace exporter: %w", cfg.Protocol, err)
		}

		tracerProvider := sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(traceExporter),
			sdktrace.WithResource(res),
		)
		otel.SetTracerProvider(tracerProvider)
		shutdowns = append(shutdowns, tracerProvider.Shutdown)
	}

	var updatedLogger *slog.Logger
	if cfg.LogsExporter != "none" {
		var logExporter sdklog.Exporter
		switch cfg.Protocol {
		case "grpc":
			logExporter, err = otlploggrpc.New(ctx)
		case "http/protobuf", "http":
			logExporter, err = otlploghttp.New(ctx)
		default:
			return nil, nil, fmt.Errorf("unsupported OTLP protocol %q, must be \"grpc\" or \"http/protobuf\"", cfg.Protocol)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("creating OTLP %s log exporter: %w", cfg.Protocol, err)
		}

		logProvider := sdklog.NewLoggerProvider(
			sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
			sdklog.WithResource(res),
		)
		otelHandler := otelslog.NewHandler("garmin_exporter", otelslog.WithLoggerProvider(logProvider))
		updatedLogger = slog.New(multiHandler{logger.Handler(), otelHandler})
		shutdowns = append(shutdowns, logProvider.Shutdown)
	}

	logger.Info("OTLP enabled",
		"protocol", cfg.Protocol,
		"metrics_exporter", cfg.MetricsExporter,
		"traces_exporter", cfg.TracesExporter,
		"logs_exporter", cfg.LogsExporter,
		"interval", cfg.Interval,
	)

	return func(ctx context.Context) error {
		for _, shutdown := range shutdowns {
			if err := shutdown(ctx); err != nil {
				return err
			}
		}
		return nil
	}, updatedLogger, nil
}
