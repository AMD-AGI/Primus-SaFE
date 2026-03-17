#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <node-info>"
  echo "Example: $0 {\"expectedEphemeralStorage\": 100000000000, \"observedEphemeralStorage\": 95000000000}"
  exit 2
fi

expectedStorage=`echo "$1" | jq '.expectedEphemeralStorage'`
observedStorage=`echo "$1" | jq '.observedEphemeralStorage'`

if [ -z "$expectedStorage" ] || [ "$expectedStorage" == "null" ]; then
  echo "Error: failed to get expectedEphemeralStorage from input: $1"
  exit 2
fi

if [ -z "$observedStorage" ] || [ "$observedStorage" == "null" ]; then
  echo "Error: failed to get observedEphemeralStorage from input: $1"
  exit 2
fi

# observedEphemeralStorage < expectedEphemeralStorage * 0.9 -> error
# Avoid floating point: compare observed*10 < expected*9
if [ $((observedStorage * 10)) -lt $((expectedStorage * 9)) ]; then
  echo "Error: observedEphemeralStorage ($observedStorage) is less than expectedEphemeralStorage ($expectedStorage)"
  exit 1
fi

exit 0
