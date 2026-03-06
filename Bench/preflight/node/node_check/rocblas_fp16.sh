#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rocblas-bench" to benchmark(fp16) the GEMM performance of rocBLAS

DIR_NAME="/opt/rocBLAS"
if [ ! -d "$DIR_NAME" ]; then
  echo "the directory $DIR_NAME does not exist" >&2
  exit 1
fi

LOG_FILE="/tmp/roc_blas_fp16_bench.log"
max_retries=7
best_gflops=0
success=0

threshold=0
if [[ "$GPU_PRODUCT" == *"MI355X"* ]]; then
  threshold=300768
else
  threshold=117540
fi

for attempt in $(seq 1 $max_retries); do
  $DIR_NAME/build/release/clients/staging/rocblas-bench -f gemm_strided_batched_ex --transposeA N --transposeB T -m 1024 -n 2048 -k 512 --a_type h --lda 1024 --stride_a 4096 --b_type h --ldb 2048 --stride_b 4096 --c_type s --ldc 1024 --stride_c 2097152 --d_type s --ldd 1024 --stride_d 2097152 --compute_type s --alpha 1.1 --beta 1 --batch_count 5 >$LOG_FILE
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
