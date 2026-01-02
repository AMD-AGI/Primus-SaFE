#!/bin/bash
###############################################################################
# Copyright (c) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
#
# See LICENSE for license information.
###############################################################################

# Source unified configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/config.sh"

print_usage() {
cat << EOF
Usage: bash $(basename "$0") [options]

Options:
    --allocate, -a      Use salloc to allocate resources (default behavior)
    --no-allocate       Disable auto-allocation (requires existing SLURM job)
    --warmup[=VALUE]    Control container image warmup (true/false, on/off, yes/no)
                        Default: true (enabled)
    --help, -h          Show this help message

Environment variables (configured in config.sh, can be overridden):

    NNODES              Number of nodes (default: ${NNODES})
    PARTITION           SLURM partition (default: ${PARTITION})
    TIME                Job time limit (default: ${TIME})
    CPUS_PER_TASK       CPUs per task (default: ${CPUS_PER_TASK})
    MASTER_PORT         Master port (default: ${MASTER_PORT})
    IMAGE               Container image (default: ${IMAGE})
    NCCL_SOCKET_IFNAME  NCCL socket interface (default: ${NCCL_SOCKET_IFNAME})
    GLOO_SOCKET_IFNAME  Gloo socket interface (default: ${GLOO_SOCKET_IFNAME})
    NODELIST            Specific nodes to use (e.g., chi[2770-2772])
    EXCLUDE_NODES       Nodes to exclude from allocation (ignored if NODELIST is set)
    ENABLE_IMAGE_WARMUP Enable container image warmup on all nodes (default: true)

Examples:

    # Run with automatic resource allocation and image warmup (default)
    NNODES=2 bash $(basename "$0")
    
    # Run without auto-allocation (requires existing SLURM job)
    bash $(basename "$0") --no-allocate
    
    # Run without image warmup for faster startup
    bash $(basename "$0") --warmup=false
    
    # Run with image warmup explicitly enabled
    bash $(basename "$0") --warmup=true
    
    # Alternative warmup control syntax
    bash $(basename "$0") --warmup=off
    bash $(basename "$0") --warmup=on
    
    # Run with custom exclude list
    EXCLUDE_NODES="chi[2770-2772]" bash $(basename "$0")
    
    # Run without excluding any nodes
    EXCLUDE_NODES="" bash $(basename "$0")
    
    # Run with specific nodelist
    NODELIST="chi[2770-2772,2774]" bash $(basename "$0")
    
    # Run with extended nodelist (example)
    NODELIST="chi[2770-2772,2774,2798]" bash $(basename "$0")

EOF
}

# Parse command line arguments
USE_SALLOC=true  # Default to true for automatic allocation
while [[ $# -gt 0 ]]; do
    case $1 in
        --allocate|-a)
            USE_SALLOC=true
            shift
            ;;
        --no-allocate)  # Add option to disable auto-allocation
            USE_SALLOC=false
            shift
            ;;
        --warmup=*)
            # Extract value after '='
            warmup_value="${1#*=}"
            case "${warmup_value,,}" in  # Convert to lowercase
                true|on|yes|1)
                    export ENABLE_IMAGE_WARMUP=true
                    ;;
                false|off|no|0)
                    export ENABLE_IMAGE_WARMUP=false
                    ;;
                *)
                    echo "[ERROR] Invalid warmup value: ${warmup_value}"
                    echo "       Valid values: true, false, on, off, yes, no, 1, 0"
                    exit 1
                    ;;
            esac
            shift
            ;;
        --warmup)
            # If no value provided, default to true
            export ENABLE_IMAGE_WARMUP=true
            shift
            ;;
        --help|-h)
            print_usage
            exit 0
            ;;
        *)
            # Pass through other arguments
            break
            ;;
    esac
done

# Override NNODES default to 2 for slurm (unless NODELIST is specified)
if [ -n "${NODELIST}" ]; then
    echo "[INFO] NODELIST is specified, NNODES will be determined by SLURM from the nodelist"
else
    export NNODES=${NNODES:-2}
fi
export PRIMUSBENCH_PATH="${PRIMUSBENCH_PATH:-$(pwd)}"

# Nodes to exclude from allocation (can be overridden by environment variable)
export EXCLUDE_NODES="${EXCLUDE_NODES:-}"

# Create log directory
LOG_FILE="${LOG_DIR}/log_slurm.txt"
mkdir -p "$LOG_DIR"

# Function to warm up container image on all nodes
warmup_image() {
    if [ "$ENABLE_IMAGE_WARMUP" != "true" ]; then
        echo "[INFO] Image warmup disabled (ENABLE_IMAGE_WARMUP=$ENABLE_IMAGE_WARMUP)"
        return 0
    fi
    
    echo "[INFO] Starting container image warmup on all nodes..."
    echo "[INFO] Pulling image: ${IMAGE}"
    echo "[INFO] Will wait until all nodes have the image (no timeout)"
    echo ""
    
    # Determine container runtime
    local container_cmd=""
    if command -v podman &> /dev/null; then
        container_cmd="podman"
    elif command -v docker &> /dev/null; then
        container_cmd="docker"
    else
        echo "[WARNING] No container runtime (docker/podman) found, skipping warmup"
        return 0
    fi
    
    # Build srun command for warmup
    WARMUP_SRUN_CMD="srun --exclusive --export ALL --ntasks-per-node=1"
    if [ -n "${NNODES}" ]; then
        WARMUP_SRUN_CMD="${WARMUP_SRUN_CMD} -N ${NNODES}"
    fi
    
    # Run warmup on all nodes in parallel (no timeout)
    ${WARMUP_SRUN_CMD} bash -c "
        hostname=\$(hostname)
        echo \"[\${hostname}] Starting image pull...\"
        start_time=\$(date +%s)
        
        # Check if image already exists first
        if ${container_cmd} images | grep -q \$(echo ${IMAGE} | cut -d: -f1); then
            echo \"[\${hostname}] ✓ Image already exists locally\"
            exit 0
        else
            # Try to pull the image
            if ${container_cmd} pull ${IMAGE} 2>&1; then
                end_time=\$(date +%s)
                duration=\$((end_time - start_time))
                echo \"[\${hostname}] ✓ Successfully pulled image in \${duration} seconds\"
                exit 0
            else
                echo \"[\${hostname}] ✗ ERROR: Failed to pull image\"
                exit 1
            fi
        fi
    "
    
    local warmup_status=$?
    
    # Handle exit status
    if [ $warmup_status -eq 0 ]; then
        echo ""
        if [ -n "${NNODES}" ]; then
            echo "[SUCCESS] ✓ Image warmup completed successfully on all ${NNODES} nodes"
        else
            echo "[SUCCESS] ✓ Image warmup completed successfully on all allocated nodes"
        fi
        return 0
    else
        echo ""
        echo "[ERROR] Image warmup failed on one or more nodes (status: ${warmup_status})"
        echo "[ERROR] Aborting: All nodes must successfully prepare the image"
        echo "[INFO] To skip warmup, use --warmup=false"
        return 1
    fi
}

# Function to run the actual benchmark
run_benchmark() {
    # Build srun command
    SRUN_CMD="srun --exclusive --export ALL --ntasks-per-node=1"
    
    # If NNODES is set, use it; otherwise srun will use all allocated nodes
    if [ -n "${NNODES}" ]; then
        SRUN_CMD="${SRUN_CMD} -N ${NNODES}"
    fi
    
    ${SRUN_CMD} bash -c "
            readarray -t node_array < <(scontrol show hostnames \"\$SLURM_JOB_NODELIST\")
            if [ \"\$SLURM_NODEID\" = \"0\" ]; then
                echo \"========== Slurm cluster info ==========\"
                echo \"SLURM_NODELIST: \${node_array[*]}\"
                echo \"SLURM_NNODES: \${SLURM_NNODES}\"
                echo \"SLURM_GPUS_ON_NODE: \${SLURM_GPUS_ON_NODE}\"
                echo \"\"
            fi
            export MASTER_ADDR=\${node_array[0]}
            export MASTER_PORT=\${MASTER_PORT}
            export NNODES=\${SLURM_NNODES}
            export NODE_RANK=\${SLURM_PROCID}
            export GPUS_PER_NODE=\${SLURM_GPUS_ON_NODE}
            export IMAGE=\${IMAGE}
            cd ${PRIMUSBENCH_PATH} && bash run_local.sh \"\$@\" 2>&1 | tee ${LOG_FILE}
        " bash "$@"
}

# Main execution
if [ "$USE_SALLOC" = true ]; then
    # Use salloc for interactive allocation
    if [ -n "${NODELIST}" ]; then
        echo "[INFO] Using salloc to allocate nodes from nodelist..."
    else
        echo "[INFO] Using salloc to allocate ${NNODES} nodes..."
    fi
    
    # Build salloc command based on whether NODELIST is specified
    SALLOC_CMD="salloc --exclusive --ntasks-per-node=1 --cpus-per-task=$CPUS_PER_TASK --partition=$PARTITION -t $TIME"
    
    if [ -n "${NODELIST}" ]; then
        echo "[INFO] Using specific nodes: ${NODELIST}"
        SALLOC_CMD="${SALLOC_CMD} --nodelist=${NODELIST}"
    else
        if [ -n "${EXCLUDE_NODES}" ]; then
            echo "[INFO] Excluding nodes: ${EXCLUDE_NODES}"
            SALLOC_CMD="${SALLOC_CMD} --exclude=${EXCLUDE_NODES}"
        fi
        SALLOC_CMD="${SALLOC_CMD} -N $NNODES"
    fi
    
    # Execute salloc with the built command
    ${SALLOC_CMD} bash -c "
            echo '[INFO] Nodes allocated:'
            scontrol show hostnames \$SLURM_JOB_NODELIST
            echo ''
            
            # Export functions and run warmup + benchmark
            cd ${PRIMUSBENCH_PATH}
            $(declare -f warmup_image)
            $(declare -f run_benchmark)
            
            # Warm up the image on all nodes
            if ! warmup_image; then
                echo '[ERROR] Image warmup failed. Exiting...'
                exit 1
            fi
            
            # Run the benchmark
            run_benchmark $@
        "
else
    # Check if we're already in a SLURM allocation
    if [ -n "$SLURM_JOB_ID" ]; then
        echo "[INFO] Already in SLURM allocation (Job ID: $SLURM_JOB_ID)"
        echo "[INFO] Running benchmark directly with srun..."
        
        # Warm up the image on all nodes
        if ! warmup_image; then
            echo "[ERROR] Image warmup failed. Exiting..."
            exit 1
        fi
        
        # Run the benchmark
        run_benchmark "$@"
    else
        echo "[ERROR] Not in a SLURM allocation and --no-allocate flag was used."
        echo "Please either:"
        echo "  1. Submit this script with sbatch"
        echo "  2. Run without --no-allocate flag (default behavior uses salloc)"
        echo "  3. Manually allocate nodes with salloc first"
        exit 1
    fi
fi