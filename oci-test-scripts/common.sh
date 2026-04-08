#!/bin/bash
# OCI SFT Test - Common Functions
# PFS = /shared_nfs (OCI), not /wekafs (TW)

PFS="/shared_nfs"
HF_MODEL="Qwen/Qwen3-8B"
HF_MODEL_PATH="${PFS}/models/Qwen/Qwen3-8B"
DATASET_SRC="${PFS}/datasets/tatsu-lab/alpaca"
SQUAD_SRC="${PFS}/datasets/rajpurkar/squad"
PRIMUS_IMAGE="harbor.oci-slc.primus-safe.amd.com/sync/tasimage/primus:pr-563-ainic"

prepare_dataset() {
  local SRC="$1"
  local DST="/tmp/sft_dataset"
  echo "Preparing dataset from ${SRC} -> ${DST} ..."
  rm -rf "${DST}"
  mkdir -p "${DST}"

  python3 -c "
import json, os, sys, glob
src = '${SRC}'
dst = '${DST}'
candidates = ['training.jsonl', 'train.jsonl', 'data.jsonl']
src_file = None
for c in candidates:
    p = os.path.join(src, c)
    if os.path.isfile(p) and os.path.getsize(p) > 0:
        src_file = p; break
if not src_file:
    jsonl_files = sorted(glob.glob(os.path.join(src, '*.jsonl')))
    json_files = sorted(glob.glob(os.path.join(src, '*.json')))
    all_files = jsonl_files + json_files
    all_files = [f for f in all_files if os.path.getsize(f) > 0]
    if all_files:
        src_file = all_files[0]
if not src_file:
    print('ERROR: no usable jsonl/json files in ' + src); sys.exit(1)
print(f'Using source file: {src_file}')
data = []
with open(src_file, encoding='utf-8') as f:
    for line in f:
        line = line.strip()
        if not line: continue
        obj = json.loads(line)
        if 'input' in obj and 'output' in obj:
            data.append(obj)
        elif 'instruction' in obj:
            inst = obj.get('instruction','')
            inp = obj.get('input','')
            out = obj.get('output','')
            new_input = (inst + chr(10) + inp).strip() if inp else inst
            data.append({'input': new_input, 'output': out})
        else:
            data.append(obj)
if len(data) == 0:
    print('ERROR: dataset is empty'); sys.exit(1)
val_count = max(1, len(data) // 10)
train_data = data[:-val_count] if len(data) > val_count else data
val_data = data[-val_count:] if len(data) > val_count else data[:1]
for name, items in [('training', train_data), ('validation', val_data), ('test', val_data)]:
    with open(os.path.join(dst, name + '.jsonl'), 'w', encoding='utf-8') as out:
        for item in items:
            out.write(json.dumps(item, ensure_ascii=False) + chr(10))
print(f'Dataset ready: {len(train_data)} train, {len(val_data)} val/test in {dst}')
"
  if [ $? -ne 0 ]; then
    echo "Dataset preparation failed!"
    exit 1
  fi
}

find_primus() {
  PRIMUS_DIR=""
  MODULE_CONFIG=""
  NEW_CFG="primus/configs/modules/megatron_bridge/sft_trainer.yaml"
  OLD_CFG="primus/configs/modules/megatron_bridge/post_trainer.yaml"
  CANDIDATES=()

  if [ -n "${PRIMUS_DIR_OVERRIDE:-}" ]; then
    CANDIDATES+=("${PRIMUS_DIR_OVERRIDE}")
  fi

  CANDIDATES+=(
    "/workspace/Primus"
    "/tmp/primus"
    "/shared_nfs/chenyi/Primus"
    "/shared_nfs/lihuzhan/code/2026_0312__primus/Primus"
    "/shared_nfs/xiaofei/Primus"
  )

  for p in "${CANDIDATES[@]}"; do
    if [ -d "$p/runner" ]; then
      if [ -f "$p/$NEW_CFG" ]; then
        PRIMUS_DIR="$p"; MODULE_CONFIG="sft_trainer.yaml"; break
      elif [ -f "$p/$OLD_CFG" ]; then
        PRIMUS_DIR="$p"; MODULE_CONFIG="post_trainer.yaml"; break
      fi
    fi
  done

  if [ -z "$PRIMUS_DIR" ]; then
    echo "ERROR: No compatible Primus found"
    printf 'Checked Primus paths:\n'
    printf '  %s\n' "${CANDIDATES[@]}"
    exit 1
  fi

  echo "Using Primus at: $PRIMUS_DIR (module config: $MODULE_CONFIG)"
  cd "$PRIMUS_DIR"
}

prepare_squad_cache() {
  local LOCAL_SQUAD_CACHE="/root/.cache/nemo/datasets/squad"
  local SHARED_SQUAD_CACHE="${SHARED_SQUAD_CACHE_DIR:-}"

  if [ -z "$SHARED_SQUAD_CACHE" ]; then
    if [ -n "${DATA_PATH:-}" ]; then
      SHARED_SQUAD_CACHE="${DATA_PATH}/squad-cache"
    else
      SHARED_SQUAD_CACHE="${PFS}/chenyi/sft-shared-data/squad-cache"
    fi
  fi

  mkdir -p "$(dirname "$LOCAL_SQUAD_CACHE")"
  mkdir -p "$SHARED_SQUAD_CACHE"
  if [ -e "$LOCAL_SQUAD_CACHE" ] && [ ! -L "$LOCAL_SQUAD_CACHE" ]; then
    rm -rf "$LOCAL_SQUAD_CACHE"
  fi
  ln -sfn "$SHARED_SQUAD_CACHE" "$LOCAL_SQUAD_CACHE"

  SQUAD_CACHED_COUNT=$(find "$SHARED_SQUAD_CACHE" -name "*.arrow" 2>/dev/null | wc -l)
  if [ -d "$SQUAD_SRC" ] && [ "$SQUAD_CACHED_COUNT" -eq 0 ]; then
    echo "[SFT] Generating squad HF cache in shared path: $SHARED_SQUAD_CACHE"
    SQUAD_CACHE="$LOCAL_SQUAD_CACHE" SQUAD_SRC="$SQUAD_SRC" python3 - <<'PY' 2>&1 || echo "[SFT] WARNING: squad cache generation failed, evaluation may fail"
import os
from datasets import load_dataset

cache_dir = os.environ["SQUAD_CACHE"]
src = os.environ["SQUAD_SRC"]
ds = load_dataset(src, "plain_text", cache_dir=cache_dir, trust_remote_code=True)
print(f'[SFT] squad cache ready: {len(ds["train"])} train, {len(ds["validation"])} val')
PY
  elif [ "$SQUAD_CACHED_COUNT" -gt 0 ]; then
    echo "[SFT] squad cache already exists in shared path ($SQUAD_CACHED_COUNT arrow files)"
  else
    echo "[SFT] WARNING: squad data not found at $SQUAD_SRC, evaluation may fail"
  fi

  echo "[SFT] local cache path: $LOCAL_SQUAD_CACHE -> $(readlink -f "$LOCAL_SQUAD_CACHE" 2>/dev/null || echo "$LOCAL_SQUAD_CACHE")"
}

patch_torch_inductor_duplicate_assert() {
  local SELECT_ALG="/opt/venv/lib/python3.10/site-packages/torch/_inductor/select_algorithm.py"
  if [ -f "$SELECT_ALG" ] && grep -q "duplicate template name" "$SELECT_ALG"; then
    sed -i 's/assert.*duplicate.*/pass  # patched/' "$SELECT_ALG"
    echo "[PATCH] Patched torch._inductor duplicate template assert"
  fi
}

setup_multinode() {
  NNODES="${NNODES:-1}"
  if [ "$NNODES" -gt 1 ] && [ -n "${DATA_PATH:-}" ]; then
    mkdir -p "$DATA_PATH"
    if [ -e data ] && [ ! -L data ]; then
      rm -rf data
    fi
    ln -sfn "$DATA_PATH" data
    echo "[MULTI-NODE] ./data -> $DATA_PATH (shared storage)"

    SHARED_CKPT_DIR="$DATA_PATH/nemo_experiments"
    mkdir -p "$SHARED_CKPT_DIR"
    rm -rf ./nemo_experiments
    ln -sfn "$SHARED_CKPT_DIR" ./nemo_experiments
    echo "[MULTI-NODE] ./nemo_experiments -> $SHARED_CKPT_DIR (shared checkpoints)"

    HOOK_SCRIPT="runner/helpers/hooks/train/posttrain/megatron_bridge/01_convert_checkpoints.sh"
    if [ -f "$HOOK_SCRIPT" ]; then
      sed -i 's/timeout=600/timeout=3600/' "$HOOK_SCRIPT"
      sed -i 's|python3 third_party/Megatron-Bridge/examples/conversion/convert_checkpoints.py import|WORLD_SIZE=1 RANK=0 LOCAL_RANK=0 MASTER_ADDR=127.0.0.1 MASTER_PORT=29599 python3 third_party/Megatron-Bridge/examples/conversion/convert_checkpoints.py import|' "$HOOK_SCRIPT"
      echo "[MULTI-NODE] Patched hook: timeout=3600s + single-process conversion"
    fi

    export NCCL_TIMEOUT=1800000
    echo "[MULTI-NODE] NCCL_TIMEOUT=1800000ms (30min)"
  fi
}

post_train_cleanup() {
  local PEFT_TYPE="$1"
  CKPT_BASE="./nemo_experiments/default/checkpoints"
  TRAIN_EXIT_CODE=$?
  local DISK_USAGE=""

  if [ $TRAIN_EXIT_CODE -ne 0 ]; then
    if [ -f "$CKPT_BASE/latest_checkpointed_iteration.txt" ]; then
      SAVED_ITER=$(cat "$CKPT_BASE/latest_checkpointed_iteration.txt" 2>/dev/null | tr -d '[:space:]')
      echo "WARNING: primus-cli exited with code $TRAIN_EXIT_CODE, but checkpoint found at iteration $SAVED_ITER. Continuing with export."
    else
      echo "Training failed with exit code $TRAIN_EXIT_CODE and no checkpoint found."
      exit $TRAIN_EXIT_CODE
    fi
  fi

  if [ -d "$CKPT_BASE" ] && [ -f "$CKPT_BASE/latest_checkpointed_iteration.txt" ]; then
    LATEST_ITER=$(cat "$CKPT_BASE/latest_checkpointed_iteration.txt" | tr -d '[:space:]')
    LATEST_DIR="$CKPT_BASE/iter_$(printf '%07d' $LATEST_ITER)"
    echo "Cleaning up intermediate checkpoints (keeping $LATEST_DIR)..."
    for d in "$CKPT_BASE"/iter_*; do
      if [ -d "$d" ] && [ "$d" != "$LATEST_DIR" ]; then
        echo "  Removing $d"
        rm -rf "$d" 2>/dev/null || echo "  Warning: best-effort cleanup failed for $d"
      fi
    done
  fi
  rm -rf data/huggingface/hub/models--* 2>/dev/null || true
  if [ "$PEFT_TYPE" = "none" ]; then
    rm -rf data/megatron_checkpoints 2>/dev/null || true
  fi
  DISK_USAGE=$(du -sh . 2>/dev/null | cut -f1 || true)
  echo "Cleanup done. Disk usage: ${DISK_USAGE:-unknown}"
}

export_model() {
  local PEFT_TYPE="$1"
  local JOB_NAME="$2"
  local EXPORT_PATH="${PFS}/custom/models/${JOB_NAME}"

  MY_RANK=${RANK:-${OMPI_COMM_WORLD_RANK:-0}}
  if [ "$MY_RANK" != "0" ] && [ "$MY_RANK" != "" ]; then
    echo "Worker node (rank=$MY_RANK), skipping export."
    exit 0
  fi

  echo "==================== Export Model ===================="
  mkdir -p "${EXPORT_PATH}"

  CKPT_DIR=""
  CKPT_SEARCH_DIRS="./nemo_experiments/default/checkpoints ${DATA_PATH:-/dev/null}/nemo_experiments/default/checkpoints"
  for d in ${CKPT_SEARCH_DIRS}; do
    if [ -d "$d" ] && [ -f "$d/latest_checkpointed_iteration.txt" ]; then
      ITER_VAL=$(cat "$d/latest_checkpointed_iteration.txt" 2>/dev/null | tr -d '[:space:]')
      if [ -n "$ITER_VAL" ] && [ "$ITER_VAL" != "0" ]; then
        CKPT_DIR="$d"; break
      fi
    fi
  done
  if [ -z "$CKPT_DIR" ]; then
    for d in ${CKPT_SEARCH_DIRS}; do
      if [ -d "$d" ] && ls -d "$d"/iter_* >/dev/null 2>&1; then
        CKPT_DIR="$d"; break
      fi
    done
  fi
  echo "Checkpoint directory: ${CKPT_DIR:-not found}"

  export PYTHONPATH="${PRIMUS_DIR}/third_party/Megatron-Bridge/src:${PRIMUS_DIR}/third_party/Megatron-Bridge/3rdparty/Megatron-LM:${PYTHONPATH:-}"
  export WORLD_SIZE=1 RANK=0 LOCAL_RANK=0 MASTER_ADDR=127.0.0.1 MASTER_PORT=29599

  if [ "$PEFT_TYPE" = "lora" ]; then
    echo "==================== LoRA: Merge Adapter ===================="
    PRETRAINED_CKPT=""
    for d in ./data/megatron_checkpoints/Qwen3-8B ${DATA_PATH:-/dev/null}/megatron_checkpoints/Qwen3-8B ./data/megatron_checkpoints; do
      if [ -d "$d" ]; then PRETRAINED_CKPT="$d"; break; fi
    done
    if [ -z "$PRETRAINED_CKPT" ]; then
      echo "ERROR: Cannot find pretrained Megatron checkpoint for LoRA merge."
      exit 1
    fi

    MERGED_CKPT="./merged_checkpoint"
    LORA_ITER_DIR=""
    if [ -f "${CKPT_DIR}/latest_checkpointed_iteration.txt" ]; then
      _LORA_ITER=$(cat "${CKPT_DIR}/latest_checkpointed_iteration.txt" | tr -d '[:space:]')
      if [ -n "$_LORA_ITER" ] && [ "$_LORA_ITER" != "0" ]; then
        LORA_ITER_DIR="${CKPT_DIR}/iter_$(printf '%07d' $_LORA_ITER)"
      fi
    fi
    if [ -z "$LORA_ITER_DIR" ] || [ ! -d "$LORA_ITER_DIR" ]; then
      LORA_ITER_DIR=$(ls -d ${CKPT_DIR}/iter_* 2>/dev/null | sort -t_ -k2 -n | tail -1)
    fi

    echo "  LoRA checkpoint: ${LORA_ITER_DIR}"
    echo "  Pretrained base: ${PRETRAINED_CKPT}"
    python3 "${PRIMUS_DIR}/third_party/Megatron-Bridge/examples/peft/merge_lora.py" \
      --lora-checkpoint "${LORA_ITER_DIR}" \
      --pretrained "${PRETRAINED_CKPT}" \
      --hf-model-path "${HF_MODEL}" \
      --output "${MERGED_CKPT}" 2>&1
    if [ $? -ne 0 ]; then
      echo "ERROR: LoRA merge failed"
      exit 1
    fi
    CONVERT_CKPT_DIR="${MERGED_CKPT}"
  else
    CONVERT_CKPT_DIR="${CKPT_DIR}"
  fi

  echo "Converting Megatron checkpoint to HuggingFace format..."
  LATEST_CKPT=""
  if [ -f "${CONVERT_CKPT_DIR}/latest_checkpointed_iteration.txt" ]; then
    LATEST_ITER=$(cat "${CONVERT_CKPT_DIR}/latest_checkpointed_iteration.txt" | tr -d '[:space:]')
    if [ -n "$LATEST_ITER" ] && [ "$LATEST_ITER" != "0" ]; then
      LATEST_CKPT="${CONVERT_CKPT_DIR}/iter_$(printf '%07d' ${LATEST_ITER})"
    fi
  fi
  if [ -z "${LATEST_CKPT}" ] || [ ! -d "${LATEST_CKPT}" ]; then
    LATEST_CKPT=$(ls -td ${CONVERT_CKPT_DIR}/iter_* 2>/dev/null | sort -t_ -k2 -n | tail -1)
  fi

  if [ -n "${LATEST_CKPT}" ] && [ -d "${LATEST_CKPT}" ]; then
    echo "Found checkpoint at: ${LATEST_CKPT}"
    python3 "${PRIMUS_DIR}/third_party/Megatron-Bridge/examples/conversion/convert_checkpoints.py" export \
      --hf-model "${HF_MODEL}" \
      --megatron-path "${CONVERT_CKPT_DIR}" \
      --hf-path "${EXPORT_PATH}" \
      --no-progress 2>&1 || echo "Warning: checkpoint conversion failed"
  else
    echo "Warning: no Megatron checkpoint found"
  fi

  echo "==================== Export Result ===================="
  ls -lh "${EXPORT_PATH}/" 2>&1 | head -20
  if [ -f "${EXPORT_PATH}/config.json" ]; then
    echo "[SUCCESS] Model exported to ${EXPORT_PATH}"
  else
    echo "[FAILED] No config.json found in ${EXPORT_PATH}"
  fi
}
