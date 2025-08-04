#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rvs -c peqt_single.conf" to qualify the PCIe bus which the GPU is connected to.
# This script can only be run on AMD MI300X chips.

nsenter --target 1 --mount --uts --ipc --net --pid -- dpkg -l | grep -q rocm-validation-suite
if [ $? -ne 0 ]; then
  echo "[ERROR] rocm-validation-suite is not found" >&2
  exit 1
fi

RVS_CONF=/opt/rocm/share/rocm-validation-suite/conf
LOG_FILE="/tmp/pcie_quality.log"
nsenter --target 1 --mount --uts --ipc --net --pid -- /opt/rocm/bin/rvs -c "${RVS_CONF}/peqt_single.conf" >$LOG_FILE
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  rm -f $LOG_FILE
  echo "[RvsPcieQuality] [ERROR] rvs failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

total=0
for i in {1..17}; do
  action="[pcie_act_$i] peqt true"
  if grep -qF "$action" "$LOG_FILE"; then
    ((total++))
  else
    echo "[RvsPcieQuality] [ERROR]: failed to qualify the PCIe bus, $action not found" >&2
    rm -f $LOG_FILE
    exit 1
  fi
done
echo "[RvsPcieQuality] [SUCCESS]: $total tests passed"
rm -f $LOG_FILE
exit 0