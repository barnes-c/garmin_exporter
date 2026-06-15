# Examples

Different deployment models that show ways to run `garmin_exporter`.

|              Example               |                                              What it shows                                              |
| ---------------------------------- | ------------------------------------------------------------------------------------------------------- |
| [`prometheus/`](./prometheus)      | Classic scrape: `garmin_exporter` + Prometheus + Grafana.                                               |
| [`otlp/`](./otlp)                  | OTel-first: `garmin_exporter` pushing OTLP to a Collector that fans out to Prometheus, Tempo, and Loki. |
| [`otelconf.yaml`](./otelconf.yaml) | File-based OTel SDK config consumed via `OTEL_CONFIG_FILE`.                                             |

## First-run MFA

If your Garmin Connect account has MFA enabled, the exporter prompts for a code on `stdin` the first time it logs in. `docker compose up -d` runs detached and can't service the prompt, so seed the token interactively once:

```shell
export GARMIN_USERNAME=you@example.com
export GARMIN_PASSWORD=yourpassword
docker compose run --rm garmin_exporter
```

Enter the MFA code when prompted, wait for the "Logged in" message, then `Ctrl+C`. The token is cached in the `garmin_data` volume, so subsequent starts are unattended:

```shell
docker compose up -d
```

Accounts without MFA can skip straight to `docker compose up -d`.
