#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rvs -c mem.conf" to test the GPU memory for hardware errors and soft errors

RVS_CONF=/opt/rocm/share/rocm-validation-suite/conf/mem.conf
if [ ! -f "${RVS_CONF}" ]; then
  echo "${RVS_CONF} does not exist" >&2
  exit 1
fi

OUTPUT=$(/opt/rocm/bin/rvs -c "$RVS_CONF" -l mem.txt)
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  echo "rvs failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

gpuCount=`/usr/bin/rocm-smi | grep '^[0-9]' |wc -l`
START=1
END=11
for i in $(seq $START $END); do
  PATTERN="mem Test $i : PASS"
  COUNT=$(echo "$OUTPUT" | grep -c "$PATTERN")
  if [ $COUNT -lt $gpuCount ]; then
    echo "the GPU memory has errors, '$PATTERN' only appeared $COUNT times, less than required $gpuCount." >&2
    exit 1
  fi
done
