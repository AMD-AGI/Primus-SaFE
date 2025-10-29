#!/bin/bash

#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <interval>"
  echo "Example: $0 43200"
  exit 2
fi

current_time=$(date +%s)
previous_time=$((current_time - $1))
since1=$(date -d "@$previous_time" +'%Y-%m-%d %H:%M:%S')
since2=$(uptime -s)

timestamp1=$(date -d "$since1" +%s)
timestamp2=$(date -d "$since2" +%s)
since=$since1
if [ $timestamp1 -lt $timestamp2 ]; then
  since=$since2
fi

tmpfile=/tmp/.os_kernel
nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/journalctl --since="$since" > $tmpfile
ret=$?
if [ $ret -ne 0 ]; then
  echo "Error: failed to exec journalctl, since=$since, ret=$ret"
  rm -f $tmpfile
  exit 2
fi

msg=`grep -i "bug: soft lockup" $tmpfile`
if [ $? -eq 0 ]; then
  echo "$msg"
  rm -f $tmpfile
  exit 1
fi

msg=`grep -i "bug: hard lockup" $tmpfile`
if [ $? -eq 0 ]; then
  echo "$msg"
  rm -f $tmpfile
  exit 1
fi
rm -f $tmpfile