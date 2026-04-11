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

# NVMe list check: capacity anomaly and firmware consistency per model
nvme_list_json=$(host nvme list -o json 2>/dev/null) || true

if [ -n "$nvme_list_json" ]; then
  device_count=$(echo "$nvme_list_json" | jq '.Devices | length' 2>/dev/null)

  if [ -n "$device_count" ] && [ "$device_count" -gt 0 ] 2>/dev/null; then
    # Pass 1: collect max capacity and unique firmware versions per model
    model_stats=$(echo "$nvme_list_json" | jq -r '
      [.Devices[] | {model: .ModelNumber, size: .PhysicalSize, fw: .Firmware}]
      | group_by(.model)
      | map({
          model: .[0].model,
          max_size: (map(.size) | max),
          fw_versions: (map(.fw) | unique | join(","))
        })
      | .[]
      | "\(.model)|\(.max_size)|\(.fw_versions)"
    ' 2>/dev/null)

    declare -A model_max_size
    declare -A model_fw_versions
    while IFS='|' read -r m_model m_max m_fws; do
      [ -z "$m_model" ] && continue
      model_max_size["$m_model"]=$m_max
      model_fw_versions["$m_model"]=$m_fws
    done <<< "$model_stats"

    # Pass 2: check each device against its model group
    device_lines=$(echo "$nvme_list_json" | jq -r '.Devices[] | "\(.DevicePath)|\(.ModelNumber)|\(.PhysicalSize)|\(.Firmware)"' 2>/dev/null)
    while IFS='|' read -r d_path d_model d_size d_fw; do
      [ -z "$d_path" ] && continue
      ctrl=$(echo "$d_path" | sed 's|/dev/||; s|n[0-9]*$||')

      # Capacity anomaly: device < 50% of the largest same-model peer
      max=${model_max_size[$d_model]:-0}
      if [ "$max" -gt 0 ] && [ "$d_size" -gt 0 ] 2>/dev/null; then
        ratio=$((d_size * 100 / max))
        if [ "$ratio" -lt 50 ]; then
          append_error "${ctrl}: capacity anomaly, size=${d_size}B vs model-max=${max}B (${ratio}%)"
        fi
      fi

      # Firmware consistency: flag if same model has mixed firmware versions
      fw_list="${model_fw_versions[$d_model]}"
      if echo "$fw_list" | grep -qF ","; then
        append_error "${ctrl}: firmware mismatch, fw=${d_fw}, model '${d_model}' has versions [${fw_list}]"
      fi
    done <<< "$device_lines"
  fi
fi

if [ "$has_error" -eq 1 ]; then
  echo "Error: NVMe health issues detected: ${error_messages}"
  exit 1
fi

exit 0
