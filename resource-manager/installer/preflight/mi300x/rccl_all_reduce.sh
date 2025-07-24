#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "all_reduce_perf" to evaluate the AllReduce operator using the RCCL tests benchmark
# This script can only be run on AMD MI300X chips.

REPO_URL="https://github.com/ROCm/rccl-tests.git"
DIR_NAME="rccl-tests"
if [ ! -d "$DIR_NAME" ]; then
  dpkg -l | grep -q git
  if [ $? -ne 0 ]; then
    apt-get update >/dev/null && apt-get -y install git >/dev/null
    if [ $? -ne 0 ]; then
      echo "[ERROR]: failed to install git" >&2
      exit 1
    fi
  fi

  git clone "$REPO_URL" >/dev/null
  if [ $? -ne 0 ]; then
    echo "[ERROR]: failed to clone $REPO_URL" >&2
    exit 1
  fi
fi
cd "$DIR_NAME" || { echo "[ERROR]: unable to access $DIR_NAME"; exit 1; }
if [ ! -f ./build/all_reduce_perf ]; then
  dpkg -l | grep -q make
  if [ $? -ne 0 ]; then
    apt-get update >/dev/null && apt-get -y install make >/dev/null
    if [ $? -ne 0 ]; then
      echo "[ERROR]: failed to install make" >&2
      exit 1
    fi
  fi

  make NCCL_HOME=/opt/rocm/ >/dev/null
  if [ $? -ne 0 ]; then
    echo "[ERROR]: failed to make all_reduce_perf" >&2
    exit 1
  fi
fi

LOG_FILE="/tmp/all_reduce_perf.log"
./build/all_reduce_perf -b 8 -e 8G -f 2 -g 8 >$LOG_FILE
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  rm -f $LOG_FILE
  echo "[RcclAllReduce] [ERROR]: all_reduce_perf failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

busbw=`grep "8589934592" $LOG_FILE |grep "2147483648" |awk '{print $8}'`
rm -f $LOG_FILE
if ! [[ "$busbw" =~ ^[0-9]+(\.[0-9]+)?$ ]]; then
  echo "[RcclAllReduce] [ERROR] invalid busbw" >&2
  exit 1
fi

if (( $(echo "$busbw < 304" | bc -l) )); then
  echo "[RcclAllReduce] [ERROR] the result($busbw GB/s) is less than the threshold(304 GB/s) at a message size of 8589934592B." >&2
  exit 1
fi
echo "[RcclAllReduce] [SUCCESS] tests passed, result: $busbw GB/s"
exit 0