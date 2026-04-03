#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <node-info>"
  echo "Example: $0 {\"expectedGpuCount\": 8}"
  exit 2
fi

JSON_FILE="/tmp/rocm-smi.json"

if [ ! -f "${JSON_FILE}" ]; then
  exit 0
fi

expectedCount=$(echo "$1" | jq '.expectedGpuCount')
if [ -z "$expectedCount" ] || [ "$expectedCount" == "null" ] || [ "$expectedCount" -le 0 ]; then
  echo "Error: failed to get expectedGpuCount from input: $1"
  exit 2
fi

actualCount=$(jq '[keys[] | select(startswith("card"))] | length' "${JSON_FILE}" 2>/dev/null)
if [ -z "$actualCount" ]; then
  echo "Error: failed to parse GPU count from ${JSON_FILE}"
  exit 2
fi

if [ $actualCount -ne $expectedCount ]; then
  echo 'Error: The actual number of GPU cards is' $actualCount, 'but the expected value is' $expectedCount
  exit 1
fi