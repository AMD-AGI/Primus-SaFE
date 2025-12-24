package detection

import (
	"fmt"
	"strings"
)

// TrainingConfig extracted training configuration from wandb data
type TrainingConfig struct {
	ModelName         string `json:"model_name,omitempty"`
	TrainingMode      string `json:"training_mode,omitempty"`
	Precision         string `json:"precision,omitempty"`
	BuildingFramework string `json:"building_framework,omitempty"`
}

// ConfigExtractor extracts training configuration from WandB evidence
type ConfigExtractor struct{}

// NewConfigExtractor creates a new config extractor
func NewConfigExtractor() *ConfigExtractor {
	return &ConfigExtractor{}
}

// ExtractTrainingConfig extracts training configuration from WandB detection request
func (e *ConfigExtractor) ExtractTrainingConfig(req *WandBDetectionRequest, detectionResult *DetectionResult) *TrainingConfig {
	config := &TrainingConfig{}

	// Extract model name
	config.ModelName = e.extractModelName(req.Evidence.WandB.Config, req.Evidence.Environment)

	// Extract training mode
	config.TrainingMode = e.extractTrainingMode(
		req.Evidence.WandB.Config,
		req.Evidence.Environment,
		detectionResult,
	)

	// Extract precision
	config.Precision = e.extractPrecision(req.Evidence.WandB.Config, req.Evidence.Environment)

	// Extract building framework
	config.BuildingFramework = e.extractBuildingFramework(req, detectionResult)

	return config
}

// extractModelName extracts model name from config or environment variables
func (e *ConfigExtractor) extractModelName(config map[string]interface{}, env map[string]string) string {
	// Priority 1: from config
	configKeys := []string{
		"model_name",
		"model",
		"model_id",
		"model_type",
		"model_name_or_path",
		"pretrained_model_name",
		"base_model",
		"pretrained_model_name_or_path",
	}

	for _, key := range configKeys {
		if val, ok := config[key]; ok && val != nil {
			strVal := fmt.Sprintf("%v", val)
			if strVal != "" && strVal != "<nil>" {
				return e.normalizeModelName(strVal)
			}
		}
	}

	// Priority 2: from environment variables
	envKeys := []string{
		"MODEL_NAME",
		"MODEL_ID",
		"PRIMUS_MODEL",
		"HF_MODEL_NAME",
		"PRETRAINED_MODEL",
	}

	for _, key := range envKeys {
		if val, ok := env[key]; ok && val != "" {
			return e.normalizeModelName(val)
		}
	}

	// Priority 3: try to extract from nested config
	if modelConfig, ok := config["model"].(map[string]interface{}); ok {
		for _, key := range []string{"name", "model_name", "id"} {
			if val, ok := modelConfig[key]; ok && val != nil {
				strVal := fmt.Sprintf("%v", val)
				if strVal != "" && strVal != "<nil>" {
					return e.normalizeModelName(strVal)
				}
			}
		}
	}

	return ""
}

// normalizeModelName normalizes model name to standard format
func (e *ConfigExtractor) normalizeModelName(name string) string {
	// Extract just the model name if it's a full path
	if strings.Contains(name, "/") {
		parts := strings.Split(name, "/")
		name = parts[len(parts)-1]
	}

	return name
}

// extractTrainingMode infers training mode from config and environment
func (e *ConfigExtractor) extractTrainingMode(config map[string]interface{}, env map[string]string, result *DetectionResult) string {
	// Priority 1: explicit configuration
	explicitKeys := []string{"training_mode", "mode", "train_mode"}
	for _, key := range explicitKeys {
		if val, ok := config[key]; ok && val != nil {
			strVal := strings.ToLower(fmt.Sprintf("%v", val))
			if strVal != "" && strVal != "<nil>" {
				return e.normalizeTrainingMode(strVal)
			}
		}
	}

	// Priority 2: from environment variables
	envKeys := []string{"TRAINING_MODE", "MODE", "TRAIN_MODE"}
	for _, key := range envKeys {
		if val, ok := env[key]; ok && val != "" {
			return e.normalizeTrainingMode(strings.ToLower(val))
		}
	}

	// Priority 3: infer from LoRA/QLoRA configuration
	loraKeys := []string{"lora_r", "lora_alpha", "lora_dropout", "peft_config", "lora_config", "use_lora"}
	hasLora := false
	for _, key := range loraKeys {
		if _, exists := config[key]; exists {
			hasLora = true
			break
		}
	}

	if hasLora {
		// Check if QLoRA (quantized LoRA)
		if e.isQuantized(config) {
			return "finetune_qlora"
		}
		return "finetune_lora"
	}

	// Priority 4: infer from config content
	configStr := strings.ToLower(fmt.Sprintf("%v", config))

	// Check for finetune indicators
	finetuneIndicators := []string{"finetune", "fine_tune", "fine-tune", "sft", "rlhf", "dpo", "ppo"}
	for _, indicator := range finetuneIndicators {
		if strings.Contains(configStr, indicator) {
			return "finetune_fw"
		}
	}

	// Priority 5: infer from framework
	if result != nil && result.Framework != "" {
		framework := strings.ToLower(result.Framework)
		if strings.Contains(framework, "transformers") {
			return "hf_pretrain"
		}
	}

	// Default to pretrain
	return "pretrain"
}

// isQuantized checks if the model is quantized (for QLoRA detection)
func (e *ConfigExtractor) isQuantized(config map[string]interface{}) bool {
	quantKeys := []string{
		"load_in_4bit", "load_in_8bit",
		"quantization_config", "bnb_config",
		"use_qlora", "use_4bit", "use_8bit",
	}

	for _, key := range quantKeys {
		if val, exists := config[key]; exists {
			if boolVal, ok := val.(bool); ok && boolVal {
				return true
			}
			// Also check for truthy values
			strVal := strings.ToLower(fmt.Sprintf("%v", val))
			if strVal == "true" || strVal == "1" {
				return true
			}
		}
	}

	return false
}

// normalizeTrainingMode normalizes training mode to standard format
func (e *ConfigExtractor) normalizeTrainingMode(mode string) string {
	modeMap := map[string]string{
		"pretrain":       "pretrain",
		"pre_train":      "pretrain",
		"pre-train":      "pretrain",
		"pretraining":    "pretrain",
		"hf_pretrain":    "hf_pretrain",
		"finetune":       "finetune_fw",
		"fine_tune":      "finetune_fw",
		"fine-tune":      "finetune_fw",
		"finetuning":     "finetune_fw",
		"sft":            "finetune_fw",
		"lora":           "finetune_lora",
		"finetune_lora":  "finetune_lora",
		"qlora":          "finetune_qlora",
		"finetune_qlora": "finetune_qlora",
	}

	if normalized, ok := modeMap[mode]; ok {
		return normalized
	}

	return mode
}

// extractPrecision extracts precision/dtype from config or environment
func (e *ConfigExtractor) extractPrecision(config map[string]interface{}, env map[string]string) string {
	// Priority 1: direct precision config keys
	precisionKeys := []string{
		"precision", "dtype", "fp_precision",
		"mixed_precision", "compute_dtype",
		"torch_dtype", "model_dtype",
	}

	for _, key := range precisionKeys {
		if val, ok := config[key]; ok && val != nil {
			strVal := strings.ToLower(fmt.Sprintf("%v", val))
			if precision := e.normalizePrecision(strVal); precision != "" {
				return precision
			}
		}
	}

	// Priority 2: boolean flags
	boolFlags := map[string]string{
		"fp8":  "fp8",
		"bf16": "bf16",
		"fp16": "fp16",
		"fp32": "fp32",
	}

	for key, precision := range boolFlags {
		if val, exists := config[key]; exists {
			if boolVal, ok := val.(bool); ok && boolVal {
				return precision
			}
			strVal := strings.ToLower(fmt.Sprintf("%v", val))
			if strVal == "true" || strVal == "1" {
				return precision
			}
		}
	}

	// Priority 3: environment variables
	envKeys := []string{"PRECISION", "DTYPE", "COMPUTE_DTYPE", "TORCH_DTYPE"}
	for _, key := range envKeys {
		if val, ok := env[key]; ok && val != "" {
			if precision := e.normalizePrecision(strings.ToLower(val)); precision != "" {
				return precision
			}
		}
	}

	// Priority 4: check nested training_args
	if trainingArgs, ok := config["training_args"].(map[string]interface{}); ok {
		for _, key := range []string{"bf16", "fp16"} {
			if val, exists := trainingArgs[key]; exists {
				if boolVal, ok := val.(bool); ok && boolVal {
					return key
				}
			}
		}
	}

	return ""
}

// normalizePrecision normalizes precision string to standard format
func (e *ConfigExtractor) normalizePrecision(precision string) string {
	precisionMap := map[string]string{
		"fp8":             "fp8",
		"float8":          "fp8",
		"bf16":            "bf16",
		"bfloat16":        "bf16",
		"fp16":            "fp16",
		"float16":         "fp16",
		"half":            "fp16",
		"fp32":            "fp32",
		"float32":         "fp32",
		"float":           "fp32",
		"mixed":           "mixed",
		"mixed_precision": "mixed",
	}

	// Direct match
	if normalized, ok := precisionMap[precision]; ok {
		return normalized
	}

	// Check if contains precision keywords
	for key, normalized := range precisionMap {
		if strings.Contains(precision, key) {
			return normalized
		}
	}

	return ""
}

// extractBuildingFramework extracts building framework from detection result and environment
func (e *ConfigExtractor) extractBuildingFramework(req *WandBDetectionRequest, result *DetectionResult) string {
	var buildingFramework string

	// Priority 1: use detected wrapper framework
	if result != nil && result.WrapperFramework != "" {
		buildingFramework = result.WrapperFramework
	}

	// Priority 2: from hints
	if buildingFramework == "" && len(req.Hints.WrapperFrameworks) > 0 {
		buildingFramework = req.Hints.WrapperFrameworks[0]
	}

	// If no wrapper framework, check base framework
	if buildingFramework == "" {
		if result != nil && result.BaseFramework != "" {
			buildingFramework = result.BaseFramework
		} else if len(req.Hints.BaseFrameworks) > 0 {
			buildingFramework = req.Hints.BaseFrameworks[0]
		}
	}

	if buildingFramework == "" {
		return ""
	}

	// Try to get version from environment
	env := req.Evidence.Environment
	versionEnvKeys := map[string][]string{
		"primus":    {"PRIMUS_VERSION", "BUILDING_FRAMEWORK_VERSION"},
		"jax":       {"JAX_VERSION"},
		"deepspeed": {"DEEPSPEED_VERSION", "DS_VERSION"},
	}

	if keys, ok := versionEnvKeys[buildingFramework]; ok {
		for _, key := range keys {
			if version, exists := env[key]; exists && version != "" {
				return fmt.Sprintf("%s-%s", buildingFramework, version)
			}
		}
	}

	// Also check wrapper_frameworks evidence for version
	if wrapperInfo, ok := req.Evidence.WrapperFrameworks[buildingFramework]; ok {
		if version, exists := wrapperInfo["version"]; exists && version != nil {
			versionStr := fmt.Sprintf("%v", version)
			if versionStr != "" && versionStr != "unknown" && versionStr != "<nil>" {
				return fmt.Sprintf("%s-%s", buildingFramework, versionStr)
			}
		}
	}

	// Also check base_frameworks evidence for version
	if baseInfo, ok := req.Evidence.BaseFrameworks[buildingFramework]; ok {
		if version, exists := baseInfo["version"]; exists && version != nil {
			versionStr := fmt.Sprintf("%v", version)
			if versionStr != "" && versionStr != "unknown" && versionStr != "<nil>" {
				return fmt.Sprintf("%s-%s", buildingFramework, versionStr)
			}
		}
	}

	return buildingFramework
}
