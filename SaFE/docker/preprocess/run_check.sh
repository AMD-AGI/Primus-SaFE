#!/bin/sh

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

script_path="/shared-data/scripts"

while true; do
  if [ -d "$script_path" ]; then
    for file in "$script_path"/*; do
      if [ -e "$file" ]; then
        break
      fi
    done && [ -e "$file" ] && break
  fi
  sleep 60
done

pid_list=""
for script_file in "$script_path"/*; do
  chmod +x "$script_file"
  if [ -f "$script_file" ]; then
    echo "Running $script_file"
    /bin/sh "$script_file" &
    pid_list="$pid_list $!"
  fi
done

while [ -n "$pid_list" ]; do
  new_pid_list=""
  for pid in $pid_list; do
    kill -0 "$pid" 2>/dev/null
    running=$?

    if [ $running -ne 0 ]; then
      wait "$pid"
      exit_code=$?
      if [ $exit_code -ne 0 ]; then
        echo "Script exited with error code $exit_code" >&2
        exit $exit_code
      fi
    else
      new_pid_list="$new_pid_list $pid"
    fi
  done

  pid_list="$new_pid_list"
  if [ -n "$pid_list" ]; then
    sleep 1
  fi
done

exit 0