#!/usr/bin/env bash
# Captures Phase 2 proof from the deployed AWS stack into proof/phase2-cloud/ (called by capture-proof.sh phase2).
set -uo pipefail
source "$(dirname "$0")/aws-env.sh"
source "$CLOUD_ENV"
P=proof/phase2-cloud
mkdir -p $P/screenshots $P/logs $P/tests
R="scripts/redact.sh"
export DEPLOYER_IP
DEPLOYER_IP=$(curl -fsS -m 5 https://checkip.amazonaws.com 2>/dev/null | tr -d '[:space:]' || echo "__no_ip__")

TOKEN=$(aws cognito-idp admin-initiate-auth --user-pool-id "$USER_POOL_ID" --client-id "$CLIENT_ID" --auth-flow ADMIN_USER_PASSWORD_AUTH \
  --auth-parameters "USERNAME=$COGNITO_USER,PASSWORD=$COGNITO_PASSWORD" --query AuthenticationResult.AccessToken --output text)
START_MS=$(( $(date +%s) * 1000 ))
START_ISO=$(date -u +%Y-%m-%dT%H:%M:%SZ)
GROUP=$(tfo schedule_group); MISSION=$(tfo mission_bucket); TABLE=$(tfo live_table)

# poll EventBridge schedules while the pass runs: the booking creates a one-time schedule
( for i in $(seq 1 90); do
    n=$(aws scheduler list-schedules --group-name "$GROUP" --query 'Schedules[0].Name' --output text 2>/dev/null)
    if [ -n "$n" ] && [ "$n" != "None" ]; then aws scheduler get-schedule --group-name "$GROUP" --name "$n" | $R > $P/logs/eventbridge-schedule.json; break; fi
    sleep 2; done ) &
POLL=$!

echo "== Playwright against CloudFront: book -> live telemetry -> SLA + billing"
(cd web/console && PHASE=cloud BASE_URL="$CONSOLE_URL" API_URL="$API_URL" TOKEN="$TOKEN" SHOT_DIR="$(wp "$ROOT/$P/screenshots")" \
  PW_REPORT="$(wp "$ROOT/$P/tests")/playwright-report" PW_JUNIT="$(wp "$ROOT/$P/tests")/playwright-junit.xml" pnpm exec playwright test console.spec.ts 2>&1 | tee "$ROOT/$P/tests/playwright.log")
PW=${PIPESTATUS[0]}
wait $POLL 2>/dev/null
sleep 5

echo "== CLI evidence"
aws iot describe-endpoint --endpoint-type iot:Data-ATS | $R > $P/logs/iot-describe-endpoint.txt
aws iot list-things --query 'things[].{name:thingName,project:attributes.project}' --output json | $R >> $P/logs/iot-describe-endpoint.txt
for s in tokyo svalbard santiago; do aws iot get-policy --policy-name gsc-station-$s --query 'policyDocument' --output text 2>/dev/null | $R; done > $P/logs/iot-station-policies.json
{ echo "# agents (local Docker, X.509 thing certs) connecting and publishing to AWS IoT Core"; for s in tokyo svalbard santiago; do docker logs --since "$START_ISO" gs-aws-agents-agent-$s-1 2>&1 | grep -E 'mqtt connected|event|plan|uploaded' | head -40; done; } | $R > $P/logs/iot-mqtt-test-client.log
aws logs filter-log-events --log-group-name /aws/lambda/gsc-ingest --start-time $START_MS --query 'events[].message' --output text 2>/dev/null | grep -v '^$' | head -150 | $R > $P/logs/sqs-ingest-lambda.log
aws logs filter-log-events --log-group-name /aws/lambda/gsc-scheduler --start-time $START_MS --query 'events[].message' --output text 2>/dev/null | head -60 | $R > $P/logs/scheduler-lambda.log
aws logs filter-log-events --log-group-name /aws/lambda/gsc-api --start-time $START_MS --query 'events[].message' --output text 2>/dev/null | head -80 | $R > $P/logs/api-lambda.log
aws logs filter-log-events --log-group-name "/aws/events/gsc-domain-events" --start-time $START_MS --query 'events[].message' --output text 2>/dev/null | head -20 | $R > $P/logs/eventbridge-domain-events.log
[ -s $P/logs/eventbridge-schedule.json ] || aws scheduler list-schedules --group-name "$GROUP" --output json | $R > $P/logs/eventbridge-schedule.json
aws s3api list-objects-v2 --bucket "$MISSION" --prefix mission/ --output json | $R > $P/logs/s3-mission-object.json
KEY=$(aws s3api list-objects-v2 --bucket "$MISSION" --prefix mission/ --query 'Contents[0].Key' --output text)
[ "$KEY" != "None" ] && aws s3api head-object --bucket "$MISSION" --key "$KEY" --output json | $R >> $P/logs/s3-mission-object.json
aws dynamodb scan --table-name "$TABLE" --output json | $R > $P/logs/dynamodb-station-live.json
aws sqs get-queue-attributes --queue-url "$(tfo queue_url)" --attribute-names All --query Attributes --output json | $R > $P/logs/sqs-queue-attributes.json
aws xray get-trace-summaries --start-time "$START_ISO" --end-time "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --output json | $R > $P/logs/xray-trace-summary.json
aws rds describe-db-clusters --db-cluster-identifier "$(tfo db_cluster)" --query 'DBClusters[].{id:DBClusterIdentifier,engine:Engine,version:EngineVersion,mode:EngineMode,scaling:ServerlessV2ScalingConfiguration,status:Status,deletionProtection:DeletionProtection}' --output json | $R > $P/logs/aurora-cluster.json
aws lambda list-functions --query "Functions[?starts_with(FunctionName,'gsc-')].{name:FunctionName,package:PackageType,memory:MemorySize,vpc:VpcConfig.VpcId,tracing:TracingConfig.Mode}" --output json | $R > $P/logs/lambda-functions.json
aws ec2 describe-vpc-endpoints --filters Name=tag:Project,Values=ground-station-cloud --query 'VpcEndpoints[].{service:ServiceName,type:VpcEndpointType,state:State}' --output json | $R > $P/logs/vpc-endpoints.json
aws iot get-topic-rule --rule-name gsc_ingest --output json | $R > $P/logs/iot-topic-rule.json
aws cognito-idp describe-user-pool --user-pool-id "$USER_POOL_ID" --query 'UserPool.{id:Id,name:Name,status:Status}' --output json | $R > $P/logs/cognito-user-pool.json
{ echo "# terraform state (resources managed from infra/aws)"; tf state list; echo; echo "# outputs (non-sensitive)"; tf output | grep -v station_certs; } | $R > $P/resources.txt

# CloudWatch metrics rendered to PNG from the CLI (there is no console login)
WIDGET=$(cat <<EOF
{"metrics":[["AWS/Lambda","Invocations","FunctionName","gsc-api",{"stat":"Sum"}],["AWS/Lambda","Invocations","FunctionName","gsc-ingest",{"stat":"Sum"}],["AWS/Lambda","Invocations","FunctionName","gsc-scheduler",{"stat":"Sum"}]],"view":"timeSeries","title":"Lambda invocations during the demo pass","region":"$AWS_REGION","period":60,"start":"-PT30M","end":"P0D","width":900,"height":400}
EOF
)
aws cloudwatch get-metric-widget-image --metric-widget "$WIDGET" --output text --query MetricWidgetImage | base64 -d > $P/screenshots/05-cw-lambda-invocations.png 2>/dev/null || echo "metric widget not rendered"

find $P -type f \( -name '*.log' -o -name '*.json' -o -name '*.txt' \) -exec sh -c 'for f; do scripts/redact.sh < "$f" > "$f.tmp" && mv "$f.tmp" "$f"; done' _ {} +
echo "phase2 proof captured (playwright exit $PW)"
exit "$PW"
