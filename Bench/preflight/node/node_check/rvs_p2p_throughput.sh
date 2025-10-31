#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rvs -c pbqt_single.conf" to do throughput test between all P2P pairs

RVS_CONF=/opt/rocm/share/rocm-validation-suite/conf/$GPU_PRODUCT/pbqt_single.conf
if [ ! -f "${RVS_CONF}" ]; then
  echo "${RVS_CONF} does not exist" >&2
  exit 1
fi

LOG_FILE="/tmp/pbqt_single.log"
/opt/rocm/bin/rvs -c "${RVS_CONF}" >$LOG_FILE
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  rm -f $LOG_FILE
  echo "rvs failed with exit code: $EXIT_CODE" >&2
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
      echo "$line, peers is not true" >&2
      rm -f $LOG_FILE
      exit 1
    fi
  elif [[ "$line" == *"$current_action] p2p-bandwidth"* ]]; then
    if ! echo "$line" | grep -qE "([0-9]+\.)?[0-9]+ GBps"; then
      echo "$line, throughput is not found" >&2
      rm -f $LOG_FILE
      exit 1
    else
      throughput=$(echo "$line" | grep -oE "([0-9]+\.)?[0-9]+ GBps" | awk '{print $1}')
      if (( $(echo "$throughput <= 0" | bc -l) )); then
        echo "$line, throughput($throughput) <= 0" >&2
        rm -f $LOG_FILE
        exit 1
      fi
    fi
  fi
done < "$LOG_FILE"

rm -f $LOG_FILE