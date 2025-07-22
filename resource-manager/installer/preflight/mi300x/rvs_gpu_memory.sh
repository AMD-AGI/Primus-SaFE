#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rvs -c mem.conf" to test the GPU memory for hardware errors and soft errors
# This script can only be run on AMD MI300X chips.

dpkg -l | grep -q rocm-validation-suite
if [ $? -ne 0 ]; then
  apt-get update >/dev/null 2>&1
  apt install -y rocm-validation-suite >/dev/null 2>error
  if [ $? -ne 0 ]; then
    cat error && rm -f error
    echo "[ERROR] failed to install rocm-validation-suite" >&2
    exit 1
  fi
  rm -f error
fi

export PATH=$PATH:/opt/rocm/bin
export RVS_CONF=/opt/rocm/share/rocm-validation-suite/conf
OUTPUT=$(rvs -c "${RVS_CONF}/mem.conf" -l mem.txt 2>&1)
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  echo "$OUTPUT"
  echo "[RvsGpuMemory] [ERROR] rvs failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

gpuCount=`rocm-smi | grep '^[0-9]' |wc -l`
START=1
END=11
ALL_PASS=true
for i in $(seq $START $END); do
  PATTERN="mem Test $i : PASS"
  COUNT=$(echo "$OUTPUT" | grep -c "$PATTERN")
  if [ $COUNT -lt $gpuCount ]; then
    echo "[RvsGpuMemory] [ERROR] '$PATTERN' only appeared $COUNT times, less than required $gpuCount." >&2
    ALL_PASS=false
    break
  fi
done

if [ "$ALL_PASS" = true ]; then
  echo "[RvsGpuMemory] [SUCCESS] All mem tests from $START to $END passed."
  exit 0
else
  exit 1
fi