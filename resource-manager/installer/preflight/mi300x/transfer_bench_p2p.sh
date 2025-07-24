#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "TransferBench p2p" to measure bandwidth of unidirectional and bidirectional copy between CPU and GPU.
# This script can only be run on AMD MI300X chips.

REPO_URL="https://github.com/ROCm/TransferBench.git"
DIR_NAME="TransferBench"
LOG_FILE="/tmp/transfer_p2p.log"
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

dpkg -l | grep -q make
if [ $? -ne 0 ]; then
  apt-get update >/dev/null && apt-get -y install make >/dev/null
  if [ $? -ne 0 ]; then
    echo "[ERROR]: failed to install make" >&2
    exit 1
  fi
fi

CC=hipcc make > /dev/null
if [ $? -ne 0 ]; then
  echo "[ERROR]: failed to make TransferBench" >&2
  exit 1
fi

./TransferBench p2p >$LOG_FILE
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
  echo "[TransferBenchP2P] [ERROR]: $line2, some averages are less than 43.9." >&2
  exit 1
fi
echo "[TransferBenchP2P] [SUCCESS]: tests passed"
exit 0
