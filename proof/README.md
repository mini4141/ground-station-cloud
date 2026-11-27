# Proof index

Everything here is produced by scripts (`scripts/test-all.sh`, `scripts/capture-proof.sh <phase>`), never by hand. Logs are redacted (`scripts/redact.sh`).

## Phase 1: local stack (Docker Compose + kind)

Screenshots (headless Playwright against the running console, real data flowing through the stack):

| File | Proves |
|---|---|
| [01-stations-map.png](phase1-local/screenshots/01-stations-map.png) | Three stations registered by Terraform through the custom provider, with live agent state |
| [02-passes-timeline.png](phase1-local/screenshots/02-passes-timeline.png) | SGP4 pass predictions per station on the Gantt timeline |
| [03-booking.png](phase1-local/screenshots/03-booking.png) | Booking a pass (selected pass, confirmation, booking row) |
| [04-live-telemetry.png](phase1-local/screenshots/04-live-telemetry.png) | Live lock, SNR, az/el during the pass, read from DynamoDB (local) once per second |
| [05-sla.png](phase1-local/screenshots/05-sla.png) | SLA per station and satellite updated after the pass |
| [06-billing.png](phase1-local/screenshots/06-billing.png) | Billing minutes x station rate |
| [07-jaeger-trace.png](phase1-local/screenshots/07-jaeger-trace.png) | OpenTelemetry trace of `GET /passes` with its SQL spans |

Tests (`phase1-local/tests/`):

| File | Proves |
|---|---|
| [go-unit.xml](phase1-local/tests/go-unit.xml), [go-unit.log](phase1-local/tests/go-unit.log) | SGP4 visibility window for a fixed TLE, scheduler conflict detection, GS-232 and modem protocol parsers |
| [go-integration.xml](phase1-local/tests/go-integration.xml) | API + Postgres: create station, compute passes, book, 409 on overlap, plan published |
| [provider-acceptance.log](phase1-local/tests/provider-acceptance.log) | `TF_ACC=1` acceptance test of `groundstation_station` (create, update, import) against the local API |
| [agent-e2e.log](phase1-local/tests/agent-e2e.log), [agent-e2e-mqtt-messages.log](phase1-local/tests/agent-e2e-mqtt-messages.log) | Simulated pass end to end: rotator moves, modem locks, telemetry over MQTT, mission file in storage |
| [playwright-report/](phase1-local/tests/playwright-report), [playwright-junit.xml](phase1-local/tests/playwright-junit.xml) | Console flow: book a pass, watch live telemetry, SLA and billing update |
| [k6-summary.txt](phase1-local/tests/k6-summary.txt), [k6-summary.json](phase1-local/tests/k6-summary.json) | `GET /passes` at 20 rps for 30 s, p95 under 500 ms |

Logs (`phase1-local/logs/`):

| File | Proves |
|---|---|
| [terraform-provider-apply.log](phase1-local/logs/terraform-provider-apply.log) | `terraform apply` creating 3 stations through the custom provider |
| [terraform-provider-plan-clean.log](phase1-local/logs/terraform-provider-plan-clean.log) | `terraform plan` afterwards reports no changes |
| [mqtt-sub-all.log](phase1-local/logs/mqtt-sub-all.log) | `mosquitto_sub -t 'gs/#'` over mTLS: heartbeats, plan, telemetry, events |
| [agent-pass-tokyo.log](phase1-local/logs/agent-pass-tokyo.log) | Tokyo agent: serial (GS-232) and TCP (modem) exchanges through the whole pass |
| [kind-get-all.txt](phase1-local/logs/kind-get-all.txt) | api, ingest and scheduler running on kind from the Helm chart |
| [api-service.log](phase1-local/logs/api-service.log), [ingest-service.log](phase1-local/logs/ingest-service.log), [scheduler-service.log](phase1-local/logs/scheduler-service.log) | Structured JSON service logs |
| [prometheus-metrics-api.txt](phase1-local/logs/prometheus-metrics-api.txt), [prometheus-query.json](phase1-local/logs/prometheus-query.json) | Prometheus metrics scraped from the services |
| [mission-objects.txt](phase1-local/logs/mission-objects.txt) | Mission files stored by the pass uploads |

## Phase 2: AWS

Filled in by `scripts/capture-proof.sh phase2`; see `phase2-cloud/`.

## Phase 3: destroy

`phase3-destroy/verify-destroyed.txt` is the output of `scripts/verify-destroyed.sh` after `scripts/destroy.sh`.
