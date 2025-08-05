#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rvs -c gst_single.conf" to stress the GPU FLOPS performance.
# This script can only be run on AMD MI300X chips.

nsenter --target 1 --mount --uts --ipc --net --pid -- dpkg -l | grep -q rocm-validation-suite
if [ $? -ne 0 ]; then
  echo "[ERROR] rocm-validation-suite is not found" >&2
  exit 1
fi

RVS_CONF=/opt/rocm/share/rocm-validation-suite/conf
OUTPUT=$(nsenter --target 1 --mount --uts --ipc --net --pid -- /opt/rocm/bin/rvs -c "${RVS_CONF}/MI300X/gst_single.conf")
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  echo "[RvsPerformance] [ERROR] rvs failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

tmpfile="/tmp/match_lines.txt"
echo "$OUTPUT" | grep "met: FALSE" > $tmpfile
if [ -s /tmp/match_lines.txt ]; then
  cat $tmpfile && rm -f $tmpfile
  echo "[RvsPerformance] [ERROR] failed to do the GPU FLOPS performance test, Found 'met: FALSE' in output." >&2
  exit 1
fi
echo "[RvsPerformance] [SUCCESS] tests passed."
rm -f $tmpfile
exit 0
