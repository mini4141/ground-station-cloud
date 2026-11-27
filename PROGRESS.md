# PROGRESS

Resume point: read this file first; continue at the first unticked item.

## Phases
- [ ] Phase 0: scaffold
- [ ] Phase 1: local build, tests green, CI green, proof index complete
- [ ] Phase 2: AWS showcase + proof
- [ ] Phase 3: destroy + verify-destroyed proof
- [ ] Phase 4: docs/PORTFOLIO.md + README final

## Coverage matrix (SPECS §2)
- [ ] Virtualise diverse ground station networks (stations API + TF provider + agents)
- [ ] Fast Go backend (stations, satellites, passes, scheduler, ingest)
- [ ] MQTT data path (commands, telemetry, mission data)
- [ ] Service management UI (scheduling, SLA, billing)
- [ ] Software talking to modems/rotators over TCP and serial
- [ ] Efficiency/security/reliability tooling (TF provider, mTLS, IoT policies, CI drift checks)
- [ ] Design/code review artefacts (PR template, docs/adr)
- [ ] Linux/Unix, Go + TypeScript
- [ ] AWS; unit, integration, acceptance, UI and performance (k6) tests
- [ ] SQL + Git (sqlc/Postgres)
- [ ] Go production, K8s + serverless (Lambda, API GW, DynamoDB, SQS), IoT Core, IaC, drivers, React/TS
- [ ] Orbital mechanics (SGP4 visibility + az/el)
- [ ] Dev env: oapi-codegen, sqlc, Lambda images, IoT mTLS, SQS bus, Aurora/PG, DynamoDB, S3, Cognito, CloudFront, EventBridge, custom TF provider, Prometheus, OpenTelemetry

## Features (SPECS §5)
- [ ] F1 stations + Terraform provider
- [ ] F2 satellites + pass prediction
- [ ] F3 booking + scheduling
- [ ] F4 edge agent pass execution
- [ ] F5 telemetry ingest + live console
- [ ] F6 SLA + billing
- [ ] F7 operator auth

## Phase 1 proof (SPECS §9)
- [ ] tests/: go unit/integration JUnit, provider acceptance log, agent e2e log, Playwright report, k6 summary
- [ ] screenshots 01-stations-map, 02-passes-timeline, 03-booking, 04-live-telemetry, 05-sla, 06-billing, 07-jaeger-trace
- [ ] logs: terraform-provider-apply.log, mqtt-sub-all.log, agent-pass-tokyo.log, kind-get-all.txt

## Phase 2 proof (SPECS §10)
- [ ] screenshots 01-cf-passes, 02-cf-live-telemetry, 03-cf-sla, 04-cf-billing
- [ ] logs: iot-describe-endpoint, iot-mqtt-test-client, sqs-ingest-lambda, eventbridge-schedule, s3-mission-object, xray-trace-summary, resources.txt

## Phase 3 (SPECS §11)
- [ ] destroy.sh run, verify-destroyed.txt shows nothing billable
