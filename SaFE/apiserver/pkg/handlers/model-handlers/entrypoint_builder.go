/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"fmt"
	"strings"
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
	"Qwen/Qwen3-8B":                {Recipe: "qwen.qwen3", Flavor: "qwen3_8b_finetune_config", Size: "8b"},
	"Qwen/Qwen3-32B":               {Recipe: "qwen.qwen3", Flavor: "qwen3_32b_finetune_config", Size: "32b"},
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
}

var trainPresets = map[string]map[string]TrainPreset{
	"8b": {
		"none": {TrainIters: 1000, GlobalBatchSize: 128, MicroBatchSize: 1, SeqLength: 2048, FinetuneLr: 1e-4, TpSize: 1},
		"lora": {TrainIters: 1000, GlobalBatchSize: 128, MicroBatchSize: 1, SeqLength: 2048, FinetuneLr: 1e-4, TpSize: 1},
	},
	"32b": {
		"none": {TrainIters: 200, GlobalBatchSize: 8, MicroBatchSize: 1, SeqLength: 8192, FinetuneLr: 5e-6, TpSize: 1},
		"lora": {TrainIters: 200, GlobalBatchSize: 32, MicroBatchSize: 4, SeqLength: 8192, FinetuneLr: 1e-4, TpSize: 1},
	},
	"70b": {
		"none": {TrainIters: 200, GlobalBatchSize: 128, MicroBatchSize: 1, SeqLength: 2048, FinetuneLr: 5e-6, TpSize: 8},
		"lora": {TrainIters: 200, GlobalBatchSize: 128, MicroBatchSize: 1, SeqLength: 2048, FinetuneLr: 1e-4, TpSize: 8},
	},
}

// ==================== Default Value Population ====================

const (
	DefaultImage            = "rocm/primus:v26.1"
	DefaultGpuCount         = 8
	DefaultCpu              = "128"
	DefaultMemory           = "1024Gi"
	DefaultEphemeralStorage = "300Gi"
	DefaultPrimusPath       = "/shared_nfs/xiaofei/Primus"
)

// FillSftDefaults populates zero-valued fields with smart defaults based on model size and peft type.
func FillSftDefaults(req *CreateSftJobRequest, modelSize string) {
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
		tc.LrWarmupIters = 50
	}
	if tc.EvalInterval == 0 {
		tc.EvalInterval = 30
	}
	if tc.SaveInterval == 0 {
		tc.SaveInterval = 50
	}
	if tc.PrecisionConfig == "" {
		tc.PrecisionConfig = "bf16_mixed"
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
		req.Image = DefaultImage
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
}

// BuildEntrypoint generates the shell script that writes Primus YAML configs and invokes primus-cli.
func BuildEntrypoint(cfg EntrypointConfig) string {
	modelYaml := buildModelYaml(cfg)
	expYaml := buildExperimentYaml(cfg)

	return fmt.Sprintf(`cd %s
cat > /tmp/sft_model.yaml << 'MODELEOF'
%s
MODELEOF
cat > /tmp/sft_experiment.yaml << 'EXPEOF'
%s
EXPEOF
./runner/primus-cli direct -- train posttrain --config /tmp/sft_experiment.yaml`,
		cfg.PrimusPath, modelYaml, expYaml)
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
	fmt.Fprintf(&sb, "    rewrite: true\n")
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
	sb.WriteString("    model: /tmp/sft_model.yaml\n")
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
	fmt.Fprintf(&sb, "      finetune_lr: %e\n", tc.FinetuneLr)
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
