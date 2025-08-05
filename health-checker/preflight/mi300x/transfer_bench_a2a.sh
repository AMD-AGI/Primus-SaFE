#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "TransferBench a2a" to measure the data transfer rates between each GPU and all connected GPUs.
# This script can only be run on AMD MI300X chips.

DIR_NAME="/root/TransferBench"
nsenter --target 1 --mount --uts --ipc --net --pid -- ls -d $DIR_NAME >/dev/null
if [ $? -ne 0 ]; then
  echo "[ERROR]: the directory $DIR_NAME does not exist" >&2
  exit 1
fi

LOG_FILE="/tmp/transfer_a2a.log"
nsenter --target 1 --mount --uts --ipc --net --pid -- $DIR_NAME/TransferBench a2a >$LOG_FILE
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  rm -f $LOG_FILE
  echo "[TransferBenchA2A] [ERROR]: TransferBench failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

line=`grep -E 'Average[[:space:]]+bandwidth' "$LOG_FILE"`
bandwidth=$(echo $line | awk '{print $5}')
rm -f $LOG_FILE
if [[ -z "$bandwidth" ]]; then
  echo "[TransferBenchA2A] [ERROR] $line, Could not extract bandwidth value." >&2
  exit 1
fi
if ! [[ "$bandwidth" =~ ^[0-9]+(\.[0-9]+)?$ ]]; then
  echo "[TransferBenchA2A] [ERROR] Extracted bandwidth value is not a valid number: $bandwidth" >&2
  exit 1
fi
result=$(echo "$bandwidth < 32.9" | bc -l)
if [[ "$result" -eq 1 ]]; then
  echo "[TransferBenchA2A] [ERROR]: the data transfer rates does not meet the standard. average bandwidth($bandwidth) is less than threshold(32.9)" >&2
  exit 1
fi
echo "[TransferBenchA2A] [SUCCESS]: tests passed"
exit 0

