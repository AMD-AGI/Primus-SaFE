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

ulimit -n 65535
ulimit -u 10240

if [ "$ENABLE_NODE_OUTPUT" == "true" ]; then
  export LOG_HEADER="[NODE-$RANK: $(hostname)] "
fi

echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] started to diagnose"

export WORLD_SIZE=${WORLD_SIZE}
export RANK=${RANK}
export MASTER_ADDR=${MASTER_ADDR}
export MASTER_PORT=${MASTER_PORT}
export NCCL_SOCKET_IFNAME=${NCCL_SOCKET_IFNAME:-"eth0"}
export SSH_PORT=${SSH_PORT:-22}
export BNIC=${BNIC:-48}
export BXGMI=${BXGMI:-315}
export MAX_RETRY=${MAX_RETRY:-2}
export NCCL_PXN_DISABLE=${NCCL_PXN_DISABLE:-1}
export NCCL_P2P_NET_CHUNKSIZE=${NCCL_P2P_NET_CHUNKSIZE:-524288}
export ENABLE_AINIC=${ENABLE_AINIC:-"false"}

# Set GID index based on device type:
# - ionic: GID 0 or 1 (RoCEv2)
# - bnxt_re: GID 3
NCCL_IB_GID_INDEX=${NCCL_IB_GID_INDEX:-3}
if [[ "$ENABLE_AINIC" == "true" ]]; then
  NCCL_IB_GID_INDEX=1
fi

export NCCL_TIMEOUT=7200
export TORCH_DISTRIBUTED_DEFAULT_TIMEOUT=$NCCL_TIMEOUT
export GLOO_TIMEOUT=$NCCL_TIMEOUT
export WAIT=${WAIT:-true}

# ======================================================
# Phase 1: Check the node list file or set up SSH access
# ======================================================
export NODES_FILE=${NODES_FILE:-"/root/hosts"}
if [ ! -f "$NODES_FILE" ]; then
  echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] $NODES_FILE does not exist"
  bash ../ssh/run.sh
  if [ $? -ne 0 ]; then
    echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] failed to generate nodes file"
    exit 1
  fi
fi

# random sort
readonly NODES_FILE_ORIGIN="${NODES_FILE}.origin"
cp "$NODES_FILE" "$NODES_FILE_ORIGIN"
shuf $NODES_FILE > "temp_nodes_file"
mv "temp_nodes_file" $NODES_FILE

readonly NODES_FILE_BAK="$NODES_FILE.bak"
# backup nodes file
if [ "$MAX_RETRY" -gt 1 ]; then
  cp "$NODES_FILE" "$NODES_FILE_BAK"
fi

# ========================================
# Phase 2: Rank 0 runs diagnosis tasks
# ========================================

ret=0
if [[ "$RANK" == "0" ]]; then
  echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] Starting diagnosis tasks..."

  remove_unhealthy_nodes() {
    local unhealthy_list="$1"
    if [ -n "$unhealthy_list" ]; then
      local temp_nodes_file=$(mktemp)
      IFS=', ' read -ra unhealthy_array <<< "$unhealthy_list"

      while IFS= read -r node || [ -n "$node" ]; do
        local is_unhealthy=false
        for unhealthy_node in "${unhealthy_array[@]}"; do
          if [ "$node" = "$unhealthy_node" ]; then
            echo "[INFO] Node $node is unhealthy, removing from node list."
            is_unhealthy=true
            break
          fi
        done

        if [ "$is_unhealthy" = false ]; then
          echo "$node" >> "$temp_nodes_file"
        fi
      done < "$NODES_FILE"

      mv "$temp_nodes_file" "$NODES_FILE"
    fi
  }

  declare -A unhealthy_nodes_intersection
  # Define test types and parameters
  TEST_TYPES=(0 1)
  TEST_NAMES=("all_reduce_perf" "alltoall_perf")

  # Run diagnosis tests
  for run in $(seq 1 $MAX_RETRY); do
    echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] Starting diagnosis run $run/$MAX_RETRY..."
    # restore the nodes file from backup.
    if [ "$run" -gt 1 ]; then
      cp "$NODES_FILE_BAK" "$NODES_FILE"
    fi
    cat "$NODES_FILE"
    unset current_run_unhealthy
    declare -A current_run_unhealthy

    # Run all_reduce_perf test and alltoall_perf test
    for i in "${!TEST_TYPES[@]}"; do
      if [ ! -s "$NODES_FILE" ]; then
        break
      fi
      test_type=${TEST_TYPES[$i]}
      test_name=${TEST_NAMES[$i]}

      echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] Running $test_name (Run $run)..."
      log_file=$(mktemp) && touch "$log_file"
      tail -f "$log_file" &
      tail_pid=$! && sleep 0.5
      BNIC="$BNIC" BXGMI="$BXGMI" python3 -u binary_diagnose.py \
        --socket_ifname "$NCCL_SOCKET_IFNAME" \
        --ib_hca "$NCCL_IB_HCA" \
        --ib_gid_index "$NCCL_IB_GID_INDEX" \
        --ssh_port "$SSH_PORT" \
        --enable_ainic "$ENABLE_AINIC" \
        --nodes_file "$NODES_FILE" \
        --rccl_test_type "$test_type" \
        --rccl_debug "$NCCL_DEBUG" > "$log_file" 2>&1
      test_ret=$?
      sync && sleep 2 && test_output=$(cat "$log_file") && kill $tail_pid 2>/dev/null && rm -f "$log_file"

      if [[ $test_ret -ne 0 ]]; then
        echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] Diagnosis failed for $test_name in run $run"
        unhealthy_list=$(echo "$test_output" | python3 extract_nodes.py)
        if [ -n "$unhealthy_list" ]; then
          IFS=',' read -ra nodes <<< "$unhealthy_list"
          for node in "${nodes[@]}"; do
            current_run_unhealthy["$node"]=1
          done
          remove_unhealthy_nodes "$unhealthy_list"
        fi
      fi
    done

    # Run IB bandwidth test
    echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] Running ib_write_bw.sh (Run $run)..."
    log_file=$(mktemp) && touch "$log_file"
    tail -f "$log_file" &
    tail_pid=$! && sleep 0.5
    python3 -u ib_write_bw.py \
      --socket_ifname "$NCCL_SOCKET_IFNAME" \
      --ib_hca "$NCCL_IB_HCA" \
      --ib_gid_index "$NCCL_IB_GID_INDEX" \
      --ssh_port "$SSH_PORT" \
      --nodes_file "$NODES_FILE" > "$log_file" 2>&1
    test_ret=$?
    sync && sleep 2 && test_output=$(cat "$log_file") && kill $tail_pid 2>/dev/null && rm -f "$log_file"

    if [[ $test_ret -ne 0 ]]; then
      echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] Diagnosis failed for ib_write_bw in run $run"
      # Debug output removed to avoid "Argument list too long" error when echoing large variables
      unhealthy_list=$(echo "$test_output" | python3 extract_nodes.py)
      if [ -n "$unhealthy_list" ]; then
        IFS=',' read -ra nodes <<< "$unhealthy_list"
        for node in "${nodes[@]}"; do
          current_run_unhealthy["$node"]=1
        done
      fi
    fi

    # Find the intersection of multiple tests to identify the common unhealthy nodes.
    if [ "$run" -eq 1 ]; then
      for node in "${!current_run_unhealthy[@]}"; do
        unhealthy_nodes_intersection["$node"]=1
      done
    else
      for node in "${!unhealthy_nodes_intersection[@]}"; do
        if [ -z "${current_run_unhealthy[$node]}" ]; then
          unset unhealthy_nodes_intersection["$node"]
        fi
      done
    fi

    if [ ${#current_run_unhealthy[@]} -eq 0 ]; then
      echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] All nodes passed diagnosis in run $run. Exiting early."
      break
    fi
    echo
  done

  echo "=================================================="
  if [ ${#unhealthy_nodes_intersection[@]} -gt 0 ]; then
    ret=1
    unhealthy_list=()
    for node in "${!unhealthy_nodes_intersection[@]}"; do
      unhealthy_list+=("'$node'")
    done
    printf "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] [NETWORK] [ERROR] Final unhealthy nodes: ["
    if [ ${#unhealthy_list[@]} -gt 0 ]; then
      echo -n "${unhealthy_list[0]}"
      for ((i=1; i<${#unhealthy_list[@]}; i++)); do
        echo -n ", ${unhealthy_list[i]}"
      done
    fi
    echo "]"
  else
    echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] [NETWORK] [SUCCESS] ✅ All diagnosis tests passed."
  fi
  echo "=================================================="
fi

# ========================================
# Phase 3: All nodes wait for rank 0 to finish diagnosis
# ========================================
if [[ "$WAIT" == "true" ]]; then
  echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] Waiting for rank 0 to complete diagnosis..."
  echo "RANK=$RANK, NODE_RANK=$NODE_RANK, MASTER_ADDR=$MASTER_ADDR WORLD_SIZE=$WORLD_SIZE MASTER_PORT=$MASTER_PORT"
  CUDA_VISIBLE_DEVICES="" torchrun \
    --nproc_per_node=1 \
    --nnodes=$WORLD_SIZE \
    --node_rank=$RANK \
    --master_addr=$MASTER_ADDR \
    --master_port=$MASTER_PORT \
    wait_ready.py
fi

# ========================================
# Finalize
# ========================================
mv "$NODES_FILE_ORIGIN" "$NODES_FILE"
rm -f "$NODES_FILE_BAK"
echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] finished diagnosing"
exit $ret