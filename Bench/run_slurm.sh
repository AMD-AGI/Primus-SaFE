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
    EXCLUDE_NODES       Nodes to exclude from allocation (default: chi[2770-2772,...])

Examples:

    # Run with automatic resource allocation (default)
    NNODES=2 bash $(basename "$0")
    
    # Run without auto-allocation (requires existing SLURM job)
    bash $(basename "$0") --no-allocate
    
    # Run with custom exclude list
    EXCLUDE_NODES="chi[2770-2772]" bash $(basename "$0")
    
    # Run without excluding any nodes
    EXCLUDE_NODES="" bash $(basename "$0")

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

# Override NNODES default to 2 for slurm
export NNODES=${NNODES:-2}
export PRIMUSBENCH_PATH="${PRIMUSBENCH_PATH:-$(pwd)}"

# Nodes to exclude from allocation (can be overridden by environment variable)
export EXCLUDE_NODES="${EXCLUDE_NODES:-}"

# Create log directory
LOG_FILE="${LOG_DIR}/log_slurm.txt"
mkdir -p "$LOG_DIR"

# Function to run the actual benchmark
run_benchmark() {
    srun -N "${NNODES}" \
        --exclusive \
        --export ALL \
        --ntasks-per-node=1 \
        bash -c "
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
    echo "[INFO] Using salloc to allocate ${NNODES} nodes..."
    echo "[INFO] Excluding nodes: ${EXCLUDE_NODES}"
    salloc --exclusive \
        --ntasks-per-node=1 \
        --cpus-per-task=$CPUS_PER_TASK \
        --partition=$PARTITION \
        --exclude="${EXCLUDE_NODES}" \
        -N $NNODES \
        -t $TIME \
        bash -c "
            echo '[INFO] Nodes allocated:'
            scontrol show hostnames \$SLURM_JOB_NODELIST
            echo ''
            
            # Run the benchmark within the allocation
            cd ${PRIMUSBENCH_PATH}
            $(declare -f run_benchmark)
            run_benchmark $@
        "
else
    # Check if we're already in a SLURM allocation
    if [ -n "$SLURM_JOB_ID" ]; then
        echo "[INFO] Already in SLURM allocation (Job ID: $SLURM_JOB_ID)"
        echo "[INFO] Running benchmark directly with srun..."
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