# OTLP push (metrics + traces + logs)

`garmin_exporter` pushing OTLP to an OTel Collector that fans signals out to Prometheus, Tempo, and Loki. Grafana is wired up to all three.

## Stack

| Service           | Port             | Notes                                                  |
| ----------------- | ---------------- | ------------------------------------------------------ |
| `garmin_exporter` | 10045            | `/metrics` still served; pushes OTLP/gRPC to collector |
| `otel-collector`  | 4317, 4318, 8889 | OTLP gRPC/HTTP receivers, fans out to sinks            |
| `prometheus`      | 9090             | Receives metrics via remote-write                      |
| `tempo`           | 3200             | Receives traces via OTLP                               |
| `loki`            | 3100             | Receives logs via OTLP                                 |
| `grafana`         | 3000             | All three pre-provisioned as datasources               |

## How the wiring works

The exporter is configured from [`../otelconf.yaml`](../otelconf.yaml), mounted into the container and selected with `OTEL_CONFIG_FILE=/etc/garmin_exporter/otelconf.yaml`. That single file replaces the `OTEL_METRICS_EXPORTER`, `OTEL_TRACES_EXPORTER`, `OTEL_LOGS_EXPORTER`, `OTEL_EXPORTER_OTLP_*`, and `OTEL_TRACES_SAMPLER*` env vars.

The config declares:

- A periodic metric reader pushing OTLP/gRPC to `otel-collector:4317` every 60s.
- A batch span processor pushing OTLP/gRPC to the same endpoint, sampled at 10% with `parent_based` + `trace_id_ratio_based`.
- A batch log record processor pushing OTLP/gRPC to the same endpoint.

`/metrics` continues to be served alongside the OTLP push. Set `--web.prometheus=false` on the exporter to disable it.

## Run

```shell
cp .env.example .env
# edit .env with your Garmin credentials

# First run (foreground, so you can answer the MFA prompt if needed):
docker compose run --rm garmin_exporter

# Once the token is cached, bring the full stack up:
docker compose up -d
```

Open <http://localhost:3000>. Metrics arrive via the `Prometheus` datasource, traces via `Tempo`, logs via `Loki`. Trace and log entries are linked: clicking a trace ID in Loki jumps to Tempo, and Tempo spans expose links back to Loki and Prometheus.

## Reset

```shell
docker compose down -v
```

This removes the cached Garmin token and all backing storage.
