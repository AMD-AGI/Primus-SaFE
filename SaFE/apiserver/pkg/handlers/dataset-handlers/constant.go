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

// DatasetTypeDescriptions contains descriptions for all dataset types
var DatasetTypeDescriptions = []DatasetTypeInfo{
	{Value: DatasetTypeSFT, Label: "SFT (Supervised Fine-Tuning)", Description: "Dataset for supervised fine-tuning"},
	{Value: DatasetTypeDPO, Label: "DPO (Direct Preference Optimization)", Description: "Dataset for preference optimization"},
	{Value: DatasetTypePretrain, Label: "Pretrain", Description: "Dataset for large-scale pretraining"},
	{Value: DatasetTypeRLHF, Label: "RLHF", Description: "Dataset for reinforcement learning from human feedback"},
	{Value: DatasetTypeInference, Label: "Inference", Description: "Dataset for batch inference"},
	{Value: DatasetTypeEvaluation, Label: "Evaluation", Description: "Dataset for model evaluation and benchmarking"},
	{Value: DatasetTypeOther, Label: "Other", Description: "Other types of datasets"},
}

// DatasetTemplates contains template examples for each dataset type
var DatasetTemplates = map[string]DatasetTemplateResponse{
	DatasetTypeSFT: {
		Type:        DatasetTypeSFT,
		Description: "SFT dataset for supervised fine-tuning with instruction-response pairs",
		Format:      "jsonl",
		Schema: map[string]string{
			"instruction": "string (required) - User input or question",
			"input":       "string (optional) - Additional context",
			"output":      "string (required) - Expected model response",
		},
		Example: `{"instruction": "Translate to English", "input": "Bonjour", "output": "Hello"}
{"instruction": "Write a poem about spring", "output": "Spring arrives with gentle rain..."}`,
	},
	DatasetTypeDPO: {
		Type:        DatasetTypeDPO,
		Description: "DPO dataset for direct preference optimization with chosen and rejected responses",
		Format:      "jsonl",
		Schema: map[string]string{
			"prompt":   "string (required) - The input prompt",
			"chosen":   "string (required) - The preferred response",
			"rejected": "string (required) - The less preferred response",
		},
		Example: `{"prompt": "Explain quantum computing", "chosen": "Quantum computing uses quantum bits...", "rejected": "It's just faster computers..."}`,
	},
	DatasetTypePretrain: {
		Type:        DatasetTypePretrain,
		Description: "Pretrain dataset for large-scale language model pretraining",
		Format:      "jsonl or txt",
		Schema: map[string]string{
			"text": "string (required) - Raw text content for pretraining",
		},
		Example: `{"text": "The quick brown fox jumps over the lazy dog. This is a sample document for pretraining."}`,
	},
	DatasetTypeRLHF: {
		Type:        DatasetTypeRLHF,
		Description: "RLHF dataset for reinforcement learning from human feedback",
		Format:      "jsonl",
		Schema: map[string]string{
			"prompt":     "string (required) - The input prompt",
			"response":   "string (required) - Model response",
			"reward":     "float (required) - Human feedback score",
			"preference": "string (optional) - Preference ranking",
		},
		Example: `{"prompt": "Write a helpful response", "response": "Here's how I can help...", "reward": 0.85}`,
	},
	DatasetTypeInference: {
		Type:        DatasetTypeInference,
		Description: "Inference dataset for batch inference tasks",
		Format:      "jsonl",
		Schema: map[string]string{
			"id":     "string (optional) - Unique identifier for the request",
			"prompt": "string (required) - Input prompt for inference",
		},
		Example: `{"id": "req_001", "prompt": "Summarize this article: ..."}
{"id": "req_002", "prompt": "Translate: Hello world"}`,
	},
	DatasetTypeEvaluation: {
		Type:        DatasetTypeEvaluation,
		Description: "Evaluation dataset for model benchmarking and testing",
		Format:      "jsonl",
		Schema: map[string]string{
			"question":       "string (required) - Test question or prompt",
			"answer":         "string (required) - Expected answer",
			"category":       "string (optional) - Category or topic",
			"difficulty":     "string (optional) - Difficulty level",
			"reference":      "string (optional) - Reference or source",
			"answer_choices": "array (optional) - Multiple choice options",
		},
		Example: `{"question": "What is 2+2?", "answer": "4", "category": "math", "difficulty": "easy"}`,
	},
	DatasetTypeOther: {
		Type:        DatasetTypeOther,
		Description: "Custom dataset format for other use cases",
		Format:      "jsonl, csv, or txt",
		Schema: map[string]string{
			"data": "any - Custom data structure based on your needs",
		},
		Example: `{"data": "Your custom data format here"}`,
	},
}

// IsValidDatasetType checks if the given type is a valid dataset type
func IsValidDatasetType(t string) bool {
	return ValidDatasetTypes[t]
}
