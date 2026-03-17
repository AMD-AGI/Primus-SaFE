// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package dag

import "time"

// TaskType identifies the kind of work a sub-task performs.
type TaskType string

const (
	TaskTypeImageAnalysis      TaskType = "image_analysis"
	TaskTypeLabelEnvCollection TaskType = "label_env_collection"
	TaskTypeWaitPodReady       TaskType = "wait_pod_ready"
	TaskTypeProcessCollection  TaskType = "process_collection"
	TaskTypeAssembleSubmit     TaskType = "assemble_submit"
	TaskTypeWaitClassification TaskType = "wait_classification"
)

// MasterTaskStatus represents the lifecycle state of a master task.
type MasterTaskStatus string

const (
	MasterStatusPending   MasterTaskStatus = "pending"
	MasterStatusRunning   MasterTaskStatus = "running"
	MasterStatusCompleted MasterTaskStatus = "completed"
	MasterStatusFailed    MasterTaskStatus = "failed"
	MasterStatusSkipped   MasterTaskStatus = "skipped"
)

// SubTaskStatus represents the lifecycle state of a single sub-task.
type SubTaskStatus string

const (
	SubStatusPending   SubTaskStatus = "pending"
	SubStatusRunning   SubTaskStatus = "running"
	SubStatusCompleted SubTaskStatus = "completed"
	SubStatusFailed    SubTaskStatus = "failed"
	SubStatusSkipped   SubTaskStatus = "skipped"
)

// MasterTask groups all sub-tasks for a single workload intent analysis.
type MasterTask struct {
	ID          int64            `json:"id"`
	ClusterID   string           `json:"cluster_id"`
	WorkloadUID string           `json:"workload_uid"`
	Status      MasterTaskStatus `json:"status"`
	SubTasks    []*SubTask       `json:"sub_tasks"`
	CreatedAt   time.Time        `json:"created_at"`
}

// SubTask is one step in the DAG.
type SubTask struct {
	ID           int64                  `json:"id"`
	MasterTaskID int64                  `json:"master_task_id"`
	TaskType     TaskType               `json:"task_type"`
	Status       SubTaskStatus          `json:"status"`
	Dependencies []TaskType             `json:"dependencies"`
	Result       map[string]interface{} `json:"result,omitempty"`
	Error        string                 `json:"error,omitempty"`
	StartedAt    *time.Time             `json:"started_at,omitempty"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
}

// IsTerminal returns true if the sub-task has reached a final state.
func (s *SubTask) IsTerminal() bool {
	return s.Status == SubStatusCompleted || s.Status == SubStatusFailed || s.Status == SubStatusSkipped
}

// AllDependenciesMet returns true when every dependency has status completed.
func (s *SubTask) AllDependenciesMet(taskMap map[TaskType]*SubTask) bool {
	for _, dep := range s.Dependencies {
		dt, ok := taskMap[dep]
		if !ok || dt.Status != SubStatusCompleted {
			return false
		}
	}
	return true
}
