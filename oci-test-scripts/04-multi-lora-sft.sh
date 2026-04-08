#!/bin/bash
# OCI Test: 多机 LoRA SFT (8B, 8 GPU × 2 nodes)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
JOB_NAME="oci-test-multi-lora-$(date +%s)"
echo "========== OCI Test: Multi-Node LoRA SFT =========="
echo "Job: $JOB_NAME"

source "${SCRIPT_DIR}/common.sh"

prepare_dataset "$DATASET_SRC"
find_primus

mkdir -p primus/configs/models/megatron_bridge
cat > primus/configs/models/megatron_bridge/sft_custom_model.yaml << 'MODELEOF'
recipe: qwen.qwen3
flavor: qwen3_8b_finetune_config
hf_path: Qwen/Qwen3-8B
dataset_format: alpaca
dataset:
    dataset_name: "/tmp/sft_dataset"
MODELEOF

sed "s/%MODULE_CONFIG%/$MODULE_CONFIG/g" > /tmp/sft_experiment.yaml << 'EXPEOF'
work_group: ${PRIMUS_TEAM:amd}
user_name: ${PRIMUS_USER:root}
exp_name: oci-test-multi-lora
workspace: ./output
modules:
  post_trainer:
    framework: megatron_bridge
    config: %MODULE_CONFIG%
    model: sft_custom_model.yaml
    overrides:
      stderr_sink_level: DEBUG
      tensor_model_parallel_size: 1
      pipeline_model_parallel_size: 1
      context_parallel_size: 1
      sequence_parallel: false
      peft: "lora"
      pretrained_checkpoint: ./data/megatron_checkpoints/Qwen3-8B
      train_iters: 20
      global_batch_size: 128
      micro_batch_size: 1
      seq_length: 2048
      eval_interval: 30
      save_interval: 10
      finetune_lr: 0.0001000000
      min_lr: 0
      lr_warmup_iters: 5
      precision_config: bf16_mixed
EXPEOF

mkdir -p "./output/${PRIMUS_TEAM:-amd}/${PRIMUS_USER:-root}/oci-test-multi-lora"

prepare_squad_cache
patch_torch_inductor_duplicate_assert
setup_multinode

./runner/primus-cli direct -- train posttrain --config /tmp/sft_experiment.yaml
post_train_cleanup "lora"
export_model "lora" "$JOB_NAME"
