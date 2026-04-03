#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

if [ "$#" -lt 3 ]; then
  exit 2
fi

MOUNT_POINT="$1"
NFS_SERVER="$2"
NFS_PATH="$3"

# Check if mount point is already mounted and accessible
if nsenter --target 1 --mount --uts --ipc --net --pid -- mountpoint -q "$MOUNT_POINT" 2>/dev/null \
  && nsenter --target 1 --mount --uts --ipc --net --pid -- ls "$MOUNT_POINT" >/dev/null 2>&1; then
  exit 0
fi

# Create mount point only if directory does not exist
if ! nsenter --target 1 --mount --uts --ipc --net --pid -- test -d "$MOUNT_POINT"; then
  nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/mkdir -p "$MOUNT_POINT"
  if [ $? -ne 0 ]; then
    echo "Error: Failed to create directory: $MOUNT_POINT"
    exit 1
  fi
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- mount -t nfs4 "$NFS_SERVER:$NFS_PATH" "$MOUNT_POINT"
if [ $? -ne 0 ]; then
  echo "Error: NFS mount failed: $NFS_SERVER:$NFS_PATH -> $MOUNT_POINT"
  exit 1
fi

echo "NFS mounted successfully: $NFS_SERVER:$NFS_PATH -> $MOUNT_POINT"
exit 0
