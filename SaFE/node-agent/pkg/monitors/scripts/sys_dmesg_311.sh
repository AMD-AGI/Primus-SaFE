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

# Format: "SEVERITY|CATEGORY|THRESHOLD|GREP_EXTENDED_REGEX"
# THRESHOLD: minimum match count to trigger alert (CRITICAL=1, HIGH=3 by default)
PATTERNS=(
	# ── CRITICAL (recommend drain) — node hardware faults only ──
	'CRITICAL|AMD GPU ECC|1|uncorrectable hardware errors detected in .* block'
	'CRITICAL|AMD GPU Poison|1|amdgpu.*poison'
	'CRITICAL|AMD ACA|1|\[Hardware Error\].*Accelerator Check Architecture.*uncorrectable'
	'CRITICAL|Kernel Panic|1|kernel panic'
	'CRITICAL|Memory ECC|1|EDAC.*Uncorrected'
	'CRITICAL|CPU MCE|1|MCE.*Hardware Error'
	'CRITICAL|CPU MCE|1|mce:.*Machine check'
	'CRITICAL|Network NIC PCIe|1|mlx5_core.*PCI slot is unavailable'
	'CRITICAL|PCIe AER|1|pcieport.*AER.*(Uncorrectable|Fatal)'
	'CRITICAL|AMD GPU Uncorrectable|1|amdgpu.*uncorrectable'
	'CRITICAL|AMD GPU Fatal|1|amdgpu.*fatal'
	'CRITICAL|PCIe DPC|1|DPC:.*containment'
	'CRITICAL|PCIe Bus Fatal|1|PCIe Bus Error.*Fatal'
	'CRITICAL|PCIe AER Recovery|1|AER.*recovery failed'
	'CRITICAL|NVMe Controller Dead|1|nvme.*CSTS=0xffffffff'
	'CRITICAL|Kernel Hard Lockup|1|bug: hard lockup'
	'CRITICAL|Weka All FEs Down|1|wekafsio.*ALL FEs down'
	# ── HIGH (investigate) — node-level infrastructure issues ──
	'HIGH|AMD GPU Hang|5|amdgpu.*hang'
	'HIGH|AMD GPU Reset|5|amdgpu.*reset'
	'HIGH|Network NIC Init|3|mlx5_core.*init failed'
	'HIGH|AMD ACA|500|\[Hardware Error\].*Accelerator Check Architecture'
	'HIGH|Kernel Lockup|3|bug: soft lockup'
	'HIGH|Network Link|5|mlx5_core.*Link down'
	'HIGH|Network NIC Temp|3|mlx5_core.*High temperature'
	'HIGH|InfiniBand|500|infiniband.*wait status -512'
	'HIGH|Weka Storage|3|wekafsgw.*FE was down'
	'HIGH|NFS|500|nfs.*not responding'
	'HIGH|Disk IO|20|blk_update_request.*I/O error.*nvme'
)

for entry in "${PATTERNS[@]}"; do
	severity="${entry%%|*}"
	rest="${entry#*|}"
	category="${rest%%|*}"
	rest="${rest#*|}"
	threshold="${rest%%|*}"
	pattern="${rest#*|}"

	if [ "$category" = "AMD GPU Reset" ]; then
		matches=$(grep -E "$pattern" "$_jtmp" 2>/dev/null | grep -v "init context")
	else
		matches=$(grep -E "$pattern" "$_jtmp" 2>/dev/null)
	fi

	[ -z "$matches" ] && continue

	count=$(echo "$matches" | wc -l)
	if [ "$count" -ge "$threshold" ]; then
		last=$(echo "$matches" | tail -1)
		echo "[${severity}] [${category}] [${since} ~ ${current_time_fmt}] (${count}x): ${last}"
		exit 1
	fi
done

exit 0
