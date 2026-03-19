#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail
export PATH="/usr/bin:/bin:${PATH:-}"

if [ "$#" -lt 1 ]; then
  exit 0
fi

clusterId="${1:-}"
if [ -z "$clusterId" ] || [ "$clusterId" != "oci-slc" ]; then
  exit 0
fi

NSENTER="nsenter --target 1 --mount --uts --ipc --net --pid --"
FSTAB="/etc/fstab"

# Bind mount pairs: source target
BIND_MOUNTS=(
  "/mnt/m2m_nobackup/kubelet:/var/lib/kubelet"
  "/mnt/m2m_nobackup/containerd:/var/lib/containerd"
)

for pair in "${BIND_MOUNTS[@]}"; do
  source="${pair%%:*}"
  target="${pair##*:}"
  fstab_line="${source} ${target} none bind 0 0"

  # Skip if fstab entry already exists (ignore commented lines)
  ${NSENTER} grep -v '^[[:space:]]*#' "$FSTAB" 2>/dev/null | grep -qF "${source} ${target}"
  if [ $? -eq 0 ]; then
    continue
  fi

  # Create source directory if not exists
  ${NSENTER} test -d "$source" 2>/dev/null
  if [ $? -ne 0 ]; then
    ${NSENTER} mkdir -p "$source" 2>/dev/null
    if [ $? -ne 0 ]; then
      echo "Error: failed to create source directory: $source"
      exit 2
    fi
  fi

  # Mount if not already mounted
  ${NSENTER} mountpoint -q "$target" 2>/dev/null
  if [ $? -ne 0 ]; then
    # Stop service before mount
    if [[ "$target" == *kubelet* ]]; then
      ${NSENTER} systemctl stop kubelet 2>/dev/null || true
    elif [[ "$target" == *containerd* ]]; then
      ${NSENTER} systemctl stop containerd 2>/dev/null || true
    fi

    # Copy target data to source (preserve existing data to persistent storage)
    ${NSENTER} test -d "$target" 2>/dev/null
    if [ $? -eq 0 ]; then
      ${NSENTER} cp -a "$target"/. "$source"/ 2>/dev/null || true
    fi

    # Ensure target exists for mount
    ${NSENTER} mkdir -p "$target" 2>/dev/null

    ${NSENTER} mount --bind "$source" "$target" 2>/dev/null
    if [ $? -ne 0 ]; then
      echo "Error: failed to mount --bind $source $target"
      if [[ "$target" == *kubelet* ]]; then
        ${NSENTER} systemctl start kubelet 2>/dev/null || true
      elif [[ "$target" == *containerd* ]]; then
        ${NSENTER} systemctl start containerd 2>/dev/null || true
      fi
      exit 1
    fi

    # Restart service after successful mount
    if [[ "$target" == *kubelet* ]]; then
      ${NSENTER} systemctl restart kubelet 2>/dev/null || true
    elif [[ "$target" == *containerd* ]]; then
      ${NSENTER} systemctl restart containerd 2>/dev/null || true
    fi
  fi

  # Add to fstab
  ${NSENTER} sh -c "echo '${fstab_line}' >> ${FSTAB}" 2>/dev/null
  if [ $? -ne 0 ]; then
    echo "Error: failed to append to $FSTAB"
    exit 1
  fi
done
