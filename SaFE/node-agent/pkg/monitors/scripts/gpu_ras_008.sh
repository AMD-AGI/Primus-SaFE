#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

host() { nsenter --target 1 --mount --uts --ipc --net --pid -- "$@"; }

if [ ! -f "/tmp/rocm-smi.json" ]; then
  exit 0
fi

data=$(host /usr/bin/rocm-smi --showrasinfo all 2>&1)
if [ $? -ne 0 ]; then
  echo "Error: failed to execute rocm-smi --showrasinfo. $data"
  exit 1
fi

gpu_id=""
echo "$data" | while read -r line; do
  line=$(echo "$line" | sed 's/^ *//;s/ *$//')
  if [[ -z "$line" ]]; then
    continue
  fi
  if [[ "$line" =~ ^GPU ]]; then
    gpu_id=$(echo "$line" | awk -F":" '{print $1}')
    continue
  fi
  status=$(echo "$line" | awk '{print $2}')
  if [ "$status" != "ENABLED" ]; then
    continue
  fi
  uncorrect_error=$(echo "$line" | awk '{print $NF}')
  if [[ "$uncorrect_error" =~ ^[0-9]+$ ]] && (( uncorrect_error > 0 )); then
    block=$(echo "$line" | awk '{print $1}')
    echo "Warning: an uncorrectable error is detected in the block($block) of $gpu_id"
    exit 1
  fi
done
