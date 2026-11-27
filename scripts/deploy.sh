#!/usr/bin/env bash
# Phase 2: deploys the stack to AWS from this PC (profile ground-station-cloud-deployer, ap-southeast-2).
#   1 build + push Lambda images   2 terraform apply   3 IoT certs -> .local/certs/aws   4 warm DB + test user
#   5 console to S3/CloudFront     6 stations via the custom provider   7 seed   8 local agents -> IoT Core
set -euo pipefail
source "$(dirname "$0")/aws-env.sh"
require_identity || { echo "AWS identity check failed" >&2; exit 1; }

TAG="$(date -u +%Y%m%d%H%M%S)"
ADOT_ARN="arn:aws:lambda:ap-southeast-2:901920570463:layer:aws-otel-collector-amd64-ver-0-117-0:1"

echo "== 1/8 ECR repositories + Lambda container images ($TAG)"
tf init -input=false >/dev/null
tf apply -auto-approve -input=false -target=aws_ecr_repository.svc -var "image_tag=$TAG" >/dev/null
ACCOUNT=$(aws sts get-caller-identity --query Account --output text)
REGISTRY="$ACCOUNT.dkr.ecr.$AWS_REGION.amazonaws.com"
aws ecr get-login-password | docker login --username AWS --password-stdin "$REGISTRY" >/dev/null

# ADOT collector layer, baked into the images (container-image Lambdas cannot attach layers)
if [ ! -d build/adot/extensions ]; then
  mkdir -p build/adot-dl build/adot
  URL=$(aws lambda get-layer-version-by-arn --arn "$ADOT_ARN" --query Content.Location --output text)
  curl -fsS -o build/adot-dl/layer.zip "$URL"
  (cd build/adot && (unzip -q -o ../adot-dl/layer.zip || tar -xf ../adot-dl/layer.zip))
fi
for svc in api ingest scheduler; do
  docker buildx build --platform linux/amd64 -q -f deploy/Dockerfile --build-arg CMD=$svc --build-context adot=build/adot \
    -t "$REGISTRY/gsc-$svc:$TAG" --provenance=false --push .
done

echo "== 2/8 terraform apply (VPC, Aurora, IoT Core, SQS, Lambda, API Gateway, Cognito, CloudFront, EventBridge)"
tf apply -auto-approve -input=false -var "image_tag=$TAG" | tee .local/logs/terraform-aws-apply.log | tail -5

API=$(tfo api_url); CONSOLE=$(tfo console_url); POOL=$(tfo user_pool_id); CLIENT=$(tfo user_pool_client_id)
COGDOM=$(tfo cognito_domain); IOT=$(tfo iot_endpoint); WEB=$(tfo web_bucket); CF=$(tfo cloudfront_id)

echo "== 3/8 IoT X.509 certificates -> $CERT_DIR (gitignored)"
mkdir -p "$CERT_DIR"
curl -fsS -o "$CERT_DIR/ca.crt" https://www.amazontrust.com/repository/AmazonRootCA1.pem
tfj station_certs | node -e '
const fs=require("fs");let d="";process.stdin.on("data",c=>d+=c).on("end",()=>{
  const m=JSON.parse(d);for(const [s,v] of Object.entries(m)){
    fs.writeFileSync(process.argv[1]+"/"+s+".crt",v.cert_pem);fs.writeFileSync(process.argv[1]+"/"+s+".key",v.private_key,{mode:0o644});}})' "$CERT_DIR"

echo "== 4/8 warm the API (runs the DB migrations through the Lambda) + Cognito operator"
for i in $(seq 1 12); do curl -fsS -m 70 "$API/healthz" >/dev/null && break || sleep 5; done
USER=demo
PASS="Gs-$(node -e 'console.log(require("crypto").randomBytes(9).toString("hex"))')1A"
aws cognito-idp admin-create-user --user-pool-id "$POOL" --username "$USER" --message-action SUPPRESS >/dev/null 2>&1 || true
aws cognito-idp admin-set-user-password --user-pool-id "$POOL" --username "$USER" --password "$PASS" --permanent
TOKEN=$(aws cognito-idp admin-initiate-auth --user-pool-id "$POOL" --client-id "$CLIENT" --auth-flow ADMIN_USER_PASSWORD_AUTH \
  --auth-parameters "USERNAME=$USER,PASSWORD=$PASS" --query AuthenticationResult.AccessToken --output text)
{
  echo "API_URL=$API"; echo "CONSOLE_URL=$CONSOLE"; echo "IOT_ENDPOINT=$IOT"; echo "COGNITO_USER=$USER"; echo "COGNITO_PASSWORD=$PASS"
  echo "USER_POOL_ID=$POOL"; echo "CLIENT_ID=$CLIENT"
} > "$CLOUD_ENV"
curl -fsS -m 70 -H "Authorization: Bearer $TOKEN" "$API/stations" >/dev/null # proves JWT authorizer -> Lambda -> Aurora

echo "== 5/8 console -> S3 + CloudFront"
(cd web/console && pnpm install --frozen-lockfile >/dev/null && pnpm build >/dev/null)
cat > web/console/dist/config.json <<EOF
{ "apiUrl": "$API", "authMode": "cognito", "cognitoDomain": "$COGDOM", "cognitoClientId": "$CLIENT" }
EOF
aws s3 sync web/console/dist "s3://$WEB" --delete --only-show-errors
aws cloudfront create-invalidation --distribution-id "$CF" --paths "/*" >/dev/null

echo "== 6/8 stations through the custom Terraform provider against the API Gateway URL"
TF_STATE_NAME=stations-aws scripts/tf-local.sh "$API" "$TOKEN" apply -auto-approve -no-color 2>&1 | tee .local/logs/terraform-provider-apply-aws.log | tail -8

echo "== 7/8 seed satellites + simulated clock"
go run ./cmd/seed -api "$API" -token "$TOKEN"

echo "== 8/8 local edge agents -> AWS IoT Core (mutual TLS, X.509 thing certificates)"
IOT_ENDPOINT="$IOT" docker compose -f local/docker-compose.aws.yml up -d --build

echo "deployed: console $CONSOLE  api $API"
echo "next: scripts/capture-proof.sh phase2   then   scripts/destroy.sh && scripts/verify-destroyed.sh"
