# garmin_exporter

[![Garmin Connect](https://img.shields.io/badge/Garmin%20Connect-007CC3?logo=garmin&logoColor=white)](https://connect.garmin.com)
[![GitHub Release](https://img.shields.io/github/v/release/barnes-c/garmin_exporter)](https://github.com/barnes-c/garmin_exporter/releases/latest)
[![GHCR](https://img.shields.io/badge/ghcr.io-barnes--c%2Fgarmin__exporter-blue?logo=github)](https://github.com/barnes-c/garmin_exporter/pkgs/container/garmin_exporter)
[![License](https://img.shields.io/github/license/barnes-c/garmin_exporter)](LICENSE)

OTel-native Prometheus exporter for [Garmin Connect](https://connect.garmin.com/). Serves `/metrics` on `:10045` and can optionally push metrics, traces, and logs via OTLP.

## Quick start

```shell
docker run -d -p 10045:10045 -v garmin_data:/data \
  -e GARMIN_USERNAME=you@example.com \
  -e GARMIN_PASSWORD=yourpassword \
  ghcr.io/barnes-c/garmin_exporter:latest \
  --token-file=/data/garmin_token.json
```

Or from source: `make build && GARMIN_USERNAME=... GARMIN_PASSWORD=... ./garmin_exporter`.

**MFA:** on first login the exporter prompts for a one-time code on stdin, so run it interactively once (`docker run -it`, or `stdin_open`/`tty` in Compose). The OAuth2 token is then cached to `--token-file` and refreshed automatically.

See [`examples/`](examples) for Prometheus + Grafana and OTLP (Tempo, Loki) setups, and [`examples/dashboards`](examples/dashboards) for Grafana dashboards.

## Configuration

Every flag has a `GARMIN_*` env var equivalent (e.g. `--cache.ttl` → `GARMIN_CACHE_TTL`). Prefer env vars for credentials, since flags are visible in `ps`. Run `--help` for the full list.

| Flag                   | Default             | Purpose                                         |
| ---------------------- | ------------------- | ----------------------------------------------- |
| `--username`           | *(required)*        | Garmin Connect email                            |
| `--password`           | *(required)*        | Garmin Connect password                         |
| `--token-file`         | `garmin_token.json` | Cached OAuth2 token                             |
| `--cache.ttl`          | `1h`                | Garmin API refresh interval                     |
| `--activity-limit`     | `30`                | Recent activities fetched per refresh           |
| `--log.level`          | `info`              | `debug`, `info`, `warn`, `error`                |
| `--web.listen-address` | `:10045`            | Listen address                                  |
| `--web.telemetry-path` | `/metrics`          | Prometheus endpoint                             |
| `--web.prometheus`     | `true`              | Set `false` for OTLP-push-only                  |
| `--web.health-path`    | `/healthz`          | Liveness probe                                  |
| `--web.ready-path`     | `/readyz`           | Readiness probe                                 |

### OpenTelemetry

Configured via the standard [OTel SDK env vars](https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/) (`OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`, …), or an `otelconf` YAML via `OTEL_CONFIG_FILE` ([example](examples/otelconf.yaml)). Unlike the spec, `OTEL_METRICS_EXPORTER`, `OTEL_TRACES_EXPORTER`, and `OTEL_LOGS_EXPORTER` default to `none`, so nothing is pushed until you opt in.

## Metrics

All metrics are gauges named `garmin.<area>.<name>` (`garmin_<area>_<name>` in Prometheus), with the unit in the name. `garmin.collector.up{collector}` reports per-collector health, `garmin.cache.last_refresh_timestamp_seconds` when the Garmin data was last fetched.

| Area | Metrics |
| ------------------- | --- |
| `activity` | `recent`, `recent_duration_seconds`, `recent_distance_meters`, `recent_kilocalories`, `last_timestamp_seconds` (by `type`); `lifetime` |
| `blood_pressure` | `systolic_mmhg`, `diastolic_mmhg`, `pulse_bpm` |
| `body` | `weight_grams`, `bmi`, `fat_percent`, `water_percent`, `bone_mass_grams`, `muscle_mass_grams`, `visceral_fat`, `metabolic_age_years`, `last_weigh_in_timestamp_seconds` |
| `body_composition` | Same as `body`, averaged (`*_avg`) |
| `cycling` | `ftp_watts` |
| `device` | `registered`, `info` (`device_id`, `name`, `status`) |
| `fitness_age` | `years`, `chronological_years`, `achievable_years`, `previous_years`, `component_value`, `component_potential_years` (by `component`) |
| `gear` | `max_meters`, `active`, `total_distance_meters`, `total_activities` (by `gear_name`, `gear_type`) |
| `goals` | `active`, `earned_badges` |
| `golf` | `last_round_score`, `last_round_to_par` |
| `heart_rate` | `resting_bpm`, `min_bpm`, `max_bpm`, `seven_day_avg_resting_bpm` |
| `hydration` | `intake_ml`, `goal_ml`, `daily_avg_ml`, `sweat_loss_ml`, `activity_intake_ml` |
| `intensity` | `weekly_goal_minutes`, `weekly_moderate_minutes`, `weekly_vigorous_minutes`, `weekly_total_minutes` |
| `lactate_threshold` | `running_speed_mps`, `running_heart_rate_bpm`, `cycling_heart_rate_bpm` |
| `personal_records` | `value` (`pr_type`) |
| `respiration` | `avg_waking_bpm`, `highest_bpm`, `lowest_bpm` |
| `running_tolerance` | `score`, `level` |
| `sleep` | Stages (`total`/`deep`/`light`/`rem`/`awake`/`nap_seconds`), `score` (by `component`), `need_baseline_seconds`, `need_actual_seconds`, respiration, SpO2, HRV (last night, weekly, baseline), `avg_heart_rate_bpm`, `avg_stress`, `body_battery_change`, `awakenings`, `restless_moments` |
| `spo2` | `avg_percent`, `lowest_percent`, `seven_day_avg_percent`, `latest_percent` |
| `stress` | `avg_level`, `max_level` |
| `training` | `readiness_score`, `readiness_recovery_seconds`, `readiness_factor_percent` (by `factor`), `vo2max_generic`, `vo2max_cycling`, race predictions (5k, 10k, half, full), `endurance_score`, `hill_score` |
| `training_status` | `code`, `info` (`status`), `weekly_training_load`, `acwr_percent`, `acwr_ratio`, monthly loads (aerobic low/high, anaerobic), `paused` |
| `wellness` | Steps, distance, calories, activity time, floors, body battery, stress durations and `stress_time_percent` (by `level`), intensity minutes |

Exact names are in [`collector/`](collector/), or just `curl localhost:10045/metrics`.

## Development

```sh
make all       # fmt, vet, lint, build, test
make test      # go test -race ./...
make snapshot  # local goreleaser build
```

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[Apache-2.0](LICENSE)
