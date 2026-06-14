# Garmin Exporter

[![GitHub Release](https://img.shields.io/github/v/release/barnes-c/garmin_exporter)][releases]
[![Build Status](https://github.com/barnes-c/garmin_exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/barnes-c/garmin_exporter/actions/workflows/ci.yml)
[![golangci-lint](https://github.com/barnes-c/garmin_exporter/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/barnes-c/garmin_exporter/actions/workflows/golangci-lint.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/barnes-c/garmin_exporter)][goreportcard]

Prometheus exporter for [Garmin Connect](https://connect.garmin.com) health and training metrics.

Listens on port **10045** by default.

## Usage

### Binary

```bash
export GARMIN_USERNAME=you@example.com
export GARMIN_PASSWORD=yourpassword
./garmin_exporter
```

### Docker

```bash
docker volume create garmin_data

docker run -d \
  -p 10045:10045 \
  -e GARMIN_USERNAME=you@example.com \
  -e GARMIN_PASSWORD=yourpassword \
  -v garmin_data:/data \
  barnesbiz/garmin_exporter:latest \
  --garmin.token-file=/data/garmin_token.json
```

Images are available on both [Docker Hub][dockerhub] (`barnesbiz/garmin_exporter`) and [GitHub Container Registry][ghcr] (`ghcr.io/barnes-c/garmin_exporter`).

### Docker Compose

```yaml
services:
  garmin_exporter:
    image: barnesbiz/garmin_exporter:latest
    ports:
      - "10045:10045"
    environment:
      GARMIN_USERNAME: you@example.com
      GARMIN_PASSWORD: yourpassword
    volumes:
      - garmin_data:/data
    command:
      - --garmin.token-file=/data/garmin_token.json
    restart: unless-stopped

volumes:
  garmin_data:
```

## Authentication

The exporter authenticates using your Garmin Connect username and password via the mobile SSO flow. On first start it performs a full login and caches the OAuth2 token to disk. Subsequent starts load the cached token and refresh it automatically. No re-login needed until the refresh token expires.

### Multi-factor authentication

If your Garmin account has MFA enabled, the exporter will prompt for the one-time code on stdin during login:

```
MFA code (check your email):
```

The exporter must be run interactively for this first login. Once the token is cached to disk, subsequent starts load it automatically and MFA is not required again until the refresh token expires.

For Docker Compose, add `stdin_open: true` and `tty: true` for the initial run so the prompt is reachable. Once the token is cached these can be removed.

If login fails, the exporter keeps serving metrics and retries in the background with exponential backoff. The retry delay starts at 1 minute and grows up to 3 hours. While login is failing, Garmin data collectors report no data until a later login attempt succeeds.

Credentials are passed via flags or environment variables:

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--garmin.username` | `GARMIN_USERNAME` | *(required)* | Garmin Connect email address |
| `--garmin.password` | `GARMIN_PASSWORD` | *(required)* | Garmin Connect password |
| `--garmin.token-file` | — | `garmin_token.json` | Path to the cached OAuth2 token file |
| `--garmin.activity-limit` | — | `30` | Number of recent activities to fetch per scrape |
| `--cache.ttl` | — | `1h` | How often to refresh data from Garmin Connect |

Authentication status is exposed with these metrics:

| Metric | Description |
|--------|-------------|
| `garmin_auth_login_success` | `1` if the most recent login attempt succeeded, `0` otherwise |
| `garmin_auth_next_retry_timestamp_seconds` | Unix timestamp of the next scheduled login attempt, or `0` when no retry is scheduled |

For scrape liveness, use `/readyz` (see [Health and readiness](#health-and-readiness)) — it returns `503` when the auth client is missing or when the cached snapshot is stale relative to `--cache.ttl`.

## Collectors

All collectors are enabled by default. Individual collectors can be disabled with `--no-collector.<name>` or a specific set enabled with `--collector.<name>`.

### Enabled by default

| Name | Description |
|------|-------------|
| `wellness` | Daily summary: steps, calories, BMR, distance, active seconds, floors, heart rate, body battery, respiration, stress durations |
| `sleep` | Sleep stages (total, deep, light, REM, awake, nap), restless moments, sleep respiration, SpO2, stress, and HRV |
| `heartrate` | Daily resting, min, max, and 7-day average resting heart rate |
| `spo2` | Blood oxygen saturation: daily average, lowest, and 7-day average |
| `respiration` | Breathing rate: average waking, highest, and lowest |
| `stress` | All-day stress: average and peak level |
| `hydration` | Hydration intake, goal, daily average, sweat loss, and activity intake |
| `intensity` | Weekly intensity minutes: goal, moderate, and vigorous |
| `training` | Training readiness score, VO2 max (running + cycling), fitness age, race predictions, endurance score, hill score |
| `activities` | Recent activities aggregated by type: count, total duration, total distance, total calories, last timestamp, and all-time lifetime count |
| `body` | Today's weigh-in: weight, BMI, body fat %, body water %, bone mass, muscle mass, visceral fat, metabolic age |
| `body_composition` | 30-day averages of the same body composition metrics |
| `blood_pressure` | Most recent blood pressure reading: systolic, diastolic, pulse |
| `cycling` | Cycling functional threshold power (FTP) in watts |
| `devices` | Count of registered Garmin devices and an info metric per device (name, type, status) |
| `goals` | Number of active fitness goals and total earned badges |
| `personalrecords` | Personal records by type (e.g. fastest 5K, longest ride) in raw Garmin units |
| `gear` | Registered equipment: retirement distance limit, notification threshold, and active status |
| `trainingstatus` | Aggregated training status from the primary device: status code, weekly load, ACWR, and monthly aerobic/anaerobic load |
| `lactatethreshold` | Latest lactate threshold: running speed (m/s), running HR, and cycling HR |
| `runningtolerance` | Most recent running tolerance score and level |

### Disabled by default

| Name | Description |
|------|-------------|
| `golf` | Most recent golf round: total score and score relative to par |

Enable with `--collector.golf`.

## Building

```bash
git clone https://github.com/barnes-c/garmin_exporter.git
cd garmin_exporter
make build
./garmin_exporter --garmin.username=you@example.com --garmin.password=yourpassword
```

## Testing

```bash
make test
```

## OpenTelemetry

The exporter is OTel-native: every collector registers OTel observable instruments on a single MeterProvider. The `/metrics` endpoint is served by the OTel SDK's Prometheus reader, which always emits the legacy Prometheus names (`garmin_heartrate_resting_bpm`, etc.). Additional signals (traces, logs) and push exporters layer on top of the same MeterProvider.

OTLP push and the trace/log pipelines are **disabled by default** — you have to opt in with one of the signal-selector flags below. Set any of `--otel.metrics-exporter`, `--otel.traces-exporter`, or `--otel.logs-exporter` to `otlp` (or set `OTEL_EXPORTER_OTLP_ENDPOINT`) to activate them.

### Configuration

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--otel.metrics-exporter` | `OTEL_METRICS_EXPORTER` | *(disabled)* | Comma-separated push exporters (`otlp`, `console`, `none`). `/metrics` is always served regardless. |
| `--otel.traces-exporter` | `OTEL_TRACES_EXPORTER` | *(disabled)* | Traces exporter (`otlp`, `console`, `none`). |
| `--otel.logs-exporter` | `OTEL_LOGS_EXPORTER` | *(disabled)* | Logs exporter (`otlp`, `console`, `none`). When enabled, the exporter's logs are tee'd through OTel in addition to stderr. |
| `--otel.otlp.endpoint` | `OTEL_EXPORTER_OTLP_ENDPOINT` | *(empty)* | OTLP collector endpoint (e.g. `localhost:4317`). |
| `--otel.otlp.protocol` | `OTEL_EXPORTER_OTLP_PROTOCOL` | `grpc` | Transport protocol: `grpc` or `http/protobuf`. |
| `--otel.otlp.interval` | — | `15s` | OTLP metrics push interval. Independent of `--cache.ttl`; each push sends the most recent cached values without triggering a Garmin API call. |
| `--otel.trace-sample-rate` | — | `1.0` | Trace sample rate (0 < rate ≤ 1). |
| `--otel.service-name` | `OTEL_SERVICE_NAME` | `garmin_exporter` | `service.name` resource attribute. |
| `--web.prometheus` | — | `true` | Serve `/metrics`. Disable for OTLP-push-only deployments. |
| `--otel.config-file` | `OTEL_CONFIG_FILE` | *(unused)* | Path to an OTel declarative YAML config (otelconf). When set, every other `--otel.*` flag is ignored per the OTel spec. |

The OTLP exporters also respect the standard `OTEL_EXPORTER_OTLP_*` environment variables for headers, TLS certificates, compression, and timeouts. See the [OTel specification](https://opentelemetry.io/docs/specs/otel/protocol/exporter/) for the full list.

### Examples

```bash
# gRPC to a local collector — push metrics in addition to /metrics
./garmin_exporter \
  --otel.metrics-exporter=otlp \
  --otel.otlp.endpoint=localhost:4317

# HTTP with authentication, all three signals
OTEL_EXPORTER_OTLP_HEADERS="Authorization=Bearer YOUR_TOKEN" \
  ./garmin_exporter \
    --otel.metrics-exporter=otlp \
    --otel.traces-exporter=otlp \
    --otel.logs-exporter=otlp \
    --otel.otlp.endpoint=https://otlp.example.com \
    --otel.otlp.protocol=http/protobuf
```

### Docker Compose

```yaml
services:
  garmin_exporter:
    image: barnesbiz/garmin_exporter:latest
    ports:
      - "10045:10045"
    environment:
      GARMIN_USERNAME: you@example.com
      GARMIN_PASSWORD: yourpassword
      OTEL_METRICS_EXPORTER: otlp
      OTEL_EXPORTER_OTLP_ENDPOINT: https://otlp.example.com
      OTEL_EXPORTER_OTLP_PROTOCOL: http/protobuf
      OTEL_EXPORTER_OTLP_HEADERS: "Authorization=Bearer YOUR_TOKEN"
    volumes:
      - garmin_data:/data
    command:
      - --garmin.token-file=/data/garmin_token.json
    restart: unless-stopped
```

## Health and readiness

The exporter exposes two endpoints for liveness and readiness probes:

| Path | Status | Meaning |
|------|--------|---------|
| `/healthz` | always `200 OK` | Process is alive |
| `/readyz` | `200 OK` when Garmin login has succeeded and the cached snapshot is fresh, otherwise `503 Service Unavailable` | Exporter is ready to serve fresh metrics |

`/readyz` runs two checks: `auth` is healthy once login has succeeded (and flips back to unhealthy if a re-auth fails); `scrape` is healthy as long as the most recent successful refresh is no older than `3 × --cache.ttl`.

Example Kubernetes probes:

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 10045
readinessProbe:
  httpGet:
    path: /readyz
    port: 10045
```

## TLS

The exporter supports TLS and basic auth via the [exporter-toolkit web configuration](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md):

```bash
./garmin_exporter --web.config.file=web-config.yml
```

[releases]: https://github.com/barnes-c/garmin_exporter/releases/latest
[ghcr]: https://github.com/barnes-c/garmin_exporter/pkgs/container/garmin_exporter
[dockerhub]: https://hub.docker.com/r/barnesbiz/garmin_exporter
[goreportcard]: https://goreportcard.com/report/github.com/barnes-c/garmin_exporter
