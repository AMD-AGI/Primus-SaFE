#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

container_count=$(nsenter --target 1 --mount --uts --ipc --net --pid -- docker ps --format '{{.ID}}' | wc -l)
if [ $? -ne 0 ]; then
  exit 0
fi
if [ "$container_count" -le 0 ]; then
  exit 0
fi

# If you have obtained the node information and specified a workspace where operations are permitted, you can directly remove Docker.
if [ "$#" -ge 2 ]; then
  workspaceId=`echo "$1" |jq '.workspaceId'`
  if [ -z "$workspaceId" ] || [ "$workspaceId" == "null" ] ; then
    echo "Error: failed to get workspaceId from input: $1"
    exit 1
  fi
  workspace_ids=$(echo "$2" | tr ',' ' ')
  for id in $workspace_ids; do
    if [ "$id" != "$workspaceId" ]; then
      continue
    fi
    container_list=$(nsenter --target 1 --mount --uts --ipc --net --pid -- docker ps -q)
    if [ -n "$container_list" ]; then
      nsenter --target 1 --mount --uts --ipc --net --pid -- docker stop $container_list
    fi
    nsenter --target 1 --mount --uts --ipc --net --pid -- apt remove -y docker.io
    if [ $? -eq 0 ]; then
      echo "Docker has been removed successfully."
      exit 0
    fi
    break
  done
fi

echo "A Docker process exists"
exit 1