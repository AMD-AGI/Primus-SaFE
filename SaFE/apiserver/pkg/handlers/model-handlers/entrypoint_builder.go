/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"fmt"
	"strings"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// ==================== Model Recipe Mapping ====================
// Extracted from Primus project: primus/configs/models/megatron_bridge/*.yaml

// ModelRecipe maps a HuggingFace model name to its Primus recipe and flavor.
type ModelRecipe struct {
	Recipe string // e.g. "qwen.qwen3"
	Flavor string // e.g. "qwen3_8b_finetune_config"
	Size   string // "8b" | "32b" | "70b" — used to look up training presets
}

var modelRecipes = map[string]ModelRecipe{
	"Qwen/Qwen3-8B":                 {Recipe: "qwen.qwen3", Flavor: "qwen3_8b_finetune_config", Size: "8b"},
	"Qwen/Qwen3-32B":                {Recipe: "qwen.qwen3", Flavor: "qwen3_32b_finetune_config", Size: "32b"},
	"meta-llama/Meta-Llama-3.1-70B": {Recipe: "llama.llama3", Flavor: "llama31_70b_finetune_config", Size: "70b"},
}

// InferModelRecipe returns the Primus recipe for a given HF model name.
// Falls back to fuzzy matching on common substrings.
func InferModelRecipe(hfModelName string) (ModelRecipe, error) {
	if r, ok := modelRecipes[hfModelName]; ok {
		return r, nil
	}
	lower := strings.ToLower(hfModelName)
	for name, r := range modelRecipes {
		if strings.Contains(lower, strings.ToLower(name)) {
			return r, nil
		}
	}
	return ModelRecipe{}, fmt.Errorf("unsupported model: %s (supported: %s)", hfModelName, supportedModelNames())
}

func supportedModelNames() string {
	names := make([]string, 0, len(modelRecipes))
	for k := range modelRecipes {
		names = append(names, k)
	}
	return strings.Join(names, ", ")
}

// ==================== Training Preset Table ====================
// Extracted from Primus project: examples/megatron_bridge/configs/MI355X/*.yaml

// TrainPreset holds default training hyperparameters for a model size + peft combination.
type TrainPreset struct {
	TrainIters      int
	GlobalBatchSize int
	MicroBatchSize  int
	SeqLength       int
	FinetuneLr      float64
	TpSize          int
	LrWarmupIters   int
	SaveInterval    int
}

var trainPresets = map[string]map[string]TrainPreset{
	"8b": {
		"none": {TrainIters: 100, GlobalBatchSize: 8, MicroBatchSize: 1, SeqLength: 2048, FinetuneLr: 5e-6, TpSize: 1, LrWarmupIters: 5, SaveInterval: 50},
		"lora": {TrainIters: 1000, GlobalBatchSize: 128, MicroBatchSize: 1, SeqLength: 2048, FinetuneLr: 1e-4, TpSize: 1, LrWarmupIters: 50, SaveInterval: 500},
	},
	"32b": {
		"none": {TrainIters: 1000, GlobalBatchSize: 8, MicroBatchSize: 1, SeqLength: 2048, FinetuneLr: 5e-6, TpSize: 8, LrWarmupIters: 10, SaveInterval: 500},
		"lora": {TrainIters: 200, GlobalBatchSize: 32, MicroBatchSize: 4, SeqLength: 8192, FinetuneLr: 1e-4, TpSize: 1, LrWarmupIters: 50, SaveInterval: 100},
	},
	"70b": {
		"none": {TrainIters: 200, GlobalBatchSize: 128, MicroBatchSize: 1, SeqLength: 2048, FinetuneLr: 5e-6, TpSize: 8, LrWarmupIters: 50, SaveInterval: 100},
		"lora": {TrainIters: 200, GlobalBatchSize: 128, MicroBatchSize: 1, SeqLength: 2048, FinetuneLr: 1e-4, TpSize: 8, LrWarmupIters: 50, SaveInterval: 100},
	},
}

// ==================== Default Value Population ====================

const (
	DefaultSftImageTag      = "sync/primus:v26.1"
	DefaultSftImageFallback = "docker.io/sync/primus:v26.1"
	DefaultGpuCount         = 8
	DefaultCpu              = "128"
	DefaultMemory           = "1024Gi"
	DefaultEphemeralStorage = "2048Gi"
	DefaultPrimusPath       = "/tmp/primus"
	PrimusGitRepo           = "https://github.com/AMD-AGI/Primus.git"
	PrimusGitCommit         = "1dd3ebe8" // compatible with pr-609-ainic / pr-624-ainic images
	DefaultPriority         = 1          // medium: HighPriorityInt=2, MedPriorityInt=1, LowPriorityInt=0
)

// GetDefaultSftImage returns the default SFT training image using the cluster's harbor registry.
// It extracts the registry hostname from the ops_job download_image config (which is already
// populated by Helm with the correct per-cluster harbor address).
// e.g. "harbor.project1.tw325.primus-safe.amd.com/proxy/primussafe/s3-downloader:latest"
//
//	-> registry host = "harbor.project1.tw325.primus-safe.amd.com"
//	-> result = "harbor.project1.tw325.primus-safe.amd.com/sync/primus:v26.1"
func GetDefaultSftImage() string {
	downloadImage := commonconfig.GetDownloadJoImage()
	if idx := strings.Index(downloadImage, "/"); idx > 0 {
		registryHost := downloadImage[:idx]
		return fmt.Sprintf("%s/%s", registryHost, DefaultSftImageTag)
	}
	return DefaultSftImageFallback
}

// FillSftDefaults populates zero-valued fields with smart defaults based on model size and peft type.
func FillSftDefaults(req *CreateSftJobRequest, modelSize string) {
	if req.Priority == 0 {
		req.Priority = DefaultPriority
	}
	if req.ExportModel == nil {
		t := true
		req.ExportModel = &t
	}
	tc := &req.TrainConfig
	if tc.Peft == "" {
		tc.Peft = "none"
	}
	if tc.DatasetFormat == "" {
		tc.DatasetFormat = "alpaca"
	}

	preset, ok := trainPresets[modelSize][tc.Peft]
	if !ok {
		preset = trainPresets["8b"]["none"]
	}

	if tc.TrainIters == 0 {
		tc.TrainIters = preset.TrainIters
	}
	if tc.GlobalBatchSize == 0 {
		tc.GlobalBatchSize = preset.GlobalBatchSize
	}
	if tc.MicroBatchSize == 0 {
		tc.MicroBatchSize = preset.MicroBatchSize
	}
	if tc.SeqLength == 0 {
		tc.SeqLength = preset.SeqLength
	}
	if tc.FinetuneLr == 0 {
		tc.FinetuneLr = preset.FinetuneLr
	}
	if tc.TensorModelParallelSize == 0 {
		tc.TensorModelParallelSize = preset.TpSize
	}
	if tc.PipelineModelParallelSize == 0 {
		tc.PipelineModelParallelSize = 1
	}
	if tc.ContextParallelSize == 0 {
		tc.ContextParallelSize = 1
	}
	if tc.LrWarmupIters == 0 {
		tc.LrWarmupIters = preset.LrWarmupIters
	}
	if tc.EvalInterval == 0 {
		tc.EvalInterval = 30
	}
	if tc.SaveInterval == 0 {
		tc.SaveInterval = preset.SaveInterval
		if tc.SaveInterval < 1 {
			tc.SaveInterval = 1
		}
	}
	if tc.PrecisionConfig == "" {
		tc.PrecisionConfig = "bf16_mixed"
	}

	// Ensure save_interval allows at least one checkpoint before the last iteration.
	// Primus patches skip saving at the final iteration, so save_interval must be
	// strictly less than train_iters to guarantee a saved checkpoint for export.
	if tc.SaveInterval >= tc.TrainIters {
		tc.SaveInterval = tc.TrainIters / 2
		if tc.SaveInterval < 1 {
			tc.SaveInterval = 1
		}
	}

	if tc.Peft == "lora" {
		if tc.PeftDim == 0 {
			tc.PeftDim = 16
		}
		if tc.PeftAlpha == 0 {
			tc.PeftAlpha = 32
		}
	}

	if req.Image == "" {
		req.Image = GetDefaultSftImage()
	}
	if req.NodeCount == 0 {
		req.NodeCount = 1
	}
	if req.GpuCount == 0 {
		req.GpuCount = DefaultGpuCount
	}
	if req.Cpu == "" {
		req.Cpu = DefaultCpu
	}
	if req.Memory == "" {
		req.Memory = DefaultMemory
	}
	if req.EphemeralStorage == "" {
		req.EphemeralStorage = DefaultEphemeralStorage
	}
}

// ==================== Entrypoint Builder ====================

// EntrypointConfig holds all parameters needed to generate a Primus CLI entrypoint script.
type EntrypointConfig struct {
	PrimusPath    string
	Recipe        string
	Flavor        string
	HfPath        string // HF model name or local path
	DatasetPath   string
	DatasetFormat string // "alpaca" | "squad"
	ExpName       string
	ModelSize     string // "8b" | "32b" | "70b"
	TrainConfig   SftTrainConfig

	ExportModel bool
	Workspace   string
	ModelId     string
	BaseModel   string
	SftJobId    string
}

// BuildEntrypoint generates the shell script that writes Primus YAML configs and invokes primus-cli.
func BuildEntrypoint(cfg EntrypointConfig) string {
	preparedDatasetDir := "/tmp/sft_dataset"
	cfgForYaml := cfg
	cfgForYaml.DatasetPath = preparedDatasetDir
	modelYaml := buildModelYaml(cfgForYaml)
	expYaml := buildExperimentYaml(cfgForYaml)

	script := fmt.Sprintf(`# ==================== Prepare Dataset ====================
SRC_DATASET="%s"
DATASET_DIR="%s"
echo "Preparing dataset from ${SRC_DATASET} -> ${DATASET_DIR} ..."
rm -rf "${DATASET_DIR}"
mkdir -p "${DATASET_DIR}"

python3 -c "
import json, os, sys, glob
src = '${SRC_DATASET}'
dst = '${DATASET_DIR}'
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

# ==================== Find/Clone Primus ====================
PRIMUS_DIR=""
SFT_CONFIG="primus/configs/modules/megatron_bridge/sft_trainer.yaml"
for p in /workspace/Primus %s; do
  if [ -d "$p/runner" ] && [ -f "$p/$SFT_CONFIG" ]; then PRIMUS_DIR="$p"; break; fi
done
if [ -z "$PRIMUS_DIR" ]; then
  echo "Compatible Primus not found (missing $SFT_CONFIG), cloning compatible version (%s)..."
  rm -rf %s
  git clone %s %s
  cd %s && git checkout %s && git submodule update --init --recursive && cd -
  PRIMUS_DIR="%s"
fi
echo "Using Primus at: $PRIMUS_DIR"
cd "$PRIMUS_DIR"
mkdir -p primus/configs/models/megatron_bridge
cat > primus/configs/models/megatron_bridge/sft_custom_model.yaml << 'MODELEOF'
%s
MODELEOF
cat > /tmp/sft_experiment.yaml << 'EXPEOF'
%s
EXPEOF
./runner/primus-cli direct -- train posttrain --config /tmp/sft_experiment.yaml
TRAIN_EXIT_CODE=$?

# Check if training actually produced checkpoints (torchrun may return non-zero
# during distributed cleanup even when training completed successfully)
CKPT_BASE="./nemo_experiments/default/checkpoints"
if [ $TRAIN_EXIT_CODE -ne 0 ]; then
  if [ -f "$CKPT_BASE/latest_checkpointed_iteration.txt" ]; then
    SAVED_ITER=$(cat "$CKPT_BASE/latest_checkpointed_iteration.txt" 2>/dev/null | tr -d '[:space:]')
    echo "WARNING: primus-cli exited with code $TRAIN_EXIT_CODE, but checkpoint found at iteration $SAVED_ITER. Continuing with export."
  else
    echo "Training failed with exit code $TRAIN_EXIT_CODE and no checkpoint found. Skipping model export."
    exit $TRAIN_EXIT_CODE
  fi
fi

# Cleanup: remove intermediate checkpoints and HF cache to free ephemeral storage
# Keep only the latest checkpoint (needed for export), delete everything else
CKPT_BASE="./nemo_experiments/default/checkpoints"
if [ -d "$CKPT_BASE" ] && [ -f "$CKPT_BASE/latest_checkpointed_iteration.txt" ]; then
  LATEST_ITER=$(cat "$CKPT_BASE/latest_checkpointed_iteration.txt" | tr -d '[:space:]')
  LATEST_DIR="$CKPT_BASE/iter_$(printf '%%07d' $LATEST_ITER)"
  echo "Cleaning up intermediate checkpoints (keeping $LATEST_DIR)..."
  for d in "$CKPT_BASE"/iter_*; do
    if [ -d "$d" ] && [ "$d" != "$LATEST_DIR" ]; then
      echo "  Removing $d"
      rm -rf "$d"
    fi
  done
fi
# Remove HF model cache (already converted to Megatron format)
rm -rf data/huggingface/hub/models--* 2>/dev/null
# For LoRA, keep pretrained Megatron checkpoint (needed for merge); for full SFT, remove it
if [ "%s" = "none" ]; then
  rm -rf data/megatron_checkpoints 2>/dev/null
fi
echo "Cleanup done. Disk usage: $(du -sh . 2>/dev/null | cut -f1)"`,
		cfg.DatasetPath, preparedDatasetDir,
		cfg.PrimusPath, PrimusGitCommit, cfg.PrimusPath, PrimusGitRepo, cfg.PrimusPath, cfg.PrimusPath, PrimusGitCommit, cfg.PrimusPath, modelYaml, expYaml,
		cfg.TrainConfig.Peft)

	if cfg.ExportModel {
		script += buildExportScript(cfg)
	}

	return script
}

// buildExportScript generates shell commands to convert the trained checkpoint
// to HuggingFace format, copy it to PFS, and register it as a new Model.
// For LoRA training, an extra merge step is inserted before the HF conversion.
func buildExportScript(cfg EntrypointConfig) string {
	exportPath := fmt.Sprintf("/wekafs/custom/models/%s", cfg.SftJobId)
	displayName := fmt.Sprintf("%s-finetuned", strings.ToLower(cfg.ExpName))
	isLoRA := cfg.TrainConfig.Peft == "lora"

	var sb strings.Builder

	// --- Locate trained checkpoint ---
	sb.WriteString(`

# ==================== Convert Megatron Checkpoint to HuggingFace ====================
`)
	fmt.Fprintf(&sb, "EXPORT_PATH=%q\n", exportPath)
	sb.WriteString(`CKPT_DIR=""
CKPT_SEARCH_DIRS="./nemo_experiments/default/checkpoints ./output/checkpoints ${PRIMUS_DIR}/nemo_experiments/default/checkpoints /tmp/primus/nemo_experiments/default/checkpoints"

# First pass: prefer dirs with latest_checkpointed_iteration.txt (real trained checkpoints)
for d in ${CKPT_SEARCH_DIRS}; do
  if [ -d "$d" ] && [ -f "$d/latest_checkpointed_iteration.txt" ]; then
    ITER_VAL=$(cat "$d/latest_checkpointed_iteration.txt" 2>/dev/null | tr -d '[:space:]')
    if [ -n "$ITER_VAL" ] && [ "$ITER_VAL" != "0" ]; then
      CKPT_DIR="$d"; break
    fi
  fi
done

# Second pass: dirs with iter_* subdirectories (checkpoint saved but maybe no latest file)
if [ -z "$CKPT_DIR" ]; then
  for d in ${CKPT_SEARCH_DIRS}; do
    if [ -d "$d" ] && ls -d "$d"/iter_* >/dev/null 2>&1; then
      HIGHEST_ITER=$(ls -d "$d"/iter_* 2>/dev/null | sort -t_ -k2 -n | tail -1)
      if [ -n "$HIGHEST_ITER" ] && [ "$(basename "$HIGHEST_ITER")" != "iter_0000000" ]; then
        CKPT_DIR="$d"; break
      fi
    fi
  done
fi

# Third pass: any existing checkpoint dir (including pretrained iter_0000000)
if [ -z "$CKPT_DIR" ]; then
  for d in ${CKPT_SEARCH_DIRS}; do
    if [ -d "$d" ]; then CKPT_DIR="$d"; break; fi
  done
fi

echo "Checkpoint directory: ${CKPT_DIR:-not found}"
HF_EXPORT_PATH="${EXPORT_PATH}"
mkdir -p "${HF_EXPORT_PATH}"

export PYTHONPATH="${PRIMUS_DIR}/third_party/Megatron-Bridge/src:${PRIMUS_DIR}/third_party/Megatron-Bridge/3rdparty/Megatron-LM:${PYTHONPATH:-}"
`)

	// --- LoRA: merge adapter into base model before export ---
	if isLoRA {
		hfModelBasename := cfg.HfPath
		if idx := strings.LastIndex(hfModelBasename, "/"); idx >= 0 {
			hfModelBasename = hfModelBasename[idx+1:]
		}
		fmt.Fprintf(&sb, `
# ==================== LoRA: Merge Adapter into Base Model ====================
PRETRAINED_CKPT=""
PRETRAINED_SEARCH="./data/megatron_checkpoints/%s ./data/megatron_checkpoints"
for d in ${PRETRAINED_SEARCH}; do
  if [ -d "$d" ]; then PRETRAINED_CKPT="$d"; break; fi
done

if [ -z "$PRETRAINED_CKPT" ]; then
  echo "ERROR: Cannot find pretrained Megatron checkpoint for LoRA merge."
  echo "Searched: ${PRETRAINED_SEARCH}"
  exit 1
fi

MERGED_CKPT="./merged_checkpoint"

# Resolve the actual iter_* subdirectory (merge_lora.py needs the distributed checkpoint dir, not the parent)
LORA_ITER_DIR=""
if [ -f "${CKPT_DIR}/latest_checkpointed_iteration.txt" ]; then
  _LORA_ITER=$(cat "${CKPT_DIR}/latest_checkpointed_iteration.txt" | tr -d '[:space:]')
  if [ -n "$_LORA_ITER" ] && [ "$_LORA_ITER" != "0" ]; then
    LORA_ITER_DIR="${CKPT_DIR}/iter_$(printf '%%07d' $_LORA_ITER)"
  fi
fi
if [ -z "$LORA_ITER_DIR" ] || [ ! -d "$LORA_ITER_DIR" ]; then
  LORA_ITER_DIR=$(ls -d ${CKPT_DIR}/iter_* 2>/dev/null | sort -t_ -k2 -n | tail -1)
fi
if [ -z "$LORA_ITER_DIR" ] || [ ! -d "$LORA_ITER_DIR" ]; then
  echo "ERROR: No iter_* checkpoint found in ${CKPT_DIR} for LoRA merge."
  exit 1
fi

echo "Merging LoRA adapter into base model..."
echo "  LoRA checkpoint: ${LORA_ITER_DIR}"
echo "  Pretrained base: ${PRETRAINED_CKPT}"
echo "  Output:          ${MERGED_CKPT}"

python3 "${PRIMUS_DIR}/third_party/Megatron-Bridge/examples/peft/merge_lora.py" \
  --lora-checkpoint "${LORA_ITER_DIR}" \
  --pretrained "${PRETRAINED_CKPT}" \
  --hf-model-path "%s" \
  --output "${MERGED_CKPT}" 2>&1
MERGE_EXIT=$?

if [ $MERGE_EXIT -ne 0 ]; then
  echo "ERROR: LoRA merge failed with exit code $MERGE_EXIT"
  echo "Falling back to direct export (may fail for LoRA checkpoints)..."
  CONVERT_CKPT_DIR="${CKPT_DIR}"
else
  echo "LoRA merge succeeded. Merged checkpoint at: ${MERGED_CKPT}"
  CONVERT_CKPT_DIR="${MERGED_CKPT}"
  rm -rf "${CKPT_DIR}" 2>/dev/null
  rm -rf "${PRETRAINED_CKPT}" 2>/dev/null
fi
echo "Disk usage after merge: $(du -sh . 2>/dev/null | cut -f1)"
`, hfModelBasename, cfg.HfPath)
	} else {
		sb.WriteString(`CONVERT_CKPT_DIR="${CKPT_DIR}"
`)
	}

	// --- Convert Megatron → HuggingFace ---
	fmt.Fprintf(&sb, `
echo "Converting Megatron checkpoint to HuggingFace format..."

LATEST_CKPT=""
if [ -f "${CONVERT_CKPT_DIR}/latest_checkpointed_iteration.txt" ]; then
  LATEST_ITER=$(cat "${CONVERT_CKPT_DIR}/latest_checkpointed_iteration.txt" | tr -d '[:space:]')
  if [ -n "$LATEST_ITER" ] && [ "$LATEST_ITER" != "0" ]; then
    LATEST_CKPT="${CONVERT_CKPT_DIR}/iter_$(printf '%%07d' ${LATEST_ITER})"
  fi
fi
if [ -z "${LATEST_CKPT}" ] || [ ! -d "${LATEST_CKPT}" ]; then
  LATEST_CKPT=$(ls -td ${CONVERT_CKPT_DIR}/iter_* 2>/dev/null | sort -t_ -k2 -n | tail -1)
fi

if [ -n "${LATEST_CKPT}" ] && [ -d "${LATEST_CKPT}" ]; then
  echo "Found checkpoint at: ${LATEST_CKPT}"
  python3 "${PRIMUS_DIR}/third_party/Megatron-Bridge/examples/conversion/convert_checkpoints.py" export \
    --hf-model "%s" \
    --megatron-path "${CONVERT_CKPT_DIR}" \
    --hf-path "${HF_EXPORT_PATH}" \
    --no-progress 2>&1 || echo "Warning: checkpoint conversion failed, falling back to raw copy"

  if [ ! -f "${HF_EXPORT_PATH}/config.json" ]; then
    echo "Warning: conversion did not produce config.json, copying raw output as fallback"
    cp -r ./output/* "${HF_EXPORT_PATH}/" 2>/dev/null || true
  fi
else
  echo "Warning: no Megatron checkpoint found, copying raw output"
  cp -r ./output/* "${HF_EXPORT_PATH}/" 2>/dev/null || true
fi
rm -rf "${CONVERT_CKPT_DIR}" 2>/dev/null
echo "Model exported to ${HF_EXPORT_PATH}"
`, cfg.HfPath)

	// --- Register model ---
	fmt.Fprintf(&sb, `
# ==================== Register Model ====================
APISERVER="http://primus-safe-apiserver.primus-safe.svc:8088"
echo "Registering model in Model Square..."
curl -s -X POST "${APISERVER}/api/v1/playground/models" \
  -H "Content-Type: application/json" \
  -H "userId: ${SFT_USER_ID:-system}" \
  -H "userName: ${SFT_USER_NAME:-system}" \
  -d '{
    "displayName": "%s",
    "description": "Fine-tuned from %s via SFT (job: %s)",
    "source": {
      "accessMode": "local_path",
      "localPath": "%s",
      "modelName": "%s"
    },
    "workspace": "%s",
    "origin": "fine_tuned",
    "sftJobId": "%s",
    "baseModel": "%s"
  }' || echo "Warning: failed to register model, but training output is saved at ${EXPORT_PATH}"
echo "Model export complete."`,
		displayName,
		cfg.BaseModel, cfg.SftJobId,
		exportPath,
		displayName,
		cfg.Workspace,
		cfg.SftJobId,
		cfg.BaseModel,
	)

	return sb.String()
}

func buildModelYaml(cfg EntrypointConfig) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "recipe: %s\n", cfg.Recipe)
	fmt.Fprintf(&sb, "flavor: %s\n", cfg.Flavor)
	fmt.Fprintf(&sb, "hf_path: %s\n", cfg.HfPath)
	if cfg.DatasetFormat == "alpaca" {
		sb.WriteString("dataset_format: alpaca\n")
	}
	fmt.Fprintf(&sb, "dataset:\n")
	fmt.Fprintf(&sb, "    dataset_name: \"%s\"\n", cfg.DatasetPath)
	return strings.TrimRight(sb.String(), "\n")
}

func buildExperimentYaml(cfg EntrypointConfig) string {
	tc := cfg.TrainConfig
	var sb strings.Builder

	sb.WriteString("work_group: ${PRIMUS_TEAM:amd}\n")
	sb.WriteString("user_name: ${PRIMUS_USER:root}\n")
	fmt.Fprintf(&sb, "exp_name: %s\n", cfg.ExpName)
	sb.WriteString("workspace: ./output\n")
	sb.WriteString("modules:\n")
	sb.WriteString("  post_trainer:\n")
	sb.WriteString("    framework: megatron_bridge\n")
	sb.WriteString("    config: sft_trainer.yaml\n")
	sb.WriteString("    model: sft_custom_model.yaml\n")
	sb.WriteString("    overrides:\n")
	sb.WriteString("      stderr_sink_level: DEBUG\n")

	// Parallelism
	fmt.Fprintf(&sb, "      tensor_model_parallel_size: %d\n", tc.TensorModelParallelSize)
	fmt.Fprintf(&sb, "      pipeline_model_parallel_size: %d\n", tc.PipelineModelParallelSize)
	fmt.Fprintf(&sb, "      context_parallel_size: %d\n", tc.ContextParallelSize)
	fmt.Fprintf(&sb, "      sequence_parallel: %v\n", tc.SequenceParallel)

	// 32B and 70B need extra parallelism fields
	if cfg.ModelSize == "32b" || cfg.ModelSize == "70b" {
		sb.WriteString("      pipeline_dtype: null\n")
		sb.WriteString("      virtual_pipeline_model_parallel_size: null\n")
		sb.WriteString("      use_megatron_fsdp: false\n")
	}

	// PEFT
	fmt.Fprintf(&sb, "      peft: \"%s\"\n", tc.Peft)
	if tc.Peft == "lora" || cfg.ModelSize == "32b" || cfg.ModelSize == "70b" {
		fmt.Fprintf(&sb, "      packed_sequence: %v\n", tc.PackedSequence)
	}
	if tc.Peft == "lora" && cfg.ModelSize == "70b" {
		fmt.Fprintf(&sb, "      peft_dim: %d\n", tc.PeftDim)
		fmt.Fprintf(&sb, "      peft_alpha: %d\n", tc.PeftAlpha)
	}

	// Training
	fmt.Fprintf(&sb, "      train_iters: %d\n", tc.TrainIters)
	fmt.Fprintf(&sb, "      global_batch_size: %d\n", tc.GlobalBatchSize)
	fmt.Fprintf(&sb, "      micro_batch_size: %d\n", tc.MicroBatchSize)
	fmt.Fprintf(&sb, "      seq_length: %d\n", tc.SeqLength)
	fmt.Fprintf(&sb, "      eval_interval: %d\n", tc.EvalInterval)
	fmt.Fprintf(&sb, "      save_interval: %d\n", tc.SaveInterval)

	// Optimizer
	fmt.Fprintf(&sb, "      finetune_lr: %.10f\n", tc.FinetuneLr)
	fmt.Fprintf(&sb, "      min_lr: %g\n", tc.MinLr)
	fmt.Fprintf(&sb, "      lr_warmup_iters: %d\n", tc.LrWarmupIters)

	// 32B/70B extra optimizer fields
	if cfg.ModelSize == "32b" || cfg.ModelSize == "70b" {
		sb.WriteString("      lr_decay_iters: null\n")
	}

	// 70B LoRA specific
	if tc.Peft == "lora" && cfg.ModelSize == "70b" {
		sb.WriteString("      use_distributed_optimizer: false\n")
		sb.WriteString("      cross_entropy_loss_fusion: false\n")
	}

	// W&B (disabled by default)
	if cfg.ModelSize == "32b" || cfg.ModelSize == "70b" {
		sb.WriteString("      wandb_project: null\n")
		sb.WriteString("      wandb_entity: null\n")
		sb.WriteString("      wandb_exp_name: null\n")
	}

	// Precision
	fmt.Fprintf(&sb, "      precision_config: %s\n", tc.PrecisionConfig)
	if cfg.ModelSize == "32b" || cfg.ModelSize == "70b" {
		sb.WriteString("      comm_overlap_config: null\n")
	}

	// 32B recompute configuration
	if cfg.ModelSize == "32b" {
		sb.WriteString("      recompute_granularity: full\n")
		sb.WriteString("      recompute_method: uniform\n")
		sb.WriteString("      recompute_num_layers: 1\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}
