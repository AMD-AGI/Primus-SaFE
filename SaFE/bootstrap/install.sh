#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -euo pipefail
if ! command -v helm &> /dev/null; then
  echo "Error: helm command not found. Please install it first."
  exit 1
fi

if ! command -v kubectl &> /dev/null; then
  echo "Error: kubectl command not found. Please install it first."
  exit 1
fi

# Do not modify the value of namespace
NAMESPACE="primus-safe"

# Create namespace if it doesn't exist (ignore error if already exists)
kubectl create namespace "$NAMESPACE" 2>/dev/null || true

get_input_with_default() {
  local prompt="$1"
  local default_value="$2"
  local input
  read -rp "$prompt" input
  input=$(echo "$input" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
  if [ -z "$input" ]; then
      echo "$default_value"
  else
      echo "$input"
  fi
}

convert_to_boolean() {
  local value="$1"
  if [[ "$value" == "y" ]]; then
      echo "true"
  else
      echo "false"
  fi
}

install_or_upgrade_helm_chart() {
  local chart_name="$1"
  local values_yaml="$2"
  local chart_path="./$chart_name"

  if helm -n "$NAMESPACE" list | grep -q "^$chart_name"; then
      helm upgrade "$chart_name" "$chart_path" -n "$NAMESPACE" -f $values_yaml
  else
      helm install "$chart_name" "$chart_path" -n "$NAMESPACE" -f $values_yaml --create-namespace
  fi
  echo "✅ $chart_name installed in namespace("$NAMESPACE")"
  echo
}

echo "============================"
echo "🔧 Step 1: Input Parameters"
echo "============================"

shopt -s nocasematch

default_ethernet_nic="eno0"
default_rdma_nic="rdma0,rdma1,rdma2,rdma3,rdma4,rdma5,rdma6,rdma7"
default_cluster_scale="small"
default_storage_class="local-path"

ethernet_nic=$(get_input_with_default "Enter ethernet nic($default_ethernet_nic): " "$default_ethernet_nic")
rdma_nic=$(get_input_with_default "Enter rdma nic($default_rdma_nic): " "$default_rdma_nic")
cluster_scale=$(get_input_with_default "Enter cluster scale, choose 'small/medium/large' ($default_cluster_scale): " "$default_cluster_scale")
storage_class=$(get_input_with_default "Enter storage class($default_storage_class): " "$default_storage_class")
support_lens=$(get_input_with_default "Support Primus-lens ? (y/n): " "n")
lens_enable=$(convert_to_boolean "$support_lens")

support_s3=$(get_input_with_default "Support S3 ? (y/n): " "n")
s3_enable=$(convert_to_boolean "$support_s3")
s3_endpoint=""
s3_bucket=""
s3_access_key=""
s3_secret_key=""
if [[ "$s3_enable" == "true" ]]; then
  s3_endpoint=$(get_input_with_default "Enter S3 endpoint (empty to disable S3): " "")
  s3_bucket=$(get_input_with_default "Enter S3 bucket(empty to disable S3): " "")
  s3_access_key=$(get_input_with_default "Enter S3 access-key(empty to disable S3): " "")
  s3_secret_key=$(get_input_with_default "Enter S3 secret-key(empty to disable S3): " "")
fi

build_image_secret=$(get_input_with_default "Create image pull secret ? (y/n): " "n")
image_registry=""
image_username=""
image_password=""
if [[ "$build_image_secret" == "y" ]]; then
  image_registry=$(get_input_with_default "Enter image registry (e.g. registry.example.com): " "")
  image_username=$(get_input_with_default "Enter image registry username: " "")
  image_password=$(get_input_with_default "Enter image registry password: " "")
fi

ingress=$(get_input_with_default "Enter the ingress name (nginx/higress): " "nginx")
sub_domain=""
if [[ "$ingress" == "higress" ]]; then
  sub_domain=$(get_input_with_default "Enter cluster name(lowercase with hyphen): " "amd")
fi

support_sso=$(get_input_with_default "Support SSO ? (y/n): " "n")
sso_enable=$(convert_to_boolean "$support_sso")
sso_endpoint=""
sso_client_id=""
sso_client_secret=""
sso_redirect_uri=""
if [[ "$sso_enable" == "true" ]]; then
  sso_endpoint=$(get_input_with_default "Enter SSO endpoint (empty to disable SSO): " "")
  sso_client_id=$(get_input_with_default "Enter SSO client id(empty to disable SSO): " "")
  sso_client_secret=$(get_input_with_default "Enter SSO client secret(empty to disable SSO): " "")
  sso_redirect_uri=$(get_input_with_default "Enter SSO redirect uri(empty to disable SSO): " "")
fi

install_node_agent=$(get_input_with_default "install node-agent ? (y/n): " "n")

echo "✅ Ethernet nic: \"$ethernet_nic\""
echo "✅ Rdma nic: \"$rdma_nic\""
echo "✅ Cluster Scale: \"$cluster_scale\""
echo "✅ Storage Class: \"$storage_class\""
echo "✅ Support Primus-lens: \"$lens_enable\""
echo "✅ Support Primus-s3: \"$s3_enable\""
if [[ "$s3_enable" == "true" ]]; then
  echo "✅ S3 Endpoint: \"$s3_endpoint\""
  echo "✅ S3 Bucket: \"$s3_bucket\""
  echo "✅ S3 Access Key: \"$s3_access_key\""
  echo "✅ S3 Secret Key: \"$s3_secret_key\""
fi
if [[ "$build_image_secret" == "y" ]]; then
  echo "✅ Image registry: \"$image_registry\""
  echo "✅ Image username: \"$image_username\""
fi
echo "✅ Ingress Name: \"$ingress\""
if [[ "$ingress" == "higress" ]]; then
  echo "✅ Cluster Name: \"$sub_domain\""
fi
if [[ "$sso_enable" == "true" ]]; then
  echo "✅ SSO Endpoint: \"$sso_endpoint\""
  echo "✅ SSO Client ID: \"$sso_client_id\""
  echo "✅ SSO Client Secret: \"$sso_client_secret\""
  echo "✅ SSO Redirect URI: \"$sso_redirect_uri\""
fi

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

echo
echo "============================================================="
echo "🔧 Step 2: generate image-pull-secret, s3-secret, sso-secret"
echo "============================================================="

IMAGE_PULL_SECRET="$NAMESPACE-image"
if kubectl get secret "$IMAGE_PULL_SECRET" -n "$NAMESPACE" >/dev/null 2>&1; then
  echo "⚠️ Image pull secret $IMAGE_PULL_SECRET already exists in namespace \"$NAMESPACE\", overwriting..."
fi
if [[ "$build_image_secret" == "y" ]] && [[ -n "$image_registry" ]] && [[ -n "$image_username" ]] && [[ -n "$image_password" ]]; then
  kubectl create secret docker-registry "$IMAGE_PULL_SECRET" \
    --docker-server="$image_registry" \
    --docker-username="$image_username" \
    --docker-password="$image_password" \
    --namespace="$NAMESPACE" \
    --dry-run=client -o yaml | kubectl apply -f - \
    && kubectl label secret "$IMAGE_PULL_SECRET" -n "$NAMESPACE" primus-safe.secret.type=image primus-safe.display.name="$IMAGE_PULL_SECRET" --overwrite
  echo "✅ Image pull secret($IMAGE_PULL_SECRET) created in namespace \"$NAMESPACE\""
else
  kubectl create secret generic "$IMAGE_PULL_SECRET" \
    --namespace="$NAMESPACE" \
    --from-literal=.dockerconfigjson='{}' \
    --type=kubernetes.io/dockerconfigjson \
    --dry-run=client -o yaml | kubectl apply -f - \
    && kubectl label secret "$IMAGE_PULL_SECRET" -n "$NAMESPACE" primus-safe.secret.type=image primus-safe.display.name="$IMAGE_PULL_SECRET" --overwrite
  echo "✅ Empty Image pull secret($IMAGE_PULL_SECRET) created in namespace \"$NAMESPACE\""
fi

S3_SECRET="$NAMESPACE-s3"
if kubectl get secret "$S3_SECRET" -n "$NAMESPACE" >/dev/null 2>&1; then
  echo "⚠️ Image pull secret $S3_SECRET already exists in namespace \"$NAMESPACE\", overwriting..."
fi
if [[ "$s3_enable" == "true" ]] && [[ -n "$s3_endpoint" ]] && [[ -n "$s3_bucket" ]] && [[ -n "$s3_access_key" ]] && [[ -n "$s3_secret_key" ]]; then
  kubectl create secret generic $S3_SECRET \
    --namespace=$NAMESPACE \
    --from-literal=access_key="$s3_access_key" \
    --from-literal=bucket="$s3_bucket" \
    --from-literal=endpoint="$s3_endpoint" \
    --from-literal=secret_key="$s3_secret_key" \
    --dry-run=client -o yaml | kubectl apply -f -
  echo "✅ S3 secret($S3_SECRET) created in namespace \"$NAMESPACE\""
else
  s3_enable="false"
fi

SSO_SECRET="$NAMESPACE-sso"
if kubectl get secret "$SSO_SECRET" -n "$NAMESPACE" >/dev/null 2>&1; then
  echo "⚠️ Image pull secret $SSO_SECRET already exists in namespace \"$NAMESPACE\", overwriting..."
fi
if [[ "$sso_enable" == "true" ]] && [[ -n "$sso_endpoint" ]] && [[ -n "$sso_client_id" ]] && [[ -n "$sso_client_secret" ]] && [[ -n "$sso_redirect_uri" ]]; then
  kubectl create secret generic $SSO_SECRET \
    --namespace=$NAMESPACE \
    --from-literal=id="$sso_client_id" \
    --from-literal=secret="$sso_client_secret" \
    --from-literal=endpoint="$sso_endpoint" \
    --from-literal=redirect_uri="$sso_redirect_uri" \
    --dry-run=client -o yaml | kubectl apply -f -
  echo "✅ SSO secret($SSO_SECRET) created in namespace \"$NAMESPACE\""
else
  sso_enable="false"
fi

echo
echo "========================================="
echo "🔧 Step 3: install grafana-operator"
echo "========================================="

cd ../charts/
src_values_yaml="grafana-operator/values.yaml"
if [ ! -f "$src_values_yaml" ]; then
  echo "Error: $src_values_yaml does not exist"
  exit 1
fi
values_yaml="grafana-operator/.values.yaml"
cp "$src_values_yaml" "${values_yaml}"

sed -i "s/imagePullSecrets: \[\]/imagePullSecrets:\n  - name: $IMAGE_PULL_SECRET/" "$values_yaml"
install_or_upgrade_helm_chart "grafana-operator" "$values_yaml"
rm -f "$values_yaml"

echo
echo "========================================="
echo "🔧 Step 4: install primus-safe admin plane"
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
if [[ "$ingress" == "higress" ]]; then
  sed -i "s/^.*sub_domain:.*/  sub_domain: \"$sub_domain\"/" "$values_yaml"
fi
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

install_or_upgrade_helm_chart "primus-pgo" "$values_yaml"
echo "⏳ Waiting for Postgres Operator pod..."
for i in {1..30}; do
  if kubectl get pods -n "$NAMESPACE" | grep "primus-pgo"| grep -q "Running"; then
    echo "✅ Postgres Operator is running."
    break
  fi
  echo "⏳ [$i/30] Waiting for postgres-operator..."
  sleep 5
done
echo

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


if [[ "$install_node_agent" == "y" ]]; then
  echo
  echo "========================================="
  echo "🔧 Step 5: install primus-safe data plane"
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
fi

echo
echo "========================================="
echo "🔧 Step 6: All completed!"
echo "========================================="

cd ../../bootstrap
cat > .env <<EOF
ethernet_nic=$ethernet_nic
rdma_nic=$rdma_nic
cluster_scale=$cluster_scale
storage_class=$storage_class
lens_enable=$lens_enable
s3_enable=$s3_enable
sso_enable=$sso_enable
ingress=$ingress
sub_domain=$sub_domain
install_node_agent=$install_node_agent
EOF