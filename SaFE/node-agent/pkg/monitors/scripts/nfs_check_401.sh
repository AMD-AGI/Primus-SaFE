#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <mount_point> [nfs_server] [nfs_path]"
  echo "Example: $0 /nfs"
  echo "Example: $0 /nfs 45.76.27.91 /mnt/nvme0"
  exit 2
fi

MOUNT_POINT="$1"
NFS_SERVER="${2:-}"
NFS_PATH="${3:-}"

if [ -z "$MOUNT_POINT" ]; then
  echo "Error: mount_point cannot be empty"
  exit 2
fi

# Check if mount point exists and is accessible (ls avoids df+grep false matches)
if nsenter --target 1 --mount --uts --ipc --net --pid -- ls "$MOUNT_POINT" >/dev/null 2>&1; then
  exit 0
fi

# Mount does not exist and nfs_server/nfs_path not provided: cannot mount
if [ -z "$NFS_SERVER" ] || [ -z "$NFS_PATH" ]; then
  echo "Error: mount point $MOUNT_POINT does not exist"
  exit 1
fi

# Create mount point and mount
nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/mkdir -p "$MOUNT_POINT"
if [ $? -ne 0 ]; then
  echo "Error: Failed to create directory: $MOUNT_POINT"
  exit 1
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- mount -t nfs4 "$NFS_SERVER:$NFS_PATH" "$MOUNT_POINT"
if [ $? -ne 0 ]; then
  echo "Error: NFS mount failed: $NFS_SERVER:$NFS_PATH -> $MOUNT_POINT"
  exit 1
fi

echo "NFS mounted successfully: $NFS_SERVER:$NFS_PATH -> $MOUNT_POINT"
exit 0
