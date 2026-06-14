package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

// buildResource composes the OTel Resource from SDK metadata, host/process
// attributes, the user-provided OTEL_RESOURCE_ATTRIBUTES env var, and the
// service identity supplied by the caller.
func buildResource(ctx context.Context, serviceName, serviceVersion string) (*resource.Resource, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
		resource.WithContainer(),
		resource.WithFromEnv(),
		resource.WithHost(),
		resource.WithOS(),
		resource.WithProcess(),
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf("building OTel resource: %w", err)
	}
	return res, nil
}
