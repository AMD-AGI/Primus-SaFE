#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rocblas-bench" to benchmark(fp32) the GEMM performance of rocBLAS

DIR_NAME="/opt/rocBLAS"
if [ ! -d "$DIR_NAME" ]; then
  echo "the directory $DIR_NAME does not exist" >&2
  exit 1
fi

LOG_FILE="/tmp/roc_blas_fp32_bench.log"
max_retries=7
best_gflops=0
success=0
threshold=84690

for attempt in $(seq 1 $max_retries); do
  $DIR_NAME/build/release/clients/staging/rocblas-bench -f gemm -r s -m 4000 -n 4000 -k 4000 --lda 4000 --ldb 4000 --ldc 4000 --transposeA N --transposeB T >$LOG_FILE
  EXIT_CODE=$?
  if [ $EXIT_CODE -ne 0 ]; then
    rm -f $LOG_FILE
    echo "[WARNING]: rocblas-bench failed with exit code: $EXIT_CODE" >&2
    continue
  fi
  gflops=$(tail -1 $LOG_FILE| awk -F"," '{print $(NF-1)}' | tr -d ' ')
  if (( $(echo "$gflops > $best_gflops" | bc -l) )); then
    best_gflops=$gflops
  fi

  rm -f $LOG_FILE
  result=$(echo "$gflops >= $threshold" | bc -l)
  if [[ "$result" -eq 1 ]]; then
    echo "[INFO] result: $gflops"
    success=1
    break
  else
    echo "[WARNING] Attempt $attempt failed, Gflops ($gflops) is less than the threshold($threshold)" >&2
  fi
done

if [[ $success -ne 1 ]]; then
  echo "Gflops($best_gflops) < threshold($threshold)" >&2
  exit 1
fi
