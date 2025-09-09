#!/bin/bash
#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# File: run.sh
# Main entry point: Executes tasks in sequence:
#   1. SSH key and config synchronization
#   2. Rank 0 runs diagnosis
#   3. All nodes wait and exit

echo "[NODE-$RANK]: started at $(date +'%Y.%m.%d %H:%M:%S')"

export WORLD_SIZE=${WORLD_SIZE}
export RANK=${RANK}
export MASTER_ADDR=${MASTER_ADDR}
export MASTER_PORT=${MASTER_PORT}
export GLOO_SOCKET_IFNAME=${GLOO_SOCKET_IFNAME:-"eth0"}
export SSH_PORT=${SSH_PORT:-22}
export BNIC=${BNIC:-50}
export BXGMI=${BXGMI:-315}
export NCCL_IB_GID_INDEX=${NCCL_IB_GID_INDEX:-3}
export TORCH_DISTRIBUTED_DEFAULT_TIMEOUT=3600
export NCCL_TIMEOUT=3600
export GLOO_TIMEOUT=3600

# ========================================
# Phase 1: Synchronize SSH keys and config
# ========================================
source ssh_sync.sh

if [ $? -ne 0 ]; then
  echo "[ERROR] SSH synchronization failed"
  exit 1
fi

# ========================================
# Phase 2: Rank 0 runs diagnosis tasks
# ========================================
diag_ret=0
if [[ "$RANK" == "0" ]]; then
  echo "[NODE-0] Starting diagnosis tasks..."

  # Configuration file paths
  readonly NODES_FILE="/root/hosts"
  readonly DEBUG_FLAG=$(echo "$NCCL_DEBUG" | tr '[:upper:]' '[:lower:]')
  readonly DEBUG_MODE=${DEBUG_FLAG:-""}
  debug_arg=""
  if [[ "$DEBUG_MODE" == "info" ]]; then
    debug_arg="--debug"
  fi

  # Generate list of nodes from SSH config
  grep "^Host " "/root/.ssh/config" | awk '{print $2}' | sort | uniq > "$NODES_FILE"
  echo "Diagnosing the following nodes:"
  cat "$NODES_FILE"

  # Define test types and parameters
  TEST_TYPES=(0 1)
  TEST_NAMES=("all_reduce_perf" "alltoall_perf")
  TEST_MAX_SIZE=("2G" "64M")

  for i in "${!TEST_TYPES[@]}"; do
    test_type=${TEST_TYPES[$i]}
    test_name=${TEST_NAMES[$i]}
    max_bytes=${TEST_MAX_SIZE[$i]}

    echo "[NODE-0] Running $test_name ..."
    BNIC="$BNIC" BXGMI="$BXGMI" python3 binary_diagnose.py \
      --socket-ifname "$NCCL_SOCKET_IFNAME" \
      --ib-hca "$NCCL_IB_HCA" \
      --ib-gid-index "$NCCL_IB_GID_INDEX" \
      --ssh-port "$SSH_PORT" \
      --nodes-file "$NODES_FILE" \
      --max-bytes "$max_bytes" \
      --rccl-test-type "$test_type" \
      $debug_arg

    if [[ $? -ne 0 ]]; then
      echo "[NODE-0] Diagnosis failed for $test_name"
      diag_ret=1
    fi
  done

  # Run IB bandwidth test
  echo "[NODE-0] Running ib_write_bw.sh..."
  bash ib_write_bw.sh "$NCCL_IB_HCA" "$NCCL_SOCKET_IFNAME" "$NCCL_IB_GID_INDEX" "$NODES_FILE"
  if [[ $? -ne 0 ]]; then
    echo "[NODE-0] Diagnosis failed for ib_write_bw"
    diag_ret=1
  fi

  if [ $diag_ret -eq 0 ]; then
    echo "[SUCCESS] ✅ All diagnosis tests passed."
  fi
fi

# ========================================
# Phase 3: All nodes wait for rank 0 to finish diagnosis
# ========================================
echo "[NODE-$RANK] Waiting for rank 0 to complete diagnosis..."
torchrun \
  --nproc_per_node=1 \
  --nnodes=$WORLD_SIZE \
  --node_rank=$RANK \
  --master_addr=$MASTER_ADDR \
  --master_port=$MASTER_PORT \
  wait_ready.py

# ========================================
# Finalize
# ========================================
echo "[NODE-$RANK] ended at $(date +'%Y.%m.%d %H:%M:%S')"
exit $diag_ret