/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
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

// Dataset status constants
const (
	DatasetStatusReady = "Ready" // Dataset is ready for use
)

// S3 path prefix for datasets
const (
	DatasetS3Prefix = "datasets"
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

// IsValidDatasetType checks if the given type is a valid dataset type
func IsValidDatasetType(t string) bool {
	return ValidDatasetTypes[t]
}
