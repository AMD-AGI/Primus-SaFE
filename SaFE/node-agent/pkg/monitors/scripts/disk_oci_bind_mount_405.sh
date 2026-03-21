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

# If we stopped kubelet only for the containerd migration step, restart it on exit (success or handled failure).
kubelet_stopped_for_containerd=0

recover_services() {
  if [[ "${kubelet_stopped_for_containerd:-0}" -eq 1 ]]; then
    ${NSENTER} systemctl start kubelet 2>/dev/null || true
    kubelet_stopped_for_containerd=0
  fi
}

# Bind mount pairs: source target (kubelet first, then containerd)
BIND_MOUNTS=(
  "/mnt/m2m_nobackup/kubelet:/var/lib/kubelet"
  "/mnt/m2m_nobackup/containerd:/var/lib/containerd"
)

for pair in "${BIND_MOUNTS[@]}"; do
  source="${pair%%:*}"
  target="${pair##*:}"
  fstab_line="${source} ${target} none bind 0 0"

  # If fstab already lists this bind and the mount is active, nothing to do.
  # If fstab lists it but nothing is mounted (append-only run, failed boot, or wrong order), apply bind now.
  ${NSENTER} grep -v '^[[:space:]]*#' "$FSTAB" 2>/dev/null | grep -qF "${source} ${target}"
  if [ $? -eq 0 ]; then
    if ${NSENTER} mountpoint -q "$target" 2>/dev/null; then
      continue
    fi
    if ! ${NSENTER} test -d "$source" 2>/dev/null; then
      echo "Error: ${source} missing — is /mnt/m2m_nobackup mounted before this bind? (check fstab order / boot)"
      exit 1
    fi
    ${NSENTER} mkdir -p "$target" 2>/dev/null
    if ! ${NSENTER} mount --bind "$source" "$target" 2>/dev/null; then
      echo "Error: fstab has bind ${source} -> ${target} but mount --bind failed (busy or parent fs not ready)"
      exit 1
    fi
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
    # If source and target already resolve to the same inode (bind mount or symlink), cp would
    # error with "are the same file" — nothing to migrate; only fstab is needed below.
    t_stat=$(${NSENTER} stat -L -c '%d:%i' "$target" 2>/dev/null || true)
    s_stat=$(${NSENTER} stat -L -c '%d:%i' "$source" 2>/dev/null || true)
    if [ -n "$t_stat" ] && [ -n "$s_stat" ] && [ "$t_stat" = "$s_stat" ]; then
      :
    else
      # Stop service before mount. For containerd, stop kubelet first so nothing holds
      # /var/lib/containerd open (otherwise cp can fail with busy files / partial copy).
      if [[ "$target" == *containerd* ]]; then
        ${NSENTER} systemctl stop kubelet 2>/dev/null || true
        kubelet_stopped_for_containerd=1
      fi
      if [[ "$target" == *kubelet* ]]; then
        ${NSENTER} systemctl stop kubelet 2>/dev/null || true
      elif [[ "$target" == *containerd* ]]; then
        ${NSENTER} systemctl stop containerd 2>/dev/null || true
      fi
      # Allow shims / sockets to release (containerd can take a few seconds)
      if [[ "$target" == *containerd* ]]; then
        for _ in {1..30}; do
          ${NSENTER} systemctl is-active --quiet containerd 2>/dev/null && sleep 1 && continue
          break
        done
      fi

      # Copy target data to source (preserve existing data to persistent storage)
      ${NSENTER} test -d "$target" 2>/dev/null
      if [ $? -eq 0 ]; then
        if ! ${NSENTER} cp -a "$target"/. "$source"/; then
          echo "Error: failed to copy ${target} to ${source}, restarting services and aborting"
          if [[ "$target" == *kubelet* ]]; then
            ${NSENTER} systemctl start kubelet 2>/dev/null || true
          elif [[ "$target" == *containerd* ]]; then
            ${NSENTER} systemctl start containerd 2>/dev/null || true
            recover_services
          fi
          exit 1
        fi
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
          recover_services
        fi
        exit 1
      fi

      # Restart service after successful mount
      if [[ "$target" == *kubelet* ]]; then
        ${NSENTER} systemctl restart kubelet 2>/dev/null || true
      elif [[ "$target" == *containerd* ]]; then
        ${NSENTER} systemctl restart containerd 2>/dev/null || true
        recover_services
      fi
    fi
  fi

  # Add to fstab (re-check: loop start vs append can race with another cron / concurrent run)
  ${NSENTER} grep -v '^[[:space:]]*#' "$FSTAB" 2>/dev/null | grep -qF "${source} ${target}"
  if [ $? -eq 0 ]; then
    continue
  fi
  ${NSENTER} sh -c "echo '${fstab_line}' >> ${FSTAB}" 2>/dev/null
  if [ $? -ne 0 ]; then
    echo "Error: failed to append to $FSTAB"
    exit 1
  fi

  # fstab is only used at boot / mount -a; ensure the bind is active in the running system too
  # (covers same-inode skip path that wrote fstab without a live bind).
  if ! ${NSENTER} mountpoint -q "$target" 2>/dev/null; then
    if ! ${NSENTER} test -d "$source" 2>/dev/null; then
      echo "Error: ${source} missing after fstab update — mount /mnt/m2m_nobackup first"
      exit 1
    fi
    ${NSENTER} mkdir -p "$target" 2>/dev/null
    if ! ${NSENTER} mount --bind "$source" "$target" 2>/dev/null; then
      echo "Error: failed to mount --bind $source $target after updating fstab"
      exit 1
    fi
  fi
done
