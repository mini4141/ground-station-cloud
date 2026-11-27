#!/usr/bin/env bash
# Runs every local test layer and exits non-zero on failure. Reports land in $TEST_OUT (default proof/phase1-local/tests).
#   Go unit -> Go integration -> provider acceptance -> agent e2e -> Playwright -> k6 -> (KIND=1) kind/Helm
set -euo pipefail
source "$(dirname "$0")/env.sh"
OUT="${TEST_OUT:-$ROOT/proof/phase1-local/tests}"
SHOTS="${SHOT_DIR:-$ROOT/proof/phase1-local/screenshots}"
mkdir -p "$OUT" "$SHOTS"
rm -rf "$OUT"/*

echo "== format + vet"
test -z "$(gofmt -l .)" || { gofmt -l .; echo "gofmt needed"; exit 1; }
go vet ./... 2>&1 | tee "$OUT/go-vet.log"
go vet -tags 'integration e2e' ./... 

echo "== Go unit tests (SGP4 window, scheduler conflicts, protocol parsers)"
go tool gotestsum --junitfile "$(wp "$OUT")/go-unit.xml" --format testname -- -count=1 ./internal/... ./edge/... 2>&1 | tee "$OUT/go-unit.log"

if ! curl -fsS -m 3 http://localhost:8080/healthz >/dev/null 2>&1; then
  echo "== starting local stack"
  scripts/dev-up.sh
fi
TOKEN=$(curl -fsS -X POST http://localhost:9000/token -d '{"operator":"demo"}' | sed -E 's/.*"access_token":"([^"]+)".*/\1/')

echo "== Go integration (API + Postgres: create station -> passes -> book)"
go tool gotestsum --junitfile "$(wp "$OUT")/go-integration.xml" --format testname -- -tags integration -count=1 ./internal/server/ 2>&1 | tee "$OUT/go-integration.log"

echo "== Terraform provider acceptance (TF_ACC=1 against the local API)"
TF_ACC=1 GROUNDSTATION_ENDPOINT=http://localhost:8080 GROUNDSTATION_TOKEN="$TOKEN" \
  go tool gotestsum --junitfile "$(wp "$OUT")/provider-acceptance.xml" --format standard-verbose -- -count=1 ./provider/ 2>&1 | tee "$OUT/provider-acceptance.log"

echo "== Agent e2e (simulated pass: rotator, modem, MQTT telemetry, mission file)"
E2E_MQTT_LOG="$(wp "$OUT")/agent-e2e-mqtt-messages.log" UPLOAD_DIR=../.local/uploads MQTT_CA=../.local/certs/ca.crt MQTT_CERT=../.local/certs/cloud.crt MQTT_KEY=../.local/certs/cloud.key \
  go tool gotestsum --junitfile "$(wp "$OUT")/agent-e2e.xml" --format standard-verbose -- -tags e2e -count=1 -timeout 5m ./e2e/ 2>&1 | tee "$OUT/agent-e2e.log"

echo "== Playwright (book a pass -> live telemetry -> SLA + billing)"
(cd web/console && pnpm install --frozen-lockfile >/dev/null && pnpm lint && pnpm exec playwright install chromium >/dev/null 2>&1 || true)
(cd web/console && SHOT_DIR="$(wp "$SHOTS")" PW_REPORT="$(wp "$OUT")/playwright-report" PW_JUNIT="$(wp "$OUT")/playwright-junit.xml" pnpm exec playwright test 2>&1 | tee "$OUT/playwright.log")

echo "== k6 smoke: GET /passes 20 rps for 30 s, p95 < 500 ms"
SAT=$(curl -fsS -H "Authorization: Bearer $TOKEN" http://localhost:8080/satellites | sed -E 's/.*"id":"([^"]+)","name":"ISS.*/\1/')
mkdir -p .local/k6 && chmod 777 .local/k6
docker run --rm -e TOKEN="$TOKEN" -e SAT_ID="$SAT" -v "$(pwd -W 2>/dev/null || pwd)/tests/k6:/scripts:ro" -v "$(pwd -W 2>/dev/null || pwd)/.local/k6:/out" \
  --add-host host.docker.internal:host-gateway grafana/k6:1.2.3 run --summary-export=/out/summary.json /scripts/passes.js 2>&1 | tee "$OUT/k6-summary.txt"
cp .local/k6/summary.json "$OUT/k6-summary.json"

if [ "${KIND:-0}" = "1" ]; then
  echo "== kind + Helm"
  scripts/kind-up.sh && kubectl --context kind-gs-local wait --for=condition=available deployment --all --timeout=120s
  kubectl --context kind-gs-local port-forward svc/gs-api 18080:8080 >/dev/null 2>&1 &
  PF=$!; sleep 3
  curl -fsS http://localhost:18080/healthz | tee "$OUT/kind-api-healthz.json"
  kill $PF || true
  cp .local/logs/kind-get-all.txt "$ROOT/proof/phase1-local/logs/kind-get-all.txt"
  scripts/kind-down.sh
fi
echo "ALL TESTS PASSED"
