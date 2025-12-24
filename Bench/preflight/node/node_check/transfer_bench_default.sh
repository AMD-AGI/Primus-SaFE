#!/bin/bash

#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the default configuration to conduct performance testing through parallel transfers.

DIR_NAME="/opt/TransferBench"
if [ ! -d "$DIR_NAME" ]; then
  echo "the directory $DIR_NAME does not exist" >&2
  exit 1
fi

LOG_FILE="/tmp/transfer_default.log"
max_retries=5
success=0
last_error=""

for attempt in $(seq 1 $max_retries); do
  if [ $attempt -gt 1 ]; then
    sleep 5
  fi
 "$DIR_NAME/TransferBench" "$DIR_NAME/examples/example.cfg" >"$LOG_FILE" 2>&1
  EXIT_CODE=$?
  if [ $EXIT_CODE -ne 0 ]; then
    error_lines=$(grep -i '\[ERROR\]\|error\|failed' "$LOG_FILE" 2>/dev/null | head -5)
    last_error="TransferBench failed with exit code: $EXIT_CODE. Errors: $error_lines"
    rm -f "$LOG_FILE"
    echo "[WARNING] Attempt $attempt failed: $last_error" >&2
    continue
  fi

  # Check for HIP errors in log even if exit code is 0
  if grep -q '\[ERROR\]' "$LOG_FILE"; then
    error_lines=$(grep '\[ERROR\]' "$LOG_FILE" 2>/dev/null)
    last_error="TransferBench encountered HIP error: $error_lines"
    rm -f "$LOG_FILE"
    echo "[WARNING] Attempt $attempt failed: $last_error" >&2
    continue
  fi

  unset results
  declare -A results
  test_num=""

  while IFS= read -r line; do
    line=$(echo "$line" | sed 's/^[[:space:]]*//')
    if [[ "$line" =~ ^Test[[:space:]]([0-9]+): ]]; then
      test_num="Test_${BASH_REMATCH[1]}"
    elif [[ "$line" =~ ^Executor:* ]]; then
      sum_value=$(echo "$line" | awk '{print $(NF-2)}')
      if [[ "$test_num" == "Test_3" ]]; then
        if [[ -z "${results[Test_3_GPU00]}" ]]; then
          results[Test_3_GPU00]="$sum_value"
        else
          results[Test_3_GPU01]="$sum_value"
        fi
      else
        results["$test_num"]="$sum_value"
      fi
    fi
  done < "$LOG_FILE"
  rm -f "$LOG_FILE"

  check_result() {
    local test_name="$1"
    local threshold="$2"
    local value="${results[$test_name]}"
    if [[ -z "$value" ]]; then
      echo "[WARNING] Attempt $attempt failed: missing $test_name value" >&2
      return 1
    fi
    local result=$(echo "$value >= $threshold" | bc -l)
    if [[ "$result" -eq 1 ]]; then
      echo "[INFO] $test_name: value($value) >= threshold($threshold)"
      return 0
    else
      echo "[WARNING] Attempt $attempt failed: $test_name: value($value) < threshold($threshold)" >&2
      return 1
    fi
  }

  check_result_with_error() {
    local test_name="$1"
    local threshold="$2"
    local value="${results[$test_name]}"
    if [[ -z "$value" ]]; then
      last_error="missing $test_name value"
      return 1
    fi
    local result=$(echo "$value >= $threshold" | bc -l)
    if [[ "$result" -ne 1 ]]; then
      last_error="$test_name: value($value) < threshold($threshold)"
      return 1
    fi
    return 0
  }

  if check_result_with_error "Test_1" 42.4 && \
     check_result_with_error "Test_2" 43.5 && \
     check_result_with_error "Test_3_GPU00" 28.7 && \
     check_result_with_error "Test_3_GPU01" 35.0 && \
     check_result_with_error "Test_4" 1137 && \
     check_result_with_error "Test_5" 0 && \
     check_result_with_error "Test_6" 43.7; then
    success=1
    echo "[INFO] All tests passed on attempt $attempt"
    break
  fi
done

if [[ $success -ne 1 ]]; then
  echo "$last_error" >&2
  exit 1
fi