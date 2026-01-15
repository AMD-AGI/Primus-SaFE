/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

// DatasetType represents a dataset type with its metadata
type DatasetType struct {
	Name        string            // Type identifier (e.g., "sft", "dpo")
	Description string            // Human readable description
	Schema      map[string]string // Field schema for this type
}

// Dataset types registry
var DatasetTypes = map[string]DatasetType{
	"sft": {
		Name:        "sft",
		Description: "SFT dataset for supervised fine-tuning with instruction-response pairs",
		Schema: map[string]string{
			"instruction": "string (required) - User input or question",
			"input":       "string (optional) - Additional context",
			"output":      "string (required) - Expected model response",
		},
	},
	"dpo": {
		Name:        "dpo",
		Description: "DPO dataset for direct preference optimization with chosen and rejected responses",
		Schema: map[string]string{
			"prompt":   "string (required) - The input prompt",
			"chosen":   "string (required) - The preferred response",
			"rejected": "string (required) - The less preferred response",
		},
	},
	"pretrain": {
		Name:        "pretrain",
		Description: "Pretrain dataset for large-scale language model pretraining",
		Schema: map[string]string{
			"text": "string (required) - Raw text content for pretraining",
		},
	},
	"rlhf": {
		Name:        "rlhf",
		Description: "RLHF dataset for reinforcement learning from human feedback",
		Schema: map[string]string{
			"prompt":     "string (required) - The input prompt",
			"response":   "string (required) - Model response",
			"reward":     "float (required) - Human feedback score",
			"preference": "string (optional) - Preference ranking",
		},
	},
	"inference": {
		Name:        "inference",
		Description: "Inference dataset for batch inference tasks",
		Schema: map[string]string{
			"id":     "string (optional) - Unique identifier for the request",
			"prompt": "string (required) - Input prompt for inference",
		},
	},
	"evaluation": {
		Name:        "evaluation",
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
	"other": {
		Name:        "other",
		Description: "Custom dataset format for other use cases",
		Schema: map[string]string{
			"data": "any - Custom data structure based on your needs",
		},
	},
}

// S3 path prefix for datasets
const (
	DatasetS3Prefix = "datasets"
)

// S3 secret name for dataset download
const (
	DatasetS3Secret = "primus-safe-s3"
)

// IsValidDatasetType checks if the given type is a valid dataset type
func IsValidDatasetType(t string) bool {
	_, ok := DatasetTypes[t]
	return ok
}

// GetDatasetTypeDescriptions returns all dataset types with their descriptions and schemas
func GetDatasetTypeDescriptions() []DatasetTypeInfo {
	types := make([]DatasetTypeInfo, 0, len(DatasetTypes))
	for _, dt := range DatasetTypes {
		types = append(types, DatasetTypeInfo{
			Name:        dt.Name,
			Description: dt.Description,
			Schema:      dt.Schema,
		})
	}
	return types
}

