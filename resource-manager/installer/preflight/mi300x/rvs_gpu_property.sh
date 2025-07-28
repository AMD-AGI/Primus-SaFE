#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rvs -c gpup_single.conf" to validate GPU properties.
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
OUTPUT=$(nsenter --target 1 --mount --uts --ipc --net --pid -- /opt/rocm/bin/rvs -c "${RVS_CONF}/gpup_single.conf")
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  echo "[RvsGpuProperty] [ERROR] rvs failed with exit code: $EXIT_CODE" >&2
  exit 1
fi
if echo "$OUTPUT" | grep -i "error" > /dev/null; then
  echo "$OUTPUT"
  echo "[RvsGpuProperty] [ERROR] failed to validate GPU properties, 'error' is found" >&2
  exit 1
fi
echo "[RvsGpuProperty] [SUCCESS] tests passed"
exit 0
