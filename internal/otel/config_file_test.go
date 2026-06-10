package otel

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSetupFromYAML_Happy(t *testing.T) {
	res, err := Setup(context.Background(), newTestLogger(), Config{
		ServiceName:       "ovs-exporter-test",
		PrometheusEnabled: true,
		ConfigFile:        "testdata/valid_minimal.yaml",
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() { _ = res.Shutdown(context.Background()) })

	if res.PromHandler == nil {
		t.Fatal("PromHandler nil; /metrics would not serve")
	}
	if res.Meter == nil || res.Tracer == nil {
		t.Fatal("Meter or Tracer is nil")
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	res.PromHandler.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("/metrics returned %d, want 200", rr.Code)
	}
}

func TestSetupFromYAML_PrometheusDisabled(t *testing.T) {
	res, err := Setup(context.Background(), newTestLogger(), Config{
		ServiceName:       "ovs-exporter-test",
		PrometheusEnabled: false,
		ConfigFile:        "testdata/valid_minimal.yaml",
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() { _ = res.Shutdown(context.Background()) })

	if res.PromHandler != nil {
		t.Fatal("PromHandler should be nil when PrometheusEnabled=false")
	}
}

func TestSetupFromYAML_RejectsPullReader(t *testing.T) {
	_, err := Setup(context.Background(), newTestLogger(), Config{
		ServiceName:       "ovs-exporter-test",
		PrometheusEnabled: true,
		ConfigFile:        "testdata/invalid_pull_reader.yaml",
	})
	if err == nil {
		t.Fatal("expected error for pull reader, got nil")
	}
	if !strings.Contains(err.Error(), "pull reader") {
		t.Fatalf("error %q does not mention pull reader", err)
	}
	if !strings.Contains(err.Error(), "/metrics") {
		t.Fatalf("error %q does not mention /metrics carve-out", err)
	}
}

func TestSetupFromYAML_MissingFile(t *testing.T) {
	_, err := Setup(context.Background(), newTestLogger(), Config{
		ServiceName:       "ovs-exporter-test",
		PrometheusEnabled: true,
		ConfigFile:        "testdata/does_not_exist.yaml",
	})
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error %q does not wrap os.ErrNotExist", err)
	}
}

func TestSetupFromYAML_BadYAML(t *testing.T) {
	_, err := Setup(context.Background(), newTestLogger(), Config{
		ServiceName:       "ovs-exporter-test",
		PrometheusEnabled: true,
		ConfigFile:        "testdata/invalid_syntax.yaml",
	})
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
	if !strings.Contains(err.Error(), "parse YAML") {
		t.Fatalf("error %q does not mention parse failure", err)
	}
}

func TestSetupFromYAML_ClearsConfigFileEnv(t *testing.T) {
	t.Setenv(envVarConfigFile, "testdata/valid_minimal.yaml")

	res, err := Setup(context.Background(), newTestLogger(), Config{
		ServiceName:       "ovs-exporter-test",
		PrometheusEnabled: true,
		ConfigFile:        "testdata/valid_minimal.yaml",
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() { _ = res.Shutdown(context.Background()) })

	if _, set := os.LookupEnv(envVarConfigFile); set {
		t.Fatalf("%s should be unset after Setup", envVarConfigFile)
	}
}
