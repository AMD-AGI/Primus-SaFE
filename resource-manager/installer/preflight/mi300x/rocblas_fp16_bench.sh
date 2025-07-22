#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rocblas-bench" to benchmark(fp16) the GEMM performance of rocBLAS
# This script can only be run on AMD MI300X chips.

dpkg -l | grep -q "libgtest-dev"
if [ $? -ne 0 ]; then
  apt install -y libgtest-dev >/dev/null 2>error
  if [ $? -ne 0 ]; then
    cat error && rm -f error
    echo "[ERROR]: failed to install libgtest-dev" >&2
    exit 1
  fi
  rm -f error
fi
REPO_URL="https://github.com/ROCm/rocBLAS.git"
DIR_NAME="rocBLAS"
if [ ! -d "$DIR_NAME" ]; then
  git clone "$REPO_URL" >/dev/null 2>error
  if [ $? -ne 0 ]; then
    cat error && rm -f error
    echo "[ERROR]: failed to clone $REPO_URL" >&2
    exit 1
  fi
  rm -f error
fi
cd "$DIR_NAME" || { echo "[ERROR]: unable to access $DIR_NAME" >&2; exit 1; }
if [ ! -f ./build/release/clients/staging/rocblas-bench ]; then
  git checkout rocm-6.2.0 >/dev/null 2>&1
  ./install.sh --clients-only --library-path /opt/rocm >/dev/null 2>error
  if [ $? -ne 0 ]; then
    cat error && rm -f error
    echo "[ERROR]: failed to install rocm" >&2
    exit 1
  fi
  rm -f error
fi

LOG_FILE="/tmp/roc_blas_fp16_bench.log"
./build/release/clients/staging/rocblas-bench -f gemm_strided_batched_ex --transposeA N --transposeB T -m 1024 -n 2048 -k 512 --a_type h --lda 1024 --stride_a 4096 --b_type h --ldb 2048 --stride_b 4096 --c_type s --ldc 1024 --stride_c 2097152 --d_type s --ldd 1024 --stride_d 2097152 --compute_type s --alpha 1.1 --beta 1 --batch_count 5 >$LOG_FILE 2>&1
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  cat $LOG_FILE && rm -f $LOG_FILE
  echo "[RocBlasFp16] [ERROR]: rocblas-bench failed with exit code: $EXIT_CODE" >&2
  exit 1
fi
gflops=$(tail -1 $LOG_FILE| awk -F"," '{print $(NF-1)}' | tr -d ' ')
threshold=130600
rm -f $LOG_FILE
result=$(echo "$gflops >= $threshold" | bc -l)
if [[ "$result" -ne 1 ]]; then
  echo "[RocBlasFp16] [ERROR] Gflops ($gflops) is less than the threshold($threshold)" >&2
  exit 1
fi
echo "[RocBlasFp16] [SUCCESS] tests passed, result: $gflops"
exit 0