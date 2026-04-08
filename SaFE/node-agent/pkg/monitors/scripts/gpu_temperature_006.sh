#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

JSON_FILE="/tmp/rocm-smi.json"

if [ ! -f "${JSON_FILE}" ]; then
  exit 0
fi

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <temperature>"
  echo "Example: $0 100"
  exit 2
fi

threshold=$1

# Extract junction temperature from each card
temps=$(jq -r 'to_entries[] | select(.key | startswith("card")) | "\(.key)=\(.value["Temperature (Sensor junction) (C)"])"' "${JSON_FILE}" 2>/dev/null)
if [ -z "$temps" ]; then
  echo "Error: failed to parse temperature from ${JSON_FILE}"
  exit 2
fi

while IFS= read -r line; do
  card=$(echo "$line" | cut -d'=' -f1)
  temp_str=$(echo "$line" | cut -d'=' -f2)
  temp=$(echo "$temp_str / 1" | bc 2>/dev/null)
  if [ -n "$temp" ] && [ "$temp" -ge "$threshold" ]; then
    echo "Warning: ${card} temperature ${temp_str}C exceeds threshold ${threshold}C"
    exit 1
  fi
done <<< "$temps"
