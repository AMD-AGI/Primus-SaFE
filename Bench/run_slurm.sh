#!/bin/bash
###############################################################################
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#
# See LICENSE for license information.
###############################################################################
export IMAGE=${IMAGE:-primussafe/superbench:202408191128}
export CONFIG=${CONFIG:-amd_mi300.yaml}
PORT=$((( RANDOM % 10000 ) + 20000 ))

HOSTS=("$@")
all_node_list=$(scontrol show hostnames "$SLURM_JOB_NODELIST")

declare -A node_ip_map
declare -A ip_node_map

for n in $all_node_list; do
  ip=$(getent hosts "$n" | awk '{print $1}')
  node_ip_map[$n]=$ip
  ip_node_map[$ip]=$n
done

for n in "${!node_ip_map[@]}"; do
  echo "$n -> ${node_ip_map[$n]}"
done

for n in "${!ip_node_map[@]}"; do
  echo "$n -> ${ip_node_map[$n]}"
done

TIMESTMAP=$(date +'%Y-%m-%d_%H-%M-%S')
PRIMUSBENCH_PATH=$(pwd)
if [ -z "$OUTPUT_PATH" ]; then
    if [ -n "$SHARE_PATH" ]; then
        OUTPUT_PATH="$SHARE_PATH/output/$TIMESTMAP"
    else
        OUTPUT_PATH="$PRIMUSBENCH_PATH/output/$TIMESTMAP"
    fi
fi
mkdir -p $OUTPUT_PATH
export PREFLIGHT_NODE_IMAGE=${PREFLIGHT_NODE_IMAGE:-"docker.io/primussafe/diagnose_node:202509222007"}
preflightr_node_logname=${OUTPUT_PATH}/preflight_node.log
srun --export=ALL,DOCKER_IMAGE=${PREFLIGHT_NODE_IMAGE} \
    bash $PRIMUSBENCH_PATH/preflight/node/slurm/start_docker.sh 2>&1 | tee $preflightr_node_logname

nodes=($(awk '/All check passed/ {gsub(/[\[\]:]/,""); print $2}' $preflightr_node_logname))
node_list=$(IFS=,; echo "${nodes[*]}")
echo "nodes---${nodes[@]}"
echo "node_list---${node_list}"

preflight_network_logname=${OUTPUT_PATH}/preflight_network.log
export PREFLIGHT_NETWORK_IMAGE=${PREFLIGHT_NETWORK_IMAGE:-"docker.io/primussafe/diagnose_network:202509222007"}
srun --export=ALL,DOCKER_IMAGE=${PREFLIGHT_NETWORK_IMAGE},SSH_PORT=$(( RANDOM % 9999 + 30001 )) \
  --nodelist=${node_list} \
    bash $PRIMUSBENCH_PATH/preflight/network/slurm/start_docker.sh 2>&1 | tee $preflight_network_logname

ips=($(awk '/Final unhealthy nodes/{getline; print}' $preflight_network_logname))

if [ ${#ips[@]} -gt 0 ]; then
    echo "Found unhealthy nodes:"
    for ip in "${ips[@]}"; do
        echo "  $ip"
    done
fi

declare -A unhealthy
while read -r line; do
    if [[ "$line" =~ \[(ERROR|WARNING)\] ]]; then
        node=$(echo "$line" | grep -oP '\[NODE-[0-9]+: \K[^\]]+')
        if [[ -n "$node" ]]; then
            msg=$(echo "$line" | sed -E 's/.*\] (❌: )?//')
            unhealthy["$node"]="$msg"
        fi
    fi
done < "$preflight_node_logname"

echo "unhealthy nodes :"
for n in "${!unhealthy[@]}"; do
    echo "$n -> ${unhealthy[$n]}"
done
