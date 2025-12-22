#!/bin/bash

# Multi-GPU training launcher with immediate error detection
set -e  # Exit on error

#############################################################################
# Configuration and Setup
#############################################################################

readonly SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
readonly LOG_DIR="/tmp/model_check_logs_$$_$(date +%s)"
readonly SEPARATOR="============================================================"

export PYTHONPATH="$SCRIPT_DIR:$PYTHONPATH"

#############################################################################
# Helper Functions
#############################################################################

print_separator() {
    echo "$SEPARATOR"
}

log_info() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

log_error() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] ERROR: $*" >&2
}

# Display GPU launch information
show_gpu_info() {
    local gpu_id=$1
    local log_file=$2
    echo ""
    echo "[GPU $gpu_id] Launching training process:"
    printf "  %-20s %s\n" "CUDA Device:" "$gpu_id"
    printf "  %-20s %s\n" "Environment:" "CUDA_VISIBLE_DEVICES=$gpu_id GPU_RANK=$gpu_id"
    printf "  %-20s %s\n" "Command:" "python3 pretrain_main.py $*"
    printf "  %-20s %s\n" "Working dir:" "$(pwd)"
    printf "  %-20s %s\n" "Log file:" "$log_file"
    printf "  %-20s %s\n" "Process ID:" "${PIDS[$gpu_id]}"
    printf "  %-20s %s\n" "Status:" "Started successfully"
}

# Display log file summary
show_log_summary() {
    local log_file=$1
    [ ! -f "$log_file" ] && return
    
    # Suppress log summary output for cleaner display
    # local log_size=$(wc -c < "$log_file")
    # local log_lines=$(wc -l < "$log_file")
    # printf "  %-20s %s bytes (%s lines)\n" "Log size:" "$log_size" "$log_lines"
    # 
    # if [ $log_lines -gt 0 ]; then
    #     echo "  Last output:"
    #     tail -n 3 "$log_file" | grep -v "ERROR\|====" | sed 's/^/      /'
    # fi
}

# Extract and clean error messages from log
extract_error() {
    local log_file=$1
    local gpu_id=$2
    
    [ ! -f "$log_file" ] && { echo "No log file found"; return; }
    
    # Find first error or last meaningful line
    local error_line=$(grep -E "EXITING DUE TO|NaN DETECTED|Exception|Traceback|ERROR" "$log_file" 2>/dev/null | \
                      grep -v "====" | head -1 || \
                      tail -n 10 "$log_file" 2>/dev/null | grep -v "^$\|====" | tail -1 || \
                      echo "No error message found")
    
    # Clean prefixes and format
    echo "$error_line" | sed -e 's/GPU[0-9]*:\(ERROR\|INFO\)[: |]*//g' \
                             -e 's/\[GPU [0-9]*\] //g' \
                             -e 's/ERROR[: |]*//g' \
                             -e 's/ | / /g'
}

# Process cleanup function
cleanup() {
    [ "${CLEANUP_DONE:-0}" -eq 1 ] && return
    CLEANUP_DONE=1
    
    # log_error "Stopping all GPU processes..."
    
    # First, kill all child processes of this script
    local children=$(jobs -p 2>/dev/null)
    if [ -n "$children" ]; then
        # Silently terminate child processes
        { kill -TERM $children; } >/dev/null 2>&1 || true
        sleep 1
        { kill -KILL $children; } >/dev/null 2>&1 || true
    fi
    
    # Then kill tracked PIDs
    for gpu_id in "${!PIDS[@]}"; do
        local pid="${PIDS[$gpu_id]}"
        [ -z "$pid" ] && continue
        
        if kill -0 "$pid" 2>/dev/null; then
            log_error "  Stopping GPU $gpu_id (PID $pid)"
            { kill -TERM "$pid"; } >/dev/null 2>&1 || true
        fi
    done
    
    # Give processes time to cleanup
    sleep 1
    
    # Force kill any remaining processes
    for gpu_id in "${!PIDS[@]}"; do
        local pid="${PIDS[$gpu_id]}"
        [ -z "$pid" ] && continue
        
        if kill -0 "$pid" 2>/dev/null; then
            log_error "  Force killing GPU $gpu_id (PID $pid)"
            { kill -KILL "$pid"; } >/dev/null 2>&1 || true
        fi
    done
    
    # Final cleanup of any orphaned python processes
    { pkill -KILL -f "pretrain_main.py"; } >/dev/null 2>&1 || true
    
    # Wait for all background jobs to finish
    wait 2>/dev/null || true
    
    # log_error "Cleanup completed. Logs: $LOG_DIR"
}

#############################################################################
# Initialization
#############################################################################

# Setup log directory
mkdir -p "$LOG_DIR"
log_info "Log directory: $LOG_DIR"

# Check and install dependencies
if ! python3 -c "import datasets" 2>/dev/null; then
    log_info "Installing required packages (this may take a few minutes)..."
    pip3 install -r "$SCRIPT_DIR/requirements.txt" || {
        log_error "Failed to install dependencies"
        exit 1
    }
    log_info "Dependencies installed successfully!"
fi

# Detect GPUs (support both CUDA and ROCm)
NUM_GPUS=$(python3 -c "
import torch
# Try CUDA first, then ROCm
if torch.cuda.is_available():
    print(torch.cuda.device_count())
elif hasattr(torch, 'hip') and torch.hip.is_available():
    print(torch.cuda.device_count())  # ROCm uses cuda namespace
else:
    # Fall back to checking rocm-smi
    import subprocess
    import re
    try:
        result = subprocess.run(['rocm-smi', '--showid'], capture_output=True, text=True)
        if result.returncode == 0:
            # Extract unique GPU IDs (e.g., GPU[0], GPU[1], etc.)
            gpu_ids = set(re.findall(r'GPU\[(\d+)\]', result.stdout))
            gpu_count = len(gpu_ids)
            print(gpu_count if gpu_count > 0 else 8)  # Default to 8 if parsing fails
        else:
            print(0)
    except:
        print(0)
" 2>/dev/null || echo "8")  # Default to 8 GPUs for AMD systems
[ "$NUM_GPUS" = "0" ] && { log_error "No GPUs detected"; exit 1; }

log_info "Detected $NUM_GPUS GPU(s)"

# Prepare dataset
log_info "Preparing dataset..."
cd "$SCRIPT_DIR"
python3 prepare_dataset.py || exit 1

#############################################################################
# Launch Training Processes
#############################################################################

# Set up signal handling before launching processes
declare -a PIDS=()
CLEANUP_DONE=0
trap 'cleanup; exit 130' INT
trap 'cleanup; exit 143' TERM
trap 'cleanup' EXIT

print_separator
echo "Launching training on $NUM_GPUS GPUs"
print_separator

for GPU_ID in $(seq 0 $((NUM_GPUS - 1))); do
    LOG_FILE="$LOG_DIR/gpu_${GPU_ID}.log"
    
    # Launch GPU process with clean output formatting
    # Use ( ) to create subshell that can be killed as a group
    # Redirect stderr to suppress "Killed" messages
    {
        (
            exec 2>&1
            # Trap signals to ensure entire pipeline is killed
            trap 'exit 143' TERM
            trap 'exit 130' INT
            
            # Use HIP_VISIBLE_DEVICES for ROCm, CUDA_VISIBLE_DEVICES for NVIDIA
            # MI355X requires HSA_OVERRIDE_GFX_VERSION
            CUDA_VISIBLE_DEVICES=$GPU_ID HIP_VISIBLE_DEVICES=$GPU_ID python3 pretrain_main.py "$@" | \
            tee "$LOG_FILE" | while IFS= read -r line; do
                # Clean line: remove GPU prefixes and replace | with space
                clean_line=$(echo "$line" | sed -e 's/^GPU[0-9]*:\(INFO\|ERROR\)[: |]*//g' \
                                                -e 's/^\[GPU [0-9]*\] //' \
                                                -e 's/ | / /g')
                echo "[GPU$GPU_ID] $clean_line"
            done
        ) 2>&3 3>&-
    } 3>&2 2>/dev/null &
    
    PIDS[$GPU_ID]=$!
    show_gpu_info $GPU_ID "$LOG_FILE"
    sleep 0.5  # Avoid resource contention
done

echo ""
print_separator
echo "All $NUM_GPUS GPUs started. Monitoring training progress..."
print_separator

#############################################################################
# Monitor Training Progress
#############################################################################

set +e  # Allow handling of process exit codes

while true; do
    all_done=true
    
    for gpu_id in "${!PIDS[@]}"; do
        pid="${PIDS[$gpu_id]}"
        [ -z "$pid" ] && continue
        
        if ! kill -0 "$pid" 2>/dev/null; then
            wait "$pid"
            exit_code=$?
            log_file="$LOG_DIR/gpu_${gpu_id}.log"
            
            # Check for critical errors in log
            has_error=$([[ $exit_code -ne 0 ]] || \
                       grep -q "EXITING DUE TO\|NaN DETECTED\|ERROR.*EXITING" "$log_file" 2>/dev/null && \
                       echo 1 || echo 0)
            
            echo ""
            if [ "$has_error" -eq 1 ]; then
                echo "[GPU $gpu_id] Process FAILED"
                printf "  %-20s %s\n" "PID:" "$pid"
                printf "  %-20s %s\n" "Exit code:" "$exit_code"
                [ $exit_code -eq 0 ] && echo "  Note: Critical error in log despite exit code 0"
                printf "  %-20s %s\n" "Error:" "$(extract_error "$log_file" "$gpu_id")"
                show_log_summary "$log_file"
                
                # Stop all processes and exit
                # log_error "Stopping all GPUs due to GPU $gpu_id failure"
                
                # Report failure first (before cleanup might cause issues)
                echo ""
                print_separator
                echo "[FAILURE] Training failed on GPU $gpu_id"
                print_separator
                log_error "$(extract_error "$log_file" "$gpu_id")"
                # echo "Log files: $LOG_DIR"
                # echo "Failed GPU log: $LOG_DIR/gpu_${gpu_id}.log"
                
                # Now cleanup and exit
                cleanup
                wait 2>/dev/null  # Wait for all background processes to finish
                exit 1
            else
                echo "[GPU $gpu_id] Process completed successfully"
                printf "  %-20s %s\n" "PID:" "$pid"
                printf "  %-20s %s\n" "Exit code:" "0 (success)"
                show_log_summary "$log_file"
            fi
            
            unset PIDS[$gpu_id]
        else
            all_done=false
        fi
    done
    
    [ "$all_done" = "true" ] && break
    sleep 1
done

#############################################################################
# Report Success
#############################################################################

echo ""
print_separator
echo "[SUCCESS] All GPU training completed successfully"
print_separator
# echo "Log files: $LOG_DIR"
exit 0