package otel

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// newPromReader constructs the OTel SDK Prometheus exporter and its HTTP
// handler. The exporter is a sdkmetric.Reader attached to the MeterProvider;
// the handler is what /metrics serves.
//
// target_info and otel_scope_info series are kept enabled (OTel SDK defaults):
// target_info exposes Resource attributes joinable in PromQL, scope_info adds
// instrumentation-scope labels useful when this exporter eventually emits
// metrics from multiple scopes.
func newPromReader(maxRequests int) (sdkmetric.Reader, http.Handler, error) {
	reg := prometheus.NewRegistry()
	exp, err := otelprom.New(otelprom.WithRegisterer(reg))
	if err != nil {
		return nil, nil, fmt.Errorf("creating Prometheus OTel exporter: %w", err)
	}

	handler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		ErrorHandling:       promhttp.ContinueOnError,
		MaxRequestsInFlight: maxRequests,
	})
	return exp, handler, nil
}
