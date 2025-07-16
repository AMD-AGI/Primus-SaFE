#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <node-info>"
  echo "Example: $0 {\"expectedGpuCount\": 8}"
  exit 2
fi

if [ ! -f "/tmp/rocm-smi" ]; then
  exit 0
fi

expectedCount=`echo "$1" |jq '.expectedGpuCount'`
if [ -z "$expectedCount" ] || [ "$expectedCount" == "null" ] || [ $expectedCount -le 0 ]; then
    echo "Error: failed to get expectedGpuCount from input: $1"
    exit 2
fi

actualCount=`cat "/tmp/rocm-smi" | grep '^[0-9]' |wc -l`
ret=$?
if [ $ret -ne 0 ]; then
  echo "Error: failed to execute rocm-smi"
  exit 2
fi

if [ $actualCount -ne $expectedCount ]; then
  echo 'Error: The actual number of GPU cards is' $actualCount, 'but the expected value is' $expectedCount
  exit 1
fi