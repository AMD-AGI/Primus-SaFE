#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

if [ "$#" -lt 1 ]; then
  echo 'Error: Missing parameter node-info. example: {"expectedGpuCount": 8}'
  exit 2
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- ls /usr/bin/rocm-smi > /dev/null
if [ $? -ne 0 ]; then
    exit 2
fi

expectedCount=`echo "$1" |jq '.expectedGpuCount'`
if [ -z "$expectedCount" ] || [ "$expectedCount" == "null" ] || [ $expectedCount -le 0 ]; then
    echo "Error: failed to get expectedGpuCount from input: $1"
    exit 2
fi

actualCount=`nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/rocm-smi | grep ^[0-9] |wc -l`
ret=$?
if [ $ret -ne 0 ]; then
  echo "Error: failed to execute rocm-smi"
  exit 2
fi

if [ $actualCount -ne $expectedCount ]; then
  echo 'Error: The actual number of GPU cards is' $actualCount, 'but the expected value is' $expectedCount
  exit 1
fi