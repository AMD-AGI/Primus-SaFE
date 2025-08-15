#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

export SSH_PORT=$SSH_PORT
bash build_ssh.sh
if [ $? -ne 0 ]; then
  echo "failed to build ssh"
  exit 1
fi

bash install.sh
if [ $? -ne 0 ]; then
  exit 1
fi

export WORK_PATH=/opt/primus-safe/diagnoser
export WORLD_SIZE=$WORLD_SIZE
export RANK=$RANK
torchrun \
  --nproc_per_node=1 \
  --nnodes=$WORLD_SIZE \
  --node_rank=$RANK \
  --master_addr=$MASTER_ADDR \
  --master_port=$MASTER_PORT \
  $WORK_PATH/sync_ssh_key.py \
  --interface $GLOO_SOCKET_IFNAME \
  --distributed-timeout-minutes 30

if [ $? -ne 0 ]; then
  echo "failed to execute sync_ssh.py"
  exit 1
fi

cat /root/.ssh/config  | grep "Host " | awk '{print $2}' | sort | uniq > /root/hosts
sleep 7200
if [[ "$RANK" == "0" ]]; then
  debug=""
  if [[ -n "$NCCL_DEBUG" ]]; then
    nccl_debug=$(echo "$NCCL_DEBUG" | tr '[:upper:]' '[:lower:]')
    if [[ "$nccl_debug" == "info" ]]; then
      debug="--debug"
    fi
  fi
  python3 $WORK_PATH/rccl_diagnose.py --socket-ifname "$NCCL_SOCKET_IFNAME" --ib-hca "$NCCL_IB_HCA" --ssh-port $SSH_PORT $debug
  if [ $? -ne 0 ]; then
    echo "failed to execute binary_search_rccl_test.py."
    exit 1
  fi
fi

torchrun \
  --nproc_per_node=1 \
  --nnodes=$WORLD_SIZE \
  --node_rank=$RANK \
  --master_addr=$MASTER_ADDR \
  --master_port=$MASTER_PORT \
  $WORK_PATH/sync_ssh_key.py \
  --interface $GLOO_SOCKET_IFNAME \
  --distributed-timeout-minutes 30 \
  --no-data-sync 1

if [ $? -ne 0 ]; then
  echo "failed to execute sync_ssh.py"
  exit 1
fi