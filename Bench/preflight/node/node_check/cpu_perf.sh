#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the perf tool to test CPU performance

if [ ! -x /usr/bin/perf ]; then
  echo "Error: /usr/bin/perf not found." >&2
  exit 1
fi

LOG_FILE="/tmp/perf_cpu.log"
/usr/bin/perf stat -e cycles,instructions,cache-misses -a -r 10 -- sleep 3 >$LOG_FILE 2>&1
EXIT_CODE=$?

if [ $EXIT_CODE -ne 0 ]; then
  log_content=$(cat "$LOG_FILE" 2>/dev/null)
  rm -f "$LOG_FILE"
  echo "perf failed with exit code: $EXIT_CODE, output: $log_content" >&2
  exit 1
fi

insn_per_cycle=$(grep 'insn per cycle' $LOG_FILE | awk '{for(i=1;i<=NF;i++){if($i=="insn") print $(i-1)}}')
rm -f $LOG_FILE
if [[ -n "$insn_per_cycle" ]]; then
  echo "[INFO] insn per cycle = $insn_per_cycle"
else
  echo "failed to get insn per cycle" >&2
  exit 1
fi
threshold=1
is_greater=$(echo "$insn_per_cycle >= $threshold" | bc -l)
if [[ "$is_greater" -ne 1 ]]; then
  echo "insn-per-cycle($insn_per_cycle) < threshold($threshold)" >&2
  exit 1
fi