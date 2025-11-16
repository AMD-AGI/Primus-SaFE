#!/bin/bash
###############################################################################
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#
# See LICENSE for license information.
###############################################################################

print_usage() {
cat << EOF
Usage: bash $(basename "$0") [--help]

Environment variables (must set before running):

    NNODES=1                                    # Number of nodes (default: 1)
    MASTER_PORT=12345     Master port [default: 12345]
    IMAGE=primussafe/primusbench:202510221446   # Image of SuperBench (default: primussafe/primusbench:202510221446)
    NCCL_SOCKET_IFNAME=eno0                     # NCCL socket interface name (default: eno0)
    GLOO_SOCKET_IFNAME=rdma0,rdma1,rdma2,rdma3,rdma4,rdma5,rdma6,rdma7                      # Gloo socket interface name (default: rdma0,rdma1,rdma2,rdma3,rdma4,rdma5,rdma6,rdma7)
Example:

    NNODES=2 IMAGE=primussafe/primusbench:202510221446 bash salloc_slurm.sh

EOF
}

export NNODES=${NNODES:-2}
export IMAGE=${IMAGE:-primussafe/primusbench:202510221446}
export PARTITION=${PARTITION:-amd-tw}
export TIME=${TIME:-4:30:00}
export MASTER_PORT=${MASTER_PORT:-12345}
export SSH_PORT=$(( RANDOM % 9999 + 30001 ))
export PRIMUSBENCH_PATH=$(pwd)
export LOG_DIR=${LOG_DIR:-"./outputs"}
LOG_FILE="${LOG_DIR}/log_slurm.txt"
mkdir -p "$LOG_DIR"

srun -N "${NNODES}" \
    --exclusive \
    --export ALL \
    --ntasks-per-node=1 \
    --cpus-per-task="${CPUS_PER_TASK:-256}" \
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
        cd ${PRIMUSBENCH_PATH} && bash run_local.sh \"\$@\" 2>&1 | tee ${LOG_FILE}
    " bash "$@"