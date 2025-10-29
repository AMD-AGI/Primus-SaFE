#!/bin/bash
set -e


export GRAFANA_ROOT_URL="${GRAFANA_ROOT_URL:-https://${CLUSTER_NAME}.lens-primus.ai/grafana}"
export GRAFANA_DOMAIN="${GRAFANA_DOMAIN:-${CLUSTER_NAME}.lens-primus.ai}"


rm -rf grafana-operator
git clone https://github.com/grafana/grafana-operator.git
helm upgrade --install -n "$NAMESPACE" grafana-operator grafana-operator/deploy/helm/grafana-operator \
  -f "$MANIFEST_DIR/grafana-operator-values.yaml.tpl"
rm -rf grafana-operator



echo "Installing Grafana in namespace: $NAMESPACE"
envsubst < "$MANIFEST_DIR/grafana.yaml.tpl" | kubectl apply -n "$NAMESPACE" -f -

envsubst < "$MANIFEST_DIR/grafana-ingress.tpl" | kubectl apply -n "$NAMESPACE" -f -

echo "Grafana installed and ingress applied."

BASE_DIR="manifests/grafana"
NAMESPACE="${NAMESPACE:-primus-lens}"
envsubst < "$BASE_DIR/datasource.yaml.tpl" | kubectl apply -n "$NAMESPACE" -f -
envsubst < "$BASE_DIR/folders.yaml.tpl" | kubectl apply -n "$NAMESPACE" -f -

echo "Grafana datasources and folders applied."

echo "Start applying Grafana dashboards..."
DASHBOARD_DIR="$BASE_DIR/dashboards"
kubectl apply -f "$DASHBOARD_DIR" -n "$NAMESPACE"
echo "Grafana dashboards applied."
