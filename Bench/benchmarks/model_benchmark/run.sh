#!/bin/bash
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Model Benchmark: clone/pull latest Primus and run actual training to measure
# throughput (tokens/s/GPU, TFLOP/s/GPU, step time).
#
# Required env vars (set by ansible playbook):
#   WORLD_SIZE, RANK, NODE_RANK, MASTER_ADDR, MASTER_PORT, GPUS_PER_NODE
#
# Optional env vars (see config.sh):
#   MODEL_BENCHMARK_PRIMUS_REPO    - git URL
#   MODEL_BENCHMARK_PRIMUS_BRANCH  - branch (default: main)
#   MODEL_BENCHMARK_CONFIG         - path to experiment YAML (defaults to local config)
#   MODEL_BENCHMARK_TRAIN_ITERS    - training iterations (default: 50)
#   MODEL_BENCHMARK_WARMUP_ITERS   - warmup iters to discard (default: 2)
#   MODEL_BENCHMARK_GBS            - adjusted global_batch_size (set by adapt_nodes.py)
#   SHARE_PATH                     - NFS path to cache Primus repo clone

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] [MODEL-BENCH] $*"; }

PRIMUS_REPO="${MODEL_BENCHMARK_PRIMUS_REPO:-https://github.com/AMD-AGI/Primus.git}"
PRIMUS_BRANCH="${MODEL_BENCHMARK_PRIMUS_BRANCH:-main}"
TRAIN_ITERS="${MODEL_BENCHMARK_TRAIN_ITERS:-50}"
WARMUP_ITERS="${MODEL_BENCHMARK_WARMUP_ITERS:-2}"

NNODES="${WORLD_SIZE:-1}"
NODE_RANK="${RANK:-0}"
GPUS="${GPUS_PER_NODE:-8}"
MASTER="${MASTER_ADDR:-localhost}"
MPORT="${MASTER_PORT:-29500}"

# ======================================================================
# Step 1: Obtain Primus source
# ======================================================================
if [ -n "${SHARE_PATH:-}" ] && [ -d "${SHARE_PATH}" ]; then
    PRIMUS_DIR="${SHARE_PATH}/primus_repo"
else
    PRIMUS_DIR="/tmp/primus_repo"
fi

acquire_primus() {
    local target="$1"

    if [ -d "$target/.git" ]; then
        log "Updating existing Primus clone at $target"
        cd "$target"
        git fetch origin "$PRIMUS_BRANCH" --depth=1 2>&1 || true
        git checkout FETCH_HEAD 2>&1 || git checkout "$PRIMUS_BRANCH" 2>&1 || true
        cd - > /dev/null
    else
        log "Cloning Primus from $PRIMUS_REPO (branch: $PRIMUS_BRANCH)"
        git clone --depth=1 --branch "$PRIMUS_BRANCH" "$PRIMUS_REPO" "$target" 2>&1
    fi
}

if [ -n "${SHARE_PATH:-}" ] && [ -d "${SHARE_PATH}" ]; then
    LOCKFILE="${SHARE_PATH}/.primus_clone.lock"
    (
        flock -w 300 9 || { log "ERROR: failed to acquire clone lock"; exit 1; }
        acquire_primus "$PRIMUS_DIR"
    ) 9>"$LOCKFILE"
else
    acquire_primus "$PRIMUS_DIR"
fi

if [ ! -d "$PRIMUS_DIR/primus" ]; then
    log "ERROR: Primus source not found at $PRIMUS_DIR/primus"
    exit 1
fi
log "Primus source ready at $PRIMUS_DIR (branch: $PRIMUS_BRANCH)"

# ======================================================================
# Step 2: Resolve experiment config
# ======================================================================
EXP_CONFIG="${MODEL_BENCHMARK_CONFIG:-$SCRIPT_DIR/configs/qwen3_8B-BF16-bench.yaml}"

if [ ! -f "$EXP_CONFIG" ]; then
    log "ERROR: experiment config not found: $EXP_CONFIG"
    exit 1
fi
log "Using experiment config: $EXP_CONFIG"

# ======================================================================
# Step 3: Launch training via torchrun
# ======================================================================
TRAIN_LOG="${OUTPUT_DIR:-/tmp}/model_benchmark_train.log"
mkdir -p "$(dirname "$TRAIN_LOG")"

cd "$PRIMUS_DIR"

OVERRIDE_ARGS=""
OVERRIDE_ARGS="$OVERRIDE_ARGS --train_iters=$TRAIN_ITERS"
OVERRIDE_ARGS="$OVERRIDE_ARGS --mock_data=true"
OVERRIDE_ARGS="$OVERRIDE_ARGS --save=null"
OVERRIDE_ARGS="$OVERRIDE_ARGS --load=null"
OVERRIDE_ARGS="$OVERRIDE_ARGS --disable_last_saving=true"
OVERRIDE_ARGS="$OVERRIDE_ARGS --log_avg_skip_iterations=$WARMUP_ITERS"
OVERRIDE_ARGS="$OVERRIDE_ARGS --log_avg_reset_interval=$TRAIN_ITERS"

if [ -n "${MODEL_BENCHMARK_GBS:-}" ]; then
    OVERRIDE_ARGS="$OVERRIDE_ARGS --global_batch_size=$MODEL_BENCHMARK_GBS"
    log "Using adapted global_batch_size=$MODEL_BENCHMARK_GBS"
fi

log "Launching training: nodes=$NNODES rank=$NODE_RANK gpus=$GPUS iters=$TRAIN_ITERS"

torchrun \
    --nproc_per_node="$GPUS" \
    --nnodes="$NNODES" \
    --node_rank="$NODE_RANK" \
    --master_addr="$MASTER" \
    --master_port="$MPORT" \
    primus/cli/main.py train pretrain \
    --config "$EXP_CONFIG" \
    $OVERRIDE_ARGS \
    2>&1 | tee "$TRAIN_LOG"

train_rc=${PIPESTATUS[0]}

if [ $train_rc -ne 0 ]; then
    log "ERROR: Training exited with code $train_rc"
    exit $train_rc
fi

# ======================================================================
# Step 4: Parse metrics (rank 0 only)
# ======================================================================
if [ "$NODE_RANK" = "0" ]; then
    RESULTS_FILE="${OUTPUT_DIR:-/tmp}/model_benchmark_results.json"
    log "Parsing training metrics from $TRAIN_LOG"

    python3 "$SCRIPT_DIR/collect_metrics.py" \
        --log-file "$TRAIN_LOG" \
        --config "$EXP_CONFIG" \
        --nodes "$NNODES" \
        --gpus-per-node "$GPUS" \
        --warmup-iters "$WARMUP_ITERS" \
        --output "$RESULTS_FILE"

    if [ -f "$RESULTS_FILE" ]; then
        log "Results written to $RESULTS_FILE"
        cat "$RESULTS_FILE"
    else
        log "WARNING: Failed to produce results JSON"
    fi
fi

log "Model benchmark completed on node rank $NODE_RANK"
