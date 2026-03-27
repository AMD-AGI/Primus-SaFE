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

if [ "$#" -lt 2 ]; then
  echo "Usage: $0 <model_keyword> <expected_count>"
  echo "Example: $0 nvme 8"
  exit 2
fi

model_keyword="$1"
expected_count="$2"

if [ -z "$model_keyword" ] || [ "$model_keyword" != "nvme" ]; then
  exit 0
fi

if ! [[ "$expected_count" =~ ^[0-9]+$ ]]; then
  echo "Error: expected_count must be a positive integer, got: $expected_count"
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
