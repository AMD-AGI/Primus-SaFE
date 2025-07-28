#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rvs -c iet_single.conf" to stress the GPU power
# This script can only be run on AMD MI300X chips.

nsenter --target 1 --mount --uts --ipc --net --pid -- dpkg -l | grep -q rocm-validation-suite
if [ $? -ne 0 ]; then
  nsenter --target 1 --mount --uts --ipc --net --pid -- apt-get update >/dev/null && apt install -y rocm-validation-suite >/dev/null
  if [ $? -ne 0 ]; then
    echo "[ERROR] failed to install rocm-validation-suite" >&2
    exit 1
  fi
fi

RVS_CONF=/opt/rocm/share/rocm-validation-suite/conf
OUTPUT=$(nsenter --target 1 --mount --uts --ipc --net --pid -- /opt/rocm/bin/rvs -c "${RVS_CONF}/MI300X/iet_single.conf")
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  echo "[RvsPower] [ERROR] rvs failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

tmpfile="/tmp/match_lines.txt"
echo "$OUTPUT" | grep "pass: FALSE" > $tmpfile
if [ -s /tmp/match_lines.txt ]; then
  cat $tmpfile && rm -f $tmpfile
  echo "[RvsPower] [ERROR] failed to do the GPU power test, Found 'pass: FALSE' in output." >&2
  exit 1
fi
echo "[RvsPower] [SUCCESS] tests passed."
rm -f $tmpfile
exit 0

