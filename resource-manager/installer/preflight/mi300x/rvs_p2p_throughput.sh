#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rvs -c pbqt_single.conf" to do throughput test between all P2P pairs
# This script can only be run on AMD MI300X chips.

dpkg -l | grep -q rocm-validation-suite
if [ $? -ne 0 ]; then
  apt-get update >/dev/null 2>&1
  apt install -y rocm-validation-suite >/dev/null 2>error
  if [ $? -ne 0 ]; then
    cat error && rm -f error
    echo "[ERROR] failed to install rocm-validation-suite" >&2
    exit 1
  fi
  rm -f error
fi

export PATH=$PATH:/opt/rocm/bin
export RVS_CONF=/opt/rocm/share/rocm-validation-suite/conf
LOG_FILE="/tmp/pbqt_single.log"
rvs -c "${RVS_CONF}/MI300X/pbqt_single.conf" >$LOG_FILE 2>&1
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  cat $LOG_FILE && rm -f $LOG_FILE
  echo "[RvsP2p] [ERROR] rvs failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

current_action=""
while IFS= read -r line; do
  if echo "$line" | grep -q "Action name"; then
    current_action=$(echo "$line" | awk -F':' '{print $2}')
    continue
  fi
  if [[ "$line" == *"$current_action] p2p "* ]]; then
    if ! echo "$line" | grep -q "peers:true"; then
      echo "[RvsP2p] [ERROR]: $line, peers is not true" >&2
      rm -f $LOG_FILE
      exit 1
    fi
  elif [[ "$line" == *"$current_action] p2p-bandwidth"* ]]; then
    if ! echo "$line" | grep -qE "([0-9]+\.)?[0-9]+ GBps"; then
      echo "[RvsP2p] [ERROR]: $line, throughtput is not found" >&2
      rm -f $LOG_FILE
      exit 1
    else
      throughput=$(echo "$line" | grep -oE "([0-9]+\.)?[0-9]+ GBps" | awk '{print $1}')
      if (( $(echo "$throughput <= 0" | bc -l) )); then
        echo "[RvsP2p] [ERROR]: $line, throughtput is less than or equal 0" >&2
        rm -f $LOG_FILE
        exit 1
      fi
    fi
  fi
done < "$LOG_FILE"
echo "[RvsP2p] [SUCCESS]: tests passed"
rm -f $LOG_FILE
exit 0