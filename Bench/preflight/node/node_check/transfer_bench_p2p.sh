
#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "TransferBench p2p" to measure bandwidth of unidirectional and bidirectional copy between CPU and GPU.

DIR_NAME="/opt/TransferBench"
if [ ! -d "$DIR_NAME" ]; then
  echo "the directory $DIR_NAME does not exist" >&2
  exit 1
fi

LOG_FILE="/tmp/transfer_p2p.log"
max_retries=5
success=0
last_error=""

for attempt in $(seq 1 $max_retries); do
  "$DIR_NAME/TransferBench" p2p >"$LOG_FILE"
  EXIT_CODE=$?
  if [ $EXIT_CODE -ne 0 ]; then
    last_error="TransferBench failed with exit code: $EXIT_CODE"
    echo "[WARNING] Attempt $attempt failed: $last_error" >&2
    rm -f "$LOG_FILE"
    continue
  fi

  mapfile -t lines < <(grep '^Averages' "$LOG_FILE" | head -n 2)
  rm -f "$LOG_FILE"
  if (( ${#lines[@]} < 2 )); then
    last_error="Expected 2 lines starting with 'Averages', but got only ${#lines[@]}"
    echo "[WARNING] Attempt $attempt failed: $last_error" >&2
    continue
  fi

  line1=${lines[0]}
  numbers1=($(echo "$line1" | awk '{for(i=4;i<=NF;i++) printf "%s ", $i}'))
  threshold=30.5
  all_above_threshold=true
  for num in "${numbers1[@]}"; do
    if (( $(echo "$num < $threshold" | bc -l) )); then
      all_above_threshold=false
      break
    fi
  done

  if [ "$all_above_threshold" = true ]; then
    echo "[INFO]: Averages (During UniDir) are greater than $threshold"
  else
    last_error="$line1, some averages are less than $threshold"
    echo "[WARNING] Attempt $attempt failed: $last_error" >&2
    continue
  fi

  line2=${lines[1]}
  numbers2=($(echo "$line2" | awk '{for(i=4;i<=NF;i++) printf "%s ", $i}'))
  all_above_threshold=true
  threshold=39.5
  average=0
  for num in "${numbers2[@]}"; do
    if (( $(echo "$num < $threshold" | bc -l) )); then
      all_above_threshold=false
      average=$num
      break
    fi
  done

  if [ "$all_above_threshold" = true ]; then
    echo "[INFO]: Averages (During BiDir) are greater than $threshold"
    success=1
    break
  else
    last_error="$line2, result($average) < threshold($threshold)"
    echo "[WARNING] Attempt $attempt failed: $last_error" >&2
    continue
  fi
done

if [[ $success -ne 1 ]]; then
  echo "$last_error" >&2
  exit 1
fi