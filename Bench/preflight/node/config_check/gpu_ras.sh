#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

if [ ! -f "/tmp/rocm-smi" ]; then
  exit 0
fi

data=`nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/rocm-smi --showrasinfo`
if [ $? -ne 0 ]; then
  echo "failed to execute rocm-smi --showrasinfo"
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
    echo "an uncorrectable error is detected in the block($block) of $gpu_id"
    exit 1
  fi
done