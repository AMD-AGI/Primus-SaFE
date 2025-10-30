#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# exmaple: NUM_NODES=8 ./tools/preflight_network/run_slurm.sh
# optional: provide exclude list via EXCLUDE_NODES (comma-separated) or EXCLUDE_FILE (one node per line)

export SSH_PORT=$(( RANDOM % 9999 + 30001 ))
export NUM_NODES=${NUM_NODES:-2}
SCRIPT_DIR=$(dirname "$(realpath "${BASH_SOURCE[0]}")")
logname="output/slurm_diagnose_network_${NUM_NODES}Nodes.log"
export PREFLIGHT_NETWORK_IMAGE="docker.io/primussafe/diagnose_network:202509222007"

# ensure output directory exists
mkdir -p output

# warm up container image across all nodes
srun -N ${NUM_NODES} \
     --exclusive \
     -t 10:30:00 \
     --ntasks-per-node=1 \
     bash -lc 'if command -v docker >/dev/null 2>&1; then docker pull "$DOCKER_IMAGE"; elif command -v podman >/dev/null 2>&1; then podman pull "$DOCKER_IMAGE"; elif command -v ctr >/dev/null 2>&1; then ctr -n k8s.io images pull "$DOCKER_IMAGE"; else echo "No container runtime found on $(hostname)"; fi' 2>&1 | tee -a $logname

srun -N ${NUM_NODES} \
     --exclusive \
     -t 04:30:00 \
     --partition=amd-tw \
     --ntasks-per-node=1 \
     bash ${SCRIPT_DIR}/start_docker.sh 2>&1 | tee $logname

errors=$(grep "\[ERROR\]" $logname)
if [ -n "$errors" ]; then
     echo
     echo "====================================="
     echo "$errors"
     echo "====================================="
fi