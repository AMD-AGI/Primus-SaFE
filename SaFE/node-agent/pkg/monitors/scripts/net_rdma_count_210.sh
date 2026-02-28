#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <node-info>"
  echo "Example: $0 {\"observeRdmaCount\": 1000}"
  exit 2
fi

observedCount=`echo "$1" |jq '.observeRdmaCount'`
if [ -z "$observedCount" ] || [ "$observedCount" == "null" ]; then
  echo "Error: failed to get rdma count from input: $1"
  exit 2
fi

if  [ $observedCount -le 0 ]; then
  echo "Error: RDMA count is $observedCount, but expected value should be greater than 0"
  exit 1
fi