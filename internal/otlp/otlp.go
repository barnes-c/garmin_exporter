// Package otlp wires the existing Prometheus registries into an OpenTelemetry
// metrics push pipeline and configures distributed tracing so the exporter can
// emit OTLP in addition to serving /metrics.
package otlp

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	prombridge "go.opentelemetry.io/contrib/bridges/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/version"
)

// Config configures the OTLP push pipeline.
type Config struct {
	Protocol string
	Interval time.Duration
}

// Setup constructs an OTLP push pipeline that reads from the given Prometheus
// gatherer, sets up a TracerProvider for distributed tracing, and returns a
// shutdown function.
func Setup(ctx context.Context, gatherer prometheus.Gatherer, logger *slog.Logger, cfg Config) (func(context.Context) error, error) {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("garmin_exporter"),
			semconv.ServiceVersion(version.Version),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTel resource: %w", err)
	}

	bridge := prombridge.NewMetricProducer(prombridge.WithGatherer(gatherer))

	var metricExporter sdkmetric.Exporter
	var traceExporter sdktrace.SpanExporter

	switch cfg.Protocol {
	case "grpc":
		metricExporter, err = otlpmetricgrpc.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("creating OTLP gRPC metric exporter: %w", err)
		}
		traceExporter, err = otlptracegrpc.New(ctx)
	case "http/protobuf", "http":
		metricExporter, err = otlpmetrichttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("creating OTLP HTTP metric exporter: %w", err)
		}
		traceExporter, err = otlptracehttp.New(ctx)
	default:
		return nil, fmt.Errorf("unsupported OTLP protocol %q, must be \"grpc\" or \"http/protobuf\"", cfg.Protocol)
	}
	if err != nil {
		return nil, fmt.Errorf("creating OTLP %s trace exporter: %w", cfg.Protocol, err)
	}

	reader := sdkmetric.NewPeriodicReader(
		metricExporter,
		sdkmetric.WithProducer(bridge),
		sdkmetric.WithInterval(cfg.Interval),
	)
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
	)

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tracerProvider)

	logger.Info("OTLP metrics and tracing enabled", "protocol", cfg.Protocol, "interval", cfg.Interval)

	return func(ctx context.Context) error {
		tErr := tracerProvider.Shutdown(ctx)
		mErr := meterProvider.Shutdown(ctx)
		if tErr != nil {
			return tErr
		}
		return mErr
	}, nil
}
