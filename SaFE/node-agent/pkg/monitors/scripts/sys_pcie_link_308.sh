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

# lspci's LnkSta: formatting varies across pciutils / kernel / platform:
#
#   1. pciutils >= 3.11 (2023+) annotates current values with explicit
#      (ok) / (downgraded) markers, e.g.
#          LnkSta: Speed 32GT/s (ok), Width x16 (ok)
#      This is the most reliable signal and should match first.
#
#   2. pciutils < 3.11 (e.g. Ubuntu 22.04 ships 3.7, Core42's 3.10) omits
#      (ok) entirely:
#          LnkSta: Speed 32GT/s, Width x16
#      For these we fall back to matching the speed/width tokens directly
#      and treat a "(downgraded)" suffix as the failure signal.
#
# Deliberately keep the branches as a chain of explicit ifs — different
# platforms / kernels produce slightly different whitespace, so a single
# clever regex doesn't generalize. Add more branches here rather than
# trying to make one pattern cover everything.
echo "$PCI_OUTPUT" | awk -v speed="$EXPECTED_SPEED" -v width="$EXPECTED_WIDTH" '
/LnkSta:/ {
    line = $0

    # Branch 1: pciutils >= 3.11 with explicit (ok) markers.
    if ((line ~ ("Speed " speed " \\(ok\\)")) && \
        (line ~ ("Width " width " \\(ok\\)"))) {
        next
    }

    # Branch 2: pciutils < 3.11. No (ok) marker — require matching speed
    # and width tokens, and reject any line carrying a (downgraded) tag.
    if (line ~ /\(downgraded\)/) {
        print "PCIe link downgraded: " line
        exit 1
    }
    if ((line ~ ("Speed " speed "(,| |$)")) && \
        (line ~ ("Width " width "(,| |$)"))) {
        next
    }

    # Fell through every branch — report the raw line for diagnosis.
    print "LnkSta mismatch (expected Speed " speed " Width " width "): " line
    exit 1
}
'

RESULT=$?

if [ $RESULT -eq 0 ]; then
  echo "[OK] All checks passed: Link is ${EXPECTED_SPEED} ${EXPECTED_WIDTH}"
else
  echo "Error: PCIe status check failed: Link speed/width mismatch or downgraded."
  exit 1
fi