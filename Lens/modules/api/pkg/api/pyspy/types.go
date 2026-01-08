// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package pyspy

import "time"

// CreateTaskRequest represents a request to create a py-spy sampling task
type CreateTaskRequest struct {
	Cluster      string `json:"cluster"`                      // Target cluster (required for multi-cluster)
	PodUID       string `json:"pod_uid" binding:"required"`
	PodName      string `json:"pod_name"`
	PodNamespace string `json:"pod_namespace"`
	NodeName     string `json:"node_name" binding:"required"` // Target node
	PID          int    `json:"pid" binding:"required"`       // Host PID
	Duration     int    `json:"duration"`                     // seconds (default: 30)
	Rate         int    `json:"rate"`                         // Hz (default: 100)
	Format       string `json:"format"`                       // flamegraph, speedscope, raw
	Native       bool   `json:"native"`
	SubProcesses bool   `json:"subprocesses"`
}

// SetDefaults sets default values for CreateTaskRequest
func (r *CreateTaskRequest) SetDefaults() {
	if r.Duration <= 0 {
		r.Duration = 30
	}
	if r.Rate <= 0 {
		r.Rate = 100
	}
	if r.Format == "" {
		r.Format = "flamegraph"
	}
}

// TaskResponse represents a py-spy task response
type TaskResponse struct {
	TaskID       string     `json:"task_id"`
	Status       string     `json:"status"`
	PodUID       string     `json:"pod_uid"`
	PodName      string     `json:"pod_name"`
	PodNamespace string     `json:"pod_namespace"`
	NodeName     string     `json:"node_name"`
	PID          int        `json:"pid"`
	Duration     int        `json:"duration"`
	Format       string     `json:"format"`
	OutputFile   string     `json:"output_file,omitempty"`
	FileSize     int64      `json:"file_size,omitempty"`
	Error        string     `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	FilePath     string     `json:"file_path,omitempty"` // Download URL path
}

// ListTasksRequest represents a request to list py-spy tasks
type ListTasksRequest struct {
	Cluster      string `form:"cluster" json:"cluster"`
	PodUID       string `form:"pod_uid" json:"pod_uid"`
	PodNamespace string `form:"pod_namespace" json:"pod_namespace"`
	NodeName     string `form:"node_name" json:"node_name"`
	Status       string `form:"status" json:"status"`
	Limit        int    `form:"limit" json:"limit"`
	Offset       int    `form:"offset" json:"offset"`
}

// SetDefaults sets default values for ListTasksRequest
func (r *ListTasksRequest) SetDefaults() {
	if r.Limit <= 0 {
		r.Limit = 50
	}
	if r.Limit > 100 {
		r.Limit = 100
	}
}

// ListTasksResponse represents a list of py-spy tasks
type ListTasksResponse struct {
	Tasks  []TaskResponse `json:"tasks"`
	Total  int64          `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

// CancelTaskRequest represents a request to cancel a py-spy task
type CancelTaskRequest struct {
	Reason string `json:"reason,omitempty"`
}

// TaskStatusResponse represents a simple status response
type TaskStatusResponse struct {
	TaskID  string `json:"task_id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

