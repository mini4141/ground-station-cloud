#!/usr/bin/env bash
# Captures proof for a phase into proof/<phase>/ (screenshots via headless Playwright, service logs, CLI output).
#   scripts/capture-proof.sh phase1     requires the local stack (scripts/dev-up.sh)
#   scripts/capture-proof.sh phase2     requires the AWS stack (scripts/deploy.sh)
set -euo pipefail
source "$(dirname "$0")/env.sh"
PHASE=${1:?usage: capture-proof.sh phase1|phase2}
case "$PHASE" in
phase1)
  P=proof/phase1-local
  mkdir -p $P/screenshots $P/logs
  TOKEN=$(curl -fsS -X POST http://localhost:9000/token -d '{"operator":"demo"}' | sed -E 's/.*"access_token":"([^"]+)".*/\1/')
  docker rm -f gs-mqtt-sub >/dev/null 2>&1 || true
  # the equivalent of: mosquitto_sub -t 'gs/#' with the cloud client certificate (mTLS)
  docker run -d --name gs-mqtt-sub --network gs-local_default -v "$(pwd -W 2>/dev/null || pwd)/.local/certs:/certs:ro" eclipse-mosquitto:2.0.21 \
    mosquitto_sub -h mosquitto -p 8883 --cafile /certs/ca.crt --cert /certs/cloud.crt --key /certs/cloud.key -i proof-sub -t 'gs/#' -v -F '%I %t %p' >/dev/null
  sleep 2
  START=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  (cd web/console && SHOT_DIR="$(wp "$ROOT/$P/screenshots")" PW_REPORT="$(wp "$ROOT/$P/tests")/playwright-report" PW_JUNIT="$(wp "$ROOT/$P/tests")/playwright-junit.xml" pnpm exec playwright test)
  sleep 2
  docker logs gs-mqtt-sub 2>&1 | scripts/redact.sh > $P/logs/mqtt-sub-all.log
  docker rm -f gs-mqtt-sub >/dev/null
  docker logs --since "$START" gs-local-agent-tokyo-1 2>&1 | scripts/redact.sh > $P/logs/agent-pass-tokyo.log
  cp .local/logs/terraform-provider-apply.log $P/logs/terraform-provider-apply.log
  { echo "# terraform plan after apply (custom provider): expect 'No changes'"; scripts/tf-local.sh http://localhost:8080 "$TOKEN" plan -no-color -detailed-exitcode 2>&1 || true; } | scripts/redact.sh > $P/logs/terraform-provider-plan-clean.log
  for s in api ingest scheduler; do tail -200 .local/logs/$s.log | scripts/redact.sh > $P/logs/$s-service.log; done
  curl -fsS http://localhost:8080/metrics | grep -E '^gs_' | head -40 > $P/logs/prometheus-metrics-api.txt || true
  curl -fsS "http://localhost:9090/api/v1/query?query=gs_pass_plans_dispatched_total" > $P/logs/prometheus-query.json || true
  ls -1 $P/mission-objects 2>/dev/null || (ls -l .local/uploads/mission | scripts/redact.sh > $P/logs/mission-objects.txt)
  ;;
phase2)
  exec scripts/capture-proof-cloud.sh
  ;;
*) echo "unknown phase"; exit 1 ;;
esac
echo "proof captured for $PHASE"
