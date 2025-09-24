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

read -rp "Enter ethernet nic(e.g., eno0): " ethernet_nic
read -rp "Enter rdma nic(e.g., rdma0,rdma1...): " rdma_nic
read -rp "Enter cluster scale(small/medium/large): " cluster_scale
read -rp "Enter storage class(e.g., rbd): " storage_class
read -rp "Enter sub domain(e.g., tas): " sub_domain
read -rp "Enter gpu nfs-server or press Enter to skip: " nfs_server
read -rp "Enter gpu nfs-path or press Enter to skip: " nfs_path
read -rp "Enter gpu nfs-mount or press Enter to skip: " nfs_mount
read -rp "Support Primus-lens? (y/n): " support_lens

ethernet_nic=$(echo "$ethernet_nic" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
rdma_nic=$(echo "$rdma_nic" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
storage_class=$(echo "$storage_class" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
sub_domain=$(echo "$sub_domain" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
nfs_server=$(echo "$nfs_server" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
nfs_path=$(echo "$nfs_path" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
nfs_mount=$(echo "$nfs_mount" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')

if [ -z "$ethernet_nic" ] || [ -z "$rdma_nic" ] || [ -z "$storage_class" ] || [ -z "$sub_domain" ]; then
  echo "Error: Please input again."
  exit 1
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

shopt -s nocasematch
log_enable=true
if [[ "$support_lens" != "y" ]]; then
  log_enable=false
fi

replicas=1
cpu=2000m
memory=4Gi
if [[ "$cluster_scale" == "medium" ]]; then
  replicas=2
  cpu=8000m
  memory=16Gi
elif [[ "$cluster_scale" == "large" ]]; then
  replicas=2
  cpu=64000m
  memory=64Gi
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
sed -i '/log:/,/^[a-z]/ s/enable: .*/enable: '"$log_enable"'/' values.yaml

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

helm upgrade -i primus-safe-cr ./primus-safe-cr -n "$NAMESPACE" --create-namespace
echo "✅ Step 2.4: primus-safe-cr installed"

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