#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rvs -c peqt_single.conf" to qualify the PCIe bus which the GPU is connected to.

RVS_CONF=/opt/rocm/share/rocm-validation-suite/conf/peqt_single.conf
if [ ! -f "${RVS_CONF}" ]; then
  echo "${RVS_CONF} does not exist" >&2
  exit 1
fi

LOG_FILE="/tmp/pcie_quality.log"
/opt/rocm/bin/rvs -c "${RVS_CONF}" >$LOG_FILE
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  rm -f $LOG_FILE
  echo "rvs failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

total=0
for i in {1..17}; do
  action="[pcie_act_$i] peqt true"
  if grep -qF "$action" "$LOG_FILE"; then
    ((total++))
  else
    echo "failed to qualify the PCIe bus, $action not found" >&2
    rm -f $LOG_FILE
    exit 1
  fi
done
rm -f $LOG_FILE