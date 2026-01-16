/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

// DatasetType is the type for dataset type
type DatasetType string

// Dataset type constants
const (
	DatasetTypeSFT        DatasetType = "sft"
	DatasetTypeDPO        DatasetType = "dpo"
	DatasetTypePretrain   DatasetType = "pretrain"
	DatasetTypeRLHF       DatasetType = "rlhf"
	DatasetTypeInference  DatasetType = "inference"
	DatasetTypeEvaluation DatasetType = "evaluation"
	DatasetTypeOther      DatasetType = "other"
)

// DatasetTypeMetadata contains metadata about a dataset type
type DatasetTypeMetadata struct {
	Description string            // Human readable description
	Schema      map[string]string // Field schema for this type
}

// DatasetTypes registry with metadata
var DatasetTypes = map[DatasetType]DatasetTypeMetadata{
	DatasetTypeSFT: {
		Description: "SFT dataset for supervised fine-tuning with instruction-response pairs",
		Schema: map[string]string{
			"instruction": "string (required) - User input or question",
			"input":       "string (optional) - Additional context",
			"output":      "string (required) - Expected model response",
		},
	},
	DatasetTypeDPO: {
		Description: "DPO dataset for direct preference optimization with chosen and rejected responses",
		Schema: map[string]string{
			"prompt":   "string (required) - The input prompt",
			"chosen":   "string (required) - The preferred response",
			"rejected": "string (required) - The less preferred response",
		},
	},
	DatasetTypePretrain: {
		Description: "Pretrain dataset for large-scale language model pretraining",
		Schema: map[string]string{
			"text": "string (required) - Raw text content for pretraining",
		},
	},
	DatasetTypeRLHF: {
		Description: "RLHF dataset for reinforcement learning from human feedback",
		Schema: map[string]string{
			"prompt":     "string (required) - The input prompt",
			"response":   "string (required) - Model response",
			"reward":     "float (required) - Human feedback score",
			"preference": "string (optional) - Preference ranking",
		},
	},
	DatasetTypeInference: {
		Description: "Inference dataset for batch inference tasks",
		Schema: map[string]string{
			"id":     "string (optional) - Unique identifier for the request",
			"prompt": "string (required) - Input prompt for inference",
		},
	},
	DatasetTypeEvaluation: {
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
	DatasetTypeOther: {
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
	_, ok := DatasetTypes[DatasetType(t)]
	return ok
}

// GetDatasetTypeDescriptions returns all dataset types with their descriptions and schemas
func GetDatasetTypeDescriptions() []DatasetTypeInfo {
	types := make([]DatasetTypeInfo, 0, len(DatasetTypes))
	for typeName, metadata := range DatasetTypes {
		types = append(types, DatasetTypeInfo{
			Name:        string(typeName),
			Description: metadata.Description,
			Schema:      metadata.Schema,
		})
	}
	return types
}

