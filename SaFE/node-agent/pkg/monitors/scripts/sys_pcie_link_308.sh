#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

EXPECTED_SPEED="32GT/s"
EXPECTED_WIDTH="x16"
JSON_FILE="/tmp/rocm-smi.json"

if [ ! -f "${JSON_FILE}" ]; then
  exit 0
fi

GPU_PRODUCT=$(jq -r '[.[] | .["Card Series"] // empty] | first // empty' "${JSON_FILE}" 2>/dev/null)
if [ -z "$GPU_PRODUCT" ]; then
  echo "Error: failed to get product from ${JSON_FILE}"
  exit 2
fi

shopt -s nocasematch
model=""
if [[ "$GPU_PRODUCT" == *"mi300x"* ]]; then
  model="1002:74a1"
elif [[ "$GPU_PRODUCT" == *"mi325x"* ]]; then
  model="1002:74a5"
elif [[ "$GPU_PRODUCT" == *"mi350x"* ]]; then
  model="1002:75a0"
elif [[ "$GPU_PRODUCT" == *"mi355x"* ]]; then
  model="1002:75a3"
else
  echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] [WARNING] The $GPU_PRODUCT is not supported" >&2
  exit 2
fi

PCI_OUTPUT=`nsenter --target 1 --mount --uts --ipc --net --pid -- lspci -d $model -vvv | grep "LnkSta:"`
if [ $? -ne 0 ]; then
  echo "Error: failed to get PCI info"
  exit 1
fi

# lspci's "(ok)" / "(downgraded)" trailing annotations were added in pciutils
# 3.11 (2023). Anything older (e.g. Ubuntu 22.04 default 3.7, Core42's 3.10)
# just prints "Speed 32GT/s, Width x16". Match the speed/width tokens on
# their own and treat a trailing "(downgraded)" as the only failure signal.
echo "$PCI_OUTPUT" | awk -v speed="$EXPECTED_SPEED" -v width="$EXPECTED_WIDTH" '
/LnkSta:/ {
    line = $0
    if (line !~ ("Speed " speed "(,| )")) {
        print "Expected Speed: " speed ", got different value: " line
        exit 1
    }
    if (line !~ ("Width " width "($| |,)")) {
        print "Expected Width: " width ", got different value: " line
        exit 1
    }
    if (line ~ /\(downgraded\)/) {
        print "PCIe link downgraded: " line
        exit 1
    }
}
'

RESULT=$?

if [ $RESULT -eq 0 ]; then
  echo "[OK] All checks passed: Link is ${EXPECTED_SPEED} ${EXPECTED_WIDTH}"
else
  echo "Error: PCIe status check failed: Link speed/width mismatch or downgraded."
  exit 1
fi