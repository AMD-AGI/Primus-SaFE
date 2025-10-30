#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Simple GPU Monitor - prints alerts if GPUs are not idle

echo "Checking GPU status..."

# Check for running processes
echo "Checking GPU processes..."
process_output=$(amd-smi process 2>/dev/null)

# Check if there are any processes (not just "No running processes detected")
active_gpus=()
current_gpu=""

while IFS= read -r line; do
    if [[ "$line" =~ ^GPU:\ ([0-9]+) ]]; then
        current_gpu="${BASH_REMATCH[1]}"
    elif [[ "$line" =~ PROCESS_INFO: ]] && [[ ! "$line" =~ "No running processes detected" ]]; then
        active_gpus+=("$current_gpu")
    fi
done <<< "$process_output"

if [[ ${#active_gpus[@]} -gt 0 ]]; then
    echo "🚨 ALERT: Found running processes on GPU(s): ${active_gpus[*]}"
    echo ""
    echo "Process details:"

    # Extract and show detailed process info for active GPUs
    current_gpu=""
    show_details=false

    while IFS= read -r line; do
        if [[ "$line" =~ ^GPU:\ ([0-9]+) ]]; then
            current_gpu="${BASH_REMATCH[1]}"
            if [[ " ${active_gpus[*]} " =~ " $current_gpu " ]]; then
                show_details=true
                echo "--- GPU $current_gpu ---"
            else
                show_details=false
            fi
        elif [[ "$show_details" == true ]]; then
            if [[ "$line" =~ (PID:|MEMORY_USAGE:|VRAM_MEM:|MEM_USAGE:|CU_OCCUPANCY:) ]]; then
                echo "$line"
            fi
        fi
    done <<< "$process_output"

    exit 1
fi

# Check GPU usage
echo "Checking GPU usage..."
rocm_output=$(rocm-smi 2>/dev/null)
active_usage_gpus=()

while read -r line; do
    if [[ "$line" =~ ^[0-9]+[[:space:]] ]]; then
        vram_usage=$(echo "$line" | awk '{print $(NF-1)}' | sed 's/%//')
        gpu_usage=$(echo "$line" | awk '{print $NF}' | sed 's/%//')
        gpu_id=$(echo "$line" | awk '{print $1}')

        if [[ "$vram_usage" != "0" ]] || [[ "$gpu_usage" != "0" ]]; then
            active_usage_gpus+=("GPU $gpu_id: VRAM=${vram_usage}%, GPU=${gpu_usage}%")
        fi
    fi
done <<< "$rocm_output"

if [[ ${#active_usage_gpus[@]} -gt 0 ]]; then
    echo "🚨 ALERT: Found active GPU usage: ${active_usage_gpus}"
    exit 1
fi

echo "✅ All GPUs are idle (0% usage, no processes)"
exit 0