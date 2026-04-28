
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

#!/bin/bash



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
lens_enable="false"
echo "✅ Robust data-plane: managed via AddonTemplate"
echo "✅ Support S3: \"$s3_enable\""
echo "✅ Support SSO: \"$sso_enable\""
echo "✅ Support Tracing: \"${tracing_enable:-false}\" (mode: ${tracing_mode:-error_only})"
echo "✅ Support LLM Gateway: \"${llm_gateway_enable:-false}\""
echo "✅ Grafana: enable=\"${grafana_enable:-true}\" (set grafana_enable=false in .env to disable)"
echo "✅ Ingress Name: \"$ingress\""
if [[ "$ingress" == "higress" ]]; then
  echo "✅ Cluster Name: \"$sub_domain\""
fi
echo "✅ Image Registry: \"$proxy_image_registry\""
echo "✅ Helm Registry: \"$helm_registry\""
echo "✅ CD Require Approval: \"$cd_require_approval\""
echo "✅ Install Node Agent: \"${install_node_agent:-y}\""
echo "✅ CSI Volume Handle: \"$csi_volume_handle\""
echo "✅ Node Agent GPU Driver: \"${node_agent_gpu_driver:-6.12.12}\""
echo "✅ Node Agent ROCm Version: \"${node_agent_rocm_version:-6.4}\""
echo "✅ Node Agent Toggles: net_bnxt_load_204=${node_agent_toggle_net_bnxt_load_204:-off}, net_ainic_load_205=${node_agent_toggle_net_ainic_load_205:-off}, net_ainic_devices_208=${node_agent_toggle_net_ainic_devices_208:-off}, sys_csi_wekafs_309=${node_agent_toggle_sys_csi_wekafs_309:-on}"

echo

replicas=1
cpu=2000m
memory=4Gi
if [[ "$cluster_scale" == "medium" ]]; then
  replicas=2
  cpu=4000m
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
sed -i "s/csi_volume_handle: \".*\"/csi_volume_handle: \"$csi_volume_handle\"/" "$values_yaml"

sed -i "s/nccl_socket_ifname: \".*\"/nccl_socket_ifname: \"$ethernet_nic\"/" "$values_yaml"
sed -i "s/nccl_ib_hca: \".*\"/nccl_ib_hca: \"$rdma_nic\"/" "$values_yaml"
sed -i "s/^.*sub_domain:.*/  sub_domain: \"$sub_domain\"/" "$values_yaml"
sed -i "s/replicas: [0-9]*/replicas: $replicas/" "$values_yaml"
sed -i "s/^.*cpu:.*/  cpu: $cpu/" "$values_yaml"
sed -i "s/^.*memory:.*/  memory: $memory/" "$values_yaml"
sed -i "s/^.*storage_class:.*/  storage_class: \"$storage_class\"/" "$values_yaml"
sed -i '/opensearch:/,/^[a-z]/ s/enable: .*/enable: true/' "$values_yaml"
sed -i '/s3:/,/^[a-z]/ s/enable: .*/enable: '"$s3_enable"'/' "$values_yaml"
if [[ "$s3_enable" == "true" ]]; then
  sed -i '/^s3:/,/^[a-z]/ s#secret: ".*"#secret: "'"$S3_SECRET"'"#' "$values_yaml"
fi
# Grafana is required for the SaFE UI (training-workload dashboard, cluster
# GPU heatmap, log-based alerts overview). Default it to "on"; only opt-out
# when the operator explicitly sets grafana_enable=false in .env.
# The previous hard-coded `enable: false` was the reason every CD run kept
# deleting the live Grafana instance and left the /lens/grafana UI 504.
sed -i '/grafana:/,/^[a-z]/ s/enable: .*/enable: '"${grafana_enable:-true}"'/' "$values_yaml"
if [[ -n "${grafana_password:-}" ]]; then
  sed -i '/grafana:/,/^[a-z]/ s/password: .*/password: "'"$grafana_password"'"/' "$values_yaml"
fi
sed -i "s/image_pull_secret: \".*\"/image_pull_secret: \"$IMAGE_PULL_SECRET\"/" "$values_yaml"
sed -i "s/ingress: \".*\"/ingress: \"$ingress\"/" "$values_yaml"
sed -i '/sso:/,/^[a-z]/ s/enable: .*/enable: '"$sso_enable"'/' "$values_yaml"
if [[ "$sso_enable" == "true" ]]; then
  sed -i '/^sso:/,/^[a-z]/ s#secret: ".*"#secret: "'"$SSO_SECRET"'"#' "$values_yaml"
fi
sed -i '/^cd:/,/^[a-z]/ s/require_approval: .*/require_approval: '"$cd_require_approval"'/' "$values_yaml"

# Configure metrics port if defined in .env
if [[ -n "${metrics_port:-}" ]]; then
  sed -i '/^job_manager:/,/^[a-z]/ s/metrics_port: .*/metrics_port: '"$metrics_port"'/' "$values_yaml"
fi

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

# Configure LLM Gateway secrets if defined in .env
if [[ -n "${llm_gateway_litellm_endpoint:-}" ]]; then
  sed -i "/^llm_gateway:/a\\
  litellm_endpoint: \"${llm_gateway_litellm_endpoint}\"\\
  litellm_admin_key: \"${llm_gateway_litellm_admin_key:-}\"\\
  litellm_team_id: \"${llm_gateway_litellm_team_id:-}\"" "$values_yaml"
fi

# Configure Langfuse proxy if defined in .env
if [[ "${langfuse_proxy_enable:-false}" == "true" ]]; then
  sed -i '/^langfuse_proxy:/,/^[a-z]/ s/enable: .*/enable: true/' "$values_yaml"
  if [[ -n "${langfuse_proxy_target:-}" ]]; then
    sed -i '/^langfuse_proxy:/,/^[a-z]/ s#target: .*#target: "'"$langfuse_proxy_target"'"#' "$values_yaml"
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
rm -f "$values_yaml"
sleep 10

echo
echo "========================================="
echo "🔧 Step 3: upgrade primus-safe cr"
echo "========================================="

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
sed -i '/global:/,/^[a-z]/ s/sub_domain: .*/sub_domain: "'"$sub_domain"'"/' "$values_yaml"

install_or_upgrade_helm_chart "primus-safe-cr" "$values_yaml"
rm -f "$values_yaml"
cd ..


echo
echo "========================================="
echo "🔧 Step 4: upgrade primus-safe data plane"
echo "========================================="

if [[ "${CALLED_BY_CD:-false}" == "true" ]]; then
  echo "⏭️  Skipping node-agent upgrade (called by cd-deploy.sh)"
elif [[ "${install_node_agent:-y}" == "n" ]]; then
  echo "⏭️  Skipping node-agent upgrade (install_node_agent=n)"
else
  cd ./node-agent/charts/
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
  sed -i "s/^.*sub_domain:.*/  sub_domain: \"$sub_domain\"/" "$values_yaml"
  sed -i "s/image_pull_secret: \".*\"/image_pull_secret: \"$IMAGE_PULL_SECRET\"/" "$values_yaml"
  sed -i "s/gpu_driver: \".*\"/gpu_driver: \"${node_agent_gpu_driver:-6.12.12}\"/" "$values_yaml"
  sed -i "s/rocm_version: \".*\"/rocm_version: \"${node_agent_rocm_version:-6.4}\"/" "$values_yaml"
  sed -i "s/net_bnxt_load_204: \".*\"/net_bnxt_load_204: \"${node_agent_toggle_net_bnxt_load_204:-off}\"/" "$values_yaml"
  sed -i "s/net_ainic_load_205: \".*\"/net_ainic_load_205: \"${node_agent_toggle_net_ainic_load_205:-off}\"/" "$values_yaml"
  sed -i "s/net_ainic_devices_208: \".*\"/net_ainic_devices_208: \"${node_agent_toggle_net_ainic_devices_208:-off}\"/" "$values_yaml"
  # WekaFS CSI container check defaults to "on" now that the script bug
  # (matched pod name, not container name) has been fixed upstream. Sites
  # that don't run WekaFS can disable it explicitly with
  # node_agent_toggle_sys_csi_wekafs_309=off in .env.
  sed -i "s/sys_csi_wekafs_309: \".*\"/sys_csi_wekafs_309: \"${node_agent_toggle_sys_csi_wekafs_309:-on}\"/" "$values_yaml"
  sed -i "s/disk_nfs_exist_check_402: \".*\"/disk_nfs_exist_check_402: \"${node_agent_toggle_disk_nfs_exist_check_402:-off}\"/" "$values_yaml"

  if [ -n "${node_agent_nfs_server:-}" ]; then
    sed -i "s|nfs_server: \".*\"|nfs_server: \"${node_agent_nfs_server}\"|" "$values_yaml"
  fi
  if [ -n "${node_agent_nfs_server_path:-}" ]; then
    sed -i "s|nfs_server_path: \".*\"|nfs_server_path: \"${node_agent_nfs_server_path}\"|" "$values_yaml"
  fi
  if [ -n "${node_agent_nfs_mount:-}" ]; then
    sed -i "s|nfs_mount: \".*\"|nfs_mount: \"${node_agent_nfs_mount}\"|" "$values_yaml"
  fi
  if [ -n "${node_agent_nfs_type:-}" ]; then
    sed -i "s|nfs_type: \".*\"|nfs_type: \"${node_agent_nfs_type}\"|" "$values_yaml"
  fi
  if [ -n "${node_agent_all_nfs_path:-}" ]; then
    sed -i "s|all_nfs_path: \".*\"|all_nfs_path: \"${node_agent_all_nfs_path}\"|" "$values_yaml"
  fi
  if [ -n "${node_agent_resolv_search_domain:-}" ]; then
    sed -i "s|resolv_search_domain: \".*\"|resolv_search_domain: \"${node_agent_resolv_search_domain}\"|" "$values_yaml"
  fi

  install_or_upgrade_helm_chart "node-agent" "$values_yaml"
  rm -f "$values_yaml"
fi

echo
echo "==============================="
echo "🔧 Step 5: All completed!"
echo "==============================="
