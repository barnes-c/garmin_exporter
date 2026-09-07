package otel

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// TestBuildResourceSchemaURL guards against the semconv version drifting away
// from the one the SDK's built-in detectors use. A mismatch makes resource.New
// fail with "conflicting Schema URL" and the exporter cannot start.
func TestBuildResourceSchemaURL(t *testing.T) {
	res, err := buildResource(context.Background(), "garmin_exporter", "test")
	if err != nil {
		t.Fatalf("buildResource() error = %v", err)
	}

	sdkRes, err := resource.New(context.Background(),
		resource.WithContainer(),
		resource.WithHost(),
		resource.WithOS(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		t.Fatalf("building SDK detector resource: %v", err)
	}

	if got, want := res.SchemaURL(), sdkRes.SchemaURL(); got != want {
		t.Errorf("resource schema URL = %q, SDK detectors use %q", got, want)
	}
	if got := res.SchemaURL(); got != semconv.SchemaURL {
		t.Errorf("resource schema URL = %q, want %q", got, semconv.SchemaURL)
	}
}
