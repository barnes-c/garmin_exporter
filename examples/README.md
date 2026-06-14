# Examples

Different deployment models that show ways to run `garmin_exporter`.

|              Example               |                                              What it shows                                              |
| ---------------------------------- | ------------------------------------------------------------------------------------------------------- |
| [`prometheus/`](./prometheus)      | Classic scrape: `garmin_exporter` + Prometheus + Grafana.                                               |
| [`otlp/`](./otlp)                  | OTel-first: `garmin_exporter` pushing OTLP to a Collector that fans out to Prometheus, Tempo, and Loki. |
| [`otelconf.yaml`](./otelconf.yaml) | File-based OTel SDK config consumed via `OTEL_CONFIG_FILE`.                                             |

## First-run MFA

If your Garmin Connect account has MFA enabled, the exporter prompts for a code on `stdin` the first time it logs in. Each example's compose file runs the exporter with a TTY attached so you can complete the prompt; subsequent restarts read the cached token from the named volume and run unattended.

Run the exporter once in the foreground to seed the token:

```shell
docker compose run --rm garmin_exporter
```

Then bring the full stack up detached:

```shell
docker compose up -d
```
