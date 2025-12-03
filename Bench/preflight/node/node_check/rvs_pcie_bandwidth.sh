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

CURRENT_ACTION=""
CURRENT_COUNT=0
FAILED=0

# Process log line by line, print result immediately when action changes
while IFS= read -r line; do
  if echo "$line" | grep -q "Action name"; then
    # Print result for previous action (if exists)
    if [ -n "$CURRENT_ACTION" ]; then
      if [ "$CURRENT_COUNT" -eq "$EXPECTED_LINES" ]; then
        echo "[INFO] $CURRENT_ACTION passed, pcie-bandwidth lines: $CURRENT_COUNT / $EXPECTED_LINES"
      else
        echo "[ERROR] $CURRENT_ACTION failed($CURRENT_COUNT), expected($EXPECTED_LINES)" >&2
        FAILED=1
      fi
    fi
    # Start new action
    CURRENT_ACTION=$(echo "$line" | awk -F':' '{print $2}')
    CURRENT_COUNT=0
    continue
  fi
  if echo "$line" | grep -q "pcie-bandwidth"; then
    CURRENT_COUNT=$((CURRENT_COUNT + 1))
  fi
done < "$LOG_FILE"

# Print result for last action
if [ -n "$CURRENT_ACTION" ]; then
  if [ "$CURRENT_COUNT" -eq "$EXPECTED_LINES" ]; then
    echo "[INFO] $CURRENT_ACTION passed, pcie-bandwidth lines: $CURRENT_COUNT / $EXPECTED_LINES"
  else
    echo "[ERROR] $CURRENT_ACTION failed($CURRENT_COUNT), expected($EXPECTED_LINES)" >&2
    FAILED=1
  fi
fi

rm -f $LOG_FILE

if [ $FAILED -eq 1 ]; then
  exit 1
fi
