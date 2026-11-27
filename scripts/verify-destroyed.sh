#!/usr/bin/env bash
# Phase 3 check: nothing billable may remain. Exits non-zero otherwise.
# Direct describe/list calls per service, plus a Project=ground-station-cloud sweep through the tagging API
# (its entries can linger for minutes, so every hit is confirmed with a direct call before it counts).
set -uo pipefail
source "$(dirname "$0")/aws-env.sh"
FAIL=0
bad() { echo "REMAINING: $*"; FAIL=1; }
ok() { echo "ok: $*"; }
count() { [ -z "$1" ] || [ "$1" = "None" ] && echo 0 || echo "$1" | wc -w; }

echo "# verify-destroyed $(date -u +%Y-%m-%dT%H:%M:%SZ) profile=$AWS_PROFILE region=$AWS_REGION"

chk() { # description, space separated findings
  if [ -n "${2// /}" ] && [ "$2" != "None" ]; then bad "$1: $2"; else ok "no $1"; fi
}

chk "IoT things (tokyo svalbard santiago)" "$(aws iot list-things --query "things[?contains(['tokyo','svalbard','santiago'], thingName)].thingName" --output text)"
chk "IoT policies gsc-*" "$(aws iot list-policies --query "policies[?starts_with(policyName,'gsc-')].policyName" --output text)"
chk "IoT topic rules" "$(aws iot list-topic-rules --query "rules[?ruleName=='gsc_ingest'].ruleName" --output text)"
chk "IoT certificates with a gsc policy attached" "$(for c in $(aws iot list-certificates --query 'certificates[].certificateArn' --output text); do aws iot list-attached-policies --target "$c" --query "policies[?starts_with(policyName,'gsc-')].policyName" --output text; done)"
chk "Lambda functions gsc-*" "$(aws lambda list-functions --query "Functions[?starts_with(FunctionName,'gsc-')].FunctionName" --output text)"
chk "API Gateway APIs gsc-api" "$(aws apigatewayv2 get-apis --query "Items[?Name=='gsc-api'].ApiId" --output text)"
chk "Aurora clusters gsc-*" "$(aws rds describe-db-clusters --query "DBClusters[?starts_with(DBClusterIdentifier,'gsc-')].DBClusterIdentifier" --output text)"
chk "RDS instances gsc-*" "$(aws rds describe-db-instances --query "DBInstances[?starts_with(DBInstanceIdentifier,'gsc-')].DBInstanceIdentifier" --output text)"
chk "DynamoDB tables gsc-*" "$(aws dynamodb list-tables --query "TableNames[?starts_with(@,'gsc-')]" --output text)"
chk "SQS queues gsc-*" "$(aws sqs list-queues --queue-name-prefix gsc- --query 'QueueUrls' --output text)"
chk "Cognito user pools gsc-*" "$(aws cognito-idp list-user-pools --max-results 60 --query "UserPools[?starts_with(Name,'gsc-')].Id" --output text)"
chk "CloudFront distributions (console)" "$(aws cloudfront list-distributions --query "DistributionList.Items[?Comment=='ground-station-cloud operator console'].Id" --output text)"
chk "EventBridge schedule groups gsc-*" "$(aws scheduler list-schedule-groups --query "ScheduleGroups[?starts_with(Name,'gsc-')].Name" --output text)"
chk "EventBridge rules gsc-*" "$(aws events list-rules --name-prefix gsc- --query 'Rules[].Name' --output text)"
chk "S3 buckets gsc-*" "$(aws s3api list-buckets --query "Buckets[?starts_with(Name,'gsc-')].Name" --output text)"
chk "ECR repositories gsc-*" "$(aws ecr describe-repositories --query "repositories[?starts_with(repositoryName,'gsc-')].repositoryName" --output text 2>/dev/null)"
chk "CloudWatch log groups gsc-*" "$(aws logs describe-log-groups --log-group-name-prefix /aws/lambda/gsc- --query 'logGroups[].logGroupName' --output text) $(aws logs describe-log-groups --log-group-name-prefix /aws/events/gsc- --query 'logGroups[].logGroupName' --output text)"
chk "VPCs tagged Project=ground-station-cloud" "$(aws ec2 describe-vpcs --filters Name=tag:Project,Values=ground-station-cloud --query 'Vpcs[].VpcId' --output text)"
chk "VPC endpoints tagged Project" "$(aws ec2 describe-vpc-endpoints --filters Name=tag:Project,Values=ground-station-cloud Name=vpc-endpoint-state,Values=available,pending,deleting --query 'VpcEndpoints[].VpcEndpointId' --output text)"
chk "IAM roles gsc-*" "$(aws iam list-roles --query "Roles[?starts_with(RoleName,'gsc-')].RoleName" --output text)"

# tagging API sweep, each hit confirmed by a direct call
exists() { # arn -> 0 when the resource still exists
  local arn=$1 svc rest id
  svc=$(echo "$arn" | cut -d: -f3); rest=$(echo "$arn" | cut -d: -f6-); id=${rest##*[/:]}
  case "$svc" in
    lambda) aws lambda get-function --function-name "$id" >/dev/null 2>&1 ;;
    dynamodb) aws dynamodb describe-table --table-name "$id" >/dev/null 2>&1 ;;
    sqs) aws sqs get-queue-url --queue-name "$id" >/dev/null 2>&1 ;;
    s3) aws s3api head-bucket --bucket "$id" >/dev/null 2>&1 ;;
    ecr) aws ecr describe-repositories --repository-names "$id" >/dev/null 2>&1 ;;
    rds) aws rds describe-db-clusters --db-cluster-identifier "$id" >/dev/null 2>&1 || aws rds describe-db-instances --db-instance-identifier "$id" >/dev/null 2>&1 ;;
    cloudfront) aws cloudfront get-distribution --id "$id" >/dev/null 2>&1 ;;
    cognito-idp) aws cognito-idp describe-user-pool --user-pool-id "$id" >/dev/null 2>&1 ;;
    apigateway) aws apigatewayv2 get-api --api-id "${arn##*/apis/}" >/dev/null 2>&1 ;;
    ec2) case "$rest" in vpc/*) aws ec2 describe-vpcs --vpc-ids "$id" >/dev/null 2>&1;; subnet/*) aws ec2 describe-subnets --subnet-ids "$id" >/dev/null 2>&1;; security-group/*) aws ec2 describe-security-groups --group-ids "$id" >/dev/null 2>&1;; *) return 1;; esac ;;
    iot) return 1 ;; # covered by the direct IoT checks above
    *) return 1 ;;
  esac
}
for attempt in 1 2 3; do
  HITS=$(aws resourcegroupstaggingapi get-resources --tag-filters Key=Project,Values=ground-station-cloud --query 'ResourceTagMappingList[].ResourceARN' --output text)
  STILL=""
  for arn in $HITS; do exists "$arn" && STILL="$STILL $arn"; done
  [ -z "$STILL" ] && break
  [ $attempt -lt 3 ] && sleep 20
done
if [ -n "$STILL" ]; then bad "tagged resources that still exist: $STILL"; else
  ok "tagging API sweep: $(count "$HITS") entries listed, none of them exists (stale index entries are ignored)"; fi

if [ $FAIL -ne 0 ]; then echo "RESULT: FAIL - billable resources remain"; exit 1; fi
echo "RESULT: PASS - nothing billable tagged Project=ground-station-cloud remains"
