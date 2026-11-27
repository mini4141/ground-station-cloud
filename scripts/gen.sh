#!/usr/bin/env bash
# Regenerates all generated code: oapi-codegen (server+client), sqlc (DB layer), openapi-typescript (console types).
set -euo pipefail
source "$(dirname "$0")/env.sh"
go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
docker run --rm -v "$(pwd -W 2>/dev/null || pwd):/src" -w /src sqlc/sqlc:1.30.0 generate
(cd web/console && pnpm gen:api)
gofmt -l -w internal/api internal/db/sqlc
echo "generated code up to date"
