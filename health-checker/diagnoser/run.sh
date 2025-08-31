#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "[NODE-$RANK]: begin time=$(date +'%Y.%m.%d %H:%M:%S')"

export SSH_PORT=$SSH_PORT
bash build_ssh.sh
if [ $? -ne 0 ]; then
  echo "failed to build ssh"
  exit 1
fi

export WORLD_SIZE=$WORLD_SIZE
export RANK=$RANK
export BNIC=${BNIC:-50}
export BXGMI=${BXGMI:-315}
export MAX_BYTES=${MAX_BYTES:-2G}
export MAX_RETRY=${MAX_RETRY:-1}
export TORCH_DISTRIBUTED_DEFAULT_TIMEOUT=1800

torchrun \
  --nproc_per_node=1 \
  --max_restarts=2 \
  --nnodes=$WORLD_SIZE \
  --node_rank=$RANK \
  --master_addr=$MASTER_ADDR \
  --master_port=$MASTER_PORT \
  sync_ssh_key.py \
  --interface $GLOO_SOCKET_IFNAME \
  --distributed-timeout-minutes 30

if [ $? -ne 0 ]; then
  echo "failed to execute sync_ssh.py"
  exit 1
fi

# Only rank 0 performs diagnosis
ret=0
if [[ "$RANK" == "0" ]]; then
  # Configuration
  readonly NODES_FILE="/root/hosts"
  readonly SSH_CONFIG="/root/.ssh/config"
  readonly DEBUG_FLAG=$(echo "$NCCL_DEBUG" | tr '[:upper:]' '[:lower:]')
  readonly DEBUG_MODE=${DEBUG_FLAG:-""}

  # Build debug flag for Python script
  debug_arg=""
  if [[ "$DEBUG_MODE" == "info" ]]; then
    debug_arg="--debug"
  fi

  # Generate list of target nodes from SSH config
  echo "begin to diagnose the following nodes"
  grep "^Host " "$SSH_CONFIG" | awk '{print $2}' | sort | uniq > "$NODES_FILE"
  cat "$NODES_FILE"

  # Array of test types: 0 = all_reduce, 1 = alltoall
  TEST_TYPES=(0 1)
  TEST_NAMES=([0]="all_reduce_perf" [1]="alltoall_perf")

  # Run diagnosis for each test type
  for run in $(seq 1 $MAX_RETRY); do
    for test_type in "${TEST_TYPES[@]}"; do
      echo "Running diagnosis for ${TEST_NAMES[$test_type]} (Run $run)..."

      BNIC="$BNIC" BXGMI="$BXGMI" python3 "binary_diagnose.py" \
        --socket-ifname "$NCCL_SOCKET_IFNAME" \
        --ib-hca "$NCCL_IB_HCA" \
        --ssh-port "$SSH_PORT" \
        --nodes-file "$NODES_FILE" \
        --max-bytes "$MAX_BYTES" \
        --rccl-test-type "$test_type" \
        $debug_arg

      if [[ $? -ne 0 ]]; then
        echo "failed to execute binary_diagnose.py for type $test_type"
        ret=1
      fi
    done
  done

  if [ $ret -eq 0 ]; then
    bash ib_read_bw.sh "$NCCL_IB_HCA" "$NCCL_SOCKET_IFNAME" "$NODES_FILE"
    ret=$?
  fi
fi

export TORCH_DISTRIBUTED_DEFAULT_TIMEOUT=10800
torchrun \
  --nproc_per_node=1 \
  --max_restarts=2 \
  --nnodes=$WORLD_SIZE \
  --node_rank=$RANK \
  --master_addr=$MASTER_ADDR \
  --master_port=$MASTER_PORT \
  sync_ssh_key.py \
  --interface $GLOO_SOCKET_IFNAME \
  --distributed-timeout-minutes 180 \
  --no-data-sync

echo "[NODE-$RANK]: end time=$(date +'%Y.%m.%d %H:%M:%S')"
if [[ "$RANK" == "0" ]]; then
  if [ $ret -eq 0 ]; then
    echo "[INFO] All nodes are healthy by binary-diagnose"
  fi
fi
exit $ret