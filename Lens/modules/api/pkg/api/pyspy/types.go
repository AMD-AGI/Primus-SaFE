// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package pyspy

import "time"

// CreateTaskRequest represents a request to create a py-spy sampling task
type CreateTaskRequest struct {
	Cluster      string `json:"cluster"`                      // Target cluster (required for multi-cluster)
	WorkloadUID  string `json:"workload_uid"`                 // Parent workload UID (optional, for querying history)
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
	WorkloadUID  string `form:"workload_uid" json:"workload_uid"` // Query tasks by workload UID (recommended for history)
	PodUID       string `form:"pod_uid" json:"pod_uid"`           // Optional: filter by specific pod
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

// ProcessTreeRequest represents a request to get process tree for a pod
type ProcessTreeRequest struct {
	PodUID       string `json:"pod_uid" binding:"required"`
	PodName      string `json:"pod_name"`      // Optional: pod name for faster lookup
	PodNamespace string `json:"pod_namespace"` // Optional: pod namespace for faster lookup
	Cluster      string `json:"cluster"`       // Target cluster (optional for multi-cluster)

	// Options
	IncludeEnv       bool `json:"include_env"`
	IncludeCmdline   bool `json:"include_cmdline"`
	IncludeResources bool `json:"include_resources"`
	IncludeGPU       bool `json:"include_gpu"`
}

// ProcessInfo represents detailed information about a process
type ProcessInfo struct {
	// Host-level information
	HostPID  int `json:"host_pid"`
	HostPPID int `json:"host_ppid"`

	// Container-level information
	ContainerPID  int `json:"container_pid,omitempty"`
	ContainerPPID int `json:"container_ppid,omitempty"`

	// Process details
	Cmdline string   `json:"cmdline"`
	Comm    string   `json:"comm"`
	Exe     string   `json:"exe,omitempty"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env"`
	Cwd     string   `json:"cwd,omitempty"`

	// Process state
	State   string `json:"state"`
	Threads int    `json:"threads"`

	// Resource usage
	CPUTime       uint64 `json:"cpu_time,omitempty"`
	MemoryRSS     uint64 `json:"memory_rss,omitempty"`
	MemoryVirtual uint64 `json:"memory_virtual,omitempty"`

	// Container context
	ContainerID   string `json:"container_id,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
	PodUID        string `json:"pod_uid,omitempty"`
	PodName       string `json:"pod_name,omitempty"`
	PodNamespace  string `json:"pod_namespace,omitempty"`

	// Process classification
	IsPython bool `json:"is_python"`
	IsJava   bool `json:"is_java"`

	// GPU binding information
	GPUDevices []GPUDeviceBinding `json:"gpu_devices,omitempty"`
	HasGPU     bool               `json:"has_gpu"`

	// Timestamps
	StartTime int64 `json:"start_time,omitempty"`

	// Tree structure
	Children []*ProcessInfo `json:"children,omitempty"`
}

// GPUDeviceBinding represents GPU device binding information
type GPUDeviceBinding struct {
	DeviceIndex int    `json:"device_index"`
	DeviceUUID  string `json:"device_uuid,omitempty"`
	DeviceName  string `json:"device_name,omitempty"`
}

// ContainerProcessTree represents the process tree for a container
type ContainerProcessTree struct {
	ContainerID   string       `json:"container_id"`
	ContainerName string       `json:"container_name"`
	ImageName     string       `json:"image_name,omitempty"`
	RootProcess   *ProcessInfo `json:"root_process"`
	ProcessCount  int          `json:"process_count"`
	PythonCount   int          `json:"python_count"`
}

// PodProcessTree represents the complete process tree for a pod
type PodProcessTree struct {
	// Pod information
	PodName      string `json:"pod_name"`
	PodNamespace string `json:"pod_namespace"`
	PodUID       string `json:"pod_uid"`
	NodeName     string `json:"node_name,omitempty"`

	// Container trees
	Containers []*ContainerProcessTree `json:"containers"`

	// Summary
	TotalProcesses int `json:"total_processes"`
	TotalPython    int `json:"total_python"`

	// Timestamps
	CollectedAt time.Time `json:"collected_at"`
}
