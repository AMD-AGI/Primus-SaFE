#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

if [ "$#" -lt 1 ]; then
  exit 2
fi

# Parse comma-separated NFS paths
IFS=',' read -ra NFS_PATHS <<< "$1"

for nfs_path in "${NFS_PATHS[@]}"; do
  # Trim whitespace
  nfs_path=$(echo "$nfs_path" | xargs)

  if [ -z "$nfs_path" ]; then
    continue
  fi

  # Check if NFS path exists and is accessible
  if ! nsenter --target 1 --mount --uts --ipc --net --pid -- test -d "$nfs_path"; then
    echo "Error: NFS path '$nfs_path' does not exist or is not accessible"
    exit 1
  fi
done

exit 0