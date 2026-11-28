# PROGRESS

All phases complete (0 to 4). Phase 2 stack ran about 1.5 h and was destroyed; `proof/phase3-destroy/verify-destroyed.txt` shows nothing billable left.

Resume point: read this file first; continue at the first unticked item.

## Phases
- [x] Phase 0: scaffold
- [x] Phase 1: local build, tests green, CI green, proof index complete
- [x] Phase 2: AWS showcase + proof
- [x] Phase 3: destroy + verify-destroyed proof
- [x] Phase 4: docs/PORTFOLIO.md + README final

## Coverage matrix (SPECS §2)
- [x] Virtualise diverse ground station networks (stations API + TF provider + agents)
- [x] Fast Go backend (stations, satellites, passes, scheduler, ingest)
- [x] MQTT data path (commands, telemetry, mission data)
- [x] Service management UI (scheduling, SLA, billing)
- [x] Software talking to modems/rotators over TCP and serial
- [x] Efficiency/security/reliability tooling (TF provider, mTLS, IoT policies, CI drift checks)
- [x] Design/code review artefacts (PR template, docs/adr)
- [x] Linux/Unix, Go + TypeScript
- [x] AWS; unit, integration, acceptance, UI and performance (k6) tests
- [x] SQL + Git (sqlc/Postgres)
- [x] Go production, K8s + serverless (Lambda, API GW, DynamoDB, SQS), IoT Core, IaC, drivers, React/TS
- [x] Orbital mechanics (SGP4 visibility + az/el)
- [x] Dev env: oapi-codegen, sqlc, Lambda images, IoT mTLS, SQS bus, Aurora/PG, DynamoDB, S3, Cognito, CloudFront, EventBridge, custom TF provider, Prometheus, OpenTelemetry

## Features (SPECS §5)
- [x] F1 stations + Terraform provider
- [x] F2 satellites + pass prediction
- [x] F3 booking + scheduling
- [x] F4 edge agent pass execution
- [x] F5 telemetry ingest + live console
- [x] F6 SLA + billing
- [x] F7 operator auth

## Phase 1 proof (SPECS §9)
- [x] tests/: go unit/integration JUnit, provider acceptance log, agent e2e log, Playwright report, k6 summary
- [x] screenshots 01-stations-map, 02-passes-timeline, 03-booking, 04-live-telemetry, 05-sla, 06-billing, 07-jaeger-trace
- [x] logs: terraform-provider-apply.log, mqtt-sub-all.log, agent-pass-tokyo.log, kind-get-all.txt

## Phase 2 proof (SPECS §10)
- [x] screenshots 01-cf-passes, 02-cf-live-telemetry, 03-cf-sla, 04-cf-billing
- [x] logs: iot-describe-endpoint, iot-mqtt-test-client, sqs-ingest-lambda, eventbridge-schedule, s3-mission-object, xray-trace-summary, resources.txt

## Phase 3 (SPECS §11)
- [x] destroy.sh run, verify-destroyed.txt shows nothing billable
