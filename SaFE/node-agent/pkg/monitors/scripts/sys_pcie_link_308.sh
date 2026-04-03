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

echo "$PCI_OUTPUT" | awk -v speed="$EXPECTED_SPEED" -v width="$EXPECTED_WIDTH" '
/LnkSta:/ {
    line = $0
    if (line !~ ("Speed " speed " \\(ok\\)")) {
        print "Expected Speed: " speed " (ok), got different value"
        exit 1
    }
    if (line !~ ("Width " width " \\(ok\\)")) {
        print "Expected Width: " width " (ok), got different value"
        exit 1
    }
}
'

RESULT=$?

if [ $RESULT -eq 0 ]; then
  echo "[OK] All checks passed: Link is ${EXPECTED_SPEED} ${EXPECTED_WIDTH} (ok)"
else
  echo "Error: PCIe status check failed: Link speed/width not (ok) or mismatch."
  exit 1
fi