#!/usr/bin/env bash
# Phase 3: tears everything down. Idempotent and safe to re-run: it first removes what would block the Terraform destroy
# (agents, stations, EventBridge schedules, bucket contents, ECR images, IoT certificate attachments), then destroys the stack.
set -uo pipefail
source "$(dirname "$0")/aws-env.sh"
require_identity || { echo "AWS identity check failed" >&2; exit 1; }
step() { echo; echo "== $*"; }

step "1/6 stop local edge agents"
IOT_ENDPOINT=unused docker compose -f local/docker-compose.aws.yml down -v --remove-orphans 2>&1 | tail -3 || true
rm -rf "$CERT_DIR"

if [ ! -f "$TF_AWS_DIR/terraform.tfstate" ] || [ "$(tf state list 2>/dev/null | wc -l)" = "0" ]; then
  echo "no terraform state with resources: nothing to destroy via terraform"
else
  API=$(tfo api_url 2>/dev/null || true)
  POOL=$(tfo user_pool_id 2>/dev/null || true)
  CLIENT=$(tfo user_pool_client_id 2>/dev/null || true)

  step "2/6 terraform destroy of the stations (custom provider)"
  if [ -f .local/tfstate/stations-aws.tfstate ] && [ -n "$API" ] && [ -f "$CLOUD_ENV" ]; then
    source "$CLOUD_ENV"
    TOKEN=$(aws cognito-idp admin-initiate-auth --user-pool-id "$POOL" --client-id "$CLIENT" --auth-flow ADMIN_USER_PASSWORD_AUTH \
      --auth-parameters "USERNAME=$COGNITO_USER,PASSWORD=$COGNITO_PASSWORD" --query AuthenticationResult.AccessToken --output text 2>/dev/null || true)
    [ -n "$TOKEN" ] && TF_STATE_NAME=stations-aws scripts/tf-local.sh "$API" "$TOKEN" destroy -auto-approve -no-color 2>&1 | tail -5 || echo "stations destroy skipped (API unreachable); they vanish with the database"
  fi

  step "3/6 EventBridge schedules, bucket contents, ECR images"
  GROUP=$(tfo schedule_group 2>/dev/null || true)
  if [ -n "$GROUP" ]; then
    for s in $(aws scheduler list-schedules --group-name "$GROUP" --query 'Schedules[].Name' --output text 2>/dev/null); do
      aws scheduler delete-schedule --group-name "$GROUP" --name "$s" && echo "deleted schedule $s"
    done
  fi
  for b in $(tfo mission_bucket 2>/dev/null) $(tfo web_bucket 2>/dev/null); do
    aws s3 rm "s3://$b" --recursive --only-show-errors 2>&1 | tail -1 || true
  done
  for r in api ingest scheduler; do
    ids=$(aws ecr list-images --repository-name "gsc-$r" --query 'imageIds[*]' --output json 2>/dev/null || echo "[]")
    [ "$ids" != "[]" ] && [ -n "$ids" ] && aws ecr batch-delete-image --repository-name "gsc-$r" --image-ids "$ids" >/dev/null && echo "deleted images of gsc-$r"
  done

  step "4/6 IoT: detach policies and things from the certificates, deactivate them"
  for cid in $(tfj iot_certificate_ids 2>/dev/null | node -e 'let d="";process.stdin.on("data",c=>d+=c).on("end",()=>{try{console.log(Object.values(JSON.parse(d)).join(" "))}catch(e){}})'); do
    arn="arn:aws:iot:$AWS_REGION:$(aws sts get-caller-identity --query Account --output text):cert/$cid"
    for p in $(aws iot list-attached-policies --target "$arn" --query 'policies[].policyName' --output text 2>/dev/null); do
      aws iot detach-policy --policy-name "$p" --target "$arn" && echo "detached policy $p"
    done
    for t in $(aws iot list-principal-things --principal "$arn" --query 'things[]' --output text 2>/dev/null); do
      aws iot detach-thing-principal --thing-name "$t" --principal "$arn" && echo "detached thing $t"
    done
    aws iot update-certificate --certificate-id "$cid" --new-status INACTIVE 2>/dev/null && echo "deactivated certificate ${cid:0:8}..."
  done

  step "5/6 terraform destroy (infra)"
  # Lambda leaves detached "available" network interfaces behind that can pin the subnet and security group for a long
  # time; delete them while terraform destroys (our own VPC only).
  VPC=$(aws ec2 describe-vpcs --filters Name=tag:Project,Values=ground-station-cloud --query 'Vpcs[0].VpcId' --output text 2>/dev/null)
  ( while kill -0 $$ 2>/dev/null; do
      if [ -n "$VPC" ] && [ "$VPC" != "None" ]; then
        for eni in $(aws ec2 describe-network-interfaces --filters Name=vpc-id,Values="$VPC" Name=status,Values=available --query "NetworkInterfaces[?starts_with(Description,'AWS Lambda VPC ENI')].NetworkInterfaceId" --output text 2>/dev/null); do
          aws ec2 delete-network-interface --network-interface-id "$eni" >/dev/null 2>&1 && echo "deleted detached Lambda ENI $eni"
        done
      fi
      sleep 20
    done ) &
  REAPER=$!
  tf destroy -auto-approve -input=false 2>&1 | tee .local/logs/terraform-aws-destroy.log | tail -15
  if [ "${PIPESTATUS[0]}" != "0" ]; then
    echo "destroy did not finish (network interfaces can lag): retrying once in 2 minutes"
    sleep 120
    tf destroy -auto-approve -input=false 2>&1 | tail -15
  fi
fi

step "6/6 local leftovers"
rm -f "$CLOUD_ENV" .local/tfstate/stations-aws.tfstate*
kill "${REAPER:-0}" 2>/dev/null
echo "destroy finished: run scripts/verify-destroyed.sh"
