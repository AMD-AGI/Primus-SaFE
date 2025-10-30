#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

SCRIPT_DIR=$(dirname "$(realpath "${BASH_SOURCE[0]}")")
PRIMUS_PATH=$(realpath "$(dirname "$0")/../..")
export PRIMUS_PATH

node_list=$(scontrol show hostnames "$SLURM_JOB_NODELIST")
mapfile -t node_array <<<"$node_list"
HEAD_NODE=${node_array[0]}
export SLURM_MASTER_ADDR=$HEAD_NODE
export SLURM_MASTER_PORT=${SLURM_MASTER_PORT:-12346}
export SLURM_GPUS_ON_NODE=${SLURM_GPUS_ON_NODE:-8}
export SLURM_WORLD_SIZE=$((SLURM_NNODES * SLURM_GPUS_ON_NODE))
export MASTER_ADDR=${SLURM_MASTER_ADDR}
export MASTER_PORT=${SLURM_MASTER_PORT}
export NNODES=${SLURM_NNODES}
export NODE_RANK=${SLURM_NODEID}
export GPUS_PER_NODE=$((SLURM_WORLD_SIZE / SLURM_NNODES))
gpus=$(seq -s, 0 $((GPUS_PER_NODE - 1)))
export HIP_VISIBLE_DEVICES=$gpus

# Note: Modify specific network configurations according to the current cluster
export NCCL_IB_HCA="rdma0,rdma1,rdma2,rdma3,rdma4,rdma5,rdma6,rdma7"
export IP_INTERFACE=eno0

# Enable high-speed DMA transfers on AMD GPUs
export HSA_ENABLE_SDMA=1  # Enable system DMA (SDMA) engine for better GPU IO throughput
# Prevent scratch memory space from being reclaimed
export HSA_NO_SCRATCH_RECLAIM=1  # Helps stabilize large memory usage patterns (e.g. KV cache, MoE experts)
export NCCL_IB_GID_INDEX=3
export NCCL_CROSS_NIC=0
export NCCL_IB_GDR_LEVEL=2
export NCCL_NET_GDR_LEVEL=2
export NCCL_SOCKET_IFNAME=${NCCL_SOCKET_IFNAME:-${IP_INTERFACE}}
export GLOO_SOCKET_IFNAME=${GLOO_SOCKET_IFNAME:-${IP_INTERFACE}}
export CUDA_DEVICE_MAX_CONNECTIONS=1 # Reducing to 1 ensures no PCIE traffic (even on single node)
export RCCL_MSCCL_ENABLE=0
export NCCL_CHECKS_DISABLE=1
export OMP_NUM_THREADS=1
export GPU_MAX_HW_QUEUES=2
export TORCH_NCCL_HIGH_PRIORITY=1
# VERSION, WARN, INFO, DEBUG
export NCCL_DEBUG="VERSION"

if [ "$SLURM_NODEID" = "0" ]; then
    echo "==========Slurm cluster info=========="
    echo "[SLURM-NODE-$SLURM_NODEID] NODELIST=${node_array[*]}"
    echo "[SLURM-NODE-$SLURM_NODEID] NODENAME=$SLURMD_NODENAME"
    echo "[SLURM-NODE-$SLURM_NODEID] SLURM_MASTER_ADDR=$SLURM_MASTER_ADDR"
    echo "[SLURM-NODE-$SLURM_NODEID] SLURM_MASTER_PORT=$SLURM_MASTER_PORT"
    echo "[SLURM-NODE-$SLURM_NODEID] SLURM_NNODES=$SLURM_NNODES"
    echo "[SLURM-NODE-$SLURM_NODEID] SLURM_GPUS_ON_NODE=$SLURM_GPUS_ON_NODE"
    echo "[SLURM-NODE-$SLURM_NODEID] SLURM_WORLD_SIZE=$SLURM_WORLD_SIZE"
    echo "[SLURM-NODE-$SLURM_NODEID] SLURM_CPUS_PER_TASK: $SLURM_CPUS_PER_TASK"
    echo "[SLURM-NODE-$SLURM_NODEID] SLURM_PROCID: $SLURM_PROCID"
    echo ""
fi

if [ "$NODE_RANK" = "0" ]; then
    echo "==========Preflight cluster info=========="
    echo "[NODE-$NODE_RANK] MASTER_ADDR: $MASTER_ADDR"
    echo "[NODE-$NODE_RANK] MASTER_PORT: $MASTER_PORT"
    echo "[NODE-$NODE_RANK] NCCL_IB_HCA: $NCCL_IB_HCA"
    echo "[NODE-$NODE_RANK] IP_INTERFACE: $IP_INTERFACE"
    echo "[NODE-$NODE_RANK] NNODES: $NNODES"
    echo "[NODE-$NODE_RANK] NODE_RANK: $NODE_RANK"
    echo "[NODE-$NODE_RANK] GPUS_PER_NODE: $GPUS_PER_NODE"
    echo "[NODE-$NODE_RANK] HIP_VISIBLE_DEVICES: $HIP_VISIBLE_DEVICES"
    echo ""

    echo "docker: $PREFLIGHT_NETWORK_IMAGE"
fi

docker ps -aq | xargs -r docker rm -f
echo "Node-${NODE_RANK} $(hostname): Clean docker containers..."

docker_podman_proxy() {
    if command -v podman &>/dev/null; then
        podman "$@"
    elif command -v docker &>/dev/null; then
        docker "$@"
    else
        echo "Neither Docker nor Podman found!" >&2
        return 1
    fi
}

CLEAN_DOCKER_CONTAINER=${CLEAN_DOCKER_CONTAINER:-1}
if [[ "${CLEAN_DOCKER_CONTAINER:-0}" == "1" ]]; then
    echo "Node-${NODE_RANK}: Cleaning up existing containers..."
    CONTAINERS=$(docker_podman_proxy ps -aq)
    if [[ -n "$CONTAINERS" ]]; then
        for cid in $CONTAINERS; do
            docker_podman_proxy rm -f "$cid"
        done
        echo "Node-${NODE_RANK}: Removed containers: $CONTAINERS"
    else
        echo "Node-${NODE_RANK}: No containers to remove."
    fi
fi

docker_podman_proxy run --rm \
    --env SLURM_MASTER_ADDR=$SLURM_MASTER_ADDR \
    --env SLURM_MASTER_PORT=$SLURM_MASTER_PORT \
    --env SLURM_PROCID=$SLURM_PROCID \
    --env SLURM_WORLD_SIZE=$SLURM_WORLD_SIZE \
    --env SLURM_NODEID=$SLURM_NODEID \
    --env SLURM_NNODES=$SLURM_NNODES \
    --env MASTER_ADDR=${MASTER_ADDR} \
    --env MASTER_PORT=${MASTER_PORT} \
    --env NNODES=${NNODES} \
    --env NODE_RANK=${NODE_RANK} \
    --env WORLD_SIZE=${NNODES} \
    --env RANK=${NODE_RANK} \
    --env GPUS_PER_NODE=${GPUS_PER_NODE} \
    --env HIP_VISIBLE_DEVICES=$HIP_VISIBLE_DEVICES \
    --env OMP_NUM_THREADS=$OMP_NUM_THREADS \
    --env GPU_MAX_HW_QUEUES=$GPU_MAX_HW_QUEUES \
    --env TORCH_NCCL_HIGH_PRIORITY=$TORCH_NCCL_HIGH_PRIORITY \
    --env NCCL_DEBUG=$NCCL_DEBUG \
    --env NCCL_CHECKS_DISABLE=$NCCL_CHECKS_DISABLE \
    --env NCCL_IB_GDR_LEVEL=2 \
    --env NCCL_NET_GDR_LEVEL=2 \
    --env NCCL_IB_HCA=$NCCL_IB_HCA \
    --env NCCL_IB_GID_INDEX=$NCCL_IB_GID_INDEX \
    --env NCCL_CROSS_NIC=$NCCL_CROSS_NIC \
    --env HSA_ENABLE_SDMA=$HSA_ENABLE_SDMA \
    --env NCCL_SOCKET_IFNAME=$NCCL_SOCKET_IFNAME \
    --env GLOO_SOCKET_IFNAME=$GLOO_SOCKET_IFNAME \
    --env CUDA_DEVICE_MAX_CONNECTIONS=$CUDA_DEVICE_MAX_CONNECTIONS \
    --env RCCL_MSCCL_ENABLE=$RCCL_MSCCL_ENABLE \
    --env ENABLE_NODE_OUTPUT=true \
    --env BNIC=48 \
    --env BXGMI=315 \
    --env MAX_RETRY=3 \
    --env SSH_PORT=$SSH_PORT \
    --ipc=host --network=host \
    --device=/dev/kfd --device=/dev/dri \
    --cap-add=SYS_PTRACE --cap-add=CAP_SYS_ADMIN \
    --security-opt seccomp=unconfined --group-add video \
    --privileged --device=/dev/infiniband \
    -v $PRIMUS_PATH:$PRIMUS_PATH \
    $PREFLIGHT_NETWORK_IMAGE /bin/bash -c "cd $PRIMUS_PATH/network && bash run.sh"