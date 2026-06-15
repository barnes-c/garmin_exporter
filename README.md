# Garmin Exporter

[![GitHub Release](https://img.shields.io/github/v/release/barnes-c/garmin_exporter)](https://github.com/barnes-c/garmin_exporter/releases/latest)
[![Build Status](https://github.com/barnes-c/garmin_exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/barnes-c/garmin_exporter/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/barnes-c/garmin_exporter)](https://goreportcard.com/report/github.com/barnes-c/garmin_exporter)
[![GHCR](https://img.shields.io/badge/ghcr.io-barnes--c%2Fgarmin__exporter-blue?logo=github)](https://github.com/barnes-c/garmin_exporter/pkgs/container/garmin_exporter)
[![Docker Hub](https://img.shields.io/docker/pulls/barnesbiz/garmin_exporter?logo=docker)](https://hub.docker.com/r/barnesbiz/garmin_exporter)

OTel-native Prometheus exporter for [Garmin Connect](https://connect.garmin.com/).

Listens on port **10045** by default. Authenticates with Garmin Connect via username and password (with MFA support), caches the OAuth2 token to disk, and refreshes it automatically. Exposes metrics at `/metrics`, and can additionally push metrics, traces, and logs to an OTLP endpoint.

If this project is useful to you, a star on the repo would be appreciated.

## Quick start

### Docker

```shell
  docker run -d -p 10045:10045 \
    -v garmin_data:/data \
    -e GARMIN_USERNAME=you@example.com \
    -e GARMIN_PASSWORD=yourpassword \
    ghcr.io/barnes-c/garmin_exporter:latest \
    --garmin.token-file=/data/garmin_token.json
```

### Binary

```shell

  make build
  GARMIN_USERNAME=you@example.com GARMIN_PASSWORD=yourpassword
  ./garmin_exporter
```

Scrape: curl localhost:10045/metrics. Probes: /healthz, /readyz.

**First login**: if your account has MFA enabled, the exporter prompts for a one-time code on stdin. Run it interactively once (add stdin_open: true and tty: true in Compose); once the token is cached to `--garmin.token-file`, subsequent restarts are unattended. Prefer the `GARMIN_USERNAME` / `GARMIN_PASSWORD` environment variables over their flags. Flag values are visible to other users on the host via `ps`.

### Examples

Checkout the examples in [`examples/`](./examples):

- [`examples/prometheus`](./examples/prometheus) — classic scrape with Prometheus + Grafana.
- [`examples/otlp`](./examples/otlp) — OTel-first setup with Prometheus, Tempo, and Loki.
- [`examples/otelconf.yaml`](./examples/otelconf.yaml) — examplary OTel SDK config consumed via `OTEL_CONFIG_FILE`.

## Configuration

Key flags (full list via `--help`):

|           Flag            |            Env var             |       Default       |                           Purpose                           |
| ------------------------- | ------------------------------ | ------------------- | ----------------------------------------------------------- |
| `--cache.ttl`             | `GARMIN_CACHE_TTL`             | `1h`                | Garmin API refresh interval; independent of scrape interval |
| `--garmin.activity-limit` | `GARMIN_ACTIVITY_LIMIT`        | `30`                | Number of recent activities to fetch per refresh            |
| `--garmin.password`       | `GARMIN_PASSWORD`              | *(required)*        | Garmin Connect password                                     |
| `--garmin.token-file`     | `GARMIN_TOKEN_FILE`            | `garmin_token.json` | Path to the cached OAuth2 token file                        |
| `--garmin.username`       | `GARMIN_USERNAME`              | *(required)*        | Garmin Connect email address                                |
| `--log.level`             | `GARMIN_LOG_LEVEL`             | `info`              | Log level (`debug`, `info`, `warn`, `error`)                |
| `--web.health-path`       | `GARMIN_WEB_HEALTH_PATH`       | `/healthz`          | Liveness probe path                                         |
| `--web.listen-address`    | `GARMIN_WEB_LISTEN_ADDRESS`    | `:10045`            | Listen address                                              |
| `--web.prometheus`        | `GARMIN_WEB_PROMETHEUS`        | `true`              | Disable for OTLP-push-only deployments                      |
| `--web.ready-path`        | `GARMIN_WEB_READY_PATH`        | `/readyz`           | Readiness probe path                                        |
| `--web.telemetry-path`    | `GARMIN_WEB_TELEMETRY_PATH`    | `/metrics`          | Prometheus endpoint                                         |

### OTel pipeline

The OTel pipeline is entirely environment-driven; see the [OTel SDK env var spec](https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/) for the full list. `/metrics` is always served unless `--web.prometheus=false`.

|           Variable            |                                  Purpose                                   |
| ----------------------------- | -------------------------------------------------------------------------- |
| `OTEL_METRICS_EXPORTER`       | Comma-separated push exporters: `otlp`, `console`, `none` (default `none`) |
| `OTEL_TRACES_EXPORTER`        | Traces exporter: `otlp`, `console`, `none` (default `none`)                |
| `OTEL_LOGS_EXPORTER`          | Logs exporter: `otlp`, `console`, `none` (default `none`)                  |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Collector endpoint, e.g. `localhost:4317`                                  |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `grpc` or `http/protobuf`                                                  |
| `OTEL_METRIC_EXPORT_INTERVAL` | OTLP metric push interval, in ms                                           |
| `OTEL_TRACES_SAMPLER`         | e.g. `parentbased_traceidratio`                                            |
| `OTEL_TRACES_SAMPLER_ARG`     | Sampler argument (e.g. `0.1`)                                              |
| `OTEL_SERVICE_NAME`           | Resource `service.name` (default `garmin_exporter`)                        |
| `OTEL_CONFIG_FILE`            | Path to `otelconf` YAML; overrides all other `OTEL_*` vars                 |

The three exporter selectors default to `none` instead of the spec default `otlp`. The OTel exporters stays silent until OTLP is opted in.

## Metrics

All instruments are observable gauges. Naming follows `garmin.<area>.<name>`. Attributes are noted in parentheses.

|                           Metric                            |     Collector      |
| ----------------------------------------------------------- | ------------------ |
| `garmin.activity.count` (`type`)                            | `activities`       |
| `garmin.activity.duration_seconds_total` (`type`)           | `activities`       |
| `garmin.activity.distance_meters_total` (`type`)            | `activities`       |
| `garmin.activity.calories_total` (`type`)                   | `activities`       |
| `garmin.activity.last_timestamp_seconds` (`type`)           | `activities`       |
| `garmin.activity.lifetime_count`                            | `activities`       |
| `garmin.blood_pressure.systolic_mmhg`                       | `blood_pressure`   |
| `garmin.blood_pressure.diastolic_mmhg`                      | `blood_pressure`   |
| `garmin.blood_pressure.pulse_bpm`                           | `blood_pressure`   |
| `garmin.body.weight_grams`                                  | `body`             |
| `garmin.body.bmi`                                           | `body`             |
| `garmin.body.fat_percent`                                   | `body`             |
| `garmin.body.water_percent`                                 | `body`             |
| `garmin.body.bone_mass_grams`                               | `body`             |
| `garmin.body.muscle_mass_grams`                             | `body`             |
| `garmin.body.visceral_fat`                                  | `body`             |
| `garmin.body.metabolic_age_years`                           | `body`             |
| `garmin.body_composition.weight_grams_avg`                  | `body_composition` |
| `garmin.body_composition.bmi_avg`                           | `body_composition` |
| `garmin.body_composition.fat_percent_avg`                   | `body_composition` |
| `garmin.body_composition.water_percent_avg`                 | `body_composition` |
| `garmin.body_composition.bone_mass_grams_avg`               | `body_composition` |
| `garmin.body_composition.muscle_mass_grams_avg`             | `body_composition` |
| `garmin.body_composition.visceral_fat_avg`                  | `body_composition` |
| `garmin.body_composition.metabolic_age_years_avg`           | `body_composition` |
| `garmin.cycling.ftp_watts`                                  | `cycling`          |
| `garmin.device.count`                                       | `devices`          |
| `garmin.device.info` (`device_id`, `name`, `status`)        | `devices`          |
| `garmin.gear.max_meters` (`gear_name`, `gear_type`)         | `gear`             |
| `garmin.gear.notified_at_meters` (`gear_name`, `gear_type`) | `gear`             |
| `garmin.gear.active` (`gear_name`, `gear_type`)             | `gear`             |
| `garmin.goals.active_total`                                 | `goals`            |
| `garmin.goals.earned_badges_total`                          | `goals`            |
| `garmin.golf.last_round_score`                              | `golf`             |
| `garmin.golf.last_round_to_par`                             | `golf`             |
| `garmin.heartrate.resting_bpm`                              | `heartrate`        |
| `garmin.heartrate.min_bpm`                                  | `heartrate`        |
| `garmin.heartrate.max_bpm`                                  | `heartrate`        |
| `garmin.heartrate.seven_day_avg_resting_bpm`                | `heartrate`        |
| `garmin.hydration.intake_ml`                                | `hydration`        |
| `garmin.hydration.goal_ml`                                  | `hydration`        |
| `garmin.hydration.daily_avg_ml`                             | `hydration`        |
| `garmin.hydration.sweat_loss_ml`                            | `hydration`        |
| `garmin.hydration.activity_intake_ml`                       | `hydration`        |
| `garmin.intensity.weekly_goal_minutes`                      | `intensity`        |
| `garmin.intensity.moderate_minutes_total`                   | `intensity`        |
| `garmin.intensity.vigorous_minutes_total`                   | `intensity`        |
| `garmin.lactatethreshold.running_speed_mps`                 | `lactatethreshold` |
| `garmin.lactatethreshold.running_heart_rate_bpm`            | `lactatethreshold` |
| `garmin.lactatethreshold.cycling_heart_rate_bpm`            | `lactatethreshold` |
| `garmin.personalrecords.value` (`pr_type`, `name`)          | `personalrecords`  |
| `garmin.respiration.avg_waking_bpm`                         | `respiration`      |
| `garmin.respiration.highest_bpm`                            | `respiration`      |
| `garmin.respiration.lowest_bpm`                             | `respiration`      |
| `garmin.runningtolerance.score`                             | `runningtolerance` |
| `garmin.runningtolerance.level`                             | `runningtolerance` |
| `garmin.sleep.total_seconds`                                | `sleep`            |
| `garmin.sleep.deep_seconds`                                 | `sleep`            |
| `garmin.sleep.light_seconds`                                | `sleep`            |
| `garmin.sleep.rem_seconds`                                  | `sleep`            |
| `garmin.sleep.awake_seconds`                                | `sleep`            |
| `garmin.sleep.nap_seconds`                                  | `sleep`            |
| `garmin.sleep.restless_moments_total`                       | `sleep`            |
| `garmin.sleep.avg_respiration_bpm`                          | `sleep`            |
| `garmin.sleep.highest_respiration_bpm`                      | `sleep`            |
| `garmin.sleep.lowest_respiration_bpm`                       | `sleep`            |
| `garmin.sleep.avg_stress`                                   | `sleep`            |
| `garmin.sleep.hrv_last_night_ms`                            | `sleep`            |
| `garmin.sleep.hrv_weekly_avg_ms`                            | `sleep`            |
| `garmin.sleep.hrv_baseline_low_upper_ms`                    | `sleep`            |
| `garmin.sleep.hrv_baseline_balanced_low_ms`                 | `sleep`            |
| `garmin.sleep.hrv_baseline_balanced_upper_ms`               | `sleep`            |
| `garmin.spo2.avg_percent`                                   | `spo2`             |
| `garmin.spo2.lowest_percent`                                | `spo2`             |
| `garmin.spo2.seven_day_avg_percent`                         | `spo2`             |
| `garmin.stress.avg_level`                                   | `stress`           |
| `garmin.stress.max_level`                                   | `stress`           |
| `garmin.training.readiness_score`                           | `training`         |
| `garmin.training.readiness_sleep_score`                     | `training`         |
| `garmin.training.readiness_recovery_minutes`                | `training`         |
| `garmin.training.readiness_hrv_weekly_avg_ms`               | `training`         |
| `garmin.training.vo2max_generic`                            | `training`         |
| `garmin.training.vo2max_cycling`                            | `training`         |
| `garmin.training.fitness_age_years`                         | `training`         |
| `garmin.training.race_prediction_5k_seconds`                | `training`         |
| `garmin.training.race_prediction_10k_seconds`               | `training`         |
| `garmin.training.race_prediction_half_marathon_seconds`     | `training`         |
| `garmin.training.race_prediction_marathon_seconds`          | `training`         |
| `garmin.training.endurance_score`                           | `training`         |
| `garmin.training.hill_score`                                | `training`         |
| `garmin.trainingstatus.status`                              | `trainingstatus`   |
| `garmin.trainingstatus.weekly_training_load`                | `trainingstatus`   |
| `garmin.trainingstatus.acwr_percent`                        | `trainingstatus`   |
| `garmin.trainingstatus.acwr_ratio`                          | `trainingstatus`   |
| `garmin.trainingstatus.aerobic_low_monthly_load`            | `trainingstatus`   |
| `garmin.trainingstatus.aerobic_high_monthly_load`           | `trainingstatus`   |
| `garmin.trainingstatus.anaerobic_monthly_load`              | `trainingstatus`   |
| `garmin.wellness.total_steps`                               | `wellness`         |
| `garmin.wellness.step_goal`                                 | `wellness`         |
| `garmin.wellness.total_distance_meters`                     | `wellness`         |
| `garmin.wellness.total_kilocalories`                        | `wellness`         |
| `garmin.wellness.active_kilocalories`                       | `wellness`         |
| `garmin.wellness.bmr_kilocalories`                          | `wellness`         |
| `garmin.wellness.active_seconds`                            | `wellness`         |
| `garmin.wellness.highly_active_seconds`                     | `wellness`         |
| `garmin.wellness.sedentary_seconds`                         | `wellness`         |
| `garmin.wellness.sleeping_seconds`                          | `wellness`         |
| `garmin.wellness.floors_ascended`                           | `wellness`         |
| `garmin.wellness.floors_descended`                          | `wellness`         |
| `garmin.wellness.floors_ascended_goal`                      | `wellness`         |
| `garmin.wellness.resting_heart_rate_bpm`                    | `wellness`         |
| `garmin.wellness.min_heart_rate_bpm`                        | `wellness`         |
| `garmin.wellness.max_heart_rate_bpm`                        | `wellness`         |
| `garmin.wellness.body_battery_latest`                       | `wellness`         |
| `garmin.wellness.body_battery_highest`                      | `wellness`         |
| `garmin.wellness.body_battery_lowest`                       | `wellness`         |
| `garmin.wellness.body_battery_charged`                      | `wellness`         |
| `garmin.wellness.body_battery_drained`                      | `wellness`         |
| `garmin.wellness.avg_waking_respiration_bpm`                | `wellness`         |
| `garmin.wellness.highest_respiration_bpm`                   | `wellness`         |
| `garmin.wellness.lowest_respiration_bpm`                    | `wellness`         |
| `garmin.wellness.avg_stress_duration_seconds`               | `wellness`         |
| `garmin.wellness.rest_stress_duration_seconds`              | `wellness`         |
| `garmin.wellness.low_stress_duration_seconds`               | `wellness`         |
| `garmin.wellness.high_stress_duration_seconds`              | `wellness`         |
| `garmin.wellness.moderate_intensity_minutes`                | `wellness`         |
| `garmin.wellness.vigorous_intensity_minutes`                | `wellness`         |
| `garmin.collector.up` (`collector`)                         | —                  |

See [`collector/`](collector/) for per-collector implementation.

## Development

```sh
make all       # fmt, vet, lint, build, test
make test      # go test -race ./...
make snapshot  # local goreleaser build
```

## License

[Apache-2.0](LICENSE)
