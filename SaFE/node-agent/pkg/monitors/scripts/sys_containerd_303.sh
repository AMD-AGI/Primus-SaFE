#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# check_containerd_health.sh
# Runs inside node-agent container. Uses nsenter to check host containerd integrity.
# Exit: 0=PASS, 1=FAIL

set -o pipefail
export PATH="/usr/bin:/bin:${PATH:-}"

NSENTER="nsenter --target 1 --mount --uts --ipc --net --pid --"
LOG_WINDOW="${CONTAINERD_LOG_WINDOW:-10 minutes ago}"
SNAP_DIR="/var/lib/containerd/io.containerd.snapshotter.v1.overlayfs/snapshots"
# Seconds between metadata/disk scans when the first pass reports a missing ID (reduces GC races).
CONTAINERD_303_RECHECK_SLEEP="${CONTAINERD_303_RECHECK_SLEEP:-2}"
# Reclaim pod file cache once resident usage crosses this many bytes.
CGROUP_RECLAIM_ABOVE=$((128 * 1024 * 1024))

host() { ${NSENTER} "$@"; }

fail() { echo "Error: $1"; exit 1; }

# Drop file cache charged to this pod. ctr stays in the pod cgroup, so snapshot
# reads accumulate there until memory.reclaim runs from the host mount namespace.
reclaim_pod_cache() {
  host sh -c 'cg=$(cut -d: -f3 /proc/self/cgroup); [ -z "$cg" ] && exit 0; cur=$(cat "/sys/fs/cgroup${cg}/memory.current" 2>/dev/null || echo 0); [ "$cur" -gt '"$CGROUP_RECLAIM_ABOVE"' ] && echo 128M > "/sys/fs/cgroup${cg}/memory.reclaim"' >/dev/null 2>&1 || true
}

# Remove temporary snapshot views and the scratch mount left by an interrupted scan.
cleanup_scan_artifacts() {
  host sh -c '
    ctr -n k8s.io snapshots ls 2>/dev/null | while read -r key _; do
      case "$key" in
        chk-view-303-*) ctr -n k8s.io snapshots rm "$key" >/dev/null 2>&1 || true ;;
      esac
    done
    if mountpoint -q /tmp/_chk 2>/dev/null; then
      umount /tmp/_chk >/dev/null 2>&1 || true
    fi
  ' >/dev/null 2>&1 || true
  reclaim_pod_cache
}

trap cleanup_scan_artifacts EXIT
cleanup_scan_artifacts

# One pass: freeze ctr snapshot list once, collect overlay IDs from mounts, print first missing dir or empty.
metadata_disk_missing_id() {
  host sh -c '
  SNAP_DIR="'"$SNAP_DIR"'"
  META_IDS=$(mktemp)
  SNAP_LS=$(mktemp)
  trap "rm -f $META_IDS $SNAP_LS" EXIT
  reclaim_if_high() {
    cg=$(cut -d: -f3 /proc/self/cgroup)
    [ -n "$cg" ] || return 0
    cur=$(cat "/sys/fs/cgroup${cg}/memory.current" 2>/dev/null || echo 0)
    if [ "$cur" -gt '"$CGROUP_RECLAIM_ABOVE"' ]; then
      echo 128M > "/sys/fs/cgroup${cg}/memory.reclaim" 2>/dev/null || true
    fi
  }

  ctr -n k8s.io snapshots ls 2>/dev/null | tail -n +2 > "$SNAP_LS" || true
  reclaim_if_high
  if [ ! -s "$SNAP_LS" ]; then
    exit 0
  fi

  for key in $(awk "{print \$1}" "$SNAP_LS"); do
    kind=$(awk -v k="$key" "\$1==k {print \$3; exit}" "$SNAP_LS")
    if [ "$kind" = "Committed" ]; then
      VIEW="chk-view-303-$$-$key"
      ctr -n k8s.io snapshots view "$VIEW" "$key" &>/dev/null || continue
      ctr -n k8s.io snapshots mount /tmp/_chk "$VIEW" 2>/dev/null \
        | grep -oP "snapshots/\K[0-9]+" >> "$META_IDS"
      ctr -n k8s.io snapshots rm "$VIEW" &>/dev/null || true
    else
      ctr -n k8s.io snapshots mount /tmp/_chk "$key" 2>/dev/null \
        | grep -oP "snapshots/\K[0-9]+" >> "$META_IDS"
    fi
    umount /tmp/_chk >/dev/null 2>&1 || true
    reclaim_if_high
  done

  sort -un "$META_IDS" | while read -r id; do
    [ -z "$id" ] && continue
    [ ! -d "$SNAP_DIR/$id" ] && echo "$id" && break
  done
  reclaim_if_high
' 2>/dev/null
}

# 1. containerd service
host systemctl is-active containerd &>/dev/null || fail "containerd service is not active"

# 2. snapshot dirs missing fs/
if host test -d "$SNAP_DIR" 2>/dev/null; then
  BAD=$(host sh -c "for d in $SNAP_DIR/*/; do [ ! -d \"\${d}fs\" ] && basename \"\$d\"; done" 2>/dev/null | head -1)
  [ -n "$BAD" ] && fail "snapshot $BAD missing fs/ subdir"
else
  fail "snapshot directory does not exist"
fi
reclaim_pod_cache

# 3. metadata vs disk: extract referenced snapshot IDs from ctr, compare with disk
MISSING_ID=$(metadata_disk_missing_id)
if [ -n "$MISSING_ID" ]; then
  sleep "$CONTAINERD_303_RECHECK_SLEEP"
  MISSING_ID2=$(metadata_disk_missing_id)
  [ -n "$MISSING_ID2" ] && [ "$MISSING_ID" = "$MISSING_ID2" ] \
    && fail "snapshot $MISSING_ID referenced in metadata but missing on disk"
fi
reclaim_pod_cache

# 4. containerd logs: broken parent chain / mount failures
SAMPLE=$(host journalctl -u containerd --since "$LOG_WINDOW" --no-pager 2>/dev/null \
  | grep -E "failed to stat parent|failed to mount.*tmpmounts|snapshots/[0-9]+/fs: no such file" \
  | grep -v "io.containerd.runtime" \
  | tail -1)
[ -n "$SAMPLE" ] && fail "$SAMPLE"

# 5. blob not found
SAMPLE=$(host ctr -n k8s.io images ls 2>&1 | grep "blob not found" | head -1)
[ -n "$SAMPLE" ] && fail "$SAMPLE"

echo "PASS"
exit 0
