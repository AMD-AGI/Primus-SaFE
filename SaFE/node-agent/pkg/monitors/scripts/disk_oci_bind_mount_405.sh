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

kubelet_stopped_for_containerd=0

host() {
  ${NSENTER} "$@"
}

recover_services() {
  if [[ "${kubelet_stopped_for_containerd:-0}" -eq 1 ]]; then
    host systemctl start kubelet 2>/dev/null || true
    kubelet_stopped_for_containerd=0
  fi
}

# True if fstab (non-comment) already has: source target ...
fstab_has_bind() {
  host grep -v '^[[:space:]]*#' "$FSTAB" 2>/dev/null | grep -qF "${1} ${2}"
}

# True if source and target already refer to the same inode (avoids cp "same file").
same_inode_pair() {
  local s t
  s=$(host stat -L -c '%d:%i' "$1" 2>/dev/null || true)
  t=$(host stat -L -c '%d:%i' "$2" 2>/dev/null || true)
  [ -n "$s" ] && [ -n "$t" ] && [ "$s" = "$t" ]
}

# Apply live bind: requires source dir (parent LV mounted). Exits on failure.
ensure_bind_live() {
  local source=$1 target=$2
  if ! host test -d "$source" 2>/dev/null; then
    echo "Error: ${source} missing — is /mnt/m2m_nobackup mounted? (check fstab order / boot)"
    exit 1
  fi
  host mkdir -p "$target" 2>/dev/null
  if ! host mount --bind "$source" "$target" 2>/dev/null; then
    echo "Error: mount --bind failed ${source} -> ${target}"
    exit 1
  fi
}

# Stop services, copy data, bind mount, restart (only when paths are not already the same inode).
migrate_pair() {
  local source=$1 target=$2
  if [[ "$target" == *containerd* ]]; then
    host systemctl stop kubelet 2>/dev/null || true
    kubelet_stopped_for_containerd=1
  fi
  if [[ "$target" == *kubelet* ]]; then
    host systemctl stop kubelet 2>/dev/null || true
  elif [[ "$target" == *containerd* ]]; then
    host systemctl stop containerd 2>/dev/null || true
  fi
  if [[ "$target" == *containerd* ]]; then
    for _ in {1..30}; do
      host systemctl is-active --quiet containerd 2>/dev/null && sleep 1 && continue
      break
    done
  fi

  if host test -d "$target" 2>/dev/null; then
    if ! host cp -a "$target"/. "$source"/; then
      echo "Error: failed to copy ${target} to ${source}, restarting services and aborting"
      if [[ "$target" == *kubelet* ]]; then
        host systemctl start kubelet 2>/dev/null || true
      elif [[ "$target" == *containerd* ]]; then
        host systemctl start containerd 2>/dev/null || true
        recover_services
      fi
      exit 1
    fi
  fi

  host mkdir -p "$target" 2>/dev/null
  if ! host mount --bind "$source" "$target" 2>/dev/null; then
    echo "Error: failed to mount --bind $source $target"
    if [[ "$target" == *kubelet* ]]; then
      host systemctl start kubelet 2>/dev/null || true
    elif [[ "$target" == *containerd* ]]; then
      host systemctl start containerd 2>/dev/null || true
      recover_services
    fi
    exit 1
  fi

  if [[ "$target" == *kubelet* ]]; then
    host systemctl restart kubelet 2>/dev/null || true
  elif [[ "$target" == *containerd* ]]; then
    host systemctl restart containerd 2>/dev/null || true
    recover_services
  fi
}

BIND_MOUNTS=(
  "/mnt/m2m_nobackup/kubelet:/var/lib/kubelet"
  "/mnt/m2m_nobackup/containerd:/var/lib/containerd"
)

restart_services() {
  ${NSENTER} systemctl start containerd 2>/dev/null || true
  ${NSENTER} systemctl start kubelet 2>/dev/null || true
}

for pair in "${BIND_MOUNTS[@]}"; do
  source="${pair%%:*}"
  target="${pair##*:}"
  fstab_line="${source} ${target} none bind 0 0"

  # Done: fstab entry + live mount
  if fstab_has_bind "$source" "$target" && host mountpoint -q "$target" 2>/dev/null; then
    continue
  fi

  # Repair: fstab says bind but runtime lost it (boot order, append-only run, etc.)
  if fstab_has_bind "$source" "$target" && ! host mountpoint -q "$target" 2>/dev/null; then
    ensure_bind_live "$source" "$target"
    continue
  fi

  # New pair: ensure source dir on persistent volume
  if ! host test -d "$source" 2>/dev/null; then
    if ! host mkdir -p "$source" 2>/dev/null; then
      echo "Error: failed to create source directory: $source"
      exit 2
    fi
  fi

  if ! host mountpoint -q "$target" 2>/dev/null; then
    if ! same_inode_pair "$source" "$target"; then
      migrate_pair "$source" "$target"
    fi
  fi

    # Copy target data to source (preserve existing data to persistent storage)
    ${NSENTER} test -d "$target" 2>/dev/null
    if [ $? -eq 0 ]; then
      ${NSENTER} cp -a "$target"/. "$source"/ 2>/dev/null
      if [ $? -ne 0 ]; then
        echo "Error: failed to copy $target to $source, restarting services and aborting"
        restart_services
        exit 1
      fi

      # Verify critical files after copy
      if [[ "$target" == *kubelet* ]]; then
        if ! ${NSENTER} test -f "$source/pki/kubelet-client-current.pem" 2>/dev/null; then
          echo "Error: kubelet client cert not found in $source/pki/ after copy, restarting services and aborting"
          restart_services
          exit 1
        fi
      fi
    fi

    # Ensure target exists for mount
    ${NSENTER} mkdir -p "$target" 2>/dev/null
    if [ $? -ne 0 ]; then
      echo "Error: failed to create target directory: $target, restarting services and aborting"
      restart_services
      exit 1
    fi

    ${NSENTER} mount --bind "$source" "$target" 2>/dev/null
    if [ $? -ne 0 ]; then
      echo "Error: failed to mount --bind $source $target, restarting services and aborting"
      restart_services
  # Append fstab if still missing (race with another monitor)
  if ! fstab_has_bind "$source" "$target"; then
    if ! host sh -c "echo '${fstab_line}' >> ${FSTAB}" 2>/dev/null; then
      echo "Error: failed to append to $FSTAB"
      exit 1
    fi
  fi

  # Add to fstab
  ${NSENTER} sh -c "echo '${fstab_line}' >> ${FSTAB}" 2>/dev/null
  if [ $? -ne 0 ]; then
    echo "Error: failed to append ${fstab_line} to $FSTAB"
    exit 1
  # fstab alone does not mount until boot/mount -a; ensure bind now (same-inode path needs this)
  if ! host mountpoint -q "$target" 2>/dev/null; then
    ensure_bind_live "$source" "$target"
  fi
done
