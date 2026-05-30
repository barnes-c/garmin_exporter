// Package otlp wires the existing Prometheus registries into an OpenTelemetry
// metrics push pipeline so the exporter can emit OTLP in addition to serving
// /metrics.
package otlp

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	prombridge "go.opentelemetry.io/contrib/bridges/prometheus"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/prometheus/client_golang/prometheus"
)

// Config configures the OTLP push pipeline.
type Config struct {
	Protocol string
	Interval time.Duration
}

// Setup constructs an OTLP push pipeline that reads from the given Prometheus
// gatherer and returns a shutdown function.
func Setup(ctx context.Context, gatherer prometheus.Gatherer, logger *slog.Logger, cfg Config) (func(context.Context) error, error) {
	bridge := prombridge.NewMetricProducer(prombridge.WithGatherer(gatherer))

	var exporter sdkmetric.Exporter
	var err error

	switch cfg.Protocol {
	case "grpc":
		exporter, err = otlpmetricgrpc.New(ctx)
	case "http/protobuf", "http":
		exporter, err = otlpmetrichttp.New(ctx)
	default:
		return nil, fmt.Errorf("unsupported OTLP protocol %q, must be \"grpc\" or \"http/protobuf\"", cfg.Protocol)
	}
	if err != nil {
		return nil, fmt.Errorf("creating OTLP %s exporter: %w", cfg.Protocol, err)
	}

	reader := sdkmetric.NewPeriodicReader(
		exporter,
		sdkmetric.WithProducer(bridge),
		sdkmetric.WithInterval(cfg.Interval),
	)

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	logger.Info("OTLP metrics push enabled", "protocol", cfg.Protocol, "interval", cfg.Interval)

	return provider.Shutdown, nil
}
