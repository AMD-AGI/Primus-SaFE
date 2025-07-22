#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "TransferBench a2a" to measure the data transfer rates between each GPU and all connected GPUs.
# This script can only be run on AMD MI300X chips.

REPO_URL="https://github.com/ROCm/TransferBench.git"
DIR_NAME="TransferBench"
if [ ! -d "$DIR_NAME" ]; then
  git clone "$REPO_URL" >/dev/null 2>error
  if [ $? -ne 0 ]; then
    cat error && rm -f error
    echo "[ERROR]: failed to clone $REPO_URL" >&2
    exit 1
  fi
  rm -f error
fi
cd "$DIR_NAME" || { echo "[ERROR]: unable to access $DIR_NAME" >&2; exit 1; }
CC=hipcc make > /dev/null 2>error
if [ $? -ne 0 ]; then
  cat error && rm -f error
  echo "[ERROR]: failed to make TransferBench" >&2
  exit 1
fi
rm -f error

LOG_FILE="/tmp/transfer_a2a.log"
./TransferBench a2a >$LOG_FILE 2>&1
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  cat $LOG_FILE && rm -f $LOG_FILE
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
  echo "[TransferBenchA2A] [ERROR]: average bandwidth is less than 32.9 (current: $bandwidth)" >&2
  exit 1
fi
echo "[TransferBenchA2A] [SUCCESS]: tests passed"
exit 0

