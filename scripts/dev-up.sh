#!/usr/bin/env bash
# Starts the whole local stack: certs, compose infra, Go services, agents, stations (Terraform + custom provider), seed data, console.
set -euo pipefail
source "$(dirname "$0")/env.sh"

scripts/gen-certs.sh
docker compose -f local/docker-compose.yml up -d --wait postgres mosquitto elasticmq dynamodb jaeger prometheus

go build -o ".local/bin/" ./cmd/...
stop_bg devoidc; stop_bg api; stop_bg ingest; stop_bg scheduler
start_bg devoidc ".local/bin/devoidc$EXE"
wait_http http://localhost:9000/healthz 30
start_bg api ".local/bin/api$EXE"
wait_http http://localhost:8080/healthz 60
HTTP_ADDR= ADMIN_ADDR=:8091 start_bg ingest ".local/bin/ingest$EXE"
ADMIN_ADDR=:8092 start_bg scheduler ".local/bin/scheduler$EXE"
wait_http http://localhost:8091/healthz 60
wait_http http://localhost:8092/healthz 60

docker compose -f local/docker-compose.yml --profile agents up -d --build agent-tokyo agent-svalbard agent-santiago
scripts/seed.sh
if [ -d web/console/node_modules ] || [ "${WITH_WEB:-1}" = "1" ]; then
  (cd web/console && pnpm install --frozen-lockfile >/dev/null && cd ../.. && start_bg web pnpm --dir web/console dev --host 127.0.0.1 --port 5173)
  wait_http http://127.0.0.1:5173/ 60
fi
echo "stack up: console http://localhost:5173  api http://localhost:8080  jaeger http://localhost:16686  prometheus http://localhost:9090"
