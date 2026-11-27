#!/usr/bin/env bash
# Stops the local stack: Go services, console dev server, agents and compose infra.
source "$(dirname "$0")/env.sh"
for n in web api ingest scheduler devoidc; do stop_bg "$n"; done
docker compose -f local/docker-compose.yml --profile agents down -v --remove-orphans
echo "local stack stopped"
