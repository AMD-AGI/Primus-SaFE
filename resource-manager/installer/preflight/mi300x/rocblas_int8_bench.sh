#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rocblas-bench" to benchmark(int8) the GEMM performance of rocBLAS
# This script can only be run on AMD MI300X chips.

dpkg -l | grep -q "libgtest-dev"
if [ $? -ne 0 ]; then
  apt-get update >/dev/null && apt install -y libgtest-dev >/dev/null
  if [ $? -ne 0 ]; then
    echo "[ERROR]: failed to install libgtest-dev" >&2
    exit 1
  fi
fi

REPO_URL="https://github.com/ROCm/rocBLAS.git"
DIR_NAME="rocBLAS"
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
cd "$DIR_NAME" || { echo "[ERROR]: unable to access $DIR_NAME" >&2; exit 1; }
if [ ! -f ./build/release/clients/staging/rocblas-bench ]; then
  git checkout rocm-6.2.0 >/dev/null && chmod +x ./install.sh && ./install.sh --clients-only --library-path /opt/rocm >/dev/null
  if [ $? -ne 0 ]; then
    echo "[ERROR]: failed to install rocm" >&2
    exit 1
  fi
fi

LOG_FILE="/tmp/roc_blas_int8_bench.log"
./build/release/clients/staging/rocblas-bench -f gemm_strided_batched_ex --transposeA N --transposeB T -m 1024 -n 2048 -k 512 --a_type i8_r --lda 1024 --stride_a 4096 --b_type i8_r --ldb 2048 --stride_b 4096 --c_type i32_r --ldc 1024 --stride_c 2097152 --d_type i32_r --ldd 1024 --stride_d 2097152 --compute_type i32_r --alpha 1.1 --beta 1 --batch_count 5 >$LOG_FILE
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
  echo "[RocBlasInt8] [ERROR] Gflops ($gflops) is less than the threshold($threshold)" >&2
  exit 1
fi
echo "[RocBlasInt8] [SUCCESS] tests passed, result: $gflops"
exit 0