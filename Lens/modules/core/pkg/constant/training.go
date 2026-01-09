// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package constant

const (
	TrainingEventStartTrain = "StartTrain"
	TrainingPerformance     = "Performance"
)

// Training data source constants - matches database enum training_data_source
const (
	DataSourceLog        = "log"        // Parsed from application logs
	DataSourceWandB      = "wandb"      // From W&B API
	DataSourceTensorFlow = "tensorflow" // From TensorFlow/TensorBoard
)
