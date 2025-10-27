#!/bin/bash

#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
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

support_s3=$(get_input_with_default "Support Primus-S3 ? (y/n): " "n")
s3_enable=$(convert_to_boolean "$support_s3")
s3_endpoint=""
if [[ "$s3_enable" == "true" ]]; then
  s3_endpoint=$(get_input_with_default "Enter S3 endpoint (empty to disable S3): " "")
  if [ -z "$s3_endpoint" ]; then
    s3_enable="false"
  fi
fi

support_ssh=$(get_input_with_default "Support ssh ? (y/n): " "n")
ssh_enable=$(convert_to_boolean "$support_ssh")
ssh_server_ip=""
if [[ "$ssh_enable" == "true" ]]; then
  ssh_server_ip=$(get_input_with_default "Enter ssh server ip(empty to disable ssh): " "")
  if [ -z "$ssh_server_ip" ]; then
    ssh_enable="false"
  fi
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

echo "✅ Ethernet nic: \"$ethernet_nic\""
echo "✅ Rdma nic: \"$rdma_nic\""
echo "✅ Cluster Scale: \"$cluster_scale\""
echo "✅ Storage Class: \"$storage_class\""
echo "✅ Support Primus-lens: \"$lens_enable\""
echo "✅ Support Primus-s3: \"$s3_enable\""
if [[ "$s3_enable" == "true" ]]; then
  echo "✅ S3 Endpoint: \"$s3_endpoint\""
fi
echo "✅ Support ssh: \"$ssh_enable\""
if [[ "$ssh_enable" == "true" ]]; then
  echo "✅ SSH Server IP: \"$ssh_server_ip\""
fi
if [[ "$build_image_secret" == "y" ]]; then
  echo "✅ Image registry: \"$image_registry\""
  echo "✅ Image username: \"$image_username\""
fi
echo "✅ Ingress Name: \"$ingress\""
if [[ "$ingress" == "higress" ]]; then
  echo "✅ Cluster Name: \"$sub_domain\""
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
echo "========================================="
echo "🔧 Step 2: generate image-pull-secret"
echo "========================================="

IMAGE_PULL_SECRET="$NAMESPACE-image"
if kubectl get secret "$IMAGE_PULL_SECRET" -n "$NAMESPACE" >/dev/null 2>&1; then
  echo "⚠️ Image pull secret $IMAGE_PULL_SECRET already exists in namespace \"$NAMESPACE\", skipping creation"
else
  if [[ "$build_image_secret" == "y" ]] && [[ -n "$image_registry" ]] && [[ -n "$image_username" ]] && [[ -n "$image_password" ]]; then
    kubectl create secret docker-registry "$IMAGE_PULL_SECRET" \
      --docker-server="$image_registry" \
      --docker-username="$image_username" \
      --docker-password="$image_password" \
      --namespace="$NAMESPACE" \
      --dry-run=client -o yaml | kubectl create -f - \
      && kubectl label secret "$IMAGE_PULL_SECRET" -n "$NAMESPACE" primus-safe.secret.type=image primus-safe.display.name="$IMAGE_PULL_SECRET" primus-safe.secret.all.workspace="true" --overwrite
    echo "✅ Image pull secret($IMAGE_PULL_SECRET) created in namespace \"$NAMESPACE\""
  else
    kubectl create secret generic "$IMAGE_PULL_SECRET" \
      --namespace="$NAMESPACE" \
      --from-literal=.dockerconfigjson='{}' \
      --type=kubernetes.io/dockerconfigjson \
      --dry-run=client -o yaml | kubectl create -f - \
      && kubectl label secret "$IMAGE_PULL_SECRET" -n "$NAMESPACE" primus-safe.secret.type=image primus-safe.display.name="$IMAGE_PULL_SECRET" primus-safe.secret.all.workspace="true" --overwrite
    echo "✅ Empty Image pull secret($IMAGE_PULL_SECRET) created in namespace \"$NAMESPACE\""
  fi
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
  sed -i '/^s3:/,/^[a-z]/ s#endpoint: ".*"#endpoint: "'"$s3_endpoint"'"#' "$values_yaml"
fi
sed -i '/grafana:/,/^[a-z]/ s/enable: .*/enable: '"$lens_enable"'/' "$values_yaml"
if [[ "$lens_enable" == "true" ]]; then
  pg_password=$(kubectl get secret -n "primus-lens" primus-lens-pguser-primus-lens -o jsonpath="{.data.password}" | base64 -d)
  sed -i '/^grafana:/,/^[a-z]/ s#password: ".*"#password: "'"$pg_password"'"#' "$values_yaml"
fi
sed -i "s/image_pull_secret: \".*\"/image_pull_secret: \"$IMAGE_PULL_SECRET\"/" "$values_yaml"
sed -i "s/ingress: \".*\"/ingress: \"$ingress\"/" "$values_yaml"
sed -i '/ssh:/,/^[a-z]/ s/enable: .*/enable: '"$ssh_enable"'/' "$values_yaml"
if [[ "$ssh_enable" == "true" ]]; then
  sed -i '/^ssh:/,/^[a-z]/ s#server_ip: ".*"#server_ip: "'"$ssh_server_ip"'"#' "$values_yaml"
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
s3_endpoint=$s3_endpoint
ingress=$ingress
sub_domain=$sub_domain
ssh_enable=$ssh_enable
ssh_server_ip=$ssh_server_ip
EOF