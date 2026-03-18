// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package robust

// Robust API response types.
// These mirror the Robust API responses but are defined locally to avoid
// any Go module dependency on the Robust codebase.

type NodeListResponse struct {
	Nodes      []NodeSummary `json:"nodes"`
	TotalNodes int           `json:"total_nodes"`
}

type NodeSummary struct {
	NodeName       string      `json:"node_name"`
	InfraType      string      `json:"infra_type"`
	HealthStatus   string      `json:"health_status"`
	ManageStatus   string      `json:"manage_status"`
	FaultCount     int         `json:"fault_count"`
	AgentConnected bool        `json:"agent_connected"`
	GPUCount       int         `json:"gpu_count"`
	GPUsAllocated  int         `json:"gpus_allocated"`
	AllocationRate float64     `json:"allocation_rate"`
	Labels         interface{} `json:"labels"`
}

type NodeDetailResponse struct {
	NodeSummary
	RDMACount        int         `json:"rdma_count"`
	RunningWorkloads int         `json:"running_workloads"`
	Resources        interface{} `json:"resources"`
	UpdatedAt        string      `json:"updated_at"`
}

type NodeDevicesResponse struct {
	NodeName  string        `json:"node_name"`
	GPUs      []GPUDevice   `json:"gpus"`
	RDMAs     []interface{} `json:"rdmas"`
	GPUCount  int           `json:"gpu_count"`
	RDMACount int           `json:"rdma_count"`
}

type GPUDevice struct {
	GPUID    int    `json:"gpu_id"`
	Model    string `json:"model"`
	Serial   string `json:"serial"`
	VRAMMB   int    `json:"vram_mb"`
	PCIEBDF  string `json:"pcie_bdf"`
	NUMANode int    `json:"numa_node"`
	Status   string `json:"status"`
}

type NodeUtilResponse struct {
	NodeName          string      `json:"node_name"`
	AvgGPUUtilization interface{} `json:"avg_gpu_utilization"`
	MaxGPUUtilization interface{} `json:"max_gpu_utilization"`
	AvgGPUPower       interface{} `json:"avg_gpu_power"`
	AvgGPUTemp        interface{} `json:"avg_gpu_temp"`
	AvgGPUMemoryPct   interface{} `json:"avg_gpu_memory_pct"`
}

type NodeWorkloadsResponse struct {
	NodeName  string            `json:"node_name"`
	Workloads []WorkloadBrief   `json:"workloads"`
	Count     int               `json:"count"`
}

type WorkloadBrief struct {
	WorkloadID   string      `json:"workload_id"`
	Name         string      `json:"name"`
	State        string      `json:"state"`
	GPUAllocated int         `json:"gpu_allocated"`
	StartTime    string      `json:"start_time"`
	Namespace    string      `json:"namespace"`
	Kind         string      `json:"kind"`
	Labels       interface{} `json:"labels"`
	Annotations  interface{} `json:"annotations"`
}

type WorkloadDetailResponse struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	InfraType   string      `json:"infra_type"`
	Namespace   string      `json:"namespace"`
	Kind        string      `json:"kind"`
	User        string      `json:"user"`
	State       string      `json:"state"`
	Nodes       string      `json:"nodes"`
	GPUs        int         `json:"gpus"`
	Gres        string      `json:"gres"`
	Command     string      `json:"command"`
	StartTime   string      `json:"start_time"`
	EndTime     string      `json:"end_time"`
	DurationSec int         `json:"duration_sec"`
	Labels      interface{} `json:"labels"`
	Annotations interface{} `json:"annotations"`
	Details     interface{} `json:"details"`
}

type WorkloadHierarchyResponse struct {
	WorkloadID string       `json:"workload_id"`
	Pods       []PodBrief   `json:"pods"`
	PodCount   int          `json:"pod_count"`
}

type PodBrief struct {
	PodUID         string `json:"pod_uid"`
	PodName        string `json:"pod_name"`
	NodeName       string `json:"node_name"`
	Phase          string `json:"phase"`
	GPUAllocated   int    `json:"gpu_allocated"`
	ContainerImage string `json:"container_image"`
	StartAt        string `json:"start_at"`
	EndAt          string `json:"end_at"`
}

type PodStatsResponse struct {
	Total     int              `json:"total"`
	Running   int              `json:"running"`
	Succeeded int              `json:"succeeded"`
	Failed    int              `json:"failed"`
	Pods      []PodDetail      `json:"pods"`
	PageNum   int              `json:"page_num"`
	PageSize  int              `json:"page_size"`
}

type PodDetail struct {
	PodUID         string `json:"pod_uid"`
	PodName        string `json:"pod_name"`
	NodeName       string `json:"node_name"`
	Namespace      string `json:"namespace"`
	Phase          string `json:"phase"`
	GPUAllocated   int    `json:"gpu_allocated"`
	ContainerImage string `json:"container_image"`
	WorkloadID     string `json:"workload_id"`
	StartAt        string `json:"start_at"`
	EndAt          string `json:"end_at"`
}

type AlertListResponse struct {
	Alerts []AlertEvent `json:"alerts"`
	Count  int          `json:"count"`
}

type AlertEvent struct {
	ID         int64  `json:"id"`
	AlertName  string `json:"alert_name"`
	Severity   string `json:"severity"`
	Status     string `json:"status"`
	Source     string `json:"source"`
	WorkloadID string `json:"workload_id"`
	NodeName   string `json:"node_name"`
	Message    string `json:"message"`
	FiredAt    string `json:"fired_at"`
	ResolvedAt string `json:"resolved_at,omitempty"`
}

type AlertSummaryResponse struct {
	FiringTotal int            `json:"firing_total"`
	BySeverity  map[string]int `json:"by_severity"`
}

type ClusterStatisticResponse struct {
	RunningWorkloadsCount       int     `json:"running_workloads_count"`
	AvgGPUAllocated             float64 `json:"avg_gpu_allocated"`
	AvgGPUUtilization           float64 `json:"avg_gpu_utilization"`
	LowUtilizationWorkloadsCount int    `json:"low_utilization_workloads_count"`
	TotalGPUAllocated           int     `json:"total_gpu_allocated"`
}

type WorkloadMetadataResponse struct {
	Namespaces       []string `json:"namespaces"`
	Kinds            []string `json:"kinds"`
	TotalWorkloads   int      `json:"total_workloads"`
	RunningWorkloads int      `json:"running_workloads"`
}

type TrainingProgressResponse struct {
	WorkloadID string           `json:"workload_id"`
	Points     []TrainingPoint  `json:"points"`
	Count      int              `json:"count"`
}

type TrainingPoint struct {
	Source    string                 `json:"source"`
	Iteration int                   `json:"iteration"`
	Metrics   map[string]interface{} `json:"metrics"`
	Timestamp string                 `json:"timestamp"`
}

type GPUAggregationResponse struct {
	Dimension string                   `json:"dimension"`
	Data      []map[string]interface{} `json:"data"`
	Total     int                      `json:"total"`
	PageNum   int                      `json:"page_num"`
	PageSize  int                      `json:"page_size"`
}

type DiagProfileResponse struct {
	WorkloadUID string                 `json:"workload_uid"`
	Workload    map[string]interface{} `json:"workload"`
	Intent      map[string]interface{} `json:"intent,omitempty"`
	Statistics  map[string]interface{} `json:"statistics,omitempty"`
}

type RealtimeStatusResponse struct {
	TotalNodes       int     `json:"total_nodes"`
	HealthyNodes     int     `json:"healthy_nodes"`
	ConnectedNodes   int     `json:"connected_nodes"`
	TotalGPUs        int     `json:"total_gpus"`
	AllocatedGPUs    int     `json:"allocated_gpus"`
	AllocationRate   float64 `json:"allocation_rate"`
	AvgGPUUtilization float64 `json:"avg_gpu_utilization"`
	RunningWorkloads int     `json:"running_workloads"`
	PendingWorkloads int     `json:"pending_workloads"`
	FiringAlerts     int     `json:"firing_alerts"`
	Timestamp        string  `json:"timestamp"`
}

type ProfilerFileListResponse struct {
	Files []ProfilerFile `json:"files"`
	Count int            `json:"count"`
}

type ProfilerFile struct {
	ID          int64  `json:"id"`
	WorkloadID  string `json:"workload_id"`
	FileName    string `json:"file_name"`
	FileType    string `json:"file_type"`
	FileSize    int64  `json:"file_size"`
	CreatedAt   string `json:"created_at"`
}
