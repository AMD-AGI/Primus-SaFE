// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package intent

import (
	"encoding/json"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigExtractor is the Layer 2 deterministic extractor that parses
// structured configuration files (JSON/YAML) referenced by the workload.
// It handles:
//   - DeepSpeed JSON config (zero_optimization, bf16, batch_size, etc.)
//   - HuggingFace training YAML config (training_type, model_name, hyperparams)
//   - FSDP config (sharding_strategy, cpu_offload, etc.)
//   - Megatron args (tensor_model_parallel_size, pipeline_model_parallel_size)
//   - Accelerate YAML config (mixed_precision, num_processes, etc.)
type ConfigExtractor struct {
	modelParser *ModelNameParser
}

// ConfigExtractionResult holds the result of L2 extraction
type ConfigExtractionResult struct {
	// Model info extracted from config
	Model *ModelInfo `json:"model,omitempty"`

	// Training details
	Training *TrainingDetail `json:"training,omitempty"`

	// Inference details
	Inference *InferenceDetail `json:"inference,omitempty"`

	// Framework stack hints
	FrameworkStack *FrameworkStack `json:"framework_stack,omitempty"`

	// Category
	Category Category `json:"category,omitempty"`

	// Field provenance
	FieldSources map[string]string `json:"field_sources,omitempty"`

	// Coverage: how much could be extracted from configs
	Coverage float64 `json:"coverage"`
}

// NewConfigExtractor creates a new L2 config extractor
func NewConfigExtractor() *ConfigExtractor {
	return &ConfigExtractor{
		modelParser: NewModelNameParser(),
	}
}

// ExtractFromConfigs parses all config file contents and merges results
func (e *ConfigExtractor) ExtractFromConfigs(configs map[string]string) *ConfigExtractionResult {
	result := &ConfigExtractionResult{
		FieldSources: make(map[string]string),
	}

	for path, content := range configs {
		if content == "" {
			continue
		}

		pathLower := strings.ToLower(path)

		// Determine config type and parse
		switch {
		case strings.Contains(pathLower, "ds_config") || strings.Contains(pathLower, "deepspeed"):
			e.parseDeepSpeedConfig(content, path, result)
		case strings.Contains(pathLower, "accelerate"):
			e.parseAccelerateConfig(content, path, result)
		case strings.HasSuffix(pathLower, ".json"):
			e.parseGenericJSON(content, path, result)
		case strings.HasSuffix(pathLower, ".yaml") || strings.HasSuffix(pathLower, ".yml"):
			e.parseGenericYAML(content, path, result)
		}
	}

	result.Coverage = e.calculateCoverage(result)
	return result
}

// ---------------------------------------------------------------------------
// DeepSpeed config parser
// ---------------------------------------------------------------------------

func (e *ConfigExtractor) parseDeepSpeedConfig(content, path string, result *ConfigExtractionResult) {
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(content), &cfg); err != nil {
		return
	}

	if result.Training == nil {
		result.Training = &TrainingDetail{}
	}
	if result.Training.Parallelism == nil {
		result.Training.Parallelism = &ParallelismConfig{}
	}
	if result.Training.HyperParams == nil {
		result.Training.HyperParams = &HyperParams{}
	}

	// Zero optimization
	if zero, ok := cfg["zero_optimization"].(map[string]interface{}); ok {
		if stage, ok := zero["stage"].(float64); ok {
			result.Training.Parallelism.ZeroStage = int(stage)
			result.FieldSources["parallelism.zero_stage"] = "config:L2:" + path
		}
	}

	// Mixed precision
	if bf16, ok := cfg["bf16"].(map[string]interface{}); ok {
		if enabled, ok := bf16["enabled"].(bool); ok && enabled {
			result.FieldSources["mixed_precision"] = "config:L2:bf16"
		}
	}
	if fp16, ok := cfg["fp16"].(map[string]interface{}); ok {
		if enabled, ok := fp16["enabled"].(bool); ok && enabled {
			result.FieldSources["mixed_precision"] = "config:L2:fp16"
		}
	}

	// Batch size
	if batchSize, ok := cfg["train_batch_size"].(float64); ok && batchSize > 0 {
		result.Training.HyperParams.BatchSize = int(batchSize)
		result.FieldSources["hyperparams.batch_size"] = "config:L2:" + path
	}
	if microBatch, ok := cfg["train_micro_batch_size_per_gpu"].(float64); ok && microBatch > 0 {
		result.Training.HyperParams.BatchSize = int(microBatch)
		result.FieldSources["hyperparams.batch_size"] = "config:L2:" + path
	}

	// Gradient accumulation
	if gradAccum, ok := cfg["gradient_accumulation_steps"].(float64); ok && gradAccum > 0 {
		result.Training.HyperParams.GradAccum = int(gradAccum)
		result.FieldSources["hyperparams.grad_accum"] = "config:L2:" + path
	}

	// Optimizer
	if optimizer, ok := cfg["optimizer"].(map[string]interface{}); ok {
		if optType, ok := optimizer["type"].(string); ok {
			result.Training.HyperParams.Optimizer = optType
			result.FieldSources["hyperparams.optimizer"] = "config:L2:" + path
		}
		if params, ok := optimizer["params"].(map[string]interface{}); ok {
			if lr, ok := params["lr"].(float64); ok {
				result.Training.HyperParams.LearningRate = lr
				result.FieldSources["hyperparams.lr"] = "config:L2:" + path
			}
		}
	}

	// Framework stack
	if result.FrameworkStack == nil {
		result.FrameworkStack = &FrameworkStack{}
	}
	result.FrameworkStack.Orchestration = "deepspeed"
	result.FieldSources["framework_stack.orchestration"] = "config:L2:" + path

	// Category: DeepSpeed config means training
	if result.Category == "" {
		result.Category = CategoryPreTraining
		result.FieldSources["category"] = "config:L2:inferred_from_deepspeed"
	}
}

// ---------------------------------------------------------------------------
// Accelerate config parser
// ---------------------------------------------------------------------------

func (e *ConfigExtractor) parseAccelerateConfig(content, path string, result *ConfigExtractionResult) {
	var cfg map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return
	}

	if result.Training == nil {
		result.Training = &TrainingDetail{}
	}
	if result.Training.Parallelism == nil {
		result.Training.Parallelism = &ParallelismConfig{}
	}

	// Mixed precision
	if mp, ok := cfg["mixed_precision"].(string); ok {
		result.FieldSources["mixed_precision"] = "config:L2:" + mp
	}

	// FSDP
	if fsdpConfig, ok := cfg["fsdp_config"].(map[string]interface{}); ok {
		result.Training.Parallelism.FSDP = true
		result.FieldSources["parallelism.fsdp"] = "config:L2:" + path
		if strategy, ok := fsdpConfig["fsdp_sharding_strategy"].(string); ok {
			result.Training.Parallelism.Strategy = strategy
		}
	}

	// DeepSpeed integration
	if dsPlugin, ok := cfg["deepspeed_plugin"].(map[string]interface{}); ok {
		if stage, ok := dsPlugin["zero_stage"].(float64); ok {
			result.Training.Parallelism.ZeroStage = int(stage)
			result.FieldSources["parallelism.zero_stage"] = "config:L2:" + path
		}
	}

	// Num processes
	if numProcs, ok := cfg["num_processes"].(float64); ok && numProcs > 0 {
		result.Training.Parallelism.DataParallel = int(numProcs)
		result.FieldSources["parallelism.dp"] = "config:L2:" + path
	}
}

// ---------------------------------------------------------------------------
// Generic YAML parser (HuggingFace training config style)
// ---------------------------------------------------------------------------

func (e *ConfigExtractor) parseGenericYAML(content, path string, result *ConfigExtractionResult) {
	var cfg map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		return
	}
	e.extractFromGenericConfig(cfg, path, result)
}

// ---------------------------------------------------------------------------
// Generic JSON parser
// ---------------------------------------------------------------------------

func (e *ConfigExtractor) parseGenericJSON(content, path string, result *ConfigExtractionResult) {
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(content), &cfg); err != nil {
		return
	}
	e.extractFromGenericConfig(cfg, path, result)
}

// ---------------------------------------------------------------------------
// Shared extraction from key-value config
// ---------------------------------------------------------------------------

func (e *ConfigExtractor) extractFromGenericConfig(cfg map[string]interface{}, path string, result *ConfigExtractionResult) {
	// Model path
	for _, key := range []string{"model_name_or_path", "model_name", "model_path", "model", "base_model"} {
		if val, ok := cfg[key].(string); ok && val != "" {
			modelInfo := e.modelParser.Parse(val)
			if modelInfo != nil && result.Model == nil {
				result.Model = modelInfo
				result.FieldSources["model"] = "config:L2:" + path
			}
			break
		}
	}

	// Training type / method
	for _, key := range []string{"training_type", "method", "task_type"} {
		if val, ok := cfg[key].(string); ok && val != "" {
			method := mapConfigMethod(val)
			if method != "" {
				if result.Training == nil {
					result.Training = &TrainingDetail{}
				}
				result.Training.Method = method
				result.FieldSources["training_method"] = "config:L2:" + path
			}
			break
		}
	}

	// Hyperparameters
	if result.Training == nil {
		result.Training = &TrainingDetail{}
	}
	if result.Training.HyperParams == nil {
		result.Training.HyperParams = &HyperParams{}
	}

	if lr := getFloatFromConfig(cfg, "learning_rate"); lr > 0 {
		result.Training.HyperParams.LearningRate = lr
		result.FieldSources["hyperparams.lr"] = "config:L2:" + path
	}
	if bs := getIntFromConfig(cfg, "per_device_train_batch_size"); bs > 0 {
		result.Training.HyperParams.BatchSize = bs
		result.FieldSources["hyperparams.batch_size"] = "config:L2:" + path
	}
	if epochs := getIntFromConfig(cfg, "num_train_epochs"); epochs > 0 {
		result.Training.HyperParams.Epochs = epochs
		result.FieldSources["hyperparams.epochs"] = "config:L2:" + path
	}
	if gradAccum := getIntFromConfig(cfg, "gradient_accumulation_steps"); gradAccum > 0 {
		result.Training.HyperParams.GradAccum = gradAccum
		result.FieldSources["hyperparams.grad_accum"] = "config:L2:" + path
	}

	// LoRA config
	if loraR := getIntFromConfig(cfg, "lora_r"); loraR > 0 {
		if result.Training == nil {
			result.Training = &TrainingDetail{}
		}
		result.Training.LoRA = &LoRAConfig{
			Rank: loraR,
		}
		if alpha := getIntFromConfig(cfg, "lora_alpha"); alpha > 0 {
			result.Training.LoRA.Alpha = alpha
		}
		if result.Training.Method == "" {
			result.Training.Method = MethodLoRA
		}
		result.FieldSources["lora_config"] = "config:L2:" + path
	}

	// Dataset
	for _, key := range []string{"dataset", "dataset_name", "train_file", "data_path"} {
		if val, ok := cfg[key].(string); ok && val != "" {
			if result.Training.Data == nil {
				result.Training.Data = &DataInfo{}
			}
			if key == "dataset" || key == "dataset_name" {
				result.Training.Data.DatasetName = val
			} else {
				result.Training.Data.Path = val
			}
			result.FieldSources["data"] = "config:L2:" + path
			break
		}
	}

	// Megatron-style parallelism
	if tp := getIntFromConfig(cfg, "tensor_model_parallel_size"); tp > 0 {
		if result.Training.Parallelism == nil {
			result.Training.Parallelism = &ParallelismConfig{}
		}
		result.Training.Parallelism.TensorParallel = tp
		result.FieldSources["parallelism.tp"] = "config:L2:" + path
	}
	if pp := getIntFromConfig(cfg, "pipeline_model_parallel_size"); pp > 0 {
		if result.Training.Parallelism == nil {
			result.Training.Parallelism = &ParallelismConfig{}
		}
		result.Training.Parallelism.PipeParallel = pp
		result.FieldSources["parallelism.pp"] = "config:L2:" + path
	}
}

func (e *ConfigExtractor) calculateCoverage(result *ConfigExtractionResult) float64 {
	total := 5.0
	extracted := 0.0

	if result.Category != "" {
		extracted++
	}
	if result.Model != nil {
		extracted++
	}
	if result.Training != nil && result.Training.Method != "" {
		extracted++
	}
	if result.Training != nil && result.Training.Parallelism != nil {
		extracted++
	}
	if result.Training != nil && result.Training.HyperParams != nil {
		extracted += 0.5
	}

	return extracted / total
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mapConfigMethod(val string) TrainingMethod {
	switch strings.ToLower(val) {
	case "sft", "supervised_fine_tuning":
		return MethodSFT
	case "dpo", "direct_preference_optimization":
		return MethodDPO
	case "rlhf", "reinforcement_learning":
		return MethodRLHF
	case "lora":
		return MethodLoRA
	case "qlora":
		return MethodQLoRA
	case "pre_training", "pretraining", "pretrain":
		return MethodPreTraining
	default:
		return ""
	}
}

func getFloatFromConfig(cfg map[string]interface{}, key string) float64 {
	if val, ok := cfg[key].(float64); ok {
		return val
	}
	return 0
}

func getIntFromConfig(cfg map[string]interface{}, key string) int {
	if val, ok := cfg[key].(float64); ok {
		return int(val)
	}
	if val, ok := cfg[key].(int); ok {
		return val
	}
	return 0
}
