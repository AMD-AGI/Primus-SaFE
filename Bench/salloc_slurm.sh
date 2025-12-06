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
    PARTITION           SLURM partition (default: ${PARTITION})
    TIME                Job time limit (default: ${TIME})
    CPUS_PER_TASK       CPUs per task (default: ${CPUS_PER_TASK})
    IMAGE               Container image (default: ${IMAGE})

Example:

    NNODES=2 bash $(basename "$0")

EOF
}

export NNODES=${NNODES:-2}
salloc --exclusive --ntasks-per-node=1 --cpus-per-task=$CPUS_PER_TASK --partition=$PARTITION -N $NNODES -t $TIME bash -c "
    echo '[INFO] nodes allocate:'
    scontrol show hostnames \$SLURM_JOB_NODELIST

    mapfile -t HOSTS < <(scontrol show hostnames \$SLURM_JOB_NODELIST)
    export HF_TOKEN=${HF_TOKEN}
    bash $PWD/run_slurm.sh \"\${HOSTS[@]}\"
"
