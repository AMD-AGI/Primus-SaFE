#!/bin/sh

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

if [ -z "${HANG_CHECK_INTERVAL}" ] || [ "${JOB_KIND}" != "PytorchJob" ]; then
  exit 0
fi

# Check required env vars
if [ -z "$RANK" ] || [ -z "$WORLD_SIZE" ]; then
  exit 0
fi

# Convert RANK and WORLD_SIZE to integers (in case they are not)
RANK=$(expr "$RANK" + 0)
WORLD_SIZE=$(expr "$WORLD_SIZE" + 0)

LAST_RANK=$(expr "$WORLD_SIZE" - 1)

# Only the last node performs the hang detection check
if [ "$RANK" -ne "$LAST_RANK" ]; then
   exit 0
fi

logpath="/var/log/pods/${POD_NAMESPACE}_${POD_NAME}_${POD_UID}/${MAIN_CONTAINER_NAME}/0.log"
previous_size=0
previous_time=$(date +%s)

# Monitor the file every 60 seconds. Terminate the process if the file remains unchanged beyond the specified duration.
while true; do
  current_size=$previous_size
  if [ -e "$logpath" ]; then
    current_size=$(wc -c < "$logpath")
  fi

  current_time=$(date +%s)

  if [ "$current_size" -ne "$previous_size" ]; then
    previous_time=$current_time
    previous_size=$current_size
  else
    time_diff=$(expr "$current_time" - "$previous_time")
    if [ "$time_diff" -gt "${HANG_CHECK_INTERVAL}" ]; then
      echo "[ERROR] the log has not changed in the past ${HANG_CHECK_INTERVAL} seconds."
      exit 100
    fi
  fi

  sleep 60
done

exit 0