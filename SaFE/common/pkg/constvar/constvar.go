/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package constvar

// InferenceModelForm represents the source of the inference model
type InferenceModelForm string

const (
	// InferenceModelFormAPI represents models imported via API
	InferenceModelFormAPI InferenceModelForm = "API"
	// InferenceModelFormModelSquare represents models from model-square
	InferenceModelFormModelSquare InferenceModelForm = "ModelSquare"
)

// InferencePhaseType represents the phase of an inference service
type InferencePhaseType string

const (
	// InferencePhasePending represents the inference service is pending
	InferencePhasePending InferencePhaseType = "Pending"
	// InferencePhaseRunning represents the inference service is running (this is the normal state for inference services)
	InferencePhaseRunning InferencePhaseType = "Running"
	// InferencePhaseFailure represents the inference service failed (terminal state, will be deleted)
	InferencePhaseFailure InferencePhaseType = "Failure"
	// InferencePhaseStopped represents the inference service is stopped (terminal state, will stop workload and delete)
	InferencePhaseStopped InferencePhaseType = "Stopped"
)
