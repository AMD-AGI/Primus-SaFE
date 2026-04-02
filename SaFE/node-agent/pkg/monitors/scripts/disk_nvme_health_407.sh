#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# NVMe health check: topology/model info, SMART critical warnings, media errors,
# and error-log non-SUCCESS entries. Skips if expectedDiskType is not "nvme".
#

set -o pipefail

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <node-info>"
  echo "Example: $0 '{\"expectedDiskType\": \"nvme\", \"expectedDiskCount\": 8}'"
  exit 2
fi

disk_type=$(echo "$1" | jq -r '.expectedDiskType')

if [ -z "$disk_type" ] || [ "$disk_type" = "null" ] || [ "$disk_type" != "nvme" ]; then
  exit 0
fi

NSENTER="nsenter --target 1 --mount --uts --ipc --net --pid --"

host() {
  ${NSENTER} "$@"
}

if ! host command -v nvme >/dev/null 2>&1; then
  echo "WARN: nvme-cli not installed on host, skipping health check"
  exit 2
fi

ERROR_LOG_THRESHOLD=100
has_error=0
error_messages=""

append_error() {
  has_error=1
  error_messages="${error_messages}${error_messages:+; }$1"
}

# Discover NVMe controllers (nvme0, nvme1, ...) not namespace devices
controllers=$(host ls -1 /sys/class/nvme/ 2>/dev/null | grep -E '^nvme[0-9]+$' || true)

if [ -z "$controllers" ]; then
  echo "Error: no NVMe controllers found in /sys/class/nvme/"
  exit 1
fi

for ctrl in $controllers; do
  dev="/dev/${ctrl}"

  # SMART log check
  smart_output=$(host nvme smart-log "$dev" 2>/dev/null) || {
    append_error "${ctrl}: failed to read smart-log"
    continue
  }

  critical_warning=$(echo "$smart_output" | grep -i 'critical_warning' | head -1 | awk -F: '{gsub(/[[:space:]]/, "", $2); print $2}')
  media_errors=$(echo "$smart_output" | grep -i 'media_errors' | head -1 | awk -F: '{gsub(/[[:space:]]/, "", $2); print $2}')
  err_log_entries=$(echo "$smart_output" | grep -i 'num_err_log_entries' | head -1 | awk -F: '{gsub(/[[:space:]]/, "", $2); print $2}')

  if [ -n "$critical_warning" ] && [ "$critical_warning" != "0" ] && [ "$critical_warning" != "0x0" ]; then
    append_error "${ctrl}: critical_warning=${critical_warning}"
  fi
  if [ -n "$media_errors" ] && [ "$media_errors" != "0" ]; then
    append_error "${ctrl}: media_errors=${media_errors}"
  fi
  if [ -n "$err_log_entries" ] && [ "$err_log_entries" != "0" ]; then
    append_error "${ctrl}: num_err_log_entries=${err_log_entries}"
  fi

  # Error log check: count non-SUCCESS status entries
  error_log=$(host nvme error-log "$dev" 2>/dev/null) || continue
  non_success_count=$(echo "$error_log" | grep -i 'status_field' | grep -iv 'SUCCESS' | wc -l)
  non_success_count=$(echo "$non_success_count" | tr -d '[:space:]')

  if [ -n "$non_success_count" ] && [ "$non_success_count" -gt "$ERROR_LOG_THRESHOLD" ] 2>/dev/null; then
    append_error "${ctrl}: error-log non-SUCCESS entries=${non_success_count} (threshold ${ERROR_LOG_THRESHOLD})"
  fi
done

if [ "$has_error" -eq 1 ]; then
  echo "Error: NVMe health issues detected: ${error_messages}"
  exit 1
fi

exit 0
