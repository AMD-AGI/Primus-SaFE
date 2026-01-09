// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import "time"

// ProcessInfo represents detailed information about a process
// Used for process tree API with GPU binding information
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
	Env     []string `json:"env,omitempty"`
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

// GPUDeviceBinding represents GPU device binding for a process
type GPUDeviceBinding struct {
	DevicePath string `json:"device_path"`           // e.g., /dev/dri/renderD128
	CardIndex  int    `json:"card_index"`            // e.g., 0, 1, 2
	UUID       string `json:"uuid,omitempty"`        // GPU UUID
	MarketName string `json:"market_name,omitempty"` // e.g., "AMD Instinct MI300X"
	BDF        string `json:"bdf,omitempty"`         // e.g., "0000:03:00.0"
}

// ContainerProcessTree represents the process tree for a container
type ContainerProcessTree struct {
	ContainerID   string         `json:"container_id"`
	ContainerName string         `json:"container_name"`
	ImageName     string         `json:"image_name,omitempty"`
	RootProcess   *ProcessInfo   `json:"root_process"`
	AllProcesses  []*ProcessInfo `json:"-"` // Internal use, not serialized
	ProcessCount  int            `json:"process_count"`
	PythonCount   int            `json:"python_count"`
}

// PodProcessTree represents the complete process tree for a pod
type PodProcessTree struct {
	PodName        string                  `json:"pod_name"`
	PodNamespace   string                  `json:"pod_namespace"`
	PodUID         string                  `json:"pod_uid"`
	NodeName       string                  `json:"node_name,omitempty"`
	Containers     []*ContainerProcessTree `json:"containers"`
	TotalProcesses int                     `json:"total_processes"`
	TotalPython    int                     `json:"total_python"`
	CollectedAt    time.Time               `json:"collected_at"`
}

// ProcessTreeRequest represents a request to get process tree
type ProcessTreeRequest struct {
	PodUID          string `json:"pod_uid" binding:"required"`
	ContainerID     string `json:"container_id,omitempty"`
	IncludeCmdline  bool   `json:"include_cmdline"`
	IncludeEnv      bool   `json:"include_env"`
	IncludeArgs     bool   `json:"include_args"`
	IncludeGPU      bool   `json:"include_gpu"`
	MaxDepth        int    `json:"max_depth,omitempty"` // 0 means unlimited
	FilterPythonOnly bool  `json:"filter_python_only"`
}

// ProcessEnvRequest represents a request to get process environment variables
type ProcessEnvRequest struct {
	PodUID      string `json:"pod_uid" binding:"required"`
	ContainerID string `json:"container_id,omitempty"`
	HostPID     int    `json:"host_pid" binding:"required"`
}

// ProcessEnvResponse represents process environment variables response
type ProcessEnvResponse struct {
	PodUID      string   `json:"pod_uid"`
	ContainerID string   `json:"container_id,omitempty"`
	HostPID     int      `json:"host_pid"`
	Env         []string `json:"env"`
}

// ProcessArgsRequest represents a request to get process arguments
type ProcessArgsRequest struct {
	PodUID      string `json:"pod_uid" binding:"required"`
	ContainerID string `json:"container_id,omitempty"`
	HostPID     int    `json:"host_pid" binding:"required"`
}

// ProcessArgsResponse represents process arguments response
type ProcessArgsResponse struct {
	PodUID      string   `json:"pod_uid"`
	ContainerID string   `json:"container_id,omitempty"`
	HostPID     int      `json:"host_pid"`
	Args        []string `json:"args"`
}

// GetPythonProcesses returns all Python processes from the tree
func (p *PodProcessTree) GetPythonProcesses() []*ProcessInfo {
	var pythonProcesses []*ProcessInfo
	for _, container := range p.Containers {
		pythonProcesses = append(pythonProcesses, container.GetPythonProcesses()...)
	}
	return pythonProcesses
}

// GetPythonProcesses returns all Python processes from the container
func (c *ContainerProcessTree) GetPythonProcesses() []*ProcessInfo {
	var pythonProcesses []*ProcessInfo
	if c.RootProcess != nil {
		pythonProcesses = collectPythonProcesses(c.RootProcess, pythonProcesses)
	}
	return pythonProcesses
}

// collectPythonProcesses recursively collects Python processes
func collectPythonProcesses(p *ProcessInfo, result []*ProcessInfo) []*ProcessInfo {
	if p.IsPython {
		result = append(result, p)
	}
	for _, child := range p.Children {
		result = collectPythonProcesses(child, result)
	}
	return result
}

// GetGPUProcesses returns all processes with GPU access from the tree
func (p *PodProcessTree) GetGPUProcesses() []*ProcessInfo {
	var gpuProcesses []*ProcessInfo
	for _, container := range p.Containers {
		gpuProcesses = append(gpuProcesses, container.GetGPUProcesses()...)
	}
	return gpuProcesses
}

// GetGPUProcesses returns all processes with GPU access from the container
func (c *ContainerProcessTree) GetGPUProcesses() []*ProcessInfo {
	var gpuProcesses []*ProcessInfo
	if c.RootProcess != nil {
		gpuProcesses = collectGPUProcesses(c.RootProcess, gpuProcesses)
	}
	return gpuProcesses
}

// collectGPUProcesses recursively collects processes with GPU access
func collectGPUProcesses(p *ProcessInfo, result []*ProcessInfo) []*ProcessInfo {
	if p.HasGPU {
		result = append(result, p)
	}
	for _, child := range p.Children {
		result = collectGPUProcesses(child, result)
	}
	return result
}

