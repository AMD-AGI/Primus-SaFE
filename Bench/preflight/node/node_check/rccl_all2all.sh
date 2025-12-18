#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "alltoall_perf" to evaluate the AllReduce operator using the RCCL tests benchmark

DIR_NAME="/opt/rccl-tests"
if [ ! -d "$DIR_NAME" ]; then
  echo "the directory $DIR_NAME does not exist" >&2
  exit 1
fi

LOG_FILE="/tmp/all2all_perf.log"
export LD_LIBRARY_PATH="/opt/rocm/lib:/opt/mpich/lib:/usr/local/lib:$LD_LIBRARY_PATH"

# Use AMD ANP plugin for RCCL communication over AINIC devices if enabled
if [ "$ENABLE_ANP" = "true" ]; then
  echo "ANP enabled, using NCCL_NET_PLUGIN=anp"
  export NCCL_NET_PLUGIN=anp
  export LD_LIBRARY_PATH="/opt/amd-anp/lib:$LD_LIBRARY_PATH"
else
  echo "ANP disabled, using default network plugin"
fi
$DIR_NAME/build/alltoall_perf -b 8 -e 8G -f 2 -g 8 >$LOG_FILE
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  cat $LOG_FILE >&2
  rm -f $LOG_FILE
  echo "alltoall_perf failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

algbw=`grep "8589934592" $LOG_FILE |grep "268435456" |awk -F" " '{print $11}'`
rm -f $LOG_FILE
if ! [[ "$algbw" =~ ^[0-9]+(\.[0-9]+)?$ ]]; then
  echo "invalid algbw($algbw)" >&2
  exit 1
fi

threshold=300
if (( $(echo "$algbw < $threshold" | bc -l) )); then
  echo "algbw($algbw GB/s) < threshold($threshold GB/s)" >&2
  exit 1
fi
echo "[INFO] result: $algbw GB/s"