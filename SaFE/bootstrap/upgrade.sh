#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
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
echo "✅ Support S3: \"$s3_enable\""
echo "✅ Support SSO: \"$sso_enable\""
echo "✅ Support Tracing: \"${tracing_enable:-false}\" (mode: ${tracing_mode:-error_only})"
echo "✅ Ingress Name: \"$ingress\""
if [[ "$ingress" == "higress" ]]; then
  echo "✅ Cluster Name: \"$sub_domain\""
fi
echo "✅ Image Registry: \"$proxy_image_registry\""
echo "✅ Helm Registry: \"$helm_registry\""
echo "✅ CD Require Approval: \"$cd_require_approval\""
echo "✅ Install Node Agent: \"${install_node_agent:-y}\""

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
SSO_SECRET="$NAMESPACE-sso"

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

safe_image=$(printf '%s\n' "$proxy_image_registry" | sed 's/[&/\]/\\&/g')
sed -i '/global:/,/^[a-z]/ s/image_registry: .*/image_registry: "'"$safe_image"'"/' "$values_yaml"

sed -i "s/nccl_socket_ifname: \".*\"/nccl_socket_ifname: \"$ethernet_nic\"/" "$values_yaml"
sed -i "s/nccl_ib_hca: \".*\"/nccl_ib_hca: \"$rdma_nic\"/" "$values_yaml"
if [[ "$ingress" == "higress" ]]; then
  sed -i "s/^.*sub_domain:.*/  sub_domain: \"$sub_domain\"/" "$values_yaml"
fi
sed -i "s/replicas: [0-9]*/replicas: $replicas/" "$values_yaml"
sed -i "s/^.*cpu:.*/  cpu: $cpu/" "$values_yaml"
sed -i "s/^.*memory:.*/  memory: $memory/" "$values_yaml"
sed -i "s/^.*storage_class:.*/  storage_class: \"$storage_class\"/" "$values_yaml"
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
sed -i '/sso:/,/^[a-z]/ s/enable: .*/enable: '"$sso_enable"'/' "$values_yaml"
if [[ "$sso_enable" == "true" ]]; then
  sed -i '/^sso:/,/^[a-z]/ s#secret: ".*"#secret: "'"$SSO_SECRET"'"#' "$values_yaml"
fi
sed -i '/^cd:/,/^[a-z]/ s/require_approval: .*/require_approval: '"$cd_require_approval"'/' "$values_yaml"

# Configure tracing if defined in .env
if [[ "${tracing_enable:-false}" == "true" ]]; then
  sed -i '/^tracing:/,/^[a-z]/ s/enable: .*/enable: true/' "$values_yaml"
  if [[ -n "${tracing_mode:-}" ]]; then
    sed -i '/^tracing:/,/^[a-z]/ s/mode: .*/mode: "'"$tracing_mode"'"/' "$values_yaml"
  fi
  if [[ -n "${tracing_sampling_ratio:-}" ]]; then
    sed -i '/^tracing:/,/^[a-z]/ s/sampling_ratio: .*/sampling_ratio: "'"$tracing_sampling_ratio"'"/' "$values_yaml"
  fi
  if [[ -n "${tracing_otlp_endpoint:-}" ]]; then
    sed -i '/^tracing:/,/^[a-z]/ s#otlp_endpoint: .*#otlp_endpoint: "'"$tracing_otlp_endpoint"'"#' "$values_yaml"
  fi
fi

# Configure proxy services if defined in .env
if [[ -n "${proxy_services:-}" ]]; then
  sed -i "/^proxy:/,/^[a-z_]*:/ { /^proxy:/! { /^[a-z_]*:/!d } }" "$values_yaml"
  sed -i "/^proxy:/a\\
  services: $proxy_services" "$values_yaml"
fi

chart_name="primus-safe"
if helm -n "$NAMESPACE" list | grep -q "^$chart_name "; then
  kubectl replace -f $chart_name/crds/ -n "$NAMESPACE" || kubectl create -f $chart_name/crds/ -n "$NAMESPACE"
  mkdir -p output
  helm template "$chart_name" -f "$values_yaml" -n "$NAMESPACE" "$chart_name" --output-dir ./output 1>/dev/null
  kubectl replace -f output/$chart_name/templates/rbac/role.yaml || kubectl create -f output/$chart_name/templates/rbac/role.yaml
  kubectl replace -f output/$chart_name/templates/webhooks/manifests.yaml || kubectl create -f output/$chart_name/templates/webhooks/manifests.yaml
  echo
  rm -rf output
fi
install_or_upgrade_helm_chart "$chart_name" "$values_yaml"

sleep 10

echo
echo "========================================="
echo "🔧 Step 3: upgrade primus-safe cr"
echo "========================================="

cd ../charts/
src_values_yaml="primus-safe-cr/values.yaml"
if [ ! -f "$src_values_yaml" ]; then
  echo "Error: $src_values_yaml does not exist"
  exit 1
fi
values_yaml="primus-safe-cr/.values.yaml"
cp "$src_values_yaml" "${values_yaml}"

if [[ -n "${helm_registry:-}" ]]; then
  sed -i '/global:/,/^[a-z]/ s/helm_registry: .*/helm_registry: "'"$helm_registry"'"/' "$values_yaml"
fi

install_or_upgrade_helm_chart "primus-safe-cr" "$values_yaml"
rm -f "$values_yaml"


echo
echo "========================================="
echo "🔧 Step 4: upgrade primus-safe data plane"
echo "========================================="

# Check if node-agent installation is enabled (default: y)
if [[ "${install_node_agent:-y}" == "n" ]]; then
  echo "⏭️  Skipping node-agent upgrade (install_node_agent=n)"
else
  cd ../node-agent/charts/
  src_values_yaml="node-agent/values.yaml"
  if [ ! -f "$src_values_yaml" ]; then
    echo "Error: $src_values_yaml does not exist"
    exit 1
  fi
  values_yaml="node-agent/.values.yaml"
  cp "$src_values_yaml" "${values_yaml}"

  sed -i '/node_agent:/,/^[a-z]/ s/image_registry: .*/image_registry: "'"$safe_image"'"/' "$values_yaml"

  sed -i "s/nccl_socket_ifname: \".*\"/nccl_socket_ifname: \"$ethernet_nic\"/" "$values_yaml"
  sed -i "s/nccl_ib_hca: \".*\"/nccl_ib_hca: \"$rdma_nic\"/" "$values_yaml"
  sed -i "s/image_pull_secret: \".*\"/image_pull_secret: \"$IMAGE_PULL_SECRET\"/" "$values_yaml"

  install_or_upgrade_helm_chart "node-agent" "$values_yaml"
  rm -f "$values_yaml"
fi

echo
echo "==============================="
echo "🔧 Step 5: All completed!"
echo "==============================="
