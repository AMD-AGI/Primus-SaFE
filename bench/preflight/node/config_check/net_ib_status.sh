#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

input_devices=$NCCL_IB_HCA

IFS=',' read -ra DEV_ARRAY <<< "$input_devices"
for dev in "${DEV_ARRAY[@]}"; do
  dev=$(echo "$dev" | xargs)

  OUTPUT=$(nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/sbin/ibstatus "$dev" 2>&1)
  ret=$?
  if [ $ret -ne 0 ]; then
    exit 1
  fi

  STATE_LINE=$(echo "$OUTPUT" | grep "state:" | head -n1)
  if [ -z "$STATE_LINE" ]; then
    exit 1
  fi

  STATE=$(echo "$STATE_LINE" | awk -F':' '{print $3}' | xargs)
  if [ "$STATE" != "ACTIVE" ]; then
    echo "device '$dev' is DOWN!"
    exit 1
  fi
done