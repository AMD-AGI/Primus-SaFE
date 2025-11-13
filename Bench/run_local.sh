#!/bin/bash
###############################################################################
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#
# See LICENSE for license information.
###############################################################################

set -e

# ------------------ Usage Help ------------------

print_usage() {
cat <<EOF
Usage: bash run_local.sh

This script launches a Primus bench task inside a Docker/Podman container.

Environment Variables:
    IMAGE   Docker image to use [Default: primussafe/primusbench:202510221446]
    MASTER_ADDR    Master node IP or hostname [Default: localhost]
    MASTER_PORT    Master node port [Default: 1234]
    NNODES         Total number of nodes [Default: 1]
    NODE_RANK      Rank of this node [Default: 0]
    GPUS_PER_NODE  GPUs per node [Default: 8]
    PRIMUS_*       Any environment variable prefixed with PRIMUS_ will be passed into the container.

Example:
   bash run_local.sh

EOF
}

if [[ "$1" == "--help" || "$1" == "-h" ]]; then
    print_usage
    exit 0
fi

IMAGE=${IMAGE:-"primussafe/primusbench:202510221446"}

export PRIMUSBENCH_PATH=$(pwd)
MASTER_ADDR=${MASTER_ADDR:-localhost}
MASTER_PORT=${MASTER_PORT:-12345}
NNODES=${NNODES:-1}
NODE_RANK=${NODE_RANK:-0}
GPUS_PER_NODE=${GPUS_PER_NODE:-8}

if [ "$NODE_RANK" = "0" ]; then
    echo "========== Cluster info =========="
    echo "MASTER_ADDR: $MASTER_ADDR"
    echo "MASTER_PORT: $MASTER_PORT"
    echo "NNODES: $NNODES"
    echo "GPUS_PER_NODE: $GPUS_PER_NODE"
    echo ""
fi

HOSTNAME=$(hostname)
ARGS=("$@")

VOLUME_ARGS=(-v "$PRIMUSBENCH_PATH":"$PRIMUSBENCH_PATH")

export CLEAN_DOCKER_CONTAINER=${CLEAN_DOCKER_CONTAINER:-0}

# ------------------ Optional Container Cleanup ------------------
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

# Note: Modify specific network configurations according to the current cluster
export NCCL_IB_HCA=${NCCL_IB_HCA:-"bnxt_re0:1,bnxt_re1:1,bnxt_re2:1,bnxt_re3:1,bnxt_re4:1,bnxt_re5:1,bnxt_re6:1,bnxt_re7:1"}
export IP_INTERFACE=${IP_INTERFACE:-"ens51f0"}
export GPU_PRODUCT=${GPU_PRODUCT:-"MI325X"}

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

HIP_VISIBLE_DEVICES=$(seq -s, 0 $((GPUS_PER_NODE - 1)))
export HIP_VISIBLE_DEVICES

docker_podman_proxy run \
    --env MASTER_ADDR=${MASTER_ADDR} \
    --env MASTER_PORT=${MASTER_PORT} \
    --env NNODES=${NNODES} \
    --env NODE_RANK=${NODE_RANK} \
    --env WORLD_SIZE=${NNODES} \
    --env RANK=${NODE_RANK} \
    --env GPUS_PER_NODE=${GPUS_PER_NODE} \
    --env OMP_NUM_THREADS=${OMP_NUM_THREADS} \
    --env GPU_MAX_HW_QUEUES=${GPU_MAX_HW_QUEUES} \
    --env TORCH_NCCL_HIGH_PRIORITY=${TORCH_NCCL_HIGH_PRIORITY} \
    --env NCCL_DEBUG=${NCCL_DEBUG} \
    --env NCCL_CHECKS_DISABLE=${NCCL_CHECKS_DISABLE} \
    --env NCCL_IB_GDR_LEVEL=2 \
    --env NCCL_NET_GDR_LEVEL=2 \
    --env NCCL_IB_HCA=${NCCL_IB_HCA} \
    --env NCCL_IB_GID_INDEX=${NCCL_IB_GID_INDEX} \
    --env NCCL_CROSS_NIC=${NCCL_CROSS_NIC} \
    --env HSA_ENABLE_SDMA=${HSA_ENABLE_SDMA} \
    --env NCCL_SOCKET_IFNAME=${NCCL_SOCKET_IFNAME} \
    --env GLOO_SOCKET_IFNAME=${GLOO_SOCKET_IFNAME} \
    --env CUDA_DEVICE_MAX_CONNECTIONS=${CUDA_DEVICE_MAX_CONNECTIONS} \
    --env RCCL_MSCCL_ENABLE=${RCCL_MSCCL_ENABLE} \
    --env SSH_PORT=${SSH_PORT} \
    --env ADD_LOG_HEADER=true \
    --env BNIC=50 \
    --env BXGMI=315 \
    --ipc=host --network=host --pid=host \
    --device=/dev/kfd --device=/dev/dri \
    --cap-add=SYS_PTRACE --cap-add=CAP_SYS_ADMIN \
    --security-opt seccomp=unconfined --group-add video \
    --privileged --device=/dev/infiniband \
    "${VOLUME_ARGS[@]}" \
    "$IMAGE" /bin/bash -c "\
        echo '[NODE-${NODE_RANK}(${HOSTNAME})]: begin, time=$(date +"%Y.%m.%d %H:%M:%S")' && \
        cd $PRIMUSBENCH_PATH && \
        bash run.sh \"\$@\" 2>&1 && \
        echo '[NODE-${NODE_RANK}(${HOSTNAME})]: end, time=$(date +"%Y.%m.%d %H:%M:%S")'
    " bash "${ARGS[@]}"