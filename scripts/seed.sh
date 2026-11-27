#!/usr/bin/env bash
# Loads demo data: stations (Terraform + custom provider), satellites from bundled TLEs, simulated clock.
set -euo pipefail
source "$(dirname "$0")/env.sh"
TOKEN=$(curl -fsS -X POST http://localhost:9000/token -d '{"operator":"demo"}' | sed -E 's/.*"access_token":"([^"]+)".*/\1/')
mkdir -p proof/phase1-local/logs
scripts/tf-local.sh http://localhost:8080 "$TOKEN" apply -auto-approve -no-color 2>&1 | tee .local/logs/terraform-provider-apply.log
go run ./cmd/seed -api http://localhost:8080 -token "$TOKEN"
