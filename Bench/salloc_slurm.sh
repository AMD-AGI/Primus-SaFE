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

    NNODES=1                      # Number of nodes (default: 1)
    CONFIG=amd_mode.yaml          # SuperBench config file (default: amd_mi300.yaml)
    IMAGE=primussafe/primusbench   # Image of SuperBench (default: primussafe/primusbench:202408191128)

Example:

    NNODES=2 CONFIG=amd_mi300.yaml bash primusbench.sh

EOF
}

export NNODES=${NNODES:-2}
export PARTITION=${PARTITION:-amd-tw}
export TIME=${TIME:-4:30:00}

salloc --exclusive --ntasks-per-node=1  --partition=$PARTITION -N $NNODES -t $TIME bash -c "
    echo '[INFO] nodes allocate:'
    scontrol show hostnames \$SLURM_JOB_NODELIST

    mapfile -t HOSTS < <(scontrol show hostnames \$SLURM_JOB_NODELIST)
    bash $PWD/run_slurm.sh \"\${HOSTS[@]}\"
"
