#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "TransferBench a2a" to measure the data transfer rates between each GPU and all connected GPUs.

DIR_NAME="/opt/TransferBench"
if [ ! -d "$DIR_NAME" ]; then
  echo "the directory $DIR_NAME does not exist" >&2
  exit 1
fi

LOG_FILE="/tmp/transfer_a2a.log"
max_retries=5
best_bandwidth=0
success=0
threshold=29.6
last_error=""

for attempt in $(seq 1 $max_retries); do
  "$DIR_NAME/TransferBench" a2a >"$LOG_FILE"
  EXIT_CODE=$?
  if [ $EXIT_CODE -ne 0 ]; then
    log_content=$(cat "$LOG_FILE" 2>/dev/null)
    rm -f "$LOG_FILE"
    last_error="TransferBench failed with exit code: $EXIT_CODE, output: $log_content"
    echo "[WARNING]: $last_error" >&2
    continue
  fi

  line=$(grep -E 'Average[[:space:]]+bandwidth' "$LOG_FILE")
  if [ -z "$line" ]; then
    rm -f "$LOG_FILE"
    last_error="Failed to find bandwidth information in output"
    echo "[WARNING]: $last_error" >&2
    continue
  fi
  
  bandwidth=$(echo "$line" | awk '{print $5}')
  rm -f "$LOG_FILE"

  if [[ -z "$bandwidth" ]]; then
    last_error="Failed to parse bandwidth value from line: $line"
    echo "[WARNING]: $last_error" >&2
    continue
  fi
  
  if ! [[ "$bandwidth" =~ ^[0-9]+(\.[0-9]+)?$ ]]; then
    last_error="invalid bandwidth value($bandwidth)"
    echo "[WARNING]: $last_error" >&2
    continue
  fi

  if (( $(echo "$bandwidth > $best_bandwidth" | bc -l) )); then
    best_bandwidth=$bandwidth
  fi

  result=$(echo "$bandwidth >= $threshold" | bc -l)
  if [[ "$result" -eq 1 ]]; then
    success=1
    echo "[INFO] bandwidth: $bandwidth"
    break
  else
    echo "[WARNING] Attempt $attempt failed, bandwidth ($bandwidth) < threshold($threshold)" >&2
  fi
done

if [[ $success -ne 1 ]]; then
  echo "average bandwidth($best_bandwidth) < threshold($threshold)" >&2
  if [[ -n "$last_error" ]]; then
    echo "last error: $last_error" >&2
  fi
  exit 1
fi