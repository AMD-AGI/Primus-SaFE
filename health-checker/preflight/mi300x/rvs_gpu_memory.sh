#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rvs -c mem.conf" to test the GPU memory for hardware errors and soft errors
# This script can only be run on AMD MI300X chips.

nsenter --target 1 --mount --uts --ipc --net --pid -- dpkg -l | grep -q rocm-validation-suite
if [ $? -ne 0 ]; then
  echo "[ERROR] rocm-validation-suite is not found" >&2
  exit 1
fi

RVS_CONF=/opt/rocm/share/rocm-validation-suite/conf
OUTPUT=$(nsenter --target 1 --mount --uts --ipc --net --pid -- /opt/rocm/bin/rvs -c "$RVS_CONF/mem.conf" -l mem.txt)
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  echo "[RvsGpuMemory] [ERROR] rvs failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

gpuCount=`nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/rocm-smi | grep '^[0-9]' |wc -l`
START=1
END=11
ALL_PASS=true
for i in $(seq $START $END); do
  PATTERN="mem Test $i : PASS"
  COUNT=$(echo "$OUTPUT" | grep -c "$PATTERN")
  if [ $COUNT -lt $gpuCount ]; then
    echo "[RvsGpuMemory] [ERROR] the GPU memory has errors, '$PATTERN' only appeared $COUNT times, less than required $gpuCount." >&2
    ALL_PASS=false
    break
  fi
done

if [ "$ALL_PASS" = true ]; then
  echo "[RvsGpuMemory] [SUCCESS] tests passed"
  exit 0
else
  exit 1
fi