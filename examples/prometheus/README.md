# Prometheus + Grafana

`garmin_exporter` scraped by Prometheus, with Grafana wired up as the UI.

## Stack

| Service             | Port   | Notes                                            |
| ------------------- | ------ | ------------------------------------------------ |
| `garmin_exporter`   | 10045  | `/metrics` Prometheus endpoint                   |
| `prometheus`        | 9090   | 30d retention, scrapes the exporter every 60s    |
| `grafana`           | 3000   | Prometheus pre-provisioned as the default source |

Grafana ships with anonymous viewer access and `admin`/`admin` as the editor login. Change both before exposing this stack outside localhost.

## Run

```shell
cp .env.example .env
# edit .env with your Garmin credentials

# First run (foreground, so you can answer the MFA prompt if needed):
docker compose run --rm garmin_exporter

# Once the token is cached, bring the full stack up:
docker compose up -d
```

Open <http://localhost:3000>. Build your own dashboard — none is bundled. Metric names follow `garmin_<area>_<name>` after the OTel→Prometheus transform (e.g. `garmin.body.weight_grams` → `garmin_body_weight_grams`). See the project [README](../../README.md#metrics) for the full list.

## Reset

```shell
docker compose down -v
```

This removes the cached Garmin token, Prometheus TSDB, and Grafana state.
