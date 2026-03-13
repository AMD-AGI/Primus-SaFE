#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Pre-download tokenizer (Qwen/Qwen3-8B) and dataset (wikitext) during Docker build.
# This avoids runtime network downloads and ReadTimeout errors.
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODEL_CHECK_DIR="$SCRIPT_DIR/../node/model_check"
CACHE_ROOT="${PRIMUSBENCH_CACHE:-/opt/primusbench_cache}"

echo "============== begin to pre-download model_check assets =============="

# Use fixed cache paths (not under workspace) so cache persists when workspace is mounted
export HF_HOME="${CACHE_ROOT}/huggingface"
export HF_HUB_DOWNLOAD_TIMEOUT="${HF_HUB_DOWNLOAD_TIMEOUT:-600}"
export HF_HUB_ETAG_TIMEOUT="${HF_HUB_ETAG_TIMEOUT:-120}"
export DATASET_CACHE_DIR="${CACHE_ROOT}/datasets"
export TRANSFORMERS_CACHE="${HF_HOME}/hub"

mkdir -p "$HF_HOME" "$DATASET_CACHE_DIR"

if [ ! -d "$MODEL_CHECK_DIR" ]; then
  echo "model_check directory not found, skipping pre-download" >&2
  exit 0
fi

cd "$MODEL_CHECK_DIR" || exit 1
export PYTHONPATH="$MODEL_CHECK_DIR:$PYTHONPATH"

if python3 prepare_dataset.py; then
  echo "Pre-download completed: tokenizer + dataset cached at ${CACHE_ROOT}"
else
  echo "Warning: Pre-download failed (network may be unavailable during build). Runtime will retry." >&2
  # Don't fail the build - runtime has retry logic
  exit 0
fi

echo "============== pre-download model_check assets successfully =============="
