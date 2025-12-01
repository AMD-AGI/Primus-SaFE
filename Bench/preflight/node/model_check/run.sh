#!/bin/bash

# Multi-GPU training launcher with immediate error detection

set -e  # Exit on error

# Setup Python path
export PYTHONPATH="$(dirname "$0"):$PYTHONPATH"

# Install dependencies if not already installed
echo "Checking dependencies..."
if ! python3 -c "import datasets" 2>/dev/null; then
    echo "Installing required packages (this may take a few minutes)..."
    echo "Installing: torch, transformers, datasets, flash-attn, etc."
    pip3 install -r "$(dirname "$0")/requirements.txt" || {
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
python3 prepare_dataset.py || exit 1

# Launch training on all GPUs
PIDS=()
for GPU_ID in $(seq 0 $((NUM_GPUS - 1))); do
    echo "Starting GPU $GPU_ID..."
    CUDA_VISIBLE_DEVICES=$GPU_ID GPU_RANK=$GPU_ID python3 pretrain_main.py "$@" &
    PIDS+=($!)
    sleep 0.5  # Small delay to avoid resource contention
done

echo "All GPUs started. Monitoring for errors..."

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
}

# Trap signals to cleanup on exit
trap cleanup EXIT INT TERM

# Monitor all processes in real-time
FAILED=0
FAILED_GPU=""
while true; do
    for i in "${!PIDS[@]}"; do
        PID=${PIDS[$i]}
        if [ -n "$PID" ]; then
            if ! kill -0 $PID 2>/dev/null; then
                # Process has exited, check exit code
                wait $PID
                EXIT_CODE=$?
                if [ $EXIT_CODE -ne 0 ]; then
                    echo "ERROR: GPU $i (PID $PID) failed with exit code $EXIT_CODE" >&2
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
    exit 0
else
    echo "Training failed on GPU $FAILED_GPU" >&2
    echo "Check logs above for error details (e.g., NaN detection)" >&2
    exit 1
fi