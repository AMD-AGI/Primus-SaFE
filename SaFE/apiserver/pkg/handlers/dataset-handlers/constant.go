/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dataset_handlers

// Dataset type constants
const (
	DatasetTypeSFT        = "sft"        // SFT (Supervised Fine-Tuning) dataset
	DatasetTypeDPO        = "dpo"        // DPO (Direct Preference Optimization) dataset
	DatasetTypePretrain   = "pretrain"   // Pretrain dataset
	DatasetTypeRLHF       = "rlhf"       // RLHF (Reinforcement Learning from Human Feedback) dataset
	DatasetTypeInference  = "inference"  // Inference dataset
	DatasetTypeEvaluation = "evaluation" // Evaluation dataset
	DatasetTypeOther      = "other"      // Other dataset
)

// S3 path prefix for datasets
const (
	DatasetS3Prefix = "datasets"
)

// S3 secret name for dataset download
const (
	DatasetS3Secret = "primus-safe-s3"
)

// ValidDatasetTypes contains all valid dataset types
var ValidDatasetTypes = map[string]bool{
	DatasetTypeSFT:        true,
	DatasetTypeDPO:        true,
	DatasetTypePretrain:   true,
	DatasetTypeRLHF:       true,
	DatasetTypeInference:  true,
	DatasetTypeEvaluation: true,
	DatasetTypeOther:      true,
}

// DatasetTypeDescriptions contains all dataset types with their descriptions and schemas
var DatasetTypeDescriptions = []DatasetTypeInfo{
	{
		Name:        DatasetTypeSFT,
		Description: "SFT dataset for supervised fine-tuning with instruction-response pairs",
		Schema: map[string]string{
			"instruction": "string (required) - User input or question",
			"input":       "string (optional) - Additional context",
			"output":      "string (required) - Expected model response",
		},
	},
	{
		Name:        DatasetTypeDPO,
		Description: "DPO dataset for direct preference optimization with chosen and rejected responses",
		Schema: map[string]string{
			"prompt":   "string (required) - The input prompt",
			"chosen":   "string (required) - The preferred response",
			"rejected": "string (required) - The less preferred response",
		},
	},
	{
		Name:        DatasetTypePretrain,
		Description: "Pretrain dataset for large-scale language model pretraining",
		Schema: map[string]string{
			"text": "string (required) - Raw text content for pretraining",
		},
	},
	{
		Name:        DatasetTypeRLHF,
		Description: "RLHF dataset for reinforcement learning from human feedback",
		Schema: map[string]string{
			"prompt":     "string (required) - The input prompt",
			"response":   "string (required) - Model response",
			"reward":     "float (required) - Human feedback score",
			"preference": "string (optional) - Preference ranking",
		},
	},
	{
		Name:        DatasetTypeInference,
		Description: "Inference dataset for batch inference tasks",
		Schema: map[string]string{
			"id":     "string (optional) - Unique identifier for the request",
			"prompt": "string (required) - Input prompt for inference",
		},
	},
	{
		Name:        DatasetTypeEvaluation,
		Description: "Evaluation dataset for model benchmarking and testing",
		Schema: map[string]string{
			"question":       "string (required) - Test question or prompt",
			"answer":         "string (required) - Expected answer",
			"category":       "string (optional) - Category or topic",
			"difficulty":     "string (optional) - Difficulty level",
			"reference":      "string (optional) - Reference or source",
			"answer_choices": "array (optional) - Multiple choice options",
		},
	},
	{
		Name:        DatasetTypeOther,
		Description: "Custom dataset format for other use cases",
		Schema: map[string]string{
			"data": "any - Custom data structure based on your needs",
		},
	},
}

// IsValidDatasetType checks if the given type is a valid dataset type
func IsValidDatasetType(t string) bool {
	return ValidDatasetTypes[t]
}
