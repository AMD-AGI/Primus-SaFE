#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

nsenter --target 1 --mount --uts --ipc --net --pid -- ls /usr/bin/rocm-smi > /dev/null
if [ $? -ne 0 ]; then
    exit 2
fi

data=`nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/rocm-smi -t |grep Temperature |grep GPU`
if [ $? -ne 0 ]; then
  echo "Error: failed to execute rocm-smi -t"
  exit 2
fi

threshold=100
while read -r line; do
  temp=$(echo "$line" | awk '{print $NF}')
  temp=$(echo "$temp / 1" | bc)
  if [ $temp -ge $threshold ]; then
    echo "Warning: GPU temperature is too high! Current temperature exceeds the safe threshold of $threshold°C."
  	exit 1
  fi
done <<< "$data"