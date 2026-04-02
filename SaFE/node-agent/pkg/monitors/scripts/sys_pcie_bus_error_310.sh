#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Scans kernel journal for "PCIe Bus Error" since max(interval, boot time).
# journalctl --since accepts absolute timestamps (see systemd.time(7)).
#

set -o pipefail

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <interval>"
  echo "Example: $0 43200"
  exit 2
fi

current_time=$(date +%s)
previous_time=$((current_time - $1))
since1=$(date -d "@$previous_time" +'%Y-%m-%d %H:%M:%S')
since2=$(uptime -s)

timestamp1=$(date -d "$since1" +%s)
timestamp2=$(date -d "$since2" +%s)
since=$since1
if [ $timestamp1 -lt $timestamp2 ]; then
  since=$since2
fi

NSENTER="nsenter --target 1 --mount --uts --ipc --net --pid --"

host() {
	${NSENTER} "$@"
}

if ! host test -x /usr/bin/journalctl; then
    echo "journalctl not found or not executable on host, skipping check"
    exit 2
fi


KEYWORD="PCIe Bus Error"
THRESHOLD=11000

_jtmp=$(mktemp) || exit 2
cleanup_pcie_journal_tmp() {
	[ -n "${_jtmp:-}" ] && rm -f -- "$_jtmp"
}
trap cleanup_pcie_journal_tmp EXIT INT TERM HUP
host /usr/bin/journalctl -k --no-pager --since "$since" 2>/dev/null >"$_jtmp" || true

count=0
last_line=""

while IFS= read -r line || [ -n "$line" ]; do
	case "$line" in
	*"$KEYWORD"*)
		last_line="$line"
		count=$((count + 1))
		;;
	esac
done < "$_jtmp"

if [ "$count" -gt "$THRESHOLD" ]; then
	if [ -n "$last_line" ]; then
		_suffix="${last_line#*${KEYWORD}}"
		echo "Since ${since}, matched keyword \"${KEYWORD}\" ${count} times; details: ${KEYWORD}${_suffix}"
	fi
	exit 1
fi

exit 0
