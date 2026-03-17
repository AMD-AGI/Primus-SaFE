// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package dag

// BuildDAGTemplate returns the canonical sub-task template for intent analysis.
//
// DAG topology:
//
//	T1 (image_analysis) ──> T2 (label_env_collection)
//	                    ──> T3 (wait_pod_ready)
//	T2 + T3 ──────────────> T4 (process_collection)
//	T4 ───────────────────> T5 (assemble_submit)
//	T5 ───────────────────> T6 (wait_classification)
func BuildDAGTemplate() []*SubTask {
	return []*SubTask{
		{
			TaskType:     TaskTypeImageAnalysis,
			Status:       SubStatusPending,
			Dependencies: nil,
		},
		{
			TaskType:     TaskTypeLabelEnvCollection,
			Status:       SubStatusPending,
			Dependencies: []TaskType{TaskTypeImageAnalysis},
		},
		{
			TaskType:     TaskTypeWaitPodReady,
			Status:       SubStatusPending,
			Dependencies: []TaskType{TaskTypeImageAnalysis},
		},
		{
			TaskType:     TaskTypeProcessCollection,
			Status:       SubStatusPending,
			Dependencies: []TaskType{TaskTypeLabelEnvCollection, TaskTypeWaitPodReady},
		},
		{
			TaskType:     TaskTypeAssembleSubmit,
			Status:       SubStatusPending,
			Dependencies: []TaskType{TaskTypeProcessCollection},
		},
		{
			TaskType:     TaskTypeWaitClassification,
			Status:       SubStatusPending,
			Dependencies: []TaskType{TaskTypeAssembleSubmit},
		},
	}
}
