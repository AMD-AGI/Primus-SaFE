#!/bin/bash

set -euo pipefail
if ! command -v helm &> /dev/null; then
  echo "Error: helm command not found. Please install Helm first."
  exit 1
fi

NAMESPACE="primus-safe"

echo "============================"
echo "🔧 Step 1: Input Parameters"
echo "============================"

shopt -s nocasematch

default_ethernet_nic="eno0"
default_rdma_nic="rdma0,rdma1,rdma2,rdma3,rdma4,rdma5,rdma6,rdma7"
default_cluster_scale="small"
default_storage_class="local-path"
default_sub_domain="test"

read -rp "Enter ethernet nic($default_ethernet_nic): " ethernet_nic
read -rp "Enter rdma nic($default_rdma_nic): " rdma_nic
read -rp "Enter cluster scale, choose 'small/medium/large' ($default_cluster_scale): " cluster_scale
read -rp "Enter storage class($default_storage_class): " storage_class
read -rp "Enter sub domain($default_sub_domain): " sub_domain
read -rp "Enter nfs-server (empty for no server): " nfs_server
read -rp "Enter nfs-path (empty for no path): " nfs_path
read -rp "Enter nfs-mount (empty for no mount): " nfs_mount
read -rp "Support Primus-lens ? (y/n): " support_lens
read -rp "Support Primus-S3 ? (y/n): " support_s3
if [[ "$support_s3" == "y" ]]; then
  read -rp "Enter S3 endpoint (empty to disable S3): " s3_endpoint
fi

ethernet_nic=$(echo "$ethernet_nic" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
rdma_nic=$(echo "$rdma_nic" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
storage_class=$(echo "$storage_class" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
sub_domain=$(echo "$sub_domain" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
nfs_server=$(echo "$nfs_server" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
nfs_path=$(echo "$nfs_path" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
nfs_mount=$(echo "$nfs_mount" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
support_lens=$(echo "$support_lens" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
s3_endpoint=$(echo "$s3_endpoint" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')

if [ -z "$ethernet_nic" ]; then
  ethernet_nic="$default_ethernet_nic"
fi
if [ -z "$rdma_nic" ]; then
  rdma_nic="$default_rdma_nic"
fi
if [ -z "$cluster_scale" ]; then
  cluster_scale="$default_cluster_scale"
fi
if [ -z "$storage_class" ]; then
  storage_class="$default_storage_class"
fi
if [ -z "$sub_domain" ]; then
  sub_domain="$default_sub_domain"
fi

echo "✅ Ethernet nic: \"$ethernet_nic\""
echo "✅ Rdma nic: \"$rdma_nic\""
echo "✅ Cluster Scale: \"$cluster_scale\""
echo "✅ Storage Class: \"$storage_class\""
echo "✅ Sub Domain: \"$sub_domain\""
echo "✅ NFS Server: \"$nfs_server\""
echo "✅ NFS Path: \"$nfs_path\""
echo "✅ NFS Mount: \"$nfs_mount\""
echo "✅ Support Primus-lens: \"$support_lens\""
echo "✅ Support Primus-s3: \"$support_s3\""
echo "✅ S3 Endpoint: \"$s3_endpoint\""
echo


opensearch_enable=true
if [[ "$support_lens" != "y" ]]; then
  opensearch_enable=false
fi

s3_enable=true
if [[ "$support_s3" != "y" || -z "$s3_endpoint" ]]; then
  s3_enable=false
fi

replicas=1
cpu=2000m
memory=4Gi
if [[ "$cluster_scale" == "medium" ]]; then
  replicas=2
  cpu=16000m
  memory=16Gi
elif [[ "$cluster_scale" == "large" ]]; then
  replicas=2
  cpu=32000m
  memory=32Gi
fi

echo "========================================="
echo "🔧 Step 2: install primus-safe admin plane"
echo "========================================="

cd ../charts/
values_yaml="primus-safe/values.yaml"
if [ ! -f "$values_yaml" ]; then
  echo "Error: $values_yaml does not exist"
  exit 1
fi

sed -i "s/nccl_socket_ifname: \".*\"/nccl_socket_ifname: \"$ethernet_nic\"/" "$values_yaml"
sed -i "s/nccl_ib_hca: \".*\"/nccl_ib_hca: \"$rdma_nic\"/" "$values_yaml"
sed -i "s/replicas: [0-9]*/replicas: $replicas/" "$values_yaml"
sed -i "s/^.*cpu:.*/  cpu: $cpu/" "$values_yaml"
sed -i "s/^.*memory:.*/  memory: $memory/" "$values_yaml"
sed -i "s/^.*storage_class:.*/  storage_class: \"$storage_class\"/" "$values_yaml"
sed -i "s/^.*sub_domain:.*/  sub_domain: \"$sub_domain\"/" "$values_yaml"
sed -i '/opensearch:/,/^[a-z]/ s/enable: .*/enable: '"$opensearch_enable"'/' "$values_yaml"
sed -i '/s3:/,/^[a-z]/ s/enable: .*/enable: '"$s3_enable"'/' "$values_yaml"
if [[ "$s3_enable" == "true" ]]; then
  sed -i '/^s3:/,/^[a-z]/ s#endpoint: ".*"#endpoint: "'"$s3_endpoint"'"#' "$values_yaml"
fi

helm upgrade -i primus-pgo ./primus-pgo -n "$NAMESPACE" --create-namespace
echo "✅ Step 2.1: primus-pgo-5.8.2 installed"
echo "⏳ Step 2.2: Waiting for Postgres Operator pod..."
for i in {1..30}; do
  if kubectl get pods -n "$NAMESPACE" | grep $NAMESPACE-db | grep -q "Running"; then
    echo "✅ Postgres Operator is running."
    break
  fi
  echo "⏳ [$i/30] Waiting for postgres-operator..."
  sleep 5
done

helm upgrade -i primus-safe ./primus-safe -n "$NAMESPACE" --create-namespace
echo "✅ Step 2.3: primus-safe installed"
echo

helm upgrade -i primus-safe-cr ./primus-safe-cr -n "$NAMESPACE" --create-namespace
echo "✅ Step 2.4: primus-safe-cr installed"
echo

echo "========================================="
echo "🔧 Step 3: install primus-safe data plane"
echo "========================================="
cd ../node-agent/charts/
values_yaml="node-agent/values.yaml"

sed -i "s/nccl_socket_ifname: \".*\"/nccl_socket_ifname: \"$ethernet_nic\"/" "$values_yaml"
sed -i "s/nccl_ib_hca: \".*\"/nccl_ib_hca: \"$rdma_nic\"/" "$values_yaml"
sed -i "s/nfs_server: \".*\"/nfs_server: \"$nfs_server\"/" "$values_yaml"
sed -i "s#nfs_path: \".*\"#nfs_path: \"$nfs_path\"#" "$values_yaml"
sed -i "s#nfs_mount: \".*\"#nfs_mount: \"$nfs_mount\"#" "$values_yaml"

helm upgrade -i node-agent ./node-agent -n "$NAMESPACE" --create-namespace
echo "✅ Step 3.1: node-agent installed"

if [[ "$support_lens" == "y" ]]; then
  export STORAGE_CLASS="$storage_class"
  bash install-grafana.sh
  echo "✅ Step 3.2: grafana installed"
fi
