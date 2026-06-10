# Probes

Package probes provides HTTP handlers for Kubernetes-style /healthz (liveness) and /readyz (readiness) probes.

Liveness is a process-level signal: an unhealthy liveness probe triggers a restart by the orchestrator, so it must not depend on external services that could legitimately be unavailable (a stale Garmin scrape should not restart the exporter container).

Readiness is a serving signal: an unhealthy readiness probe takes the instance out of rotation but does not restart it.
It aggregates a caller-supplied set of checks (Garmin auth state, scrape freshness, etc.) and returns 200 only when every check passes.
