#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the perf tool to test CPU performance

linux_tools="linux-tools-$(uname -r)"
nsenter --target 1 --mount --uts --ipc --net --pid -- dpkg -l | grep -q "$linux_tools"
if [ $? -ne 0 ]; then
  echo "[ERROR]: $linux_tools is not found" >&2
  exit 1
fi

LOG_FILE="/tmp/perf_cpu.log"
nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/perf stat -e cycles,instructions,cache-misses -a -r 10 -- sleep 3 >$LOG_FILE 2>&1
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  rm -f $LOG_FILE
  echo "[PerfCpu] [ERROR]: perf failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

insn_per_cycle=$(grep 'insn per cycle' $LOG_FILE | awk '{for(i=1;i<=NF;i++){if($i=="insn") print $(i-1)}}')
rm -f $LOG_FILE
if [[ -n "$insn_per_cycle" ]]; then
  echo "[PerfCpu] [INFO] insn per cycle = $insn_per_cycle"
else
  echo "[PerfCpu] [ERROR] failed to get insn per cycle" >&2
  exit 1
fi
threshold=1
is_greater=$(echo "$insn_per_cycle >= $threshold" | bc -l)
if [[ "$is_greater" -ne 1 ]]; then
  echo "[PerfCpu] [ERROR] failed to evaluate CPU performance, insn per cycle($insn_per_cycle) is less than the threshold($threshold)" >&2
  exit 1
fi
echo "[PerfCpu] [SUCCESS] tests passed"
exit 0