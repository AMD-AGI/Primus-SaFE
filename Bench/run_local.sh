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

# ------------------ Helper Functions ------------------

# Function to attempt normal container cleanup
normal_clean_container() {
    local container_id="$1"
    
    # Method 1: Direct force remove
    if docker_podman_proxy rm -f "$container_id" 2>/dev/null; then
        return 0
    fi
    
    # Method 2: Stop then remove
    docker_podman_proxy stop "$container_id" 2>/dev/null || true
    sleep 2
    if docker_podman_proxy rm -f "$container_id" 2>/dev/null; then
        return 0
    fi
    
    # Method 3: Kill the process directly
    local pid=$(docker_podman_proxy inspect -f '{{.State.Pid}}' "$container_id" 2>/dev/null || echo "")
    if [[ -n "$pid" ]] && [[ "$pid" != "0" ]] && [[ "$pid" != "null" ]]; then
        sudo kill -9 "$pid" 2>/dev/null || true
        sleep 2
        if docker_podman_proxy rm -f "$container_id" 2>/dev/null; then
            return 0
        fi
    fi
    
    return 1
}

# Function for aggressive cleanup when normal cleanup fails
aggressive_clean_all() {
    echo "Node-${NODE_RANK}: Normal cleanup failed, attempting AGGRESSIVE cleanup..."
    
    # Step 1: Stop all containers
    echo "Node-${NODE_RANK}: Force stopping all containers..."
    docker_podman_proxy stop $(docker_podman_proxy ps -aq) 2>/dev/null || true
    sleep 2
    
    # Step 2: Kill all containers
    echo "Node-${NODE_RANK}: Killing all container processes..."
    docker_podman_proxy kill $(docker_podman_proxy ps -aq) 2>/dev/null || true
    sleep 2
    
    # Step 3: Force remove each container with extreme measures
    local containers=$(docker_podman_proxy ps -aq)
    local all_removed=true
    
    for container in $containers; do
        echo "Node-${NODE_RANK}: Force removing container $container..."
        
        # Try standard force remove
        if docker_podman_proxy rm -f "$container" 2>/dev/null; then
            echo "Node-${NODE_RANK}:   ✓ Removed $container"
            continue
        fi
        
        # Get PID and kill directly
        local pid=$(docker_podman_proxy inspect -f '{{.State.Pid}}' "$container" 2>/dev/null || echo "")
        if [[ -n "$pid" ]] && [[ "$pid" != "0" ]] && [[ "$pid" != "null" ]]; then
            echo "Node-${NODE_RANK}:   Killing PID $pid..."
            sudo kill -9 "$pid" 2>/dev/null || true
            sleep 1
        fi
        
        # Try remove again
        if docker_podman_proxy rm -f "$container" 2>/dev/null; then
            echo "Node-${NODE_RANK}:   ✓ Removed $container after killing process"
            continue
        fi
        
        # Last resort - manual filesystem removal
        local container_dir=""
        if command -v docker &> /dev/null && docker version &> /dev/null; then
            container_dir="/var/lib/docker/containers/${container}"
        elif command -v podman &> /dev/null; then
            container_dir="/var/lib/containers/storage/overlay-containers/${container}"
        fi
        
        if [[ -n "$container_dir" ]] && [[ -d "$container_dir" ]]; then
            echo "Node-${NODE_RANK}:   WARNING: Manually removing container directory..."
            sudo rm -rf "$container_dir" 2>/dev/null || true
            
            # Try remove from docker again
            if docker_podman_proxy rm -f "$container" 2>/dev/null; then
                echo "Node-${NODE_RANK}:   ✓ Removed $container after filesystem cleanup"
                continue
            fi
        fi
        
        echo "Node-${NODE_RANK}:   ✗ Failed to remove $container (requires Docker restart)"
        all_removed=false
    done
    
    # Step 4: Clean up leftover mounts
    echo "Node-${NODE_RANK}: Cleaning up leftover mounts..."
    mount | grep -E "overlay|shm|docker|podman|containers" | awk '{print $3}' | while read mount_point; do
        if [[ -n "$mount_point" ]] && [[ "$mount_point" != "/" ]]; then
            sudo umount -f "$mount_point" 2>/dev/null || true
        fi
    done
    
    # Step 5: System prune
    echo "Node-${NODE_RANK}: Running system prune..."
    docker_podman_proxy system prune -a -f --volumes 2>/dev/null || true
    
    if [[ "$all_removed" == true ]]; then
        echo "Node-${NODE_RANK}: ✓ Aggressive cleanup successful"
        return 0
    else
        echo "Node-${NODE_RANK}: ⚠ Some containers still remain, Docker restart required"
        return 1
    fi
}

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
        # First attempt: Normal cleanup for each container
        failed_containers=""
        removed_containers=""
        
        echo "Node-${NODE_RANK}: Attempting normal cleanup..."
        for cid in $CONTAINERS; do
            if normal_clean_container "$cid"; then
                removed_containers="$removed_containers $cid"
                echo "Node-${NODE_RANK}:   ✓ Removed container $cid"
            else
                failed_containers="$failed_containers $cid"
                echo "Node-${NODE_RANK}:   ✗ Failed to remove container $cid"
            fi
        done
        
        # Report normal cleanup results
        if [[ -n "$removed_containers" ]]; then
            echo "Node-${NODE_RANK}: Normal cleanup removed:$removed_containers"
        fi
        
        # If any containers failed, automatically try aggressive cleanup
        if [[ -n "$failed_containers" ]]; then
            echo "Node-${NODE_RANK}: Failed containers detected:$failed_containers"
            echo ""
            
            # Automatic aggressive cleanup
            if aggressive_clean_all; then
                echo "Node-${NODE_RANK}: ✓ All containers cleaned after aggressive cleanup"
            else
                echo "Node-${NODE_RANK}: ⚠ WARNING: Some containers may still exist"
                echo "Node-${NODE_RANK}: Recommended actions:"
                echo "Node-${NODE_RANK}:   1. Run: sudo systemctl restart docker"
                echo "Node-${NODE_RANK}:   2. Or reboot the system"
                # Don't exit - try to continue with benchmark
            fi
        else
            echo "Node-${NODE_RANK}: ✓ All containers cleaned successfully with normal cleanup"
        fi
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