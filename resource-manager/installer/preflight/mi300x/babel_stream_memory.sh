#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# BabelStream is a benchmarking program designed to evaluate memory bandwidth performance.

DIR_NAME="/root/BabelStream"
nsenter --target 1 --mount --uts --ipc --net --pid -- ls -d $DIR_NAME >/dev/null
if [ $? -ne 0 ]; then
  echo "[ERROR]: the directory $DIR_NAME does not exist" >&2
  exit 1
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- dpkg -l | grep -q openmpi-bin
if [ $? -ne 0 ]; then
  echo "[ERROR]: openmpi-bin is not found" >&2
  exit 1
fi

LOG_FILE="/tmp/babel_stream.log"
nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/mpiexec -n 8 --allow-run-as-root $DIR_NAME/wrapper.sh >$LOG_FILE
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  rm -f $LOG_FILE
  echo "[ERROR]: mpiexec failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

declare -A thresholds=(
  ["Copy"]=4177285
  ["Mul"]=4067069
  ["Add"]=3920853
  ["Triad"]=3885301
  ["Dot"]=3660781
)
copy_sum=0 mul_sum=0 add_sum=0 triad_sum=0 dot_sum=0
copy_count=0 mul_count=0 add_count=0 triad_count=0 dot_count=0
grep -A5 '^Function' "$LOG_FILE" | awk '
$1 == "Copy" {
  copy_sum += $2;
  copy_count += 1;
}
$1 == "Mul" {
  mul_sum += $2;
  mul_count += 1;
}
$1 == "Add" {
  add_sum += $2;
  add_count += 1;
}
$1 == "Triad" {
  triad_sum += $2;
  triad_count += 1;
}
$1 == "Dot" {
  dot_sum += $2;
  dot_count += 1;
}
END {
  if (copy_count > 0)   print "Copy "   copy_sum / copy_count;
  if (mul_count > 0)    print "Mul "    mul_sum / mul_count;
  if (add_count > 0)    print "Add "    add_sum / add_count;
  if (triad_count > 0)  print "Triad "  triad_sum / triad_count;
  if (dot_count > 0)    print "Dot "    dot_sum / dot_count;
}' | while read -r func avg; do
  formatted_avg=$(echo "$avg" | awk '{printf "%f", $1}')
  threshold=${thresholds[$func]}
  is_greater=$(echo "$formatted_avg > $threshold" | bc -l)
  if [[ "$is_greater" -eq 1 ]]; then
    echo "[BabelStream] [INFO] $func average: $formatted_avg > $threshold"
  else
    echo "[BabelStream] [ERROR] failed to evaluate memory bandwidth performance, $func average($formatted_avg) is less than threshold($threshold)" >&2
    rm -f $LOG_FILE
    exit 1
  fi
done
rm -f $LOG_FILE
echo "[BabelStream] [SUCCESS] tests passed"
exit 0