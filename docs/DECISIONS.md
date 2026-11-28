# Decisions

One line each: date, decision, reason.

- 2026-11-28 Toolchain: Go 1.27.4 installed locally, go.mod pins `go 1.25`; node 24.12, pnpm 12.3, docker 29.7, kind 0.33, helm 4.3, terraform 1.17, aws-cli 2.42, openssl 3.5 (all present, nothing installed).
- 2026-11-28 sqlc runs from its official Docker image (`sqlc/sqlc`), because `go install` needs cgo (pg_query) and the host has no C compiler. CI uses the same image.
- 2026-11-28 oapi-codegen, goose, gotestsum are pinned as `go tool` dependencies in go.mod (no global installs).
- 2026-11-28 Station `name` (slug such as `tokyo`) is the MQTT topic key and IoT thing name; the UUID `id` is the API identity. Reason: IoT things/certs exist before the stations are registered through the provider.
- 2026-11-28 Simulated clock lives in a one-row Postgres table (`sim_clock`) exposed at `GET/PUT /sim/clock`, so API, scheduler and Lambdas share it. Plans carry `simNow`/`simScale` so agents derive the same clock.
- 2026-11-28 Local object storage is a folder behind HMAC-signed `PUT /uploads/...` URLs (presigned-URL equivalent); AWS uses S3 presigned PUT. Same `Storage` interface.
- 2026-11-28 Agent upload auth: plan carries an HMAC `uploadToken`; `POST /bookings/{id}/upload-url` is authorised by it (agents have no JWT). That route has no JWT authorizer in API Gateway.
- 2026-11-28 One Go binary per service runs as Lambda when `AWS_LAMBDA_RUNTIME_API` is set, otherwise as HTTP server / poller (same handlers for kind and local).
- 2026-11-28 Terraform station registrations live in `infra/stations/` (separate root module, reused for local API and AWS API) instead of a single `infra/stations.tf`, so it can target either endpoint.
- 2026-11-28 Provider lives in the main Go module (`provider/`) so it can import the generated client from `internal/`.
- 2026-11-28 go.mod directive is 1.26.0 (not 1.25): terraform-plugin-* and oapi-codegen releases require it; images use golang:1.26. Spec asks Go 1.25+, stack item unchanged.
- 2026-11-28 ADOT: Lambda container images cannot attach layers, so deploy.sh downloads the public ADOT collector layer (`aws-otel-collector-amd64-ver-0-117-0:1`) and bakes its files into the image via a named BuildKit context; traces reach X-Ray through the `xray` VPC endpoint.
- 2026-11-28 IoT data plane from Lambda: `com.amazonaws.<region>.iot.data` has no private DNS, so the Lambdas dial the interface endpoint's vpce DNS name while TLS and SigV4 keep the real ATS endpoint host (custom DialContext in `internal/iot`).
- 2026-11-28 CORS is answered by the API Lambda (CORS_ORIGIN = CloudFront URL), not by API Gateway: with a `$default` JWT route the gateway CORS config cannot answer preflights, and an unauthenticated `OPTIONS /{proxy+}` route bypasses the authorizer.
- 2026-11-28 IoT policies grant `iot:RetainPublish` on the station status topic (retained heartbeat and last-will); without it IoT Core drops the connection.
- 2026-11-28 EventBridge schedules keep `ActionAfterCompletion = NONE` so they can be captured as proof; destroy.sh deletes them before the schedule group is removed.
- 2026-11-28 Console hides frames unless the station state is `tracking`: DynamoDB keeps the last frame of the previous pass.
- 2026-11-28 CI runs on ubuntu-24.04 (Playwright does not support the newer ubuntu-latest image); scripts are committed executable via `git update-index --chmod=+x`.
- 2026-11-28 Embedded OpenAPI spec disabled in oapi-codegen output: the gzip blob differed between Windows (CRLF checkout) and Linux CI and tripped the drift check; the server does not use it.
- 2026-11-28 Cloud live chart is coarser than local: SQS event source mapping delivers telemetry in small batches, DynamoDB keeps only the latest frame, so the browser sees a new frame every ~3 s (all frames are still counted in Aurora for lock ratio).
- 2026-11-28 Aurora PostgreSQL 17.6 Serverless v2 (min 0 ACU with auto-pause, max 1) matches local PostgreSQL 17; Lambda environment variables carry the generated DB password (ephemeral stack, state local and gitignored).
