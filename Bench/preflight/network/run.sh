#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
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
  # Sanitize hostname: remove backticks to prevent command injection when LOG_HEADER is expanded
  safe_hostname=$(hostname | tr -d '`')
  export LOG_HEADER="[NODE-$RANK: ${safe_hostname}] "
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
export ENABLE_AINIC=${ENABLE_AINIC:-"false"}

# PXN settings only for non-AINIC mode (conflict with AINIC's optimized path)
if [[ "$ENABLE_AINIC" != "true" ]]; then
  export NCCL_PXN_DISABLE=${NCCL_PXN_DISABLE:-1}
  export NCCL_P2P_NET_CHUNKSIZE=${NCCL_P2P_NET_CHUNKSIZE:-524288}
else
  echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] AINIC mode enabled"
  # Ensure these are unset in AINIC mode to avoid conflicts
  unset NCCL_PXN_DISABLE
  unset NCCL_P2P_NET_CHUNKSIZE
fi

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

echo "================================================"
echo "${LOG_HEADER} RANK: $RANK"
echo "${LOG_HEADER} NCCL_SOCKET_IFNAME: $NCCL_SOCKET_IFNAME"
echo "${LOG_HEADER} NCCL_IB_HCA: $NCCL_IB_HCA"
echo "${LOG_HEADER} NCCL_IB_GID_INDEX: $NCCL_IB_GID_INDEX"
echo "${LOG_HEADER} ENABLE_AINIC: $ENABLE_AINIC"
echo "================================================"
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

echo "================================================"
cat "$NODES_FILE"
echo "================================================"

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

  # Retry bookkeeping and the final verdict. See verdict.sh for why an
  # unattributable failure has to keep a run out of the intersection rather
  # than merely be noted alongside it.
  source "$(dirname "${BASH_SOURCE[0]}")/verdict.sh"
  reset_verdict_state

  # Set when a test exits non-zero but the failure cannot be attributed to
  # any node (e.g. the harness itself errored out). Without this, such a run
  # is indistinguishable from "everything passed".
  #
  # Scoped to one run and reset at the top of each: MAX_RETRY exists so a
  # transient failure can be disproved by a retry, and that has to apply here
  # too. record_run carries the cross-run half of the story.
  harness_failure=0
  # Define test types and parameters (all_reduce first, then alltoall)
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
    declare -A current_run_unhealthy=()
    harness_failure=0

    # Step 1: Run IB bandwidth test first to filter out nodes with basic connectivity issues
    echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] Running ib_write_bw test first (Run $run)..."
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
      unhealthy_list=$(echo "$test_output" | python3 extract_nodes.py)
      if [ -n "$unhealthy_list" ]; then
        IFS=',' read -ra nodes <<< "$unhealthy_list"
        for node in "${nodes[@]}"; do
          current_run_unhealthy["$node"]=1
        done
        # Remove unhealthy nodes before RCCL tests
        remove_unhealthy_nodes "$unhealthy_list"
        echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] Removed unhealthy nodes from IB test: $unhealthy_list"
      else
        # Same reasoning as the RCCL branch below: ib_write_bw.py exits 1 for
        # failures that blame no node -- no HCA to test, get_ip() returning
        # None, or every group dying of an exception. Left unrecorded, a clean
        # RCCL run afterwards would print "All diagnosis tests passed" for a
        # cluster whose IB test never ran.
        harness_failure=1
        echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] [NETWORK] [ERROR] ib_write_bw failed (exit $test_ret) but no node could be blamed; treating as a harness failure"
      fi
    fi

    # Step 2: Run RCCL tests only on healthy nodes that passed IB test
    if [ -s "$NODES_FILE" ]; then
      echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] Running RCCL tests on nodes that passed IB test..."
      
      for i in "${!TEST_TYPES[@]}"; do
        test_type=${TEST_TYPES[$i]}
        test_name=${TEST_NAMES[$i]}
        
        if [ ! -s "$NODES_FILE" ]; then
          echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] No healthy nodes remaining, skipping $test_name"
          break
        fi

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
            echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] Removed unhealthy nodes from $test_name: $unhealthy_list"
          else
            harness_failure=1
            echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] [NETWORK] [ERROR] $test_name failed (exit $test_ret) but no node could be blamed; treating as a harness failure"
          fi
        fi
      done
    else
      echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] All nodes failed IB test, skipping RCCL tests"
    fi

    # Fold this run into the intersection of common unhealthy nodes.
    #
    # This used to seed on `run -eq 1` and only ever remove afterwards, which
    # made run 1 authoritative over a set it may not have been able to
    # produce: harness_failure is set exactly when no node was blamed, so a
    # run-1 harness error seeded the intersection with the empty set and
    # nothing run 2 found could be added back. record_run seeds from the
    # first run that actually finished instead.
    record_run "$run" "$harness_failure" "${!current_run_unhealthy[@]}"

    if [ ${#current_run_unhealthy[@]} -eq 0 ]; then
      if [ "$harness_failure" -eq 1 ]; then
        # Do NOT break here. harness_failure is set precisely when no node was
        # blamed, which is precisely when current_run_unhealthy is empty -- so
        # breaking on this branch meant the retry could never run, and one
        # transient ssh hiccup in run 1 failed the whole cluster. Retrying is
        # the whole point: a harness error that run 2 does not reproduce is
        # not a reason to condemn a cluster the retry validated.
        if [ "$run" -lt "$MAX_RETRY" ]; then
          echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] No node was blamed in run $run, but a test failed. Retrying to see whether it reproduces."
        else
          echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] No node was blamed in run $run, but a test failed, and this was the last of $MAX_RETRY run(s)."
        fi
      else
        echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] All nodes passed diagnosis in run $run. Exiting early."
        break
      fi
    fi
    echo
  done

  echo "=================================================="
  # Blamed nodes and "the check did not finish" are reported independently --
  # print_verdict is not a chain of elifs. The old `elif` meant that as soon
  # as any node was blamed, an unattributable failure in another run went
  # unmentioned, and the "Diagnosis did not complete" line Bench/run.sh greps
  # for was never printed.
  if ! print_verdict "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] "; then
    ret=1
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