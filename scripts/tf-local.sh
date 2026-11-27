#!/usr/bin/env bash
# Runs terraform in infra/stations with the custom provider from .local/bin via dev_overrides.
# Usage: scripts/tf-local.sh <endpoint> <token> <terraform args...>   (state: .local/tfstate/$TF_STATE_NAME.tfstate)
set -euo pipefail
source "$(dirname "$0")/env.sh"
endpoint=$1; token=$2; shift 2
go build -o ".local/bin/terraform-provider-groundstation$EXE" ./provider
mkdir -p .local/tfstate
WINBIN="$(cd .local/bin && (pwd -W 2>/dev/null || pwd))"
cat > .local/terraformrc <<RC
provider_installation {
  dev_overrides {
    "registry.terraform.io/mini4141/groundstation" = "$WINBIN"
  }
  direct {}
}
RC
export TF_CLI_CONFIG_FILE="$(cd .local && (pwd -W 2>/dev/null || pwd))/terraformrc"
export TF_IN_AUTOMATION=1
cmd=$1; shift
cd infra/stations
exec terraform "$cmd" -state="../../.local/tfstate/${TF_STATE_NAME:-stations-local}.tfstate" -var "endpoint=$endpoint" -var "token=$token" "$@"
