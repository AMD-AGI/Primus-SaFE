#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

if [ "$#" -lt 3 ]; then
  exit 2
fi

NFS_SERVER="$1"
NFS_PATH="$2"
MOUNT_POINT=$3
if [ -z "$NFS_SERVER" ] || [ -z "$NFS_PATH" ] || [ -z "$MOUNT_POINT" ]; then
  echo "Usage: $0 <nfs_server> <nfs_path> <nfs_mount>"
  echo "Example: $0 45.76.27.91 /mnt/nvme0 /nfs"
  exit 2
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- df -h | grep -q "$MOUNT_POINT" > /dev/null
if [ $? -eq 0 ]; then
  exit 0
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- mkdir -p "$MOUNT_POINT"
if [ $? -ne 0 ]; then
  echo "Failed to create directory: $MOUNT_POINT"
  exit 1
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- mount -t nfs4 "$NFS_SERVER:$NFS_PATH" "$MOUNT_POINT"
if [ $? -ne 0 ]; then
  echo "NFS mount failed: $NFS_SERVER:$NFS_PATH -> $MOUNT_POINT"
  exit 1
else
  echo "NFS mounted successfully: $NFS_SERVER:$NFS_PATH -> $MOUNT_POINT"
fi

