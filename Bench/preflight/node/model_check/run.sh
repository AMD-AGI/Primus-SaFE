#!/bin/bash

# Multi-GPU training launcher with immediate error detection

set -e  # Exit on error

# Setup Python path
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
export PYTHONPATH="$SCRIPT_DIR:$PYTHONPATH"

# Create log directory for GPU outputs
LOG_DIR="/tmp/model_check_logs_$$"
mkdir -p "$LOG_DIR"

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
for GPU_ID in $(seq 0 $((NUM_GPUS - 1))); do
    echo "Starting GPU $GPU_ID..."
    LOG_FILE="$LOG_DIR/gpu_${GPU_ID}.log"
    CUDA_VISIBLE_DEVICES=$GPU_ID GPU_RANK=$GPU_ID python3 pretrain_main.py "$@" > "$LOG_FILE" 2>&1 &
    PIDS+=($!)
    sleep 0.5  # Small delay to avoid resource contention
done

echo "All GPUs started. Monitoring for errors..."

# Function to extract error from log file
extract_error() {
    local log_file=$1
    local gpu_id=$2
    if [ -f "$log_file" ]; then
        # Look for NaN errors, EXITING messages, or other critical errors
        local error_line=$(grep -E "(EXITING|NaN DETECTED|ERROR|Exception|Traceback)" "$log_file" | head -1)
        if [ -n "$error_line" ]; then
            echo "GPU${gpu_id}:$error_line"
        else
            # Get last non-empty line as fallback
            local last_line=$(tail -n 5 "$log_file" | grep -v "^$" | tail -1)
            echo "GPU${gpu_id}:$last_line"
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
    # Clean up log directory
    rm -rf "$LOG_DIR" 2>/dev/null || true
}

# Trap signals to cleanup on exit
trap cleanup EXIT INT TERM

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
                if [ $EXIT_CODE -ne 0 ]; then
                    # Extract specific error from log
                    ERROR_MSG=$(extract_error "$LOG_FILE" "$i")
                    echo "ERROR: GPU $i (PID $PID) failed with exit code $EXIT_CODE"
                    echo "  Error: $ERROR_MSG"
                    # Show last few lines of log for context
                    if [ -f "$LOG_FILE" ]; then
                        echo "  Last output:"
                        tail -n 5 "$LOG_FILE" | sed 's/^/    /'
                    fi
                    FAILED=1
                    FAILED_GPU=$i
                    break 2  # Exit both loops
                else
                    echo "GPU $i (PID $PID) completed successfully"
                    PIDS[$i]=""  # Clear this PID
                fi
            fi
        fi
    done
    
    # Check if all processes completed
    ALL_DONE=1
    for PID in "${PIDS[@]}"; do
        if [ -n "$PID" ]; then
            ALL_DONE=0
            break
        fi
    done
    
    if [ $ALL_DONE -eq 1 ]; then
        break
    fi
    
    # Small delay before next check
    sleep 1
done

# Report final status
if [ $FAILED -eq 0 ]; then
    echo "All training completed successfully"
    rm -rf "$LOG_DIR" 2>/dev/null || true
    exit 0
else
    # Output error message to stderr for parent script to capture
    echo "$ERROR_MSG" >&2
    rm -rf "$LOG_DIR" 2>/dev/null || true
    exit 1
fi