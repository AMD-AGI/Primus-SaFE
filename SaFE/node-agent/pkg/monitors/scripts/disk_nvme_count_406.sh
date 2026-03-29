#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Checks the number of NVMe namespace devices via /sys/class/block (no nvme-cli needed).
# Exits 0 if count >= expected, exits 1 if count < expected,
# exits 0 (skip) if keyword is not "nvme".
#

set -o pipefail

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <node-info>"
  echo "Example: $0 '{\"expectedDiskType\": \"nvme\", \"expectedDiskCount\": 8}'"
  exit 2
fi

disk_type=$(echo "$1" | jq -r '.expectedDiskType')
expected_count=$(echo "$1" | jq -r '.expectedDiskCount')

if [ -z "$disk_type" ] || [ "$disk_type" = "null" ] || [ "$disk_type" != "nvme" ]; then
  exit 0
fi

if [ -z "$expected_count" ] || [ "$expected_count" = "null" ] || ! [[ "$expected_count" =~ ^[0-9]+$ ]] || [ "$expected_count" -le 0 ]; then
  echo "Error: expectedDiskCount must be a positive integer, got: $expected_count"
  exit 2
fi

NSENTER="nsenter --target 1 --mount --uts --ipc --net --pid --"

host() {
  ${NSENTER} "$@"
}

# Use /sys/class/block to detect NVMe namespace devices (no nvme-cli needed)
nvme_devices=$(host ls -1 /sys/class/block/ 2>/dev/null | grep -E '^nvme[0-9]+n[0-9]+$' || true)

if [ -z "$nvme_devices" ]; then
  echo "Error: no NVMe devices found in /sys/class/block/"
  exit 1
fi

actual_count=$(echo "$nvme_devices" | wc -l)

if [ "$actual_count" -ge "$expected_count" ]; then
  echo "OK: found $actual_count NVMe device(s) (expected >= $expected_count)"
  exit 0
fi

echo "Error: found $actual_count NVMe device(s), expected >= $expected_count"
exit 1
