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
output=$(${NSENTER} nicctl show dcqcn 2>/dev/null)
if [ $? -ne 0 ]; then
  echo "Error: failed to execute nicctl show dcqcn"
  exit 2
fi

# Expected patterns per NIC block (from set-oci-dcqcn addon standard)
check_block() {
  local block="$1"
  echo "$block" | grep -q "DCQCN profile id.*: 1" || return 1
  echo "$block" | grep -q "Status.*: Enabled" || return 1
  echo "$block" | grep -q "Rate increase in AI phase.*: 160" || return 1
  echo "$block" | grep -q "Rate increase byte count.*: 431068" || return 1
  echo "$block" | grep -q "Alpha update G value.*: 512" || return 1
  echo "$block" | grep -q "Alpha update interval.*: 1" || return 1
  echo "$block" | grep -q "Rate increase in HAI phase.*: 300" || return 1
  echo "$block" | grep -q "Initial alpha value.*: 64" || return 1
  echo "$block" | grep -q "Rate reduce monitor period.*: 1" || return 1
  echo "$block" | grep -q "Rate increase threshold.*: 1" || return 1
  echo "$block" | grep -q "Rate increase interval.*: 1" || return 1
  echo "$block" | grep -q "Token bucket size.*: 800000" || return 1
  echo "$block" | grep -q "DSCP value used for CNP.*: 46" || return 1
  return 0
}

# Split by "NIC :" - each block is one NIC's config
blocks=()
current=""
first=1
while IFS= read -r line; do
  if [[ "$line" =~ ^NIC[[:space:]]+:[[:space:]] ]]; then
    if [ "$first" -eq 1 ]; then
      first=0
    else
      [ -n "$current" ] && blocks+=("$current")
    fi
    current="$line"
  else
    if [ -z "$current" ]; then
      current="$line"
    else
      current="$current"$'\n'"$line"
    fi
  fi
done <<< "$output"
[ -n "$current" ] && blocks+=("$current")

nic_count=0
for block in "${blocks[@]}"; do
  [ -z "$block" ] && continue
  nic_count=$((nic_count + 1))
  if ! check_block "$block"; then
    echo "Error: NIC block $nic_count does not match expected OCI DCQCN standard"
    exit 1
  fi
done

if [ "$nic_count" -eq 0 ]; then
  echo "Error: no NIC found in nicctl show dcqcn output"
  exit 2
fi

exit 0
