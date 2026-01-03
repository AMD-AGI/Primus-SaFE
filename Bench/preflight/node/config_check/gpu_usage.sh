#!/bin/bash

#
# Copyright (c) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
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
    # Collect GPU usage info and check for actual compute usage (CU > 0)
    gpu_info=""
    has_active_compute=false
    
    current_gpu=""
    current_vram=""
    current_mem=""
    current_cu=""
    
    while IFS= read -r line; do
        if [[ "$line" =~ ^GPU:\ ([0-9]+) ]]; then
            # Add previous GPU info if exists
            if [[ -n "$current_gpu" ]] && [[ " ${active_gpus[*]} " =~ " $current_gpu " ]]; then
                # Build compact info string for this GPU
                info="GPU${current_gpu}:"
                has_data=false
                
                # Add VRAM
                if [[ -n "$current_vram" ]]; then
                    info="${info}${current_vram}"
                    has_data=true
                fi
                
                # Add MEM if non-zero
                if [[ "$current_mem" != "0.0 B" ]] && [[ "$current_mem" != "0 B" ]] && [[ -n "$current_mem" ]]; then
                    info="${info},MEM:${current_mem}"
                fi
                
                # Add CU if non-zero
                if [[ "$current_cu" != "0" ]] && [[ -n "$current_cu" ]]; then
                    info="${info},CU:${current_cu}%"
                    has_active_compute=true  # Found actual compute usage!
                fi
                
                if [[ "$has_data" == true ]]; then
                    if [[ -n "$gpu_info" ]]; then
                        gpu_info="${gpu_info} | ${info}"
                    else
                        gpu_info="${info}"
                    fi
                fi
            fi
            
            # Reset for new GPU
            current_gpu="${BASH_REMATCH[1]}"
            current_vram=""
            current_mem=""
            current_cu=""
        elif [[ "$line" =~ VRAM_MEM:\ (.+) ]]; then
            current_vram="${BASH_REMATCH[1]}"
        elif [[ "$line" =~ MEM_USAGE:\ (.+) ]] && [[ ! "$line" =~ MEMORY_USAGE: ]]; then
            current_mem="${BASH_REMATCH[1]}"
        elif [[ "$line" =~ CU_OCCUPANCY:\ ([0-9]+) ]]; then
            current_cu="${BASH_REMATCH[1]}"
        fi
    done <<< "$process_output"
    
    # Add last GPU info
    if [[ -n "$current_gpu" ]] && [[ " ${active_gpus[*]} " =~ " $current_gpu " ]]; then
        info="GPU${current_gpu}:"
        has_data=false
        
        if [[ -n "$current_vram" ]]; then
            info="${info}${current_vram}"
            has_data=true
        fi
        
        if [[ "$current_mem" != "0.0 B" ]] && [[ "$current_mem" != "0 B" ]] && [[ -n "$current_mem" ]]; then
            info="${info},MEM:${current_mem}"
        fi
        
        if [[ "$current_cu" != "0" ]] && [[ -n "$current_cu" ]]; then
            info="${info},CU:${current_cu}%"
            has_active_compute=true  # Found actual compute usage!
        fi
        
        if [[ "$has_data" == true ]]; then
            if [[ -n "$gpu_info" ]]; then
                gpu_info="${gpu_info} | ${info}"
            else
                gpu_info="${info}"
            fi
        fi
    fi
    
    # Only report error if there's actual compute usage (CU > 0)
    if [[ "$has_active_compute" == true ]]; then
        echo "🚨 ALERT: Found active GPU compute on GPU(s): ${active_gpus[*]}"
        
        # Check if all GPUs have the same values for compact display
        first_pattern=""
        all_same=true
        IFS='|' read -ra gpu_array <<< "$gpu_info"
        
        for gpu_entry in "${gpu_array[@]}"; do
            # Extract just the usage pattern (remove GPU number)
            pattern=$(echo "$gpu_entry" | sed 's/GPU[0-9]://')
            pattern=$(echo "$pattern" | xargs)  # Trim whitespace
            
            if [[ -z "$first_pattern" ]]; then
                first_pattern="$pattern"
            elif [[ "$pattern" != "$first_pattern" ]]; then
                all_same=false
                break
            fi
        done
        
        # Print result in most compact form
        if [[ "$all_same" == true ]] && [[ ${#active_gpus[@]} -gt 1 ]]; then
            echo "Details: GPU[${active_gpus[0]}-${active_gpus[-1]}]: $first_pattern"
        else
            echo "Details: $gpu_info"
        fi
        
        echo "Status: Active GPU compute detected (CU > 0%)"
        exit 1
    else
        # Only monitoring processes (CU = 0), this is OK
        echo "ℹ️  INFO: Found processes on GPU(s): ${active_gpus[*]}"
        
        # Check if all GPUs have the same values for compact display
        first_pattern=""
        all_same=true
        IFS='|' read -ra gpu_array <<< "$gpu_info"
        
        for gpu_entry in "${gpu_array[@]}"; do
            pattern=$(echo "$gpu_entry" | sed 's/GPU[0-9]://')
            pattern=$(echo "$pattern" | xargs)
            
            if [[ -z "$first_pattern" ]]; then
                first_pattern="$pattern"
            elif [[ "$pattern" != "$first_pattern" ]]; then
                all_same=false
                break
            fi
        done
        
        if [[ "$all_same" == true ]] && [[ ${#active_gpus[@]} -gt 1 ]]; then
            echo "Details: GPU[${active_gpus[0]}-${active_gpus[-1]}]: $first_pattern"
        else
            echo "Details: $gpu_info"
        fi
        
        echo "Status: All CU=0% (monitoring processes only, safe to proceed)"
        # Return success - monitoring processes are acceptable
        exit 0
    fi
fi

# Check GPU usage (as fallback if no processes were detected)
echo "Checking GPU usage percentages..."
rocm_output=$(rocm-smi 2>/dev/null)
active_compute_gpus=()

while read -r line; do
    if [[ "$line" =~ ^[0-9]+[[:space:]] ]]; then
        vram_usage=$(echo "$line" | awk '{print $(NF-1)}' | sed 's/%//')
        gpu_usage=$(echo "$line" | awk '{print $NF}' | sed 's/%//')
        gpu_id=$(echo "$line" | awk '{print $1}')

        # Only consider it active if GPU compute usage > 0 (ignore VRAM-only usage)
        if [[ "$gpu_usage" != "0" ]]; then
            active_compute_gpus+=("GPU $gpu_id: GPU=${gpu_usage}%")
        fi
    fi
done <<< "$rocm_output"

if [[ ${#active_compute_gpus[@]} -gt 0 ]]; then
    echo "🚨 ALERT: Found active GPU compute usage: ${active_compute_gpus[*]}"
    exit 1
fi

echo "✅ All GPUs idle (0% compute usage)"
exit 0