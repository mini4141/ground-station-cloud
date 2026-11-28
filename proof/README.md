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

Captured by `scripts/capture-proof.sh phase2` against the deployed stack (CloudFront console, API Gateway + Lambda, IoT Core with real X.509 thing certificates, SQS, Aurora, DynamoDB, S3). Passes were driven by the same Playwright flow, with local Docker agents connected to IoT Core.

| File | Proves |
|---|---|
| [01-cf-passes.png](phase2-cloud/screenshots/01-cf-passes.png) | Console on CloudFront showing SGP4 pass predictions from the Lambda API (Cognito token, JWT authorizer, Aurora) |
| [02-cf-live-telemetry.png](phase2-cloud/screenshots/02-cf-live-telemetry.png) | Live lock, SNR and elevation during a pass: agent -> IoT Core -> SQS -> ingest Lambda -> DynamoDB -> API |
| [03-cf-sla.png](phase2-cloud/screenshots/03-cf-sla.png), [04-cf-billing.png](phase2-cloud/screenshots/04-cf-billing.png) | SLA and billing updated after the cloud pass |
| [05-cw-lambda-invocations.png](phase2-cloud/screenshots/05-cw-lambda-invocations.png) | CloudWatch metric graph rendered from the CLI |
| [tests/playwright-report](phase2-cloud/tests/playwright-report), [tests/playwright-junit.xml](phase2-cloud/tests/playwright-junit.xml) | Playwright run against the CloudFront URL |
| [resources.txt](phase2-cloud/resources.txt) | Terraform-managed AWS resources and outputs |
| [logs/iot-describe-endpoint.txt](phase2-cloud/logs/iot-describe-endpoint.txt), [logs/iot-station-policies.json](phase2-cloud/logs/iot-station-policies.json), [logs/iot-topic-rule.json](phase2-cloud/logs/iot-topic-rule.json) | IoT endpoint and things, per-station policies, topic rule to SQS |
| [logs/iot-mqtt-test-client.log](phase2-cloud/logs/iot-mqtt-test-client.log) | Agents connecting to IoT Core over mTLS and publishing events |
| [logs/sqs-ingest-lambda.log](phase2-cloud/logs/sqs-ingest-lambda.log), [logs/scheduler-lambda.log](phase2-cloud/logs/scheduler-lambda.log), [logs/api-lambda.log](phase2-cloud/logs/api-lambda.log) | CloudWatch logs of the three Go Lambdas |
| [logs/eventbridge-schedule.json](phase2-cloud/logs/eventbridge-schedule.json), [logs/eventbridge-domain-events.log](phase2-cloud/logs/eventbridge-domain-events.log) | One-time pass-reminder schedule created by a booking, and domain events on the bus |
| [logs/s3-mission-object.json](phase2-cloud/logs/s3-mission-object.json), [logs/dynamodb-station-live.json](phase2-cloud/logs/dynamodb-station-live.json), [logs/sqs-queue-attributes.json](phase2-cloud/logs/sqs-queue-attributes.json) | Mission objects in S3, latest telemetry in DynamoDB, queue state |
| [logs/xray-trace-summary.json](phase2-cloud/logs/xray-trace-summary.json) | X-Ray traces from the ADOT collector (API spans with SQL child spans) |
| [logs/aurora-cluster.json](phase2-cloud/logs/aurora-cluster.json), [logs/lambda-functions.json](phase2-cloud/logs/lambda-functions.json), [logs/vpc-endpoints.json](phase2-cloud/logs/vpc-endpoints.json), [logs/cognito-user-pool.json](phase2-cloud/logs/cognito-user-pool.json) | Aurora Serverless v2 scale-to-zero config, container-image Lambdas in the VPC, interface endpoints (no NAT), Cognito pool |

## Phase 3: destroy

[phase3-destroy/verify-destroyed.txt](phase3-destroy/verify-destroyed.txt) is the output of `scripts/verify-destroyed.sh` after `scripts/destroy.sh` (every service checked directly, plus the `Project=ground-station-cloud` tagging sweep). [terraform-destroy-summary.txt](phase3-destroy/terraform-destroy-summary.txt) is the Terraform destroy result.
