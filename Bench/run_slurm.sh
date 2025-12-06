#!/bin/bash
###############################################################################
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#
# See LICENSE for license information.
###############################################################################

# Source unified configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/config.sh"

print_usage() {
cat << EOF
Usage: bash $(basename "$0") [--help]

Environment variables (configured in config.sh, can be overridden):

    NNODES              Number of nodes (default: ${NNODES})
    MASTER_PORT         Master port (default: ${MASTER_PORT})
    IMAGE               Container image (default: ${IMAGE})
    NCCL_SOCKET_IFNAME  NCCL socket interface (default: ${NCCL_SOCKET_IFNAME})
    GLOO_SOCKET_IFNAME  Gloo socket interface (default: ${GLOO_SOCKET_IFNAME})

Example:

    NNODES=2 bash $(basename "$0")

EOF
}

# Override NNODES default to 2 for slurm
export NNODES=${NNODES:-2}
export PRIMUSBENCH_PATH="${PRIMUSBENCH_PATH:-$(pwd)}"



LOG_FILE="${LOG_DIR}/log_slurm.txt"
mkdir -p "$LOG_DIR"

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
        export NCCL_SOCKET_IFNAME=\${NCCL_SOCKET_IFNAME}
        export GLOO_SOCKET_IFNAME=\${GLOO_SOCKET_IFNAME}
        export HF_TOKEN=\${HF_TOKEN}
        cd ${PRIMUSBENCH_PATH} && bash run_local.sh \"\$@\" 2>&1 | tee ${LOG_FILE}
    " bash "$@"