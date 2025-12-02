#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# ============================================================================
# Initialize environment and logging
# ============================================================================

if [ "$ADD_LOG_HEADER" == "true" ]; then
    export LOG_HEADER="[$(hostname)] [NODE-$RANK] "
fi

echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] start to diagnose"

# Export environment variables
export RANK=$RANK
export NCCL_SOCKET_IFNAME=${NCCL_SOCKET_IFNAME:-"eth0"}
export NCCL_IB_HCA=${NCCL_IB_HCA:-""}
export TEST_LEVEL=${TEST_LEVEL:-"BASIC"}

# ============================================================================
# Helper function to run check scripts
# ============================================================================

# Function to execute a check phase with proper error handling
# Arguments:
#   $1 - Directory name to check
#   $2 - Phase description for logging
run_check_phase() {
    local check_dir=$1
    local phase_desc=$2
    local exit_code=0
    
    # Create temporary files for logging
    local log_file=$(mktemp) && touch "$log_file"
    local err_file=$(mktemp) && touch "$err_file"
    
    # Start log tail in background
    tail -f "$log_file" &
    local tail_pid=$!
    sleep 0.5
    
    # Execute the check script in its directory
    bash -c "cd $check_dir && bash run.sh" > "$log_file" 2>"$err_file"
    exit_code=$?
    
    # Clean up log tail
    sync && sleep 2
    kill $tail_pid 2>/dev/null
    rm -f "$log_file"
    
    # Process error output if check failed
    if [ $exit_code -ne 0 ]; then
        local error_output=$(cat "$err_file" | tr -d '\n')
        echo "$error_output"
    fi
    
    # Clean up error file
    rm -f "$err_file"
    
    return $exit_code
}

# ============================================================================
# Main execution
# ============================================================================

# Initialize error collection
errors=""
has_error=0  # Track if any phase failed

# ----------------------------------------------------------------------------
# Phase 1: Check configuration on node
# ----------------------------------------------------------------------------
echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] Phase 1: Checking node configuration..."
error_output=$(run_check_phase "config_check" "config check")
if [ $? -ne 0 ]; then
    errors+="$error_output"
    has_error=1
fi

# ----------------------------------------------------------------------------
# Phase 2: Run node tests (rccl-test, cpu-perf, etc.)
# ----------------------------------------------------------------------------
echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] Phase 2: Running node checks..."
error_output=$(run_check_phase "node_check" "node check")
if [ $? -ne 0 ]; then
    if [ -n "$errors" ]; then
        errors+=" | "
    fi
    errors+="$error_output"
    has_error=1
fi

# ----------------------------------------------------------------------------
# Phase 3: Run model checks (model-train, model-inference, etc.)
# ----------------------------------------------------------------------------
echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] Phase 3: Running model checks..."
error_output=$(run_check_phase "model_check" "model check")
if [ $? -ne 0 ]; then
    if [ -n "$errors" ]; then
        errors+=" | "
    fi
    errors+="$error_output"
    has_error=1
fi

# ============================================================================
# Output final summary
# ============================================================================

ret=0
if [ -n "$errors" ]; then
    echo "${LOG_HEADER}[NODE] [ERROR]❌: $errors"
    ret=1
elif [ $has_error -eq 1 ]; then
    echo "${LOG_HEADER}[NODE] [ERROR]❌: One or more checks failed (check logs for details)"
    ret=1
else
    echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] [NODE] [SUCCESS] ✅ All checks passed"
fi

echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] diagnose finished"
exit $ret