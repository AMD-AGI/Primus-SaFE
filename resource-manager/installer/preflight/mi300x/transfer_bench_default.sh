#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the default configuration to conduct performance testing through parallel transfers.
# This script can only be run on AMD MI300X chips.

DIR_NAME="/root/TransferBench"
nsenter --target 1 --mount --uts --ipc --net --pid -- ls -d $DIR_NAME >/dev/null
if [ $? -ne 0 ]; then
  echo "[ERROR]: the directory $DIR_NAME does not exist" >&2
  exit 1
fi

LOG_FILE="/tmp/transfer_default.log"
nsenter --target 1 --mount --uts --ipc --net --pid -- $DIR_NAME/TransferBench $DIR_NAME/examples/example.cfg >$LOG_FILE
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  cat $LOG_FILE
  rm -f $LOG_FILE
  echo "[TransferBenchDefault] [ERROR]: TransferBench failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

declare -A results
test_num=""
while IFS= read -r line; do
  line=$(echo "$line" | sed 's/^[[:space:]]*//')
  if [[ "$line" =~ ^Test[[:space:]]([0-9]+): ]]; then
    test_num="Test_${BASH_REMATCH[1]}"
  elif [[ "$line" =~ ^Executor:* ]]; then
    sum_value=`echo $line | awk '{print $(NF-2)}'`
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
rm -f $LOG_FILE

check_result() {
  local test_name="$1"
  local threshold="$2"
  local value="${results[$test_name]}"
  if [[ -z "$value" ]]; then
    echo "[TransferBenchDefault] [ERROR] $test_name: missing result value" >&2
    exit 1
  fi
  local result=$(echo "$value >= $threshold" | bc -l)
  if [[ "$result" -eq 1 ]]; then
    echo "[TransferBenchDefault] [INFO] $test_name: $value >= $threshold"
  else
    echo "[TransferBenchDefault] [ERROR] the parallel transfer rates does not meet the standard. $test_name: value($value) < threshold($threshold)" >&2
    exit 1
  fi
}

check_result "Test_1" 47.1
check_result "Test_2" 48.4
check_result "Test_3_GPU00" 31.9
check_result "Test_3_GPU01" 38.9
check_result "Test_4" 1264
check_result "Test_5" 0
check_result "Test_6" 48.6

echo "[TransferBenchDefault] [SUCCESS] tests passed"
exit 0