#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "TransferBench p2p" to measure bandwidth of unidirectional and bidirectional copy between CPU and GPU.
# This script can only be run on AMD MI300X chips.

DIR_NAME="/root/TransferBench"
nsenter --target 1 --mount --uts --ipc --net --pid -- ls -d $DIR_NAME >/dev/null
if [ $? -ne 0 ]; then
  echo "[ERROR]: the directory $DIR_NAME does not exist" >&2
  exit 1
fi

LOG_FILE="/tmp/transfer_p2p.log"
nsenter --target 1 --mount --uts --ipc --net --pid -- $DIR_NAME/TransferBench p2p >$LOG_FILE
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  rm -f $LOG_FILE
  echo "[TransferBenchP2P] [ERROR]: TransferBench failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

mapfile -t lines < <(grep '^Averages' "$LOG_FILE" | head -n 2)
rm -f "$LOG_FILE"
if (( ${#lines[@]} < 2 )); then
  echo "[TransferBenchP2P] [ERROR] Expected 2 lines starting with 'Averages', but got only ${#lines[@]}" >&2
  exit 1
fi

line1=${lines[0]}
numbers1=($(echo $line1 | awk '{for(i=4;i<=NF;i++) printf "%s ", $i}'))
all_above_33_9=true
for num in "${numbers1[@]}"; do
  if (( $(echo "$num < 33.9" | bc -l) )); then
    all_above_33_9=false
    break
  fi
done

if [ "$all_above_33_9" = true ]; then
  echo "[TransferBenchP2P] [INFO]: Averages (During UniDir) are greater than 33.9."
else
  echo "[TransferBenchP2P] [ERROR]: $line1, some averages are less than 33.9." >&2
  exit 1
fi

line2=${lines[1]}
numbers2=($(echo $line2 | awk '{for(i=4;i<=NF;i++) printf "%s ", $i}'))
all_above_43_9=true
for num in "${numbers2[@]}"; do
  if (( $(echo "$num < 43.9" | bc -l) )); then
    all_above_43_9=false
    break
  fi
done

if [ "$all_above_43_9" = true ]; then
  echo "[TransferBenchP2P] [INFO]: Averages (During  BiDir) are greater than 43.9."
else
  echo "[TransferBenchP2P] [ERROR]: failed to measure bandwidth between cpu and gpu, $line2, the value is less than threshold(43.9)." >&2
  exit 1
fi
echo "[TransferBenchP2P] [SUCCESS]: tests passed"
exit 0
