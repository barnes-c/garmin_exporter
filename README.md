# Garmin Exporter

[![GitHub Release](https://img.shields.io/github/v/release/barnes-c/garmin_exporter)][releases]
[![Build Status](https://github.com/barnes-c/garmin_exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/barnes-c/garmin_exporter/actions/workflows/ci.yml)
[![golangci-lint](https://github.com/barnes-c/garmin_exporter/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/barnes-c/garmin_exporter/actions/workflows/golangci-lint.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/barnes-c/garmin_exporter)][goreportcard]

Prometheus exporter for [Garmin Connect](https://connect.garmin.com) health and training metrics.

Listens on port **10045** by default.

> **Note:** Each scrape calls the Garmin Connect API. Since health data updates at most a few times per day, a scrape interval of 6h–12h is recommended to avoid hammering Garmin's doors.

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

Authentication status is exposed with these metrics:

| Metric | Description |
|--------|-------------|
| `garmin_auth_login_success` | `1` if the most recent login attempt succeeded, `0` otherwise |
| `garmin_auth_next_retry_timestamp_seconds` | Unix timestamp of the next scheduled login attempt, or `0` when no retry is scheduled |

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

## Filtering collectors per scrape

Pass `collect[]` or `exclude[]` query parameters to scrape only specific collectors:

```
# Only wellness and sleep
curl 'localhost:10045/metrics?collect[]=wellness&collect[]=sleep'

# Everything except activities
curl 'localhost:10045/metrics?exclude[]=activities'
```

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

## OTLP metrics push

The exporter can optionally push metrics to any [OpenTelemetry](https://opentelemetry.io/) collector via OTLP, in addition to the standard Prometheus pull endpoint. OTLP push is **disabled by default** and activates when `OTEL_EXPORTER_OTLP_ENDPOINT` is set.

The [OTel Prometheus bridge](https://pkg.go.dev/go.opentelemetry.io/contrib/bridges/prometheus) reads from the existing Prometheus registries — no instrumentation changes needed.

### Configuration

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--otlp.endpoint` | `OTEL_EXPORTER_OTLP_ENDPOINT` | *(disabled)* | OTLP collector endpoint (e.g. `localhost:4317`). Enables OTLP push when set. |
| `--otlp.protocol` | `OTEL_EXPORTER_OTLP_PROTOCOL` | `grpc` | Transport protocol: `grpc` or `http/protobuf` |
| `--otlp.interval` | — | `1h` | OTLP push interval. Each push triggers a full Garmin API fetch, so keep this high to avoid rate limiting. |

The OTLP exporters also respect the standard `OTEL_EXPORTER_OTLP_*` environment variables for headers, TLS certificates, compression, and timeouts. See the [OTel specification](https://opentelemetry.io/docs/specs/otel/protocol/exporter/) for the full list.

### Examples

```bash
# gRPC to a local collector
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317 \
  ./garmin_exporter

# HTTP with authentication
OTEL_EXPORTER_OTLP_ENDPOINT=https://otlp.example.com \
OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf \
OTEL_EXPORTER_OTLP_HEADERS="Authorization=Bearer YOUR_TOKEN" \
  ./garmin_exporter
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
| `/readyz` | `200 OK` when Garmin login has succeeded and the most recent scrape produced data, otherwise `503 Service Unavailable` | Exporter is ready to serve fresh metrics |

`/readyz` reports ready as soon as login succeeds. After the first scrape it also reflects the most recent scrape: if every collector failed (typically because Garmin is unreachable or rate-limiting), it flips back to `503` until a later scrape recovers.

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
