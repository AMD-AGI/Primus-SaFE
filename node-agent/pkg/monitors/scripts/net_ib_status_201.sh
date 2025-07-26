#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

data=`nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/sbin/ibstatus`
if [ $? -ne 0 ]; then
  echo "Error: failed to execute ibstatus"
  exit 2
fi

device=""
state=""
while read -r line; do
  line=$(echo "$line" | sed 's/^ *//;s/ *$//')
  if [[ "$line" =~ ^Infiniband ]]; then
    device=$(echo "$line" | grep -oP "(?<=Infiniband device ')[^']+(?=')")
  elif [[ "$line" =~ ^state: ]]; then
    state=$(echo "$line" | awk '{print $NF}')
  elif [[ "$line" =~ ^phys" "state ]]; then
    phys_state=$(echo "$line" | awk '{print $NF}')
    if [[ "$phys_state" == "LinkUp" ]] && [[ "$state" == "DOWN" ]]; then
      echo "Error: Device '$device' is DOWN!"
      exit 1
    fi
  fi
done <<< "$data"