#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

if [ ! -f "/tmp/rocm-smi" ]; then
  exit 0
fi

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <temperature>"
  echo "Example: $0 100"
  exit 2
fi

data=`nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/rocm-smi -t |grep Temperature |grep GPU`
if [ $? -ne 0 ]; then
  echo "Error: failed to execute rocm-smi -t"
  exit 2
fi

threshold=$1
while read -r line; do
  temp=$(echo "$line" | awk '{print $NF}')
  temp=$(echo "$temp / 1" | bc)
  if [ $temp -ge $threshold ]; then
    echo "Warning: GPU temperature is too high! Current temperature exceeds the safe threshold of $threshold°C."
  	exit 1
  fi
done <<< "$data"