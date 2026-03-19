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
output=$(${NSENTER} nicctl show qos 2>/dev/null)
if [ $? -ne 0 ]; then
  echo "Error: failed to execute nicctl show qos"
  exit 2
fi

# Expected patterns per NIC block (from set-oci-qos addon standard)
check_block() {
  local block="$1"
  echo "$block" | grep -q "Classification type.*DSCP" || return 1
  echo "$block" | grep -q "DSCP bitmap.*0x0000000000000400.*priority.*0" || return 1
  echo "$block" | grep -q "DSCP bitmap.*0xffffbffffffffbff.*priority.*1" || return 1
  echo "$block" | grep -q "DSCP bitmap.*0x0000400000000000.*priority.*6" || return 1
  echo "$block" | grep -q "DSCP.*10.*priority.*0" || return 1
  echo "$block" | grep -q "DSCP.*0-9, 11-45, 47-63.*priority.*1" || return 1
  echo "$block" | grep -q "DSCP.*46.*priority.*6" || return 1
  echo "$block" | grep -q "46.*rdma-ack" || return 1
  echo "$block" | grep -q "PFC priority bitmap.*0x1" || return 1
  echo "$block" | grep -q "PFC no-drop priorities.*0" || return 1
  echo "$block" | grep -q "0.*DWRR.*99.*N/A" || return 1
  echo "$block" | grep -q "1.*DWRR.*1.*N/A" || return 1
  echo "$block" | grep -q "6.*strict.*N/A.*10" || return 1
  return 0
}

# Split by "NIC  :" - each block is one NIC's config
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
    echo "Error: NIC block $nic_count does not match expected OCI QoS standard"
    exit 1
  fi
done

if [ "$nic_count" -eq 0 ]; then
  echo "Error: no NIC found in nicctl show qos output"
  exit 2
fi

exit 0
