#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

if [ "$ADD_LOG_HEADER" == "true" ]; then
  export LOG_HEADER="[$(hostname)] [NODE-$RANK] "
fi

echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] start to diagnose"
export RANK=$RANK
export NCCL_SOCKET_IFNAME=${NCCL_SOCKET_IFNAME:-"eth0"}
export NCCL_IB_HCA=${NCCL_IB_HCA:-""}
export TEST_LEVEL=${TEST_LEVEL:-"BASIC"}

# ========================================
# Phase 1: check configuration on node
# ========================================

errors=""
log_file=$(mktemp) && touch "$log_file"
err_file=$(mktemp) && touch "$err_file"
tail -f "$log_file" &
tail_pid=$! && sleep 0.5
bash "config_check/run.sh" > "$log_file" 2>"$err_file"
exit_code=$?
sync && sleep 2 && kill $tail_pid 2>/dev/null && rm -f "$log_file"
if [ $exit_code -ne 0 ]; then
  error_output=$(cat "$err_file" | tr -d '\n')
  errors+="$error_output"
fi
rm -f "$err_file"

# ===========================================
# Phase 2: do node-tests including rccl-test,
#          cpu-perf and so on
# ==========================================

log_file=$(mktemp) && touch "$log_file"
err_file=$(mktemp) && touch "$err_file"
tail -f "$log_file" &
tail_pid=$! && sleep 0.5
bash "node_check/run.sh" > "$log_file" 2>"$err_file"
exit_code=$?
sync && sleep 2 && kill $tail_pid 2>/dev/null && rm -f "$log_file"
if [ $exit_code -ne 0 ]; then
  if [ -n "$errors" ]; then
   errors+=" | "
  fi
  error_output=$(cat "$err_file" | tr -d '\n')
  errors+="$error_output"
fi
rm -f "$err_file"


# ===========================================
# Phase 3: do model-check tests including 
#          model-train, model-inference and so on
# ===========================================

log_file=$(mktemp) && touch "$log_file"
err_file=$(mktemp) && touch "$err_file"
tail -f "$log_file" &
tail_pid=$! && sleep 0.5
bash -c "cd model_check && bash run.sh" > "$log_file" 2>"$err_file"
exit_code=$?
sync && sleep 2 && kill $tail_pid 2>/dev/null && rm -f "$log_file"
if [ $exit_code -ne 0 ]; then
  if [ -n "$errors" ]; then
   errors+=" | "
  fi
  error_output=$(cat "$err_file" | tr -d '\n')
  errors+="$error_output"
fi
rm -f "$err_file"
echo "errors: $errors,exit_code: $exit_code"
# ===========================================
# Phase 4: output summary
# ===========================================
ret=0
if [ -n "$errors" ]; then
  echo "${LOG_HEADER}[NODE] [ERROR]❌: $errors"
  ret=1
elif [ $exit_code -ne 0 ]; then
  echo "${LOG_HEADER}[NODE] [ERROR]❌: model_check failed with exit code $exit_code (check logs for details)"
  ret=1
else
  echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] [NODE] [SUCCESS] ✅ All check passed"
fi

echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] diagnose finished"
exit $ret