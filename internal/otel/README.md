# OTel

Package `otel` wires the OpenTelemetry SDK pipelines (metrics, traces, logs) used by garmin-exporter. Native OTel instruments are the source of truth; /metrics is served via the OTel SDK's Prometheus exporter, and OTLP push is an optional Reader on the same MeterProvider.

[OTel SDK env-var spec](https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/)
