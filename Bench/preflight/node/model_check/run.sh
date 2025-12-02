#!/bin/bash

# Multi-GPU training launcher with immediate error detection

set -e  # Exit on error

# Setup Python path
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
export PYTHONPATH="$SCRIPT_DIR:$PYTHONPATH"

# Create log directory for GPU outputs (with timestamp for debugging)
LOG_DIR="/tmp/model_check_logs_$$_$(date +%s)"
mkdir -p "$LOG_DIR"
echo "Log directory: $LOG_DIR"

# Install dependencies if not already installed
echo "Checking dependencies..."
if ! python3 -c "import datasets" 2>/dev/null; then
    echo "Installing required packages (this may take a few minutes)..."
    echo "Installing: torch, transformers, datasets, flash-attn, etc."
    pip3 install -r "$SCRIPT_DIR/requirements.txt" || {
        echo "Failed to install dependencies" >&2
        exit 1
    }
    echo "Dependencies installed successfully!"
fi

# Detect GPUs
NUM_GPUS=$(python3 -c "import torch; print(torch.cuda.device_count())" 2>/dev/null || echo "0")

if [ "$NUM_GPUS" = "0" ]; then
    echo "No GPUs detected" >&2
    exit 1
fi

echo "Detected $NUM_GPUS GPU(s)"

# Prepare cached dataset (only downloads/tokenizes once)
echo "Preparing dataset..."
cd "$SCRIPT_DIR"
python3 prepare_dataset.py || exit 1

# Launch training on all GPUs with output logging
PIDS=()
echo "============================================================"
echo "Launching training on $NUM_GPUS GPUs"
echo "============================================================"
for GPU_ID in $(seq 0 $((NUM_GPUS - 1))); do
    LOG_FILE="$LOG_DIR/gpu_${GPU_ID}.log"
    
    # Display detailed info about this GPU launch
    echo ""
    echo "[GPU $GPU_ID] Launching training process:"
    echo "  - CUDA Device: $GPU_ID"
    echo "  - Environment: CUDA_VISIBLE_DEVICES=$GPU_ID GPU_RANK=$GPU_ID"
    echo "  - Command: python3 pretrain_main.py $*"
    echo "  - Working dir: $(pwd)"
    echo "  - Log file: $LOG_FILE"
    
    # Start the process with output to both screen and log file
    # Use tee with process substitution to add GPU prefix for screen output
    # The entire pipeline is wrapped in a subshell to capture the right PID
    (
        exec 2>&1  # Redirect stderr to stdout
        CUDA_VISIBLE_DEVICES=$GPU_ID GPU_RANK=$GPU_ID python3 pretrain_main.py "$@" | \
        tee "$LOG_FILE" | sed "s/^/[GPU$GPU_ID] /"
    ) &
    PID=$!
    PIDS+=($PID)
    
    echo "  - Process ID: $PID"
    echo "  - Status: Started successfully"
    
    sleep 0.5  # Small delay to avoid resource contention
done
echo ""
echo "============================================================"

echo "All $NUM_GPUS GPUs started. Monitoring training progress..."
echo "============================================================"

# Function to extract error from log file
extract_error() {
    local log_file=$1
    local gpu_id=$2
    if [ -f "$log_file" ]; then
        # Priority 1: Look for EXITING messages (most critical)
        local error_line=$(grep "EXITING DUE TO" "$log_file" 2>/dev/null | head -1 || true)
        
        # Priority 2: Look for NaN DETECTED
        if [ -z "$error_line" ]; then
            error_line=$(grep "NaN DETECTED" "$log_file" 2>/dev/null | head -1 || true)
        fi
        
        # Priority 3: Look for Exception or Traceback
        if [ -z "$error_line" ]; then
            error_line=$(grep -E "(Exception|Traceback)" "$log_file" 2>/dev/null | head -1 || true)
        fi
        
        # Priority 4: Look for ERROR (but exclude separator lines)
        if [ -z "$error_line" ]; then
            error_line=$(grep "ERROR" "$log_file" 2>/dev/null | grep -v "====" | head -1 || true)
        fi
        
        if [ -n "$error_line" ]; then
            # Clean up the error line - remove GPU prefix if it's already there
            if echo "$error_line" | grep -q "GPU.*ERROR"; then
                # Extract everything after "ERROR |"
                local clean_msg=$(echo "$error_line" | sed 's/.*ERROR | //')
                echo "GPU${gpu_id}:ERROR | $clean_msg"
            elif echo "$error_line" | grep -q "GPU.*INFO"; then
                # Extract everything after "INFO |"
                local clean_msg=$(echo "$error_line" | sed 's/.*INFO | //')
                echo "GPU${gpu_id}:INFO | $clean_msg"
            else
                echo "GPU${gpu_id}:$error_line"
            fi
        else
            # Get last meaningful line as fallback (excluding empty lines and separators)
            local last_line=$(tail -n 10 "$log_file" 2>/dev/null | grep -v "^$" | grep -v "====" | tail -1 || true)
            if [ -n "$last_line" ]; then
                echo "GPU${gpu_id}:$last_line"
            else
                echo "GPU${gpu_id}:No error message found"
            fi
        fi
    else
        echo "GPU${gpu_id}:No log file found"
    fi
}

# Function to kill all processes
cleanup() {
    echo "Stopping all GPU processes..."
    for PID in "${PIDS[@]}"; do
        if kill -0 $PID 2>/dev/null; then
            kill -TERM $PID 2>/dev/null || true
        fi
    done
    # Give processes time to cleanup
    sleep 2
    # Force kill if still running
    for PID in "${PIDS[@]}"; do
        if kill -0 $PID 2>/dev/null; then
            kill -KILL $PID 2>/dev/null || true
        fi
    done
    # Clean up log directory (temporarily disabled for debugging)
    # rm -rf "$LOG_DIR" 2>/dev/null || true
    echo "Logs kept for debugging: $LOG_DIR"
}

# Trap signals to cleanup on exit
trap cleanup EXIT INT TERM

# Disable set -e for monitoring loop to handle process exit codes properly
set +e

# Monitor all processes in real-time
FAILED=0
FAILED_GPU=""
ERROR_MSG=""
while true; do
    for i in "${!PIDS[@]}"; do
        PID=${PIDS[$i]}
        if [ -n "$PID" ]; then
            if ! kill -0 $PID 2>/dev/null; then
                # Process has exited, check exit code
                wait $PID
                EXIT_CODE=$?
                LOG_FILE="$LOG_DIR/gpu_${i}.log"
                
                # Check log for critical errors regardless of exit code
                # Some errors (like NaN) might not change the exit code
                HAS_CRITICAL_ERROR=0
                if [ -f "$LOG_FILE" ]; then
                    if grep -q "EXITING DUE TO\|NaN DETECTED\|ERROR.*EXITING" "$LOG_FILE" 2>/dev/null; then
                        HAS_CRITICAL_ERROR=1
                    fi
                fi
                
                if [ $EXIT_CODE -ne 0 ] || [ $HAS_CRITICAL_ERROR -eq 1 ]; then
                    # Extract specific error from log
                    ERROR_MSG=$(extract_error "$LOG_FILE" "$i")
                    echo ""
                    echo "[GPU $i] Process FAILED"
                    echo "  - PID: $PID"
                    echo "  - Exit code: $EXIT_CODE"
                    if [ $HAS_CRITICAL_ERROR -eq 1 ] && [ $EXIT_CODE -eq 0 ]; then
                        echo "  - Note: Critical error detected in log despite exit code 0"
                    fi
                    # Display error message without duplicate GPU prefix
                    echo "  - Error: $(echo "$ERROR_MSG" | sed "s/GPU${i}://")"
                    # Show last few lines of log for context
                    if [ -f "$LOG_FILE" ]; then
                        LOG_SIZE=$(wc -c < "$LOG_FILE")
                        LOG_LINES=$(wc -l < "$LOG_FILE")
                        echo "  - Log size: $LOG_SIZE bytes ($LOG_LINES lines)"
                        echo "  - Last output (excluding errors):"
                        # Filter out error lines and separators from tail output
                        tail -n 10 "$LOG_FILE" | grep -v "ERROR\|====" | tail -n 3 | sed 's/^/      /'
                    fi
                    FAILED=1
                    FAILED_GPU=$i
                    break 2  # Exit both loops
                else
                    echo ""
                    echo "[GPU $i] Process completed successfully"
                    echo "  - PID: $PID"
                    echo "  - Exit code: 0 (success)"
                    # Check if log file has any content
                    if [ -f "$LOG_FILE" ]; then
                        LOG_SIZE=$(wc -c < "$LOG_FILE")
                        LOG_LINES=$(wc -l < "$LOG_FILE")
                        echo "  - Log size: $LOG_SIZE bytes ($LOG_LINES lines)"
                        # Show last few lines as summary
                        if [ $LOG_LINES -gt 0 ]; then
                            echo "  - Final output:"
                            tail -n 3 "$LOG_FILE" | sed 's/^/      /'
                        fi
                    fi
                    PIDS[$i]=""  # Clear this PID
                fi
            fi
        fi
    done
    
    # Check if all processes completed
    ALL_DONE=1
    RUNNING_COUNT=0
    RUNNING_GPUS=""
    for idx in "${!PIDS[@]}"; do
        if [ -n "${PIDS[$idx]}" ]; then
            ALL_DONE=0
            RUNNING_COUNT=$((RUNNING_COUNT + 1))
            RUNNING_GPUS="$RUNNING_GPUS GPU$idx"
        fi
    done
    
    if [ $ALL_DONE -eq 1 ]; then
        echo ""
        echo "============================================================"
        echo "All GPU processes have completed"
        echo "============================================================"
        break
    fi
    # Status updates are disabled to reduce noise
    # The GPU outputs themselves will show progress
    
    # Small delay before next check
    sleep 1
done

# Report final status
echo ""
echo "============================================================"
if [ $FAILED -eq 0 ]; then
    echo "[SUCCESS] All GPU training completed successfully"
    echo "============================================================"
    echo "Log files available at: $LOG_DIR"
    echo "To view logs: ls -la $LOG_DIR/"
    # rm -rf "$LOG_DIR" 2>/dev/null || true
    exit 0
else
    echo "[FAILURE] Training failed on GPU $FAILED_GPU"
    echo "============================================================"
    # Output clean error message to stderr for parent script to capture
    # The error message should already have proper format from extract_error
    echo "$ERROR_MSG" >&2
    echo ""
    echo "Log files available at: $LOG_DIR"
    echo "To view failed GPU log: cat $LOG_DIR/gpu_${FAILED_GPU}.log"
    # rm -rf "$LOG_DIR" 2>/dev/null || true
    exit 1
fi