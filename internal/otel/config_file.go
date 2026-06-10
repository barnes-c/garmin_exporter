package otel

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	otelslog "go.opentelemetry.io/contrib/bridges/otelslog"
	otelruntime "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/contrib/otelconf"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// envVarConfigFile mirrors the env var name otelconf reads itself. We
// clear it before NewSDK so the parsed config we hand over explicitly is
// the only source — otherwise NewSDK re-parses the file and silently
// overrides our injected MeterProviderOptions (where the Prom reader
// lives). We intentionally do NOT touch OTEL_EXPERIMENTAL_CONFIG_FILE:
// if an operator still has the deprecated env set, upstream's error
// surfaces and tells them to migrate.
const envVarConfigFile = "OTEL_CONFIG_FILE"

// setupFromYAML builds the SDK from an OTel declarative configuration
// file. The Prometheus reader is constructed and injected here — the
// YAML must not declare one (see rejectPullReaders).
func setupFromYAML(ctx context.Context, logger *slog.Logger, cfg Config) (*Result, error) {
	if cfg.ServiceName == "" {
		return nil, fmt.Errorf("ServiceName is required")
	}
	if cfg.PromMaxRequests == 0 {
		cfg.PromMaxRequests = 40
	}

	raw, err := os.ReadFile(cfg.ConfigFile)
	if err != nil {
		return nil, fmt.Errorf("otel: read config file %q: %w", cfg.ConfigFile, err)
	}

	parsed, err := otelconf.ParseYAML(raw)
	if err != nil {
		return nil, fmt.Errorf("otel: parse YAML config: %w", err)
	}

	if err := rejectPullReaders(parsed); err != nil {
		return nil, err
	}

	var (
		promHandler http.Handler
		meterOpts   []sdkmetric.Option
	)
	if cfg.PrometheusEnabled {
		r, h, err := newPromReader(cfg.PromMaxRequests)
		if err != nil {
			return nil, err
		}
		meterOpts = append(meterOpts, sdkmetric.WithReader(r))
		promHandler = h
		// Materialize an empty MeterProvider if the YAML omits one;
		// otherwise otelconf returns a noop MeterProvider and silently
		// drops the WithMeterProviderOptions where our Prom reader lives,
		// leaving /metrics empty.
		if parsed.MeterProvider == nil {
			parsed.MeterProvider = &otelconf.MeterProvider{}
		}
	}

	// Same clear-before-NewSDK dance as documented on envVarConfigFile.
	if _, set := os.LookupEnv(envVarConfigFile); set {
		if err := os.Unsetenv(envVarConfigFile); err != nil {
			return nil, fmt.Errorf("otel: unset %s: %w", envVarConfigFile, err)
		}
	}

	// TODO(otelconf): contrib/otelconf v0.24.0 does not expose the
	// constructed reader list and its pullReader path is a stub that
	// always errors. Until upstream supports a YAML-declared Prometheus
	// reader AND lets us extract its http.Handler, the exporter owns
	// /metrics: we inject our own Prom reader via
	// WithMeterProviderOptions and rejectPullReaders refuses any pull
	// reader in the YAML. Upstream tracking: search
	// opentelemetry-go-contrib issues for "otelconf prometheus reader
	// handler".
	sdk, err := otelconf.NewSDK(
		otelconf.WithContext(ctx),
		otelconf.WithOpenTelemetryConfiguration(*parsed),
		otelconf.WithMeterProviderOptions(meterOpts...),
	)
	if err != nil {
		return nil, fmt.Errorf("otel: build SDK from YAML: %w", err)
	}

	otel.SetMeterProvider(sdk.MeterProvider())
	otel.SetTracerProvider(sdk.TracerProvider())
	otel.SetTextMapPropagator(sdk.Propagator())

	if err := otelruntime.Start(otelruntime.WithMeterProvider(sdk.MeterProvider())); err != nil {
		logger.Warn("Failed to start runtime instrumentation", "err", err)
	}

	finalLogger := logger
	if lp := sdk.LoggerProvider(); lp != nil {
		otelHandler := otelslog.NewHandler(cfg.ServiceName,
			otelslog.WithLoggerProvider(lp),
			otelslog.WithSource(true),
		)
		finalLogger = slog.New(multiHandler{logger.Handler(), otelHandler})
	}

	logger.Info("OTel pipeline configured from YAML",
		"config_file", cfg.ConfigFile,
		"service_name", cfg.ServiceName,
		"prometheus_enabled", cfg.PrometheusEnabled,
	)

	return &Result{
		Meter:       sdk.MeterProvider().Meter(scopeName),
		Tracer:      sdk.TracerProvider().Tracer(scopeName),
		PromHandler: promHandler,
		Logger:      finalLogger,
		Shutdown:    sdk.Shutdown,
	}, nil
}

// rejectPullReaders refuses YAML configs that declare any pull metric
// reader. The exporter always owns the Prometheus reader and serves
// /metrics from its own HTTP mux; a second SDK-managed listener would
// either bind-conflict or silently leave the user without /metrics.
func rejectPullReaders(cfg *otelconf.OpenTelemetryConfiguration) error {
	if cfg == nil || cfg.MeterProvider == nil {
		return nil
	}
	for i, r := range cfg.MeterProvider.Readers {
		if r.Pull != nil {
			return fmt.Errorf(
				"otel: meter_provider.readers[%d]: pull reader (e.g. prometheus) is not allowed in YAML config — "+
					"garmin-exporter always serves /metrics from its built-in Prometheus reader. "+
					"Remove the pull reader entry; use periodic readers for OTLP push instead",
				i,
			)
		}
	}
	return nil
}
