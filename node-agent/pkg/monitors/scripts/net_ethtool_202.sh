#!/bin/bash

#
# Copyright © AMD. 2025-2026. All rights reserved.
#

data=`nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/rdma link show |grep LINK_UP |grep ACTIVE`
if [ $? -ne 0 ]; then
  echo "Error: failed to execute rmda"
  exit 2
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- ls /usr/sbin/ethtool > /dev/null
if [ $? -ne 0 ]; then
    exit 2
fi

while read -r line; do
  netdev=$(echo "$line" | awk '{print $NF}')
  nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/sbin/ethtool -t $netdev online |grep "FAIL" > /dev/null
  if [ $? -eq 0 ]; then
    echo "Error: failed to test $netdev"
    exit 1
  fi
done <<< "$data"