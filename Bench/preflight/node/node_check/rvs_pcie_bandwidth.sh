#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rvs -c pebb_single.conf" to do PCIe bandwidth benchmark between system memory and a target GPU card’s memory

RVS_CONF=/opt/rocm/share/rocm-validation-suite/conf/$GPU_PRODUCT/pebb_single.conf
if [ ! -f "${RVS_CONF}" ]; then
  echo "${RVS_CONF} does not exist" >&2
  exit 1
fi

LOG_FILE="/tmp/bandwidth.log"
/opt/rocm/bin/rvs -c "${RVS_CONF}" -l pebb.txt >$LOG_FILE
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  rm -f $LOG_FILE
  echo "rvs failed with exit code: $EXIT_CODE"
  exit 1
fi

TOTAL_GPUS=`/usr/bin/rocm-smi | grep '^[0-9]' |wc -l`
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
    echo "[INFO] $action passed, pcie-bandwidth lines: $count / $EXPECTED_LINES"
  else
    cat "$file"
    echo "$action passed($count), expected($EXPECTED_LINES)"
    rm -rf $TMP_DIR
    exit 1
  fi
done
rm -rf $TMP_DIR
