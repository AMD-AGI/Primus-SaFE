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

type ModelRecipeOverride struct {
	Recipe string
	Flavor string
	Size   string
}

var modelRecipes = map[string]ModelRecipe{
	"Qwen/Qwen3-8B":                 {Recipe: "qwen.qwen3", Flavor: "qwen3_8b_finetune_config", Size: "8b"},
	"Qwen/Qwen3-32B":                {Recipe: "qwen.qwen3", Flavor: "qwen3_32b_finetune_config", Size: "32b"},
	"meta-llama/Meta-Llama-3.1-70B": {Recipe: "llama.llama3", Flavor: "llama31_70b_finetune_config", Size: "70b"},
}

// InferModelRecipe returns the Primus recipe for a given HF model name.
// Falls back to family/size inference and finally a size-based default.
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
	return inferModelRecipeFromName(hfModelName)
}

func ResolveModelRecipe(hfModelName string, override ModelRecipeOverride) (ModelRecipe, error) {
	overrideProvided := strings.TrimSpace(override.Recipe) != "" ||
		strings.TrimSpace(override.Flavor) != "" ||
		strings.TrimSpace(override.Size) != ""
	if overrideProvided {
		return normalizeModelRecipeOverride(override)
	}
	return InferModelRecipe(hfModelName)
}

func normalizeModelRecipeOverride(override ModelRecipeOverride) (ModelRecipe, error) {
	recipe := strings.TrimSpace(override.Recipe)
	flavor := strings.TrimSpace(override.Flavor)
	size := strings.TrimSpace(override.Size)

	if recipe == "" || flavor == "" || size == "" {
		return ModelRecipe{}, fmt.Errorf(
			"recipe, flavor and modelSize must be provided together for SFT override",
		)
	}

	normalizedSize, err := normalizeModelSize(size)
	if err != nil {
		return ModelRecipe{}, err
	}

	return ModelRecipe{
		Recipe: recipe,
		Flavor: flavor,
		Size:   normalizedSize,
	}, nil
}

func inferModelRecipeFromName(hfModelName string) (ModelRecipe, error) {
	lower := strings.ToLower(strings.TrimSpace(hfModelName))
	size := inferModelSize(hfModelName)

	if strings.Contains(lower, "qwen") {
		switch size {
		case "32b":
			return ModelRecipe{
				Recipe: "qwen.qwen3",
				Flavor: "qwen3_32b_finetune_config",
				Size:   "32b",
			}, nil
		case "8b":
			return ModelRecipe{
				Recipe: "qwen.qwen3",
				Flavor: "qwen3_8b_finetune_config",
				Size:   "8b",
			}, nil
		}
	}

	if strings.Contains(lower, "llama") && size == "70b" {
		return ModelRecipe{
			Recipe: "llama.llama3",
			Flavor: "llama31_70b_finetune_config",
			Size:   "70b",
		}, nil
	}

	return defaultModelRecipeForSize(size), nil
}

func defaultModelRecipeForSize(size string) ModelRecipe {
	switch size {
	case "32b":
		return ModelRecipe{
			Recipe: "qwen.qwen3",
			Flavor: "qwen3_32b_finetune_config",
			Size:   "32b",
		}
	case "70b":
		return ModelRecipe{
			Recipe: "llama.llama3",
			Flavor: "llama31_70b_finetune_config",
			Size:   "70b",
		}
	default:
		return ModelRecipe{
			Recipe: "qwen.qwen3",
			Flavor: "qwen3_8b_finetune_config",
			Size:   "8b",
		}
	}
}

func normalizeModelSize(size string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(size)) {
	case "7b", "8b":
		return "8b", nil
	case "30b", "32b", "34b":
		return "32b", nil
	case "65b", "70b", "72b":
		return "70b", nil
	default:
		return "", fmt.Errorf("unsupported modelSize override: %s", size)
	}
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
	DefaultSharedMemory     = "500Gi"
	DefaultRdmaResource     = "1k"
	DefaultEphemeralStorage = "2048Gi"
	DefaultPrimusPath       = "/tmp/primus"
	DefaultPriority         = 1 // medium: HighPriorityInt=2, MedPriorityInt=1, LowPriorityInt=0
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
	if req.SharedMemory == "" && req.NodeCount > 1 {
		req.SharedMemory = DefaultSharedMemory
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
	PfsBasePath string // e.g. "/wekafs" or "/shared_nfs", resolved from workspace
}

// BuildEntrypoint generates the shell script that writes Primus YAML configs and invokes primus-cli.
func BuildEntrypoint(cfg EntrypointConfig) string {
	if cfg.PfsBasePath == "" {
		cfg.PfsBasePath = "/wekafs"
	}
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
pq_files = []
if not src_file:
    for d in [src, os.path.join(src, 'data')]:
        pq_files = sorted(glob.glob(os.path.join(d, '*.parquet')))
        if pq_files: break
if pq_files:
    print(f'Loading parquet: {pq_files}')
    import pandas as pd
    frames = [pd.read_parquet(f) for f in pq_files]
    df = pd.concat(frames, ignore_index=True)
    data = [dict(r) for _, r in df.iterrows()]
elif src_file:
    data = []
else:
    print('ERROR: no usable data files in ' + src); sys.exit(1)
if src_file:
    print(f'Using source file: {src_file}')
    data = []
    with open(src_file, encoding='utf-8') as f:
        for line in f:
            line = line.strip()
            if not line: continue
            obj = json.loads(line)
            data.append(obj)
for i, obj in enumerate(data):
    if 'input' in obj and 'output' in obj:
        continue
    elif 'instruction' in obj:
        inst = obj.get('instruction','')
        inp = obj.get('input','')
        out = obj.get('output','')
        new_input = (inst + chr(10) + inp).strip() if inp else inst
        data[i] = {'input': new_input, 'output': out}
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

# ==================== Find Primus ====================
PRIMUS_DIR=""
MODULE_CONFIG=""
NEW_CFG="primus/configs/modules/megatron_bridge/sft_trainer.yaml"
OLD_CFG="primus/configs/modules/megatron_bridge/post_trainer.yaml"
for p in /workspace/Primus %s; do
  if [ -d "$p/runner" ]; then
    if [ -f "$p/$NEW_CFG" ]; then
      PRIMUS_DIR="$p"; MODULE_CONFIG="sft_trainer.yaml"; break
    elif [ -f "$p/$OLD_CFG" ]; then
      PRIMUS_DIR="$p"; MODULE_CONFIG="post_trainer.yaml"; break
    fi
  fi
done
if [ -z "$PRIMUS_DIR" ]; then
  echo "ERROR: No compatible Primus found in /workspace/Primus or %s"
  exit 1
fi
echo "Using Primus at: $PRIMUS_DIR (module config: $MODULE_CONFIG)"
cd "$PRIMUS_DIR"
mkdir -p primus/configs/models/megatron_bridge
cat > primus/configs/models/megatron_bridge/sft_custom_model.yaml << 'MODELEOF'
%s
MODELEOF
sed "s/%%MODULE_CONFIG%%/$MODULE_CONFIG/g" > /tmp/sft_experiment.yaml << 'EXPEOF'
%s
EXPEOF
mkdir -p "./output/${PRIMUS_TEAM:-amd}/${PRIMUS_USER:-root}/%s"

# Prepare squad eval dataset cache from pre-downloaded data on PFS.
# Primus qwen3 flavor calls load_dataset("squad", cache_dir=SQUAD_CACHE) for
# evaluation. We pre-populate the cache by loading from the local PFS copy so
# the later call finds cached arrow files and never hits the network.
SQUAD_SRC="%s/datasets/rajpurkar/squad"
SQUAD_CACHE="/root/.cache/nemo/datasets/squad"
SHARED_SQUAD_CACHE="${SHARED_SQUAD_CACHE_DIR:-}"
if [ -z "$SHARED_SQUAD_CACHE" ] && [ -n "${DATA_PATH:-}" ]; then
  SHARED_SQUAD_CACHE="${DATA_PATH}/squad-cache"
fi
if [ -n "$SHARED_SQUAD_CACHE" ]; then
  mkdir -p "$(dirname "$SQUAD_CACHE")"
  mkdir -p "$SHARED_SQUAD_CACHE"
  if [ -e "$SQUAD_CACHE" ] && [ ! -L "$SQUAD_CACHE" ]; then
    rm -rf "$SQUAD_CACHE"
  fi
  ln -sfn "$SHARED_SQUAD_CACHE" "$SQUAD_CACHE"
  echo "[SFT] shared squad cache: $SQUAD_CACHE -> $SHARED_SQUAD_CACHE"
else
  mkdir -p "$SQUAD_CACHE"
fi
SQUAD_CACHED_COUNT=$(find "$SQUAD_CACHE" -name "*.arrow" 2>/dev/null | wc -l)
if [ -d "$SQUAD_SRC" ] && [ "$SQUAD_CACHED_COUNT" -eq 0 ]; then
  echo "[SFT] Generating squad HF cache from local PFS data..."
  python3 -c "
from datasets import load_dataset
ds = load_dataset('${SQUAD_SRC}', 'plain_text', cache_dir='${SQUAD_CACHE}', trust_remote_code=True)
print(f'[SFT] squad cache ready: {len(ds[\"train\"])} train, {len(ds[\"validation\"])} val')
" 2>&1 || echo "[SFT] WARNING: squad cache generation failed, evaluation may fail"
elif [ "$SQUAD_CACHED_COUNT" -gt 0 ]; then
  echo "[SFT] squad cache already exists ($SQUAD_CACHED_COUNT arrow files)"
else
  echo "[SFT] WARNING: squad data not found at $SQUAD_SRC, evaluation may fail"
fi

# Multi-node: redirect ./data and ./nemo_experiments to shared storage so all
# nodes see the same HF cache, Megatron checkpoints, trained checkpoints, and
# .done signal files. Also patch hooks and increase NCCL timeout.
# Runtime-detect network backend from the actual node, but still honor an
# explicit workload-level AINIC marker injected by the SFT API for OCI multi-node
# jobs so job-manager defaults cannot pull the container back to GID=3.
if [ "${USING_AINIC:-0}" = "1" ] || ls /sys/class/infiniband/ionic_* >/dev/null 2>&1 || ip link show | grep -q 'ionic_'; then
  export USING_AINIC=1
  export NCCL_IB_GID_INDEX="${NCCL_IB_GID_INDEX:-1}"
  export NCCL_DMABUF_ENABLE="${NCCL_DMABUF_ENABLE:-0}"
  export NCCL_MAX_P2P_CHANNELS="${NCCL_MAX_P2P_CHANNELS:-56}"
  export NET_OPTIONAL_RECV_COMPLETION="${NET_OPTIONAL_RECV_COMPLETION:-1}"
  export NCCL_IB_USE_INLINE="${NCCL_IB_USE_INLINE:-1}"
  export RCCL_GDR_FLUSH_GPU_MEM_NO_RELAXED_ORDERING="${RCCL_GDR_FLUSH_GPU_MEM_NO_RELAXED_ORDERING:-0}"
  export NCCL_GDR_FLUSH_DISABLE="${NCCL_GDR_FLUSH_DISABLE:-1}"
  export NCCL_IGNORE_CPU_AFFINITY="${NCCL_IGNORE_CPU_AFFINITY:-1}"
  export LD_LIBRARY_PATH="/opt/amd-anp/build:/opt/rccl/build/release:/opt/rocm/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
  if [ -z "${NCCL_IB_HCA:-}" ]; then
    DETECTED_AINIC_HCA=$(ls -d /sys/class/infiniband/ionic_* 2>/dev/null | sed 's|.*/||' | awk '{printf "%%s:1,",$1}' | sed 's/,$//')
    if [ -n "$DETECTED_AINIC_HCA" ]; then
      export NCCL_IB_HCA="$DETECTED_AINIC_HCA"
    fi
  fi
  AINIC_IFACE=$(ip -o link show | grep -oP 'ens\d+np\d+' | head -1)
  if [ -n "$AINIC_IFACE" ]; then
    export GLOO_SOCKET_IFNAME="${GLOO_SOCKET_IFNAME:-$AINIC_IFACE}"
    export NCCL_SOCKET_IFNAME="${NCCL_SOCKET_IFNAME:-$AINIC_IFACE}"
  fi
  SELECT_ALG="/opt/venv/lib/python3.10/site-packages/torch/_inductor/select_algorithm.py"
  if [ -f "$SELECT_ALG" ] && grep -q "duplicate" "$SELECT_ALG"; then
    sed -i 's/assert.*duplicate.*/pass  # patched/' "$SELECT_ALG"
    echo "[AINIC] Patched torch._inductor duplicate assert bug"
  fi
  echo "[NETWORK] AINIC final: USING_AINIC=$USING_AINIC NCCL_IB_GID_INDEX=$NCCL_IB_GID_INDEX NCCL_IB_HCA=${NCCL_IB_HCA:-unset} GLOO_SOCKET_IFNAME=${GLOO_SOCKET_IFNAME:-unset} NCCL_SOCKET_IFNAME=${NCCL_SOCKET_IFNAME:-unset}"
elif ip link show | grep -qE 'bnxt|tw-eth' || lsmod | grep -q 'bnxt_re'; then
  export NCCL_IB_GID_INDEX="${NCCL_IB_GID_INDEX:-3}"
  echo "[NETWORK] Broadcom final: NCCL_IB_GID_INDEX=$NCCL_IB_GID_INDEX"
fi

NNODES="${NNODES:-1}"
if [ "$NNODES" -gt 1 ] && [ -n "${DATA_PATH:-}" ]; then
  mkdir -p "$DATA_PATH"
  if [ -e data ] && [ ! -L data ]; then
    rm -rf data
  fi
  ln -sfn "$DATA_PATH" data
  echo "[MULTI-NODE] ./data -> $DATA_PATH (shared storage)"

  # Redirect nemo_experiments to shared storage so distributed checkpoints
  # are accessible by all nodes and persist after container exit for export
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
  export TORCH_NCCL_TRACE_BUFFER_SIZE="${TORCH_NCCL_TRACE_BUFFER_SIZE:-1048576}"
  export TORCH_NCCL_DUMP_ON_TIMEOUT="${TORCH_NCCL_DUMP_ON_TIMEOUT:-1}"
  echo "[MULTI-NODE] NCCL_TIMEOUT=1800000ms (30min)"
fi

./runner/primus-cli direct -- train posttrain --config /tmp/sft_experiment.yaml
TRAIN_EXIT_CODE=$?

# Verify that training produced a usable non-pretrained checkpoint. Distributed
# launches can occasionally exit 0/1 without leaving a valid iter_* directory.
CKPT_BASE="./nemo_experiments/default/checkpoints"
VERIFIED_LATEST_ITER=""
VERIFIED_LATEST_DIR=""
if [ -f "$CKPT_BASE/latest_checkpointed_iteration.txt" ]; then
  VERIFIED_LATEST_ITER=$(cat "$CKPT_BASE/latest_checkpointed_iteration.txt" 2>/dev/null | tr -d '[:space:]')
  if [ -n "$VERIFIED_LATEST_ITER" ] && [ "$VERIFIED_LATEST_ITER" != "0" ]; then
    VERIFIED_LATEST_DIR="$CKPT_BASE/iter_$(printf '%%07d' $VERIFIED_LATEST_ITER)"
  fi
fi
if [ -z "$VERIFIED_LATEST_DIR" ] || [ ! -d "$VERIFIED_LATEST_DIR" ]; then
  VERIFIED_LATEST_DIR=$(ls -d "$CKPT_BASE"/iter_* 2>/dev/null | sort -t_ -k2 -n | tail -1)
  if [ -n "$VERIFIED_LATEST_DIR" ] && [ "$(basename "$VERIFIED_LATEST_DIR")" = "iter_0000000" ]; then
    VERIFIED_LATEST_DIR=""
  fi
fi
if [ $TRAIN_EXIT_CODE -ne 0 ]; then
  if [ -n "$VERIFIED_LATEST_DIR" ] && [ -d "$VERIFIED_LATEST_DIR" ]; then
    echo "WARNING: primus-cli exited with code $TRAIN_EXIT_CODE, but checkpoint found at ${VERIFIED_LATEST_DIR}. Continuing with export."
  else
    echo "Training failed with exit code $TRAIN_EXIT_CODE and no usable checkpoint found. Skipping model export."
    exit $TRAIN_EXIT_CODE
  fi
fi
if [ -z "$VERIFIED_LATEST_DIR" ] || [ ! -d "$VERIFIED_LATEST_DIR" ]; then
  echo "ERROR: Training completed but no usable checkpoint was produced under $CKPT_BASE"
  ls -la "$CKPT_BASE" 2>/dev/null || true
  exit 1
fi
echo "Verified training checkpoint: ${VERIFIED_LATEST_DIR}"

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
      rm -rf "$d" 2>/dev/null || echo "  Warning: best-effort cleanup failed for $d"
    fi
  done
fi
# Remove HF model cache (already converted to Megatron format)
rm -rf data/huggingface/hub/models--* 2>/dev/null || true
# For LoRA, keep pretrained Megatron checkpoint (needed for merge); for full SFT, remove it
if [ "%s" = "none" ]; then
  rm -rf data/megatron_checkpoints 2>/dev/null || true
fi
DISK_USAGE=$(du -sh . 2>/dev/null | cut -f1 || true)
echo "Cleanup done. Disk usage: ${DISK_USAGE:-unknown}"`,
		cfg.DatasetPath, preparedDatasetDir,
		cfg.PrimusPath, cfg.PrimusPath, modelYaml, expYaml, cfg.ExpName,
		cfg.PfsBasePath,
		cfg.TrainConfig.Peft)

	if cfg.ExportModel {
		script += `

# Only master node (rank 0) performs export and model registration
MY_RANK=${RANK:-${OMPI_COMM_WORLD_RANK:-0}}
if [ "$MY_RANK" != "0" ] && [ "$MY_RANK" != "" ]; then
  echo "Worker node (rank=$MY_RANK), skipping export."
  exit 0
fi
`
		script += buildExportScript(cfg)
	}

	return script
}

// buildExportScript generates shell commands to convert the trained checkpoint
// to HuggingFace format, copy it to PFS, and register it as a new Model.
// For LoRA training, an extra merge step is inserted before the HF conversion.
func buildExportScript(cfg EntrypointConfig) string {
	pfs := cfg.PfsBasePath
	if pfs == "" {
		pfs = "/wekafs"
	}
	exportPath := fmt.Sprintf("%s/custom/models/%s", pfs, cfg.SftJobId)
	displayName := fmt.Sprintf("%s-finetuned", strings.ToLower(cfg.ExpName))
	isLoRA := cfg.TrainConfig.Peft == "lora"

	var sb strings.Builder

	// --- Locate trained checkpoint ---
	sb.WriteString(`

# ==================== Convert Megatron Checkpoint to HuggingFace ====================
`)
	fmt.Fprintf(&sb, "EXPORT_PATH=%q\n", exportPath)
	sb.WriteString(`CKPT_DIR=""
if [ -n "${VERIFIED_LATEST_DIR:-}" ] && [ -d "${VERIFIED_LATEST_DIR}" ]; then
  CKPT_DIR="$(dirname "${VERIFIED_LATEST_DIR}")"
fi
CKPT_SEARCH_DIRS="./nemo_experiments/default/checkpoints ${DATA_PATH:-/dev/null}/nemo_experiments/default/checkpoints ./output/checkpoints ${PRIMUS_DIR}/nemo_experiments/default/checkpoints /tmp/primus/nemo_experiments/default/checkpoints"

# First pass: prefer dirs with latest_checkpointed_iteration.txt (real trained checkpoints)
if [ -z "$CKPT_DIR" ]; then
  for d in ${CKPT_SEARCH_DIRS}; do
    if [ -d "$d" ] && [ -f "$d/latest_checkpointed_iteration.txt" ]; then
      ITER_VAL=$(cat "$d/latest_checkpointed_iteration.txt" 2>/dev/null | tr -d '[:space:]')
      if [ -n "$ITER_VAL" ] && [ "$ITER_VAL" != "0" ]; then
        CKPT_DIR="$d"; break
      fi
    fi
  done
fi

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

# Force single-process mode for checkpoint conversion/export to avoid DistStoreError
export WORLD_SIZE=1 RANK=0 LOCAL_RANK=0 MASTER_ADDR=127.0.0.1 MASTER_PORT=29599
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
PRETRAINED_SEARCH="./data/megatron_checkpoints/%s ${DATA_PATH:-/dev/null}/megatron_checkpoints/%s ./data/megatron_checkpoints"
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
`, hfModelBasename, hfModelBasename, cfg.HfPath)
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

HF_HAS_TOKENIZER=0
if [ -f "${HF_EXPORT_PATH}/tokenizer.json" ] || [ -f "${HF_EXPORT_PATH}/tokenizer_config.json" ]; then
  HF_HAS_TOKENIZER=1
fi
HF_HAS_WEIGHTS=0
if [ -f "${HF_EXPORT_PATH}/model.safetensors" ] || [ -f "${HF_EXPORT_PATH}/model.safetensors.index.json" ]; then
  HF_HAS_WEIGHTS=1
elif ls "${HF_EXPORT_PATH}"/*.safetensors >/dev/null 2>&1; then
  HF_HAS_WEIGHTS=1
fi
if [ ! -f "${HF_EXPORT_PATH}/config.json" ] || [ "$HF_HAS_TOKENIZER" != "1" ] || [ "$HF_HAS_WEIGHTS" != "1" ]; then
  echo "ERROR: HF export incomplete at ${HF_EXPORT_PATH}; refusing to register model."
  find "${HF_EXPORT_PATH}" -maxdepth 5 \( -type d -o -type f \) | sed -n '1,80p'
  exit 1
fi
`, cfg.HfPath)

	// --- Register model ---
	fmt.Fprintf(&sb, `
# ==================== Register Model ====================
APISERVER="http://primus-safe-apiserver.primus-safe.svc:8088"
echo "Registering model in Model Square..."
REGISTER_RESPONSE="/tmp/sft_register_model_response.json"
curl -fsS -o "${REGISTER_RESPONSE}" -X POST "${APISERVER}/api/v1/playground/models" \
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
  }'
REGISTER_EXIT=$?
if [ $REGISTER_EXIT -ne 0 ]; then
  echo "ERROR: failed to register model after successful HF export."
  exit $REGISTER_EXIT
fi
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
	sb.WriteString("    config: %MODULE_CONFIG%\n")
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
	if tc.Peft == "lora" {
		hfBasename := cfg.HfPath
		if idx := strings.LastIndex(hfBasename, "/"); idx >= 0 {
			hfBasename = hfBasename[idx+1:]
		}
		fmt.Fprintf(&sb, "      pretrained_checkpoint: ./data/megatron_checkpoints/%s\n", hfBasename)
	}
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
