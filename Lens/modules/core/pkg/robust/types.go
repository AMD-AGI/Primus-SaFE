// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package robust

import "encoding/json"

// --- Node types ---

type NodeListResp struct {
	Nodes      []NodeSummary `json:"nodes"`
	TotalNodes int           `json:"total_nodes"`
	Page       int           `json:"page"`
	PageSize   int           `json:"page_size"`
}

type NodeSummary struct {
	NodeName         string                `json:"node_name"`
	InfraType        string                `json:"infra_type"`
	HealthStatus     string                `json:"health_status"`
	ManageStatus     string                `json:"manage_status"`
	FaultCount       int                   `json:"fault_count"`
	AgentConnected   bool                  `json:"agent_connected"`
	GPUCount         int                   `json:"gpu_count"`
	GPUsAllocated    int                   `json:"gpus_allocated"`
	AllocationRate   float64               `json:"allocation_rate"`
	Labels           json.RawMessage       `json:"labels,omitempty"`
	RunningWorkloads []RunningWorkloadBrief `json:"running_workloads"`
}

type RunningWorkloadBrief struct {
	WorkloadID string `json:"workload_id"`
	Name       string `json:"name"`
}

type NodeDetailResp struct {
	NodeName         string          `json:"node_name"`
	InfraType        string          `json:"infra_type"`
	HealthStatus     string          `json:"health_status"`
	ManageStatus     string          `json:"manage_status"`
	FaultCount       int             `json:"fault_count"`
	AgentConnected   bool            `json:"agent_connected"`
	GPUCount         int             `json:"gpu_count"`
	RDMACount        int             `json:"rdma_count"`
	RunningWorkloads int             `json:"running_workloads"`
	UpdatedAt        string          `json:"updated_at"`
	Resources        json.RawMessage `json:"resources,omitempty"`
	Labels           json.RawMessage `json:"labels,omitempty"`
}

type NodeUtilizationResp struct {
	NodeName          string      `json:"node_name"`
	AvgGPUUtilization interface{} `json:"avg_gpu_utilization"`
	MaxGPUUtilization interface{} `json:"max_gpu_utilization"`
	AvgGPUPower       interface{} `json:"avg_gpu_power"`
	AvgGPUTemp        interface{} `json:"avg_gpu_temp"`
	AvgGPUMemoryPct   interface{} `json:"avg_gpu_memory_pct"`
}

type NodeWorkloadsResp struct {
	NodeName  string          `json:"node_name"`
	Workloads []WorkloadBrief `json:"workloads"`
	Count     int             `json:"count"`
}

type WorkloadBrief struct {
	WorkloadID   string `json:"workload_id"`
	Name         string `json:"name"`
	State        string `json:"state"`
	GPUAllocated int    `json:"gpu_allocated"`
	StartTime    string `json:"start_time,omitempty"`
	Namespace    string `json:"namespace,omitempty"`
}

type NodeGPUMetricsResp struct {
	NodeName  string           `json:"node_name"`
	GPUs      []GPUMetricPoint `json:"gpus"`
	GPUCount  int              `json:"gpu_count"`
	Timestamp string           `json:"timestamp"`
}

type GPUMetricPoint struct {
	GPUID       int     `json:"gpu_id"`
	Utilization float64 `json:"utilization"`
	Temperature float64 `json:"temperature"`
	Power       float64 `json:"power"`
	MemoryPct   float64 `json:"memory_pct"`
}

type NodeDevicesResp struct {
	NodeName  string                   `json:"node_name"`
	GPUs      []map[string]interface{} `json:"gpus"`
	RDMAs     []map[string]interface{} `json:"rdmas"`
	GPUCount  int                      `json:"gpu_count"`
	RDMACount int                      `json:"rdma_count"`
}

// --- Cluster types ---

type ClusterRealtimeResp struct {
	TotalNodes        int     `json:"total_nodes"`
	HealthyNodes      int     `json:"healthy_nodes"`
	ConnectedNodes    int     `json:"connected_nodes"`
	TotalGPUs         int     `json:"total_gpus"`
	AllocatedGPUs     int     `json:"allocated_gpus"`
	AllocationRate    float64 `json:"allocation_rate"`
	AvgGPUUtilization float64 `json:"avg_gpu_utilization"`
	RunningWorkloads  int     `json:"running_workloads"`
	PendingWorkloads  int     `json:"pending_workloads"`
	FiringAlerts      int     `json:"firing_alerts"`
	Timestamp         string  `json:"timestamp"`
}

type ClusterGPUUtilizationResp struct {
	AvgUtilization interface{} `json:"avg_utilization"`
	MaxUtilization interface{} `json:"max_utilization"`
	TotalPower     interface{} `json:"total_power"`
	AvgMemoryPct   interface{} `json:"avg_memory_pct"`
	AvgTemperature interface{} `json:"avg_temperature"`
	TotalGPUs      int         `json:"total_gpus"`
}

type ClusterOverviewResp struct {
	TotalNodes       int     `json:"total_nodes"`
	TotalGPUs        int     `json:"total_gpus"`
	AllocatedGPUs    int     `json:"allocated_gpus"`
	RunningWorkloads int     `json:"running_workloads"`
	HealthyNodes     int     `json:"healthy_nodes"`
	FaultedNodes     int     `json:"faulted_nodes"`
	AvgUtilization   float64 `json:"avg_utilization"`
	AllocationRate   float64 `json:"allocation_rate"`
	UpdatedAt        string  `json:"updated_at"`
}

type ClusterGPUHeatmapResp struct {
	Nodes []GPUHeatmapNodeEntry `json:"nodes"`
	Count int                   `json:"count"`
}

type GPUHeatmapNodeEntry struct {
	NodeName string            `json:"node_name"`
	GPUs     []GPUHeatmapCell  `json:"gpus"`
}

type GPUHeatmapCell struct {
	NodeName    string  `json:"node_name"`
	GpuID       string  `json:"gpu_id"`
	Utilization float64 `json:"utilization"`
	MemoryPct   float64 `json:"memory_pct"`
	Power       float64 `json:"power"`
	Allocated   bool    `json:"allocated"`
}

// --- Workload types ---

type WorkloadProfileResp struct {
	WorkloadUID string                 `json:"workload_uid"`
	Workload    map[string]interface{} `json:"workload"`
	Intent      map[string]interface{} `json:"intent,omitempty"`
	Statistics  map[string]interface{} `json:"statistics,omitempty"`
}

type WorkloadListResp struct {
	Workloads json.RawMessage `json:"workloads"`
	Count     int             `json:"count"`
}

// --- Pod types ---

type PodDetailResp struct {
	PodUID      string                   `json:"pod_uid"`
	PodName     string                   `json:"pod_name"`
	NodeName    string                   `json:"node_name"`
	Namespace   string                   `json:"namespace"`
	Phase       string                   `json:"phase"`
	GPUAllocated int                     `json:"gpu_allocated"`
	WorkloadID  string                   `json:"workload_id"`
	Devices     []map[string]interface{} `json:"devices,omitempty"`
}

type PodStatsResp struct {
	Total     int                      `json:"total"`
	Running   int                      `json:"running"`
	Succeeded int                      `json:"succeeded"`
	Failed    int                      `json:"failed"`
	Pods      []map[string]interface{} `json:"pods"`
	PageNum   int                      `json:"page_num"`
	PageSize  int                      `json:"page_size"`
}

// --- Alert types ---

type AlertListResp struct {
	Alerts []map[string]interface{} `json:"alerts"`
	Count  int                      `json:"count"`
}

type AlertSummaryResp struct {
	FiringTotal int            `json:"firing_total"`
	BySeverity  map[string]int `json:"by_severity"`
}

// --- Training types ---

type TrainingMetricsListResp struct {
	Metrics []TrainingMetricDef `json:"metrics"`
}

type TrainingMetricDef struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Category    string `json:"category"`
	Unit        string `json:"unit"`
	AggLevel    string `json:"agg_level"`
}

type TrainingMetricsDataResp struct {
	Data struct {
		Results []TrainingMetricResult `json:"results"`
	} `json:"data"`
}

type TrainingMetricResult struct {
	Name   string                  `json:"name"`
	Unit   string                  `json:"unit"`
	Series []TrainingMetricSeries  `json:"series"`
}

type TrainingMetricSeries struct {
	Labels map[string]string          `json:"labels"`
	Values []TrainingMetricDataPoint  `json:"values"`
}

type TrainingMetricDataPoint struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

// --- GPU Aggregation types ---

type GpuAggClustersResp struct {
	Clusters []string `json:"clusters"`
}

type GpuAggNamespacesResp struct {
	Namespaces []string `json:"namespaces"`
}

type GpuAggDimensionKeysResp struct {
	Keys []string `json:"keys"`
}

type GpuAggDimensionValuesResp struct {
	DimensionType string   `json:"dimension_type"`
	Values        []string `json:"values"`
}

type GpuAggPaginatedResp struct {
	Total      int                      `json:"total"`
	Page       int                      `json:"page"`
	PageSize   int                      `json:"page_size"`
	TotalPages int                      `json:"total_pages"`
	Data       []map[string]interface{} `json:"data"`
}

type GpuAggSnapshotResp struct {
	Timestamp     string                   `json:"timestamp"`
	TotalGpus     int                      `json:"total_gpus"`
	AllocatedGpus int                      `json:"allocated_gpus"`
	AllocationRate float64                 `json:"allocation_rate"`
	Nodes         []map[string]interface{} `json:"nodes"`
}
