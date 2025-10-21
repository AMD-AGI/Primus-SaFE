#!/bin/bash

#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -e

if ! command -v unzip &> /dev/null; then
  sudo apt-get update
  sudo apt-get install -y unzip
fi

MANIFEST_DIR="manifests"
export NAMESPACE=$NAMESPACE
PG_PASSWORD=$(kubectl get secret -n "primus-lens" primus-lens-pguser-primus-lens -o jsonpath="{.data.password}" | base64 -d)
export PG_PASSWORD

rm -rf grafana-operator
unzip ../charts/grafana-operator-v5.20.0.zip -d . >/dev/null
helm upgrade --install -n "$NAMESPACE" grafana-operator grafana-operator/deploy/helm/grafana-operator \
  -f "$MANIFEST_DIR/grafana-operator-values.yaml.tpl"
rm -rf grafana-operator

echo "Installing Grafana in namespace: $NAMESPACE"
envsubst < "$MANIFEST_DIR/grafana.yaml.tpl" | kubectl apply -n "$NAMESPACE" -f -

BASE_DIR="manifests/grafana"
envsubst < "$BASE_DIR/datasource.yaml.tpl" | kubectl apply -n "$NAMESPACE" -f -

echo "Grafana datasources and folders applied."

echo "Start applying Grafana dashboards..."
DASHBOARD_DIR="$BASE_DIR/dashboards"
kubectl apply -f "$DASHBOARD_DIR" -n "$NAMESPACE"
echo "Grafana dashboards applied."

kubectl apply -f "$MANIFEST_DIR/primussafe-nginx.yaml" -n "$NAMESPACE"