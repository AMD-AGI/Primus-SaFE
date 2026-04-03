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

# Check topology link accessibility from "system" section
failed=$(jq -r '.system | to_entries[] | select(.key | startswith("(Topology) Link accessibility")) | select(.value != "True") | "\(.key): \(.value)"' "${JSON_FILE}" 2>/dev/null)

if [ -n "$failed" ]; then
  echo "Error: GPU link accessibility failure. $(echo "$failed" | head -5)"
  exit 1
fi
