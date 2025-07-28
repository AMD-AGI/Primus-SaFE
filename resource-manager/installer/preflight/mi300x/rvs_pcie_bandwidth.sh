#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rvs -c pebb_single.conf" to do PCIe bandwidth benchmark between system memory and a target GPU card’s memory
# This script can only be run on AMD MI300X chips.

nsenter --target 1 --mount --uts --ipc --net --pid -- dpkg -l | grep -q rocm-validation-suite
if [ $? -ne 0 ]; then
  nsenter --target 1 --mount --uts --ipc --net --pid -- apt-get update >/dev/null && apt install -y rocm-validation-suite >/dev/null
  if [ $? -ne 0 ]; then
    echo "[ERROR] failed to install rocm-validation-suite"
    exit 1
  fi
fi

RVS_CONF=/opt/rocm/share/rocm-validation-suite/conf
LOG_FILE="/tmp/bandwidth.log"
nsenter --target 1 --mount --uts --ipc --net --pid -- /opt/rocm/bin/rvs -c "${RVS_CONF}/MI300X/pebb_single.conf" -l pebb.txt >$LOG_FILE
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  rm -f $LOG_FILE
  echo "[RvsPcieBandwidth] [ERROR] rvs failed with exit code: $EXIT_CODE"
  exit 1
fi

TOTAL_GPUS=`nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/rocm-smi | grep '^[0-9]' |wc -l`
TOTAL_CPUS=2
EXPECTED_LINES=$((TOTAL_GPUS * TOTAL_CPUS * 2))
TMP_DIR="/tmp/pcie_bandwidth_check"
mkdir -p "$TMP_DIR"
rm -f "$TMP_DIR"/*

CURRENT_ACTION=""
while IFS= read -r line; do
  if echo "$line" | grep -q "Action name"; then
    CURRENT_ACTION=$(echo "$line" | awk -F':' '{print $2}')
    continue
  fi
  if echo "$line" | grep -q "pcie-bandwidth"; then
    echo "$line" >> "$TMP_DIR/$CURRENT_ACTION"
  fi
done < "$LOG_FILE"
rm -f $LOG_FILE

for file in "$TMP_DIR"/*; do
  action=$(basename "$file")
  count=$(wc -l < "$file")
  if [ "$count" -eq "$EXPECTED_LINES" ]; then
    echo "[RvsPcieBandwidth] [INFO] $action passed, pcie-bandwidth lines: $count / $EXPECTED_LINES"
  else
    cat "$file"
    echo "[RvsPcieBandwidth] [ERROR] failed to evaluate PCIe bandwidth, $action: bandwidth value: $count / $EXPECTED_LINES"
    rm -rf $TMP_DIR
    exit 1
  fi
done
rm -rf $TMP_DIR
echo "[RvsPcieBandwidth] [SUCCESS] tests passed"
exit 0