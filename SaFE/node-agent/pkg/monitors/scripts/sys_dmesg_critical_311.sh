#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Scans kernel journal (dmesg) for critical and high-severity hardware/software
# error patterns. Exits 1 on the first match.
# Covers: GPU, CPU, memory, PCIe, network, and storage errors.
#

set -o pipefail

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <interval>"
  echo "Example: $0 43200"
  exit 2
fi

current_time=$(date +%s)
current_time_fmt=$(date -d "@$current_time" +'%Y-%m-%d %H:%M:%S')
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

_jtmp=$(mktemp) || exit 2
_cleanup() {
	[ -n "${_jtmp:-}" ] && rm -f -- "$_jtmp"
}
trap _cleanup EXIT INT TERM HUP
host /usr/bin/journalctl -k --no-pager --since "$since" 2>/dev/null >"$_jtmp" || true

# Format: "SEVERITY|CATEGORY|GREP_EXTENDED_REGEX"
PATTERNS=(
	# ── CRITICAL (recommend drain) — node hardware faults only ──
	'CRITICAL|AMD GPU ECC|uncorrectable hardware errors detected in .* block'
	'CRITICAL|AMD GPU Hang|amdgpu.*GPU hang'
	'CRITICAL|AMD GPU Reset|amdgpu.*reset'
	'CRITICAL|AMD GPU Poison|amdgpu.*poison'
	'CRITICAL|AMD ACA|\[Hardware Error\].*Accelerator Check Architecture.*uncorrectable'
	'CRITICAL|Kernel Panic|kernel panic'
	'CRITICAL|Memory ECC|EDAC.*Uncorrected'
	'CRITICAL|CPU MCE|MCE.*Hardware Error'
	'CRITICAL|CPU MCE|mce:.*Machine check'
	'CRITICAL|Network NIC Init|mlx5_core.*init failed'
	'CRITICAL|Network NIC PCIe|mlx5_core.*PCI slot is unavailable'
	'CRITICAL|PCIe AER|pcieport.*AER.*(Uncorrectable|Fatal)'
	# ── HIGH (investigate) — node-level infrastructure issues ──
	'HIGH|AMD ACA|\[Hardware Error\].*Accelerator Check Architecture'
	'HIGH|Kernel Lockup|bug: soft lockup'
	'HIGH|Kernel Lockup|bug: hard lockup'
	'HIGH|Network Link|mlx5_core.*Link down'
	'HIGH|Network NIC Temp|mlx5_core.*High temperature'
	'HIGH|InfiniBand|infiniband.*wait status -512'
	'HIGH|Weka Storage|wekafsio.*ALL FEs down'
	'HIGH|Weka Storage|wekafsgw.*FE was down'
	'HIGH|NFS|nfs.*not responding'
)

for entry in "${PATTERNS[@]}"; do
	severity="${entry%%|*}"
	rest="${entry#*|}"
	category="${rest%%|*}"
	pattern="${rest#*|}"

	if [ "$category" = "AMD GPU Reset" ]; then
		match=$(grep -E "$pattern" "$_jtmp" 2>/dev/null | grep -v "init context" | tail -1)
	else
		match=$(grep -E "$pattern" "$_jtmp" 2>/dev/null | tail -1)
	fi

	if [ -n "$match" ]; then
		echo "[${severity}] [${category}] [${since} ~ ${current_time_fmt}]: ${match}"
		exit 1
	fi
done

exit 0
