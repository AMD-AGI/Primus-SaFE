#!/bin/bash
#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -e

echo "[NODE-$RANK] [SSH-SYNC] Starting at $(date +'%H:%M:%S')"

export SSH_PORT=${SSH_PORT:-22}
export WORLD_SIZE=$WORLD_SIZE
export RANK=$RANK
export TORCH_DISTRIBUTED_DEFAULT_TIMEOUT=3600

# Step 1: Build SSH keys
echo "[NODE-$RANK] [SSH-SYNC] Running build_ssh.sh..."
bash build_ssh.sh
if [ $? -ne 0 ]; then
  echo "[NODE-$RANK] [SSH-SYNC] Failed to build SSH"
  exit 1
fi

# Step 2: Sync SSH keys and config via torchrun
echo "[NODE-$RANK] [SSH-SYNC] Syncing SSH keys and config..."

torchrun \
  --nproc_per_node=1 \
  --nnodes=$WORLD_SIZE \
  --node_rank=$RANK \
  --master_addr=$MASTER_ADDR \
  --master_port=$MASTER_PORT \
  ssh_sync.py \
  --interface $GLOO_SOCKET_IFNAME \
  --distributed-timeout-minutes 30

if [ $? -ne 0 ]; then
  echo "[NODE-$RANK] [SSH-SYNC] Failed to sync SSH keys"
  exit 1
fi

echo "[NODE-$RANK] [SSH-SYNC] Completed at $(date +'%H:%M:%S')"