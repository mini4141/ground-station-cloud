# Sourced by the cloud scripts: pins the AWS profile/region (never the default profile) and loads terraform outputs.
source "$(dirname "${BASH_SOURCE[0]}")/env.sh"
export AWS_PROFILE=ground-station-cloud-deployer
export AWS_REGION=ap-southeast-2
export AWS_DEFAULT_REGION=ap-southeast-2
unset AWS_ENDPOINT_URL_SQS AWS_ENDPOINT_URL_DYNAMODB AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY # local-stack settings from .env.example must not leak
export AWS_PAGER=""
TF_AWS_DIR="$ROOT/infra/aws"
CERT_DIR="$ROOT/.local/certs/aws"
CLOUD_ENV="$ROOT/.local/cloud.env"

tf() { terraform -chdir="$TF_AWS_DIR" "$@"; }
# tfo <output name>: raw terraform output value
tfo() { tf output -raw "$1"; }
# tfj <output name>: json terraform output
tfj() { tf output -json "$1"; }

# node-based json field extraction (no jq dependency): jget '<json>' 'a.b.c'
jget() { node -e 'let d="";process.stdin.on("data",c=>d+=c).on("end",()=>{let v=JSON.parse(d);for(const k of process.argv[1].split("."))v=v?.[k];console.log(v??"")})' "$2" <<<"$1"; }

require_identity() {
  aws sts get-caller-identity --output text --query Arn >/dev/null
}
