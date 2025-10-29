#!/bin/bash

#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# This script only applies if install.sh has been previously executed and
# the environment configuration and code directory have not changed.

set -euo pipefail
if ! command -v helm &> /dev/null; then
  echo "Error: helm command not found. Please install it first."
  exit 1
fi

if ! command -v kubectl &> /dev/null; then
  echo "Error: kubectl command not found. Please install it first."
  exit 1
fi

NAMESPACE="primus-safe"

install_or_upgrade_helm_chart() {
  local chart_name="$1"
  local values_yaml="$2"
  local chart_path="./$chart_name"

  helm upgrade -i "$chart_name" "$chart_path" -n "$NAMESPACE" -f $values_yaml
  echo "✅ $chart_name installed in namespace("$NAMESPACE")"
  echo
}

echo "====================================="
echo "🔧 Step 1: Load Parameters from .env"
echo "====================================="

if [ -f ".env" ]; then
  source .env
else
  echo "Error: .env file not found. pls execute install.sh first"
  exit 1
fi

echo "✅ Ethernet nic: \"$ethernet_nic\""
echo "✅ Rdma nic: \"$rdma_nic\""
echo "✅ Cluster Scale: \"$cluster_scale\""
echo "✅ Storage Class: \"$storage_class\""
echo "✅ Support Primus-lens: \"$lens_enable\""
echo "✅ Support Primus-s3: \"$s3_enable\""
echo "✅ Ingress Name: \"$ingress\""
if [[ "$ingress" == "higress" ]]; then
  echo "✅ Cluster Name: \"$sub_domain\""
fi

echo

replicas=1
cpu=2000m
memory=4Gi
if [[ "$cluster_scale" == "medium" ]]; then
  replicas=2
  cpu=8000m
  memory=8Gi
elif [[ "$cluster_scale" == "large" ]]; then
  replicas=2
  cpu=32000m
  memory=16Gi
fi
IMAGE_PULL_SECRET="$NAMESPACE-image"
S3_SECRET="$NAMESPACE-s3"

echo
echo "========================================="
echo "🔧 Step 2: upgrade primus-safe admin plane"
echo "========================================="

cd ../charts/
src_values_yaml="primus-safe/values.yaml"
if [ ! -f "$src_values_yaml" ]; then
  echo "Error: $src_values_yaml does not exist"
  exit 1
fi
values_yaml="primus-safe/.values.yaml"
cp "$src_values_yaml" "${values_yaml}"

sed -i "s/nccl_socket_ifname: \".*\"/nccl_socket_ifname: \"$ethernet_nic\"/" "$values_yaml"
sed -i "s/nccl_ib_hca: \".*\"/nccl_ib_hca: \"$rdma_nic\"/" "$values_yaml"
sed -i "s/replicas: [0-9]*/replicas: $replicas/" "$values_yaml"
sed -i "s/^.*cpu:.*/  cpu: $cpu/" "$values_yaml"
sed -i "s/^.*memory:.*/  memory: $memory/" "$values_yaml"
sed -i "s/^.*storage_class:.*/  storage_class: \"$storage_class\"/" "$values_yaml"
sed -i "s/^.*sub_domain:.*/  sub_domain: \"$sub_domain\"/" "$values_yaml"
sed -i '/opensearch:/,/^[a-z]/ s/enable: .*/enable: '"$lens_enable"'/' "$values_yaml"
sed -i '/s3:/,/^[a-z]/ s/enable: .*/enable: '"$s3_enable"'/' "$values_yaml"
if [[ "$s3_enable" == "true" ]]; then
  sed -i '/^s3:/,/^[a-z]/ s#secret: ".*"#secret: "'"$S3_SECRET"'"#' "$values_yaml"
fi
sed -i '/grafana:/,/^[a-z]/ s/enable: .*/enable: '"$lens_enable"'/' "$values_yaml"
if [[ "$lens_enable" == "true" ]]; then
  pg_password=$(kubectl get secret -n "primus-lens" primus-lens-pguser-primus-lens -o jsonpath="{.data.password}" | base64 -d)
  sed -i '/^grafana:/,/^[a-z]/ s#password: ".*"#password: "'"$pg_password"'"#' "$values_yaml"
fi
sed -i "s/image_pull_secret: \".*\"/image_pull_secret: \"$IMAGE_PULL_SECRET\"/" "$values_yaml"
sed -i "s/ingress: \".*\"/ingress: \"$ingress\"/" "$values_yaml"

chart_name="primus-safe"
if helm -n "$NAMESPACE" list | grep -q "^$chart_name "; then
  kubectl replace -f $chart_name/crds/ -n "$NAMESPACE"
  mkdir -p output
  helm template "$chart_name" -f "$values_yaml" -n "$NAMESPACE" "$chart_name" --output-dir ./output 1>/dev/null
  kubectl replace -f output/$chart_name/templates/rbac/role.yaml
  kubectl replace -f output/$chart_name/templates/webhooks/manifests.yaml
  echo
  rm -rf output
fi
install_or_upgrade_helm_chart "$chart_name" "$values_yaml"

install_or_upgrade_helm_chart "primus-safe-cr" "$values_yaml"
rm -f "$values_yaml"

echo
echo "========================================="
echo "🔧 Step 3: upgrade primus-safe data plane"
echo "========================================="

cd ../node-agent/charts/
src_values_yaml="node-agent/values.yaml"
if [ ! -f "$src_values_yaml" ]; then
  echo "Error: $src_values_yaml does not exist"
  exit 1
fi
values_yaml="node-agent/.values.yaml"
cp "$src_values_yaml" "${values_yaml}"

sed -i "s/nccl_socket_ifname: \".*\"/nccl_socket_ifname: \"$ethernet_nic\"/" "$values_yaml"
sed -i "s/nccl_ib_hca: \".*\"/nccl_ib_hca: \"$rdma_nic\"/" "$values_yaml"
sed -i "s/image_pull_secret: \".*\"/image_pull_secret: \"$IMAGE_PULL_SECRET\"/" "$values_yaml"

install_or_upgrade_helm_chart "node-agent" "$values_yaml"
rm -f "$values_yaml"

echo
echo "==============================="
echo "🔧 Step 4: All completed!"
echo "==============================="
