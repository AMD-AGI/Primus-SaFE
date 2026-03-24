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

host() { ${NSENTER} "$@"; }

fail() { echo "FAIL: $1"; exit 1; }

# 1. containerd service
host systemctl is-active containerd &>/dev/null || fail "containerd service is not active"

# 2. snapshot dirs missing fs/
if host test -d "$SNAP_DIR" 2>/dev/null; then
  BAD=$(host sh -c "for d in $SNAP_DIR/*/; do [ ! -d \"\${d}fs\" ] && basename \"\$d\"; done" 2>/dev/null | head -1)
  [ -n "$BAD" ] && fail "snapshot $BAD missing fs/ subdir"
else
  fail "snapshot directory does not exist"
fi

# 3. metadata vs disk: extract referenced snapshot IDs from ctr, compare with disk
MISSING_ID=$(host sh -c '
  SNAP_DIR="'"$SNAP_DIR"'"
  META_IDS=$(mktemp)
  trap "rm -f $META_IDS" EXIT

  for key in $(ctr -n k8s.io snapshots ls 2>/dev/null | tail -n +2 | awk "{print \$1}"); do
    kind=$(ctr -n k8s.io snapshots ls 2>/dev/null | grep "^${key}[[:space:]]" | awk "{print \$3}")
    if [ "$kind" = "Committed" ]; then
      VIEW="chk-view-$$"
      ctr -n k8s.io snapshots view "$VIEW" "$key" &>/dev/null || continue
      ctr -n k8s.io snapshots mount /tmp/_chk "$VIEW" 2>/dev/null \
        | grep -oP "snapshots/\K[0-9]+" >> "$META_IDS"
      ctr -n k8s.io snapshots rm "$VIEW" &>/dev/null
    else
      ctr -n k8s.io snapshots mount /tmp/_chk "$key" 2>/dev/null \
        | grep -oP "snapshots/\K[0-9]+" >> "$META_IDS"
    fi
  done

  sort -un "$META_IDS" | while read id; do
    [ ! -d "$SNAP_DIR/$id" ] && echo "$id" && break
  done
' 2>/dev/null)
[ -n "$MISSING_ID" ] && fail "snapshot $MISSING_ID referenced in metadata but missing on disk"

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
