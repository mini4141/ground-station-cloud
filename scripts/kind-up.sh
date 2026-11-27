#!/usr/bin/env bash
# Builds the service images, creates a kind cluster and installs the Helm chart (Kubernetes packaging proof).
# Requires the local compose stack (scripts/dev-up.sh) for Postgres, Mosquitto, SQS and DynamoDB.
set -euo pipefail
source "$(dirname "$0")/env.sh"
CLUSTER=gs-local
for svc in api ingest scheduler; do
  docker build -q -f deploy/Dockerfile --build-arg CMD=$svc -t gs-$svc:dev .
done
kind get clusters | grep -qx "$CLUSTER" || kind create cluster --name "$CLUSTER" --wait 120s
for svc in api ingest scheduler; do kind load docker-image gs-$svc:dev --name "$CLUSTER"; done
kubectl --context "kind-$CLUSTER" create secret generic gs-mqtt-certs \
  --from-file=ca.crt=.local/certs/ca.crt --from-file=cloud.crt=.local/certs/cloud.crt --from-file=cloud.key=.local/certs/cloud.key \
  --dry-run=client -o yaml | kubectl --context "kind-$CLUSTER" apply -f -
helm upgrade --install gs deploy/helm/gs-cloud --kube-context "kind-$CLUSTER" --wait --timeout 180s
kubectl --context "kind-$CLUSTER" get all -o wide | tee .local/logs/kind-get-all.txt
