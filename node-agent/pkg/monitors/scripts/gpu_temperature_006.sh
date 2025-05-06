#!/bin/bash

#
# Copyright © AMD. 2025-2026. All rights reserved.
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

threshold=85
while read -r line; do
  temp=$(echo "$line" | awk '{print $NF}')
  temp=$(echo "$temp / 1" | bc)
  if [ $temp -ge $threshold ]; then
    echo "Warning: GPU temperature is too high! Current temperature exceeds the safe threshold of $threshold°C."
  	exit 1
  fi
done <<< "$data"