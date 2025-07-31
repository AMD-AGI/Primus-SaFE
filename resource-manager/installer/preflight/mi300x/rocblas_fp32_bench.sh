#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rocblas-bench" to benchmark(fp32) the GEMM performance of rocBLAS
# This script can only be run on AMD MI300X chips.

DIR_NAME="/root/rocBLAS"
nsenter --target 1 --mount --uts --ipc --net --pid -- ls -d $DIR_NAME >/dev/null
if [ $? -ne 0 ]; then
  echo "[ERROR]: the directory $DIR_NAME does not exist" >&2
  exit 1
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- dpkg -l | grep -q "libgtest-dev"
if [ $? -ne 0 ]; then
  echo "[ERROR]: libgtest-dev is not found" >&2
  exit 1
fi

LOG_FILE="/tmp/roc_blas_fp32_bench.log"
nsenter --target 1 --mount --uts --ipc --net --pid -- $DIR_NAME/build/release/clients/staging/rocblas-bench -f gemm -r s -m 4000 -n 4000 -k 4000 --lda 4000 --ldb 4000 --ldc 4000 --transposeA N --transposeB T >$LOG_FILE
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  rm -f $LOG_FILE
  echo "[RocBlasFp32] [ERROR]: rocblas-bench failed with exit code: $EXIT_CODE" >&2
  exit 1
fi
gflops=$(tail -1 $LOG_FILE| awk -F"," '{print $(NF-1)}' | tr -d ' ')
threshold=94100
rm -f $LOG_FILE
result=$(echo "$gflops >= $threshold" | bc -l)
if [[ "$result" -ne 1 ]]; then
  echo "[RocBlasFp32] [ERROR] failed to evaluate the GEMM performance, Gflops ($gflops) is less than the threshold($threshold)" >&2
  exit 1
fi
echo "[RocBlasFp32] [SUCCESS] tests passed, result: $gflops"
exit 0