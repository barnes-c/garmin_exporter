package main

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

type otlpConfig struct {
	protocol string
	interval time.Duration
}

func setupOTLP(ctx context.Context, gatherer prometheus.Gatherer, logger *slog.Logger, cfg otlpConfig) (func(context.Context) error, error) {
	bridge := prombridge.NewMetricProducer(prombridge.WithGatherer(gatherer))

	var exporter sdkmetric.Exporter
	var err error

	switch cfg.protocol {
	case "grpc":
		exporter, err = otlpmetricgrpc.New(ctx)
	case "http/protobuf", "http":
		exporter, err = otlpmetrichttp.New(ctx)
	default:
		return nil, fmt.Errorf("unsupported OTLP protocol %q, must be \"grpc\" or \"http/protobuf\"", cfg.protocol)
	}
	if err != nil {
		return nil, fmt.Errorf("creating OTLP %s exporter: %w", cfg.protocol, err)
	}

	reader := sdkmetric.NewPeriodicReader(
		exporter,
		sdkmetric.WithProducer(bridge),
		sdkmetric.WithInterval(cfg.interval),
	)

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	logger.Info("OTLP metrics push enabled", "protocol", cfg.protocol, "interval", cfg.interval)

	return provider.Shutdown, nil
}
