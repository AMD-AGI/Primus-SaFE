#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rvs -c peqt_single.conf" to qualify the PCIe bus which the GPU is connected to.
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
LOG_FILE="/tmp/pcie_quality.log"
rvs -c "${RVS_CONF}/peqt_single.conf" >$LOG_FILE 2>&1
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  cat $LOG_FILE && rm -f $LOG_FILE
  echo "[RvsPcieQuality] [ERROR] rvs failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

total=0
for i in {1..17}; do
  action="[pcie_act_$i] peqt true"
  if grep -qF "$action" "$LOG_FILE"; then
    ((total++))
  else
    echo "[RvsPcieQuality] [ERROR]: $action not found" >&2
    rm -f $LOG_FILE
    exit 1
  fi
done
echo "[RvsPcieQuality] [SUCCESS]: $total tests passed"
rm -f $LOG_FILE
exit 0