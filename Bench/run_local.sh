#!/bin/bash
###############################################################################
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#
# See LICENSE for license information.
###############################################################################

set -e

# Source unified configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/config.sh"

# ------------------ Usage Help ------------------

print_usage() {
cat <<EOF
Usage: bash run_local.sh

This script launches a Primus bench task inside a Docker/Podman container.

Environment Variables (configured in config.sh, can be overridden):
    IMAGE           Docker image [Default: ${IMAGE}]
    MASTER_ADDR     Master node IP or hostname [Default: ${MASTER_ADDR}]
    MASTER_PORT     Master node port [Default: ${MASTER_PORT}]
    NNODES          Total number of nodes [Default: ${NNODES}]
    NODE_RANK       Rank of this node [Default: 0]
    GPUS_PER_NODE   GPUs per node [Default: ${GPUS_PER_NODE}]
    IP_INTERFACE    Network interface [Default: ${IP_INTERFACE}]
    PRIMUS_*        Any environment variable prefixed with PRIMUS_ will be passed into the container.

Example:
   bash run_local.sh

EOF
}

if [[ "$1" == "--help" || "$1" == "-h" ]]; then
    print_usage
    exit 0
fi

export PRIMUSBENCH_PATH="${PRIMUSBENCH_PATH:-$(pwd)}"
NODE_RANK=${NODE_RANK:-0}

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

if [[ "${CLEAN_DOCKER_CONTAINER}" == "1" ]]; then
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

# GPU visibility (all environment variables already configured via config.sh)
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
    --env NCCL_IB_GDR_LEVEL=${NCCL_IB_GDR_LEVEL} \
    --env NCCL_NET_GDR_LEVEL=${NCCL_NET_GDR_LEVEL} \
    --env NCCL_IB_HCA=${NCCL_IB_HCA} \
    --env NCCL_IB_GID_INDEX=${NCCL_IB_GID_INDEX} \
    --env NCCL_CROSS_NIC=${NCCL_CROSS_NIC} \
    --env HSA_ENABLE_SDMA=${HSA_ENABLE_SDMA} \
    --env HSA_NO_SCRATCH_RECLAIM=${HSA_NO_SCRATCH_RECLAIM} \
    --env NCCL_SOCKET_IFNAME=${NCCL_SOCKET_IFNAME} \
    --env GLOO_SOCKET_IFNAME=${GLOO_SOCKET_IFNAME} \
    --env CUDA_DEVICE_MAX_CONNECTIONS=${CUDA_DEVICE_MAX_CONNECTIONS} \
    --env RCCL_MSCCL_ENABLE=${RCCL_MSCCL_ENABLE} \
    --env SSH_PORT=${SSH_PORT} \
    --env ADD_LOG_HEADER=${ADD_LOG_HEADER} \
    --env BNIC=${BNIC} \
    --env BXGMI=${BXGMI} \
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