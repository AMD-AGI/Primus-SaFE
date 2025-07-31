#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rocblas-bench" to benchmark(int8) the GEMM performance of rocBLAS
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

LOG_FILE="/tmp/roc_blas_int8_bench.log"
nsenter --target 1 --mount --uts --ipc --net --pid -- $DIR_NAME/build/release/clients/staging/rocblas-bench -f gemm_strided_batched_ex --transposeA N --transposeB T -m 1024 -n 2048 -k 512 --a_type i8_r --lda 1024 --stride_a 4096 --b_type i8_r --ldb 2048 --stride_b 4096 --c_type i32_r --ldc 1024 --stride_c 2097152 --d_type i32_r --ldd 1024 --stride_d 2097152 --compute_type i32_r --alpha 1.1 --beta 1 --batch_count 5 >$LOG_FILE
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  rm -f $LOG_FILE
  echo "[RocBlasInt8] [ERROR]: rocblas-bench failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

gflops=$(tail -1 $LOG_FILE| awk -F"," '{print $(NF-1)}' | tr -d ' ')
threshold=162700
rm -f $LOG_FILE
result=$(echo "$gflops >= $threshold" | bc -l)
if [[ "$result" -ne 1 ]]; then
  echo "[RocBlasInt8] [ERROR] failed to evaluate the GEMM performance, Gflops ($gflops) is less than the threshold($threshold)" >&2
  exit 1
fi
echo "[RocBlasInt8] [SUCCESS] tests passed, result: $gflops"
exit 0