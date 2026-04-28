#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

# Args: <mount_point> <server> <path_or_fs_name> [type]
# When type is "wekafs": server = weka install URL/host[:port], path_or_fs_name = Weka FS name (e.g. csi-primus).
# Otherwise (missing or any other value): NFS — server = NFS host, path_or_fs_name = NFS export path.
if [ "$#" -lt 3 ]; then
  exit 2
fi

MOUNT_POINT="$1"
ARG_SERVER="$2"
ARG_PATH_OR_FS="$3"
MOUNT_TYPE="${4:-}"

NSENTER=(nsenter --target 1 --mount --uts --ipc --net --pid --)

# Check if mount point is already mounted and accessible
if "${NSENTER[@]}" mountpoint -q "$MOUNT_POINT" 2>/dev/null \
  && "${NSENTER[@]}" ls "$MOUNT_POINT" >/dev/null 2>&1; then
  exit 0
fi

# Create mount point only if directory does not exist
if ! "${NSENTER[@]}" test -d "$MOUNT_POINT"; then
  "${NSENTER[@]}" /usr/bin/mkdir -p "$MOUNT_POINT"
  if [ $? -ne 0 ]; then
    echo "Error: Failed to create directory: $MOUNT_POINT"
    exit 1
  fi
fi

if [ "$#" -ge 4 ] && [ "$MOUNT_TYPE" = "wekafs" ]; then
  WEKA_SERVER_SPEC="$ARG_SERVER"
  WEKA_FS_NAME="$ARG_PATH_OR_FS"
  _strip_scheme="${WEKA_SERVER_SPEC#http://}"
  _strip_scheme="${_strip_scheme#https://}"
  WEKA_HOST="${_strip_scheme%%[:/]*}"
  _rest="${_strip_scheme#"$WEKA_HOST"}"
  if [[ "$_rest" == :* ]]; then
    WEKA_PORT="${_rest#:}"
    WEKA_PORT="${WEKA_PORT%%/*}"
  else
    WEKA_PORT=14000
  fi
  INSTALL_URL="http://${WEKA_HOST}:${WEKA_PORT}/dist/v1/install"

  if ! "${NSENTER[@]}" bash -c "curl -fsSL '${INSTALL_URL}' | sudo bash"; then
    echo "Error: Weka driver install failed from ${INSTALL_URL}"
    exit 1
  fi

  WEKA_MOUNT_OPTS="num_cores=4,rw,relatime,writecache,inode_bits=auto,readahead_kb=32768,dentry_max_age_positive=1000,dentry_max_age_negative=0,net=enp29s0np0//vfs@4"
  "${NSENTER[@]}" mount -t wekafs -o "$WEKA_MOUNT_OPTS" "${WEKA_HOST}/${WEKA_FS_NAME}" "$MOUNT_POINT"
  if [ $? -ne 0 ]; then
    echo "Error: WekaFS mount failed: ${WEKA_HOST}/${WEKA_FS_NAME} -> $MOUNT_POINT"
    exit 1
  fi
  echo "WekaFS mounted successfully: ${WEKA_HOST}/${WEKA_FS_NAME} -> $MOUNT_POINT"
  exit 0
fi

NFS_SERVER="$ARG_SERVER"
NFS_PATH="$ARG_PATH_OR_FS"
"${NSENTER[@]}" mount -t nfs4 "${NFS_SERVER}:${NFS_PATH}" "$MOUNT_POINT"
if [ $? -ne 0 ]; then
  echo "Error: NFS mount failed: ${NFS_SERVER}:${NFS_PATH} -> $MOUNT_POINT"
  exit 1
fi

echo "NFS mounted successfully: ${NFS_SERVER}:${NFS_PATH} -> $MOUNT_POINT"
exit 0
