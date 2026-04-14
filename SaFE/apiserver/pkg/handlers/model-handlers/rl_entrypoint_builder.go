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

// ==================== RL Training Presets ====================

type RlTrainPreset struct {
	Algorithm            string
	TrainBatchSize       int
	MaxPromptLength      int
	MaxResponseLength    int
	ActorLr              float64
	MiniPatchSize        int
	MicroBatchSizePerGpu int
	RolloutN             int
	RolloutTpSize        int
	RolloutGpuMemory     float64
	TotalEpochs          int
	SaveFreq             int
	TestFreq             int
}

var rlTrainPresets = map[string]RlTrainPreset{
	"8b": {
		Algorithm: "grpo", TrainBatchSize: 128, MaxPromptLength: 512, MaxResponseLength: 512,
		ActorLr: 1e-6, MiniPatchSize: 64, MicroBatchSizePerGpu: 4,
		RolloutN: 5, RolloutTpSize: 8, RolloutGpuMemory: 0.4,
		TotalEpochs: 2, SaveFreq: 100, TestFreq: 5,
	},
	"32b": {
		Algorithm: "grpo", TrainBatchSize: 64, MaxPromptLength: 512, MaxResponseLength: 1024,
		ActorLr: 5e-7, MiniPatchSize: 32, MicroBatchSizePerGpu: 2,
		RolloutN: 5, RolloutTpSize: 8, RolloutGpuMemory: 0.4,
		TotalEpochs: 2, SaveFreq: 50, TestFreq: 5,
	},
	"70b": {
		Algorithm: "grpo", TrainBatchSize: 32, MaxPromptLength: 512, MaxResponseLength: 1024,
		ActorLr: 2e-7, MiniPatchSize: 16, MicroBatchSizePerGpu: 1,
		RolloutN: 5, RolloutTpSize: 8, RolloutGpuMemory: 0.4,
		TotalEpochs: 1, SaveFreq: 20, TestFreq: 5,
	},
}

// ==================== Default Values ====================

const (
	DefaultRlImageTag              = "proxy/primussafe/verl:0.8.0.dev-fsdp-sglang-rocm700-mi35x"
	DefaultRlImageFallback         = "primussafe/verl:0.8.0.dev-fsdp-sglang-rocm700-mi35x"
	DefaultRlMegatronImageTag      = "proxy/primussafe/verl:0.8.0.dev-megatron-sglang-rocm700-mi35x"
	DefaultRlMegatronImageFallback = "primussafe/verl:0.8.0.dev-megatron-sglang-rocm700-mi35x"
	DefaultRlCpu           = "128"
	DefaultRlMemory        = "2048Gi"
	DefaultRlSharedMemory  = "1Ti"
	DefaultRlEphemeral     = "500Gi"
	DefaultRlGpuCount      = 8
	DefaultRlNodeCount     = 2
)

// GetDefaultRlImage returns the default verl FSDP2 training image from the cluster's harbor.
func GetDefaultRlImage() string {
	downloadImage := commonconfig.GetDownloadJoImage()
	if idx := strings.Index(downloadImage, "/"); idx > 0 {
		registryHost := downloadImage[:idx]
		return fmt.Sprintf("%s/%s", registryHost, DefaultRlImageTag)
	}
	return DefaultRlImageFallback
}

// GetDefaultRlMegatronImage returns the default verl Megatron training image.
func GetDefaultRlMegatronImage() string {
	downloadImage := commonconfig.GetDownloadJoImage()
	if idx := strings.Index(downloadImage, "/"); idx > 0 {
		registryHost := downloadImage[:idx]
		return fmt.Sprintf("%s/%s", registryHost, DefaultRlMegatronImageTag)
	}
	return DefaultRlMegatronImageFallback
}

// FillRlDefaults populates zero-valued fields with smart defaults based on model size.
func FillRlDefaults(req *CreateRlJobRequest, modelSize string) {
	if req.Priority == 0 {
		req.Priority = DefaultPriority
	}
	if req.ExportModel == nil {
		t := true
		req.ExportModel = &t
	}

	tc := &req.TrainConfig
	if tc.Algorithm == "" {
		tc.Algorithm = "grpo"
	}
	if tc.Strategy == "" {
		tc.Strategy = "fsdp2"
	}
	if tc.RewardType == "" {
		tc.RewardType = "math"
	}

	preset, ok := rlTrainPresets[modelSize]
	if !ok {
		preset = rlTrainPresets["8b"]
	}

	if tc.TrainBatchSize == 0 {
		tc.TrainBatchSize = preset.TrainBatchSize
	}
	if tc.MaxPromptLength == 0 {
		tc.MaxPromptLength = preset.MaxPromptLength
	}
	if tc.MaxResponseLength == 0 {
		tc.MaxResponseLength = preset.MaxResponseLength
	}
	if tc.ActorLr == 0 {
		tc.ActorLr = preset.ActorLr
	}
	if tc.MiniPatchSize == 0 {
		tc.MiniPatchSize = preset.MiniPatchSize
	}
	if tc.MicroBatchSizePerGpu == 0 {
		tc.MicroBatchSizePerGpu = preset.MicroBatchSizePerGpu
	}
	if tc.GradClip == 0 {
		tc.GradClip = 1.0
	}
	if tc.RolloutN == 0 {
		tc.RolloutN = preset.RolloutN
	}
	if tc.RolloutTpSize == 0 {
		tc.RolloutTpSize = preset.RolloutTpSize
	}
	if tc.RolloutGpuMemory == 0 {
		tc.RolloutGpuMemory = preset.RolloutGpuMemory
	}
	if tc.TotalEpochs == 0 {
		tc.TotalEpochs = preset.TotalEpochs
	}
	if tc.SaveFreq == 0 {
		tc.SaveFreq = preset.SaveFreq
	}
	if tc.TestFreq == 0 {
		tc.TestFreq = preset.TestFreq
	}
	if tc.KlLossCoef == 0 {
		tc.KlLossCoef = 0.001
	}

	tc.UseKlLoss = true
	effectiveNodeCount := req.NodeCount
	if effectiveNodeCount == 0 {
		effectiveNodeCount = DefaultRlNodeCount
	}

	if tc.Strategy == "megatron" {
		// Megatron defaults — align 8B settings with historical successful scripts.
		if !tc.ParamOffload {
			tc.ParamOffload = true
		}
		if !tc.GradOffload {
			tc.GradOffload = true
		}
		if !tc.GradientCheckpointing {
			tc.GradientCheckpointing = true
		}
		if tc.MegatronTpSize == 0 {
			switch {
			case effectiveNodeCount <= 1:
				tc.MegatronTpSize = 1
			case modelSize == "70b":
				tc.MegatronTpSize = 8
			default:
				tc.MegatronTpSize = 4
			}
		}
		if tc.MegatronPpSize == 0 {
			switch {
			case effectiveNodeCount <= 1:
				tc.MegatronPpSize = 1
			case modelSize == "70b":
				tc.MegatronPpSize = 4
			default:
				tc.MegatronPpSize = 1
			}
		}
		if tc.MegatronCpSize == 0 {
			tc.MegatronCpSize = 1
		}
		if tc.RolloutGpuMemory == 0 {
			tc.RolloutGpuMemory = 0.85
		}
		if req.Image == "" {
			req.Image = GetDefaultRlMegatronImage()
		}
	} else {
		// FSDP2 defaults — match Xiaofei's tested configuration
		if !tc.ParamOffload {
			tc.ParamOffload = true
		}
		if !tc.OptimizerOffload {
			tc.OptimizerOffload = true
		}
		if !tc.GradientCheckpointing {
			tc.GradientCheckpointing = true
		}
		if !tc.UseTorchCompile {
			tc.UseTorchCompile = true
		}
		if req.Image == "" {
			req.Image = GetDefaultRlImage()
		}
	}
	if req.NodeCount == 0 {
		req.NodeCount = DefaultRlNodeCount
	}
	if req.GpuCount == 0 {
		req.GpuCount = DefaultRlGpuCount
	}
	if req.Cpu == "" {
		req.Cpu = DefaultRlCpu
	}
	if req.Memory == "" {
		req.Memory = DefaultRlMemory
	}
	if req.SharedMemory == "" {
		req.SharedMemory = DefaultRlSharedMemory
	}
	if req.EphemeralStorage == "" {
		req.EphemeralStorage = DefaultRlEphemeral
	}
}

// ==================== RL Entrypoint Builder ====================

// RlEntrypointConfig holds parameters for generating the verl RL training entrypoint.
type RlEntrypointConfig struct {
	ModelPath   string // HF model path on shared storage (e.g. /wekafs/custom/models/xxx)
	ModelName   string // HF model name (e.g. Qwen/Qwen3-8B)
	DatasetPath string
	NodeCount   int
	GpuCount    int
	TrainConfig RlTrainConfig

	ExportModel bool
	ExportPath  string // e.g. /wekafs/custom/models/rl-xxx
	Workspace   string
	ModelId     string
	BaseModel   string
	RlJobId     string
	ExpName     string
}

// BuildRlTrainScript generates the verl training shell script.
// This script runs as the Ray driver process via RAY_JOB_ENTRYPOINT.
func BuildRlTrainScript(cfg RlEntrypointConfig) string {
	tc := cfg.TrainConfig
	var sb strings.Builder

	sb.WriteString(`#!/bin/bash
set -e

echo "=========================================="
echo "  verl RL Training (${RL_ALGORITHM:-grpo})"
echo "=========================================="

`)
	// ROCm / RCCL environment
	sb.WriteString(`# ROCm / RCCL environment
export HIP_VISIBLE_DEVICES=0,1,2,3,4,5,6,7
export CUDA_VISIBLE_DEVICES=0,1,2,3,4,5,6,7
export RAY_EXPERIMENTAL_NOSET_ROCR_VISIBLE_DEVICES=1
export SGLANG_DISABLE_FA3=1
export GPU_MAX_HW_QUEUES=2
export TORCH_NCCL_HIGH_PRIORITY=1
export NCCL_CHECKS_DISABLE=1
export NCCL_CROSS_NIC=0
export CUDA_DEVICE_MAX_CONNECTIONS=1
export NCCL_PROTO=Simple
export RCCL_MSCCL_ENABLE=0
export NCCL_DEBUG=WARN
export TOKENIZERS_PARALLELISM=false
export HSA_NO_SCRATCH_RECLAIM=1
export PYTHONUNBUFFERED=1
export HYDRA_FULL_ERROR=1

# Runtime-detect network backend from the actual node instead of inferring it
# from workspace metadata.
if ls /sys/class/infiniband/ionic_* >/dev/null 2>&1 || ip link show | grep -q 'ionic_'; then
  export NCCL_IB_HCA=$(ip link show | grep -oP 'ionic_\d+' | sort -u | paste -sd, -)
  export NCCL_IB_GID_INDEX=1
  export USING_AINIC=1
  export NCCL_DMABUF_ENABLE=0
  export NCCL_MAX_P2P_CHANNELS=56
  export NET_OPTIONAL_RECV_COMPLETION=1
  export NCCL_IB_USE_INLINE=1
  export RCCL_GDR_FLUSH_GPU_MEM_NO_RELAXED_ORDERING=0
  export NCCL_GDR_FLUSH_DISABLE=1
  export NCCL_IGNORE_CPU_AFFINITY=1
  export LD_LIBRARY_PATH="/opt/amd-anp/build:/opt/rccl/build/release:/opt/rocm/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
  AINIC_IFACE=$(ip -o link show | grep -oP 'ens\d+np\d+' | head -1)
  if [ -n "$AINIC_IFACE" ]; then
    export GLOO_SOCKET_IFNAME="${GLOO_SOCKET_IFNAME:-$AINIC_IFACE}"
    export NCCL_SOCKET_IFNAME="${NCCL_SOCKET_IFNAME:-$AINIC_IFACE}"
  fi
  echo "[RCCL] AINIC detected: NCCL_IB_HCA=$NCCL_IB_HCA"
elif ip link show | grep -qE 'bnxt|tw-eth' || lsmod | grep -q 'bnxt_re'; then
  export NCCL_IB_GID_INDEX="${NCCL_IB_GID_INDEX:-3}"
  echo "[RCCL] Broadcom RDMA detected: NCCL_IB_GID_INDEX=$NCCL_IB_GID_INDEX"
fi
export NCCL_MIN_NCHANNELS=112

`)

	// Megatron-specific: extra env vars
	if tc.Strategy == "megatron" {
		sb.WriteString(`# ==================== Megatron Pre-flight ====================
# Ray timeouts (Megatron init is slow)
export RAY_EXPERIMENTAL_NOSET_CUDA_VISIBLE_DEVICES=1
export RAY_EXPERIMENTAL_NOSET_HIP_VISIBLE_DEVICES=1
export RAY_HEALTH_CHECK_PERIOD_MS=600000
export RAY_HEALTH_CHECK_TIMEOUT_MS=1800000
export RAY_GRPC_KEEPALIVE_TIME_MS=300000
export RAY_GRPC_KEEPALIVE_TIMEOUT_MS=1800000
export RAY_grpc_server_keepalive_time_ms=300000
export RAY_grpc_server_keepalive_timeout_ms=1800000
export RAY_gcs_server_request_timeout_seconds=1800
export RAY_object_timeout_milliseconds=1800000

# ROCm TE + performance tuning
export NVTE_FUSED_ATTN_CK=0
export SGLANG_USE_AITER=0
export TORCHINDUCTOR_MAX_AUTOTUNE=1
export HIP_FORCE_DEV_KERNARG=1

# RCCL network plugin
export LD_LIBRARY_PATH=/opt/amd-anp/build:/opt/rccl/build/release:/opt/rocm/lib:/usr/local/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}
export NCCL_NET_PLUGIN_PATH=/opt/amd-anp/build/librccl-net.so
echo "[Megatron] Pre-flight complete."

`)
	}

	// Dataset preparation (auto-detect parquet or JSONL, convert to verl format)
	fmt.Fprintf(&sb, `# ==================== Prepare Dataset ====================
DATASET_SRC="%s"
DATASET_OUT="/tmp/rl_dataset"
echo "Preparing dataset from $DATASET_SRC ..."
mkdir -p "$DATASET_OUT"

python3 << 'DATAEOF'
import json, os, sys, glob, re
try:
    import pandas as pd
except ImportError:
    import subprocess
    subprocess.check_call([sys.executable, "-m", "pip", "install", "pandas", "pyarrow", "-q"])
    import pandas as pd

src_dir = os.environ.get("DATASET_SRC", "%s")
out_dir = os.environ.get("DATASET_OUT", "/tmp/rl_dataset")

def extract_math_answer(text):
    """Extract final numeric answer from GSM8K-style '#### <number>' format."""
    m = re.search(r'####\s*(.+)', str(text))
    return m.group(1).strip() if m else str(text).strip()

# Infer data_source from dataset path for verl reward function lookup
_src_lower = src_dir.lower()
if "gsm8k" in _src_lower:
    data_source = "openai/gsm8k"
elif "math" in _src_lower:
    data_source = "openai/math"
else:
    data_source = "openai/gsm8k"

MATH_SUFFIX = ' Let\'s think step by step and output the final answer after "####".'

def to_verl_record(prompt_text, answer_text, idx):
    content = prompt_text + MATH_SUFFIX if "gsm8k" in data_source or "math" in data_source else prompt_text
    return {
            "data_source": data_source,
        "prompt": [{"role": "user", "content": content}],
        "reward_model": {"style": "rule", "ground_truth": extract_math_answer(answer_text)},
        "extra_info": {"answer": extract_math_answer(answer_text), "index": idx},
    }

# --- Strategy 1: Find existing parquet files (HuggingFace download format) ---
train_pq, test_pq = None, None
for sub in ["", "main", "default", "train", "."]:
    d = os.path.join(src_dir, sub) if sub else src_dir
    for f in sorted(glob.glob(os.path.join(d, "*.parquet"))):
        fname = os.path.basename(f).lower()
        if "train" in fname and not train_pq:
            train_pq = f
        elif "test" in fname and not test_pq:
            test_pq = f

if train_pq:
    print(f"[Parquet mode] Found train: {train_pq}")
    if test_pq:
        print(f"[Parquet mode] Found test:  {test_pq}")

    for split_name, pq_path in [("train", train_pq), ("test", test_pq or train_pq)]:
        df = pd.read_parquet(pq_path)
        cols = [c.lower() for c in df.columns]
        print(f"  {split_name}: {len(df)} rows, columns: {list(df.columns)}")

        # Check if already in verl format
        if "prompt" in cols and "reward_model" in cols:
            print(f"  -> Already in verl format, using directly.")
            df.to_parquet(os.path.join(out_dir, f"{split_name}.parquet"), index=False)
            continue

        # Convert from HuggingFace format (question/answer or instruction/output)
        prompt_col = next((c for c in df.columns if c.lower() in ["question", "instruction", "input", "prompt"]), None)
        answer_col = next((c for c in df.columns if c.lower() in ["answer", "output", "response", "solution"]), None)
        if not prompt_col:
            print(f"ERROR: cannot find prompt column in {list(df.columns)}")
            sys.exit(1)

        records = []
        for i, row in df.iterrows():
            p = str(row[prompt_col])
            a = str(row[answer_col]) if answer_col else ""
            if p.strip():
                records.append(to_verl_record(p, a, len(records)))
        out_df = pd.DataFrame(records)
        out_df.to_parquet(os.path.join(out_dir, f"{split_name}.parquet"), index=False)
        print(f"  -> Converted {len(records)} records to verl format.")

    if not test_pq:
        print("  -> No test split found, using last 10%% of train as test.")
        df = pd.read_parquet(os.path.join(out_dir, "train.parquet"))
        n = max(1, len(df) // 10)
        df.iloc[-n:].to_parquet(os.path.join(out_dir, "test.parquet"), index=False)
        df.iloc[:-n].to_parquet(os.path.join(out_dir, "train.parquet"), index=False)

# --- Strategy 2: Fall back to JSONL files ---
else:
    src_file = None
    for name in ["training.jsonl", "train.jsonl", "data.jsonl"]:
        p = os.path.join(src_dir, name)
        if os.path.isfile(p) and os.path.getsize(p) > 0:
            src_file = p
            break
    if not src_file:
        candidates = sorted(glob.glob(os.path.join(src_dir, "**", "*.jsonl"), recursive=True))
        candidates += sorted(glob.glob(os.path.join(src_dir, "**", "*.json"), recursive=True))
        candidates = [f for f in candidates if os.path.getsize(f) > 0]
        if candidates:
            src_file = candidates[0]
    if not src_file:
        print("ERROR: no parquet or JSONL files found in " + src_dir)
        sys.exit(1)

    print(f"[JSONL mode] Reading: {src_file}")
    records = []
    with open(src_file, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            obj = json.loads(line)
            prompt_text = obj.get("instruction", obj.get("input", obj.get("prompt", obj.get("question", ""))))
            if not prompt_text:
                continue
            answer = obj.get("output", obj.get("answer", obj.get("response", "")))
            records.append(to_verl_record(prompt_text, answer, len(records)))

    if not records:
        print("ERROR: dataset is empty after parsing")
        sys.exit(1)

    val_count = max(1, len(records) // 10)
    train_records = records[:-val_count] if len(records) > val_count else records
    val_records = records[-val_count:] if len(records) > val_count else records[:1]

    for name, data in [("train", train_records), ("test", val_records)]:
        pd.DataFrame(data).to_parquet(os.path.join(out_dir, f"{name}.parquet"), index=False)
        print(f"  Wrote {len(data)} records to {out_dir}/{name}.parquet")

print("Dataset preparation complete.")
DATAEOF

if [ $? -ne 0 ]; then
  echo "ERROR: Dataset preparation failed!"
  exit 1
fi

`, cfg.DatasetPath, cfg.DatasetPath)

	// Reward function
	if tc.RewardType == "math" {
		sb.WriteString(`# ==================== Reward Function (Math) ====================
# verl built-in math reward using math_verify — no custom code needed.
# The dataset extra_info.answer is used as ground truth.

`)
	}

	// verl training command
	fmt.Fprintf(&sb, `# ==================== Run verl Training ====================
NNODES=%d
GPUS_PER_NODE=%d
EXPORT_BASE="%s"
SAVE_DIR="${EXPORT_BASE}/checkpoints"
mkdir -p "$SAVE_DIR"

`, cfg.NodeCount, cfg.GpuCount, cfg.ExportPath)

	// Scale LR with node count
	fmt.Fprintf(&sb, `LR=$(python3 -c "print(%.10f * $NNODES)")
`, tc.ActorLr)

	// Scale batch sizes with node count
	fmt.Fprintf(&sb, `TRAIN_BATCH=$((${RL_TRAIN_BATCH_SIZE:-%d} * NNODES))
MINI_BATCH=$((${RL_MINI_BATCH_SIZE:-%d} * NNODES))

echo "Training config: NNODES=$NNODES, GPUS=$GPUS_PER_NODE, LR=$LR, BATCH=$TRAIN_BATCH"

python3 -m verl.trainer.main_ppo \
`, tc.TrainBatchSize, tc.MiniPatchSize)
	if tc.Strategy == "megatron" {
		// Hydra must see the Megatron config before any overrides.
		sb.WriteString("  --config-name=ppo_megatron_trainer.yaml \\\n")
	}

	// Algorithm
	advEstimator := "grpo"
	if tc.Algorithm == "ppo" {
		advEstimator = "gae"
	}
	fmt.Fprintf(&sb, "  algorithm.adv_estimator=%s \\\n", advEstimator)

	// Data
	sb.WriteString("  data.train_files=/tmp/rl_dataset/train.parquet \\\n")
	sb.WriteString("  data.val_files=/tmp/rl_dataset/test.parquet \\\n")
	sb.WriteString("  data.train_batch_size=$TRAIN_BATCH \\\n")
	fmt.Fprintf(&sb, "  data.max_prompt_length=%d \\\n", tc.MaxPromptLength)
	fmt.Fprintf(&sb, "  data.max_response_length=%d \\\n", tc.MaxResponseLength)
	sb.WriteString("  data.filter_overlong_prompts=True \\\n")
	sb.WriteString("  data.truncation=error \\\n")

	// Actor model (common)
	fmt.Fprintf(&sb, "  actor_rollout_ref.model.path=%s \\\n", cfg.ModelPath)
	sb.WriteString("  actor_rollout_ref.actor.optim.lr=$LR \\\n")
	sb.WriteString("  actor_rollout_ref.actor.ppo_mini_batch_size=$MINI_BATCH \\\n")
	fmt.Fprintf(&sb, "  actor_rollout_ref.actor.ppo_micro_batch_size_per_gpu=%d \\\n", tc.MicroBatchSizePerGpu)
	fmt.Fprintf(&sb, "  actor_rollout_ref.actor.use_kl_loss=%v \\\n", tc.UseKlLoss)
	fmt.Fprintf(&sb, "  actor_rollout_ref.actor.kl_loss_coef=%.6f \\\n", tc.KlLossCoef)
	sb.WriteString("  actor_rollout_ref.actor.kl_loss_type=low_var_kl \\\n")
	sb.WriteString("  actor_rollout_ref.actor.entropy_coeff=0 \\\n")

	if tc.Strategy == "megatron" {
		// Megatron actor strategy
		fmt.Fprintf(&sb, "  actor_rollout_ref.actor.optim.clip_grad=%.1f \\\n", tc.GradClip)
		sb.WriteString("  actor_rollout_ref.actor.strategy=megatron \\\n")
		fmt.Fprintf(&sb, "  actor_rollout_ref.actor.megatron.tensor_model_parallel_size=%d \\\n", tc.MegatronTpSize)
		fmt.Fprintf(&sb, "  actor_rollout_ref.actor.megatron.pipeline_model_parallel_size=%d \\\n", tc.MegatronPpSize)
		if tc.MegatronEpSize > 0 {
			fmt.Fprintf(&sb, "  actor_rollout_ref.actor.megatron.expert_model_parallel_size=%d \\\n", tc.MegatronEpSize)
		}
		fmt.Fprintf(&sb, "  actor_rollout_ref.actor.megatron.context_parallel_size=%d \\\n", tc.MegatronCpSize)
		fmt.Fprintf(&sb, "  actor_rollout_ref.actor.megatron.param_offload=%v \\\n", tc.ParamOffload)
		fmt.Fprintf(&sb, "  actor_rollout_ref.actor.megatron.grad_offload=%v \\\n", tc.GradOffload)
		sb.WriteString("  actor_rollout_ref.actor.megatron.optimizer_offload=False \\\n")
		sb.WriteString("  actor_rollout_ref.actor.megatron.use_mbridge=True \\\n")
		sb.WriteString("  actor_rollout_ref.actor.megatron.override_transformer_config.attention_backend=flash \\\n")
		fmt.Fprintf(&sb, "  actor_rollout_ref.model.enable_gradient_checkpointing=%v \\\n", tc.GradientCheckpointing)
	} else {
		// FSDP2 actor strategy
		fmt.Fprintf(&sb, "  actor_rollout_ref.actor.grad_clip=%.1f \\\n", tc.GradClip)
		sb.WriteString("  actor_rollout_ref.actor.strategy=fsdp2 \\\n")
		sb.WriteString("  actor_rollout_ref.actor.fsdp_config.model_dtype=bf16 \\\n")
		fmt.Fprintf(&sb, "  actor_rollout_ref.actor.fsdp_config.param_offload=%v \\\n", tc.ParamOffload)
		fmt.Fprintf(&sb, "  actor_rollout_ref.actor.fsdp_config.optimizer_offload=%v \\\n", tc.OptimizerOffload)
		fmt.Fprintf(&sb, "  actor_rollout_ref.actor.use_torch_compile=%v \\\n", tc.UseTorchCompile)
		fmt.Fprintf(&sb, "  actor_rollout_ref.model.enable_gradient_checkpointing=%v \\\n", tc.GradientCheckpointing)
	}

	// Rollout (SGLang) — common for both strategies
	fmt.Fprintf(&sb, "  actor_rollout_ref.rollout.log_prob_micro_batch_size_per_gpu=%d \\\n", tc.MicroBatchSizePerGpu*2)
	fmt.Fprintf(&sb, "  actor_rollout_ref.rollout.tensor_model_parallel_size=%d \\\n", tc.RolloutTpSize)
	sb.WriteString("  actor_rollout_ref.rollout.name=sglang \\\n")
	fmt.Fprintf(&sb, "  actor_rollout_ref.rollout.gpu_memory_utilization=%.2f \\\n", tc.RolloutGpuMemory)
	sb.WriteString("  actor_rollout_ref.rollout.free_cache_engine=True \\\n")
	fmt.Fprintf(&sb, "  actor_rollout_ref.rollout.n=%d \\\n", tc.RolloutN)
	if tc.Strategy == "megatron" {
		sb.WriteString("  actor_rollout_ref.rollout.enforce_eager=True \\\n")
	}
	if cfg.NodeCount > 1 || tc.Strategy == "megatron" {
		sb.WriteString("  actor_rollout_ref.nccl_timeout=3600 \\\n")
	}

	// Ref model
	fmt.Fprintf(&sb, "  actor_rollout_ref.ref.log_prob_micro_batch_size_per_gpu=%d \\\n", tc.MicroBatchSizePerGpu*2)
	if tc.Strategy == "megatron" {
		sb.WriteString("  actor_rollout_ref.ref.strategy=megatron \\\n")
		fmt.Fprintf(&sb, "  actor_rollout_ref.ref.megatron.tensor_model_parallel_size=%d \\\n", tc.MegatronTpSize)
		fmt.Fprintf(&sb, "  actor_rollout_ref.ref.megatron.pipeline_model_parallel_size=%d \\\n", tc.MegatronPpSize)
		if tc.MegatronEpSize > 0 {
			fmt.Fprintf(&sb, "  actor_rollout_ref.ref.megatron.expert_model_parallel_size=%d \\\n", tc.MegatronEpSize)
		}
		fmt.Fprintf(&sb, "  actor_rollout_ref.ref.megatron.context_parallel_size=%d \\\n", tc.MegatronCpSize)
		fmt.Fprintf(&sb, "  actor_rollout_ref.ref.megatron.param_offload=%v \\\n", tc.ParamOffload)
	} else {
		fmt.Fprintf(&sb, "  actor_rollout_ref.ref.fsdp_config.param_offload=%v \\\n", tc.RefParamOffload)
		fmt.Fprintf(&sb, "  actor_rollout_ref.ref.fsdp_config.reshard_after_forward=%v \\\n", tc.RefReshardAfterForward)
		sb.WriteString("  actor_rollout_ref.ref.fsdp_config.model_dtype=bf16 \\\n")
	}

	// Algorithm settings
	sb.WriteString("  algorithm.use_kl_in_reward=False \\\n")

	// Trainer
	if tc.Algorithm == "ppo" {
		sb.WriteString("  trainer.critic_warmup=0 \\\n")
	}
	sb.WriteString("  trainer.logger=console \\\n")
	fmt.Fprintf(&sb, "  trainer.project_name=%s \\\n", sanitizeForVerlConfig(cfg.ExpName))
	fmt.Fprintf(&sb, "  trainer.experiment_name=%s \\\n", sanitizeForVerlConfig(cfg.RlJobId))
	fmt.Fprintf(&sb, "  trainer.n_gpus_per_node=%d \\\n", cfg.GpuCount)
	fmt.Fprintf(&sb, "  trainer.nnodes=%d \\\n", cfg.NodeCount)
	if cfg.NodeCount > 1 {
		sb.WriteString("  trainer.ray_wait_register_center_timeout=1800 \\\n")
	}
	fmt.Fprintf(&sb, "  trainer.save_freq=%d \\\n", tc.SaveFreq)
	sb.WriteString("  trainer.default_local_dir=$SAVE_DIR \\\n")
	fmt.Fprintf(&sb, "  trainer.test_freq=%d \\\n", tc.TestFreq)
	// No save_contents override — use verl defaults. HF export done post-training.
	fmt.Fprintf(&sb, "  trainer.total_epochs=%d\n\n", tc.TotalEpochs)

	sb.WriteString(`TRAIN_EXIT=$?
if [ $TRAIN_EXIT -ne 0 ]; then
  echo "ERROR: verl training exited with code $TRAIN_EXIT"
  # Check if any checkpoint was saved
  if ls "$SAVE_DIR"/global_step_* >/dev/null 2>&1; then
    echo "WARNING: Training failed but checkpoints found, continuing with export..."
  else
    echo "No checkpoints found. Exiting."
    exit $TRAIN_EXIT
  fi
fi
echo "Training complete."

`)

	// Model export and registration
	if cfg.ExportModel {
		sb.WriteString(buildRlExportScript(cfg))
	}

	return sb.String()
}

// buildRlExportScript generates the model export and registration commands.
// verl saves HuggingFace-format checkpoints natively — no Megatron conversion needed.
// Uses a file lock to ensure export + registration only happens once (safe for multi-node).
func buildRlExportScript(cfg RlEntrypointConfig) string {
	displayName := fmt.Sprintf("%s-rl-trained", strings.ToLower(cfg.ExpName))
	var sb strings.Builder

	fmt.Fprintf(&sb, `# ==================== Export Model ====================
EXPORT_LOCK="${EXPORT_BASE}/.export_done"
CKPT_SEARCH_DIR="${EXPORT_BASE}/checkpoints"

# Guard: skip if already exported (prevents duplicate model registration in multi-node)
if [ -f "$EXPORT_LOCK" ]; then
  echo "Export already completed (lock file exists), skipping."
  exit 0
fi

LATEST_STEP=$(ls -d "$CKPT_SEARCH_DIR"/global_step_* 2>/dev/null | sort -t_ -k3 -n | tail -1)
if [ -z "$LATEST_STEP" ]; then
  echo "ERROR: No RL checkpoint found in $CKPT_SEARCH_DIR. Model export skipped."
  exit 0
fi

# Export: convert checkpoint shards to HuggingFace safetensors format
CKPT_DIR="$LATEST_STEP/actor"
[ ! -d "$CKPT_DIR" ] && CKPT_DIR="$LATEST_STEP"
FINAL_MODEL_PATH="${EXPORT_BASE}/final_model"
mkdir -p "$FINAL_MODEL_PATH"

echo "Converting checkpoint to HF format..."
python3 - "$CKPT_DIR" "$FINAL_MODEL_PATH" << 'CONVERT_HF'
import os, sys, glob, shutil
ckpt_dir, output_dir = sys.argv[1], sys.argv[2]
os.makedirs(output_dir, exist_ok=True)

# Copy tokenizer/config
hf_dir = os.path.join(ckpt_dir, "huggingface")
if os.path.isdir(hf_dir):
    for f in os.listdir(hf_dir):
        shutil.copy2(os.path.join(hf_dir, f), os.path.join(output_dir, f))
    print(f"Copied tokenizer/config from {hf_dir}")

# Check if already HF format
if glob.glob(os.path.join(ckpt_dir, "*.safetensors")) or glob.glob(os.path.join(hf_dir or "", "*.safetensors")):
    for f in glob.glob(os.path.join(hf_dir or ckpt_dir, "*.safetensors")):
        shutil.copy2(f, os.path.join(output_dir, os.path.basename(f)))
    print("Already HF format, copied safetensors.")
    sys.exit(0)

import torch, torch.distributed as dist, json
os.environ.update({"MASTER_ADDR":"localhost","MASTER_PORT":"49599","RANK":"0","WORLD_SIZE":"1"})
if not dist.is_initialized(): dist.init_process_group("gloo", rank=0, world_size=1)
from safetensors.torch import save_file

# FSDP2 shards: load DTensors via fake process group
fsdp_shards = sorted(glob.glob(os.path.join(ckpt_dir, "model_world_size_*_rank_*.pt")))
if fsdp_shards:
    all_params = {}
    for i, sp in enumerate(fsdp_shards):
        print(f"  Loading FSDP shard {i+1}/{len(fsdp_shards)}")
        sd = torch.load(sp, map_location="cpu", weights_only=False)
        if isinstance(sd, dict) and "model" in sd: sd = sd["model"]
        for k, v in sd.items():
            if hasattr(v, "_local_tensor"): v = v._local_tensor
            if hasattr(v, "full_tensor"): v = v.full_tensor()
            all_params.setdefault(k, []).append(v)
    state_dict = {k: (torch.cat(c, dim=0) if len(c) > 1 else c[0]) for k, c in all_params.items()}
    total_gb = sum(t.numel() * t.element_size() for t in state_dict.values()) / 1e9
    print(f"Merged {len(state_dict)} params ({total_gb:.1f} GB)")
    save_file(state_dict, os.path.join(output_dir, "model.safetensors"))
    print("Saved model.safetensors (FSDP2)")
    dist.destroy_process_group()
    sys.exit(0)

# Megatron dist_ckpt: DCP -> flat state dict -> map Megatron keys to HF keys
distcp_dir = os.path.join(ckpt_dir, "dist_ckpt")
if os.path.isdir(distcp_dir) and glob.glob(os.path.join(distcp_dir, "__*_*.distcp")):
    print("Found Megatron DCP, converting to HF format...")
    from torch.distributed.checkpoint.format_utils import dcp_to_torch_save
    flat_path = "/tmp/megatron_flat.pt"
    dcp_to_torch_save(distcp_dir, flat_path)
    sd = torch.load(flat_path, map_location="cpu", weights_only=False)
    model = {k:v for k,v in sd.items() if isinstance(v,torch.Tensor) and "_extra_state" not in k and not k.startswith("optimizer.")}
    del sd

    cfg_path = os.path.join(output_dir, "config.json")
    if os.path.exists(cfg_path):
        with open(cfg_path) as f: hf_cfg = json.load(f)
    else:
        hf_cfg = {"num_hidden_layers":36,"num_attention_heads":32,"num_key_value_heads":8,"hidden_size":4096}
    NL, NH, NKV, HD = hf_cfg.get("num_hidden_layers",36), hf_cfg.get("num_attention_heads",32), hf_cfg.get("num_key_value_heads",8), hf_cfg.get("hidden_size",4096)
    head_dim = HD // NH

    hf_sd = {}
    if "embedding.word_embeddings.weight" in model: hf_sd["model.embed_tokens.weight"] = model["embedding.word_embeddings.weight"]
    if "decoder.final_layernorm.weight" in model: hf_sd["model.norm.weight"] = model["decoder.final_layernorm.weight"]
    if "output_layer.weight" in model: hf_sd["lm_head.weight"] = model["output_layer.weight"]

    stacked = "decoder.layers.self_attention.linear_qkv.weight" in model
    for i in range(NL):
        p = f"model.layers.{i}."
        def _g(key):
            return model["decoder.layers." + key][i] if stacked else model.get(f"decoder.layers.{i}.{key}")
        qkv = _g("self_attention.linear_qkv.weight")
        if qkv is None: continue
        q_sz, kv_sz = NH*head_dim, NKV*head_dim
        q,k,v = qkv.split([q_sz, kv_sz, kv_sz], dim=0)
        hf_sd[p+"self_attn.q_proj.weight"], hf_sd[p+"self_attn.k_proj.weight"], hf_sd[p+"self_attn.v_proj.weight"] = q, k, v
        hf_sd[p+"self_attn.o_proj.weight"] = _g("self_attention.linear_proj.weight")
        fc1 = _g("mlp.linear_fc1.weight"); gate,up = fc1.chunk(2, dim=0)
        hf_sd[p+"mlp.gate_proj.weight"], hf_sd[p+"mlp.up_proj.weight"] = gate, up
        hf_sd[p+"mlp.down_proj.weight"] = _g("mlp.linear_fc2.weight")
        hf_sd[p+"input_layernorm.weight"] = _g("self_attention.linear_qkv.layer_norm_weight")
        hf_sd[p+"post_attention_layernorm.weight"] = _g("mlp.linear_fc1.layer_norm_weight")
        qn = _g("self_attention.q_layernorm.weight")
        kn = _g("self_attention.k_layernorm.weight")
        if qn is not None: hf_sd[p+"self_attn.q_norm.weight"] = qn
        if kn is not None: hf_sd[p+"self_attn.k_norm.weight"] = kn

    total_gb = sum(t.numel()*t.element_size() for t in hf_sd.values())/1e9
    print(f"  Mapped {len(hf_sd)} HF keys ({total_gb:.1f} GB)")
    save_file(hf_sd, os.path.join(output_dir, "model.safetensors"))
    print("Saved model.safetensors (Megatron)")
    try: os.remove(flat_path)
    except: pass
    dist.destroy_process_group()
    sys.exit(0)

dist.destroy_process_group()
print("WARNING: No recognized checkpoint format (FSDP shards or Megatron DCP).")
CONVERT_HF

echo "Export complete: $FINAL_MODEL_PATH"
ls -lh "$FINAL_MODEL_PATH/" | head -20

HF_HAS_TOKENIZER=0
if [ -f "$FINAL_MODEL_PATH/tokenizer.json" ] || [ -f "$FINAL_MODEL_PATH/tokenizer_config.json" ]; then
  HF_HAS_TOKENIZER=1
fi
HF_HAS_WEIGHTS=0
if [ -f "$FINAL_MODEL_PATH/model.safetensors" ] || [ -f "$FINAL_MODEL_PATH/model.safetensors.index.json" ]; then
  HF_HAS_WEIGHTS=1
elif ls "$FINAL_MODEL_PATH"/*.safetensors >/dev/null 2>&1; then
  HF_HAS_WEIGHTS=1
fi
if [ ! -f "$FINAL_MODEL_PATH/config.json" ] || [ "$HF_HAS_TOKENIZER" != "1" ] || [ "$HF_HAS_WEIGHTS" != "1" ]; then
  echo "ERROR: RL HF export incomplete at $FINAL_MODEL_PATH; refusing to register model."
  find "$FINAL_MODEL_PATH" -maxdepth 4 \( -type d -o -type f \) | sed -n '1,80p'
  exit 1
fi

`)

	// Register in Model Square
	fmt.Fprintf(&sb, `# ==================== Register Model in Model Square ====================
APISERVER="http://primus-safe-apiserver.primus-safe.svc:8088"
echo "Registering RL-trained model in Model Square..."
curl -s -X POST "${APISERVER}/api/v1/playground/models" \
  -H "Content-Type: application/json" \
  -H "userId: ${RL_USER_ID:-system}" \
  -H "userName: ${RL_USER_NAME:-system}" \
  -d '{
    "displayName": "%s",
    "description": "RL-trained (%s) from %s (job: %s)",
    "source": {
      "accessMode": "local_path",
      "localPath": "%s/final_model",
      "modelName": "%s"
    },
    "workspace": "%s",
    "origin": "rl_trained",
    "sftJobId": "%s",
    "baseModel": "%s"
  }' && touch "$EXPORT_LOCK" || echo "WARNING: Failed to register model, but output is saved at ${EXPORT_BASE}/final_model"
echo "RL training pipeline complete."
`,
		displayName,
		cfg.TrainConfig.Algorithm, cfg.BaseModel, cfg.RlJobId,
		cfg.ExportPath,
		displayName,
		cfg.Workspace,
		cfg.RlJobId,
		cfg.BaseModel,
	)

	return sb.String()
}

// BuildRlContainerEntrypoint generates the container init script for the head/worker pods.
// This runs before ray start to set up the environment.
func BuildRlContainerEntrypoint(trainScript string, isHead bool) string {
	var sb strings.Builder

	sb.WriteString("#!/bin/bash\nset -e\n")
	sb.WriteString("echo '[RL Init] Setting up environment...'\n\n")

	// Clean aiter JIT lock files (prevents SGLang hangs on pod restart)
	sb.WriteString(`find /sgl-workspace/aiter/aiter/jit/build -name "lock*" -exec rm -f {} \; 2>/dev/null || true
`)

	if isHead {
		// Head node writes the training script to a known path
		sb.WriteString("\n# Write training script (head only)\n")
		sb.WriteString("cat > /tmp/rl_train.sh << 'RL_TRAIN_SCRIPT_EOF'\n")
		sb.WriteString(trainScript)
		sb.WriteString("\nRL_TRAIN_SCRIPT_EOF\n")
		sb.WriteString("chmod +x /tmp/rl_train.sh\n")
		sb.WriteString("echo '[RL Init] Training script written to /tmp/rl_train.sh'\n")
	}

	sb.WriteString("\necho '[RL Init] Done.'\n")

	return sb.String()
}

func sanitizeForVerlConfig(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	if len(s) > 50 {
		s = s[:50]
	}
	return s
}
