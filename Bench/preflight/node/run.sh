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

# Save original stdout to fd 3 for direct terminal output
exec 3>&1

# Trap Ctrl+C (SIGINT) and other signals
cleanup() {
    echo ""
    echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] Interrupted by user (Ctrl+C)"
    exit 130
}
trap cleanup SIGINT SIGTERM

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
    
    # Create temporary file for error logging
    local err_file=$(mktemp)
    
    # Execute the check script in its directory
    # stdout goes directly to terminal (fd 3), stderr is tee'd to both terminal and file
    bash -c "cd $check_dir && bash run.sh" >&3 2> >(tee "$err_file" >&2)
    exit_code=$?
    
    # Check if interrupted by signal (128 + signal number, SIGINT=2 -> 130)
    if [ $exit_code -ge 128 ]; then
        rm -f "$err_file"
        return $exit_code
    fi
    
    # Return error output if check failed (for summary)
    if [ $exit_code -ne 0 ]; then
        cat "$err_file" | tr -d '\n'
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
ret=$?
if [ $ret -ge 128 ]; then
    exit $ret
fi
if [ $ret -ne 0 ]; then
    errors+="$error_output"
    has_error=1
fi

# ----------------------------------------------------------------------------
# Phase 2: Run node tests (rccl-test, cpu-perf, etc.)
# ----------------------------------------------------------------------------
echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] Phase 2: Running node checks..."
error_output=$(run_check_phase "node_check" "node check")
ret=$?
if [ $ret -ge 128 ]; then
    exit $ret
fi
if [ $ret -ne 0 ]; then
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
ret=$?
if [ $ret -ge 128 ]; then
    exit $ret
fi
if [ $ret -ne 0 ]; then
    if [ -n "$errors" ]; then
        errors+=" | "
    fi
    errors+="$error_output"
    has_error=1
fi

# ============================================================================
# Output final summary
# ============================================================================

final_ret=0
if [ -n "$errors" ]; then
    echo "${LOG_HEADER}[NODE] [ERROR]❌: $errors"
    final_ret=1
elif [ $has_error -eq 1 ]; then
    echo "${LOG_HEADER}[NODE] [ERROR]❌: One or more checks failed (check logs for details)"
    final_ret=1
else
    echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] [NODE] [SUCCESS] ✅ All checks passed"
fi

echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] diagnose finished"
exit $final_ret