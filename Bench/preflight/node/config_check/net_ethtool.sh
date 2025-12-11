#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

data=`nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/rdma link show |grep LINK_UP |grep ACTIVE`
if [ $? -ne 0 ]; then
  echo "failed to execute rdma"
  exit 2
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- ls /usr/sbin/ethtool > /dev/null
if [ $? -ne 0 ]; then
  exit 2
fi

while read -r line; do
  netdev=$(echo "$line" | awk '{print $NF}')
  # Capture ethtool output and check for both failures and unsupported operations
  output=$(nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/sbin/ethtool -t $netdev online 2>&1)
  
  # Check if operation is not supported (skip this device)
  if echo "$output" | grep -q "Operation not supported"; then
      echo "Skipping $netdev: ethtool self-test not supported"
      continue
  fi
  
  # Check for actual test failures
  if echo "$output" | grep -q "FAIL"; then
      echo "\"FAIL\" found in \"ethtool -t $netdev\""
      exit 1
  fi
done <<< "$data"