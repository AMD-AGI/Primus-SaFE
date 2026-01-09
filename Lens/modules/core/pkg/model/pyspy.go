// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import "time"

// PySpyOutputFormat represents the output format of py-spy
type PySpyOutputFormat string

const (
	PySpyFormatFlamegraph PySpyOutputFormat = "flamegraph"
	PySpyFormatSpeedscope PySpyOutputFormat = "speedscope"
	PySpyFormatRaw        PySpyOutputFormat = "raw"
)

// PySpyTaskExt represents ext field for pyspy_sample tasks
// Stored in workload_task_state.ext JSONB field
type PySpyTaskExt struct {
	// Task ID (for reference)
	TaskID string `json:"task_id"`

	// Target node (Jobs module uses this to route the request)
	TargetNodeName string `json:"target_node_name"`

	// Pod information
	PodUID       string `json:"pod_uid"`
	PodName      string `json:"pod_name"`
	PodNamespace string `json:"pod_namespace"`

	// Process information
	HostPID      int `json:"host_pid"`
	ContainerPID int `json:"container_pid,omitempty"`

	// Sampling configuration
	Duration     int    `json:"duration"`
	Rate         int    `json:"rate"`
	Format       string `json:"format"`
	Native       bool   `json:"native"`
	SubProcesses bool   `json:"subprocesses"`

	// Result (filled after completion)
	OutputFile  string `json:"output_file,omitempty"`
	FileSize    int64  `json:"file_size,omitempty"`
	Error       string `json:"error,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
}

// PySpyCompatibility represents py-spy compatibility check result
type PySpyCompatibility struct {
	Supported       bool      `json:"supported"`
	Reason          string    `json:"reason,omitempty"`
	PythonProcesses []int     `json:"python_processes,omitempty"`
	Capabilities    []string  `json:"capabilities,omitempty"`
	CheckedAt       time.Time `json:"checked_at"`
}

// PySpyCheckRequest represents a request to check py-spy compatibility
type PySpyCheckRequest struct {
	PodUID      string `json:"pod_uid" binding:"required"`
	ContainerID string `json:"container_id,omitempty"`
}

// PySpyCheckResponse represents py-spy compatibility check response
type PySpyCheckResponse struct {
	Supported       bool     `json:"supported"`
	Reason          string   `json:"reason,omitempty"`
	PythonProcesses []int    `json:"python_processes,omitempty"`
	Capabilities    []string `json:"capabilities,omitempty"`
}

// PySpySampleRequest represents API request to start py-spy sampling
type PySpySampleRequest struct {
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

// SetDefaults sets default values for PySpySampleRequest
func (r *PySpySampleRequest) SetDefaults() {
	if r.Duration <= 0 {
		r.Duration = 30
	}
	if r.Rate <= 0 {
		r.Rate = 100
	}
	if r.Format == "" {
		r.Format = string(PySpyFormatFlamegraph)
	}
}

// PySpySampleResponse represents API response after creating a task
type PySpySampleResponse struct {
	TaskID      string    `json:"task_id"`     // workload_task_state.workload_uid
	Status      string    `json:"status"`      // pending, running, completed, failed
	NodeName    string    `json:"node_name"`
	FilePath    string    `json:"file_path,omitempty"`
	FileSize    int64     `json:"file_size,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// PySpyExecuteRequest represents request for node-exporter execute API
// Called by Jobs module to execute py-spy on target node
type PySpyExecuteRequest struct {
	TaskID       string `json:"task_id"`
	PodUID       string `json:"pod_uid"`
	HostPID      int    `json:"host_pid"`
	ContainerPID int    `json:"container_pid,omitempty"`
	Duration     int    `json:"duration"`
	Rate         int    `json:"rate"`
	Format       string `json:"format"`
	Native       bool   `json:"native"`
	SubProcesses bool   `json:"subprocesses"`
}

// PySpyExecuteResponse represents response from node-exporter execute API
type PySpyExecuteResponse struct {
	Success    bool   `json:"success"`
	OutputFile string `json:"output_file"`
	FileSize   int64  `json:"file_size"`
	Error      string `json:"error,omitempty"`
}

// PySpyFileListRequest represents a request to list py-spy files
type PySpyFileListRequest struct {
	PodUID   string `json:"pod_uid,omitempty"`
	TaskID   string `json:"task_id,omitempty"`
	NodeName string `json:"node_name,omitempty"`
	Limit    int    `json:"limit"`
	Offset   int    `json:"offset"`
}

// PySpyFile represents a py-spy profiling file
type PySpyFile struct {
	TaskID       string    `json:"task_id"`
	FileName     string    `json:"file_name"`
	FilePath     string    `json:"file_path"`
	FileSize     int64     `json:"file_size"`
	Format       string    `json:"format"`
	PodUID       string    `json:"pod_uid"`
	PodName      string    `json:"pod_name"`
	PodNamespace string    `json:"pod_namespace"`
	NodeName     string    `json:"node_name"`
	PID          int       `json:"pid"`
	CreatedAt    time.Time `json:"created_at"`
}

// PySpyTaskListRequest represents a request to list py-spy tasks
type PySpyTaskListRequest struct {
	PodUID       string `json:"pod_uid,omitempty"`
	PodNamespace string `json:"pod_namespace,omitempty"`
	NodeName     string `json:"node_name,omitempty"`
	Status       string `json:"status,omitempty"`
	Limit        int    `json:"limit"`
	Offset       int    `json:"offset"`
}

// PySpyTask represents a py-spy task summary for listing
type PySpyTask struct {
	TaskID       string    `json:"task_id"`
	Status       string    `json:"status"`
	PodUID       string    `json:"pod_uid"`
	PodName      string    `json:"pod_name"`
	PodNamespace string    `json:"pod_namespace"`
	NodeName     string    `json:"node_name"`
	PID          int       `json:"pid"`
	Duration     int       `json:"duration"`
	Format       string    `json:"format"`
	OutputFile   string    `json:"output_file,omitempty"`
	FileSize     int64     `json:"file_size,omitempty"`
	Error        string    `json:"error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	StartedAt    time.Time `json:"started_at,omitempty"`
	CompletedAt  time.Time `json:"completed_at,omitempty"`
}

