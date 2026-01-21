// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package api provides the unified framework demo endpoints.
// This file demonstrates how to use the new unified API framework
// to create endpoints that work for both HTTP REST and MCP.
package api

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/filter"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/cluster"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/fault"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/node"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/rdma"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/storage"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/workload"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/sliceUtil"
)

// ===== Request/Response Structs with MCP Tags =====

// ClusterHealthRequest represents the request for cluster health check.
type ClusterHealthRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional - uses default if not specified)"`
}

// ClusterHealthResponse represents the response for cluster health check.
type ClusterHealthResponse struct {
	ClusterName   string    `json:"cluster_name"`
	Status        string    `json:"status"` // healthy, degraded, critical
	TotalNodes    int       `json:"total_nodes"`
	HealthyNodes  int       `json:"healthy_nodes"`
	GPUCount      int       `json:"gpu_count"`
	Timestamp     time.Time `json:"timestamp"`
	Message       string    `json:"message"`
}

// GPUQuickStatsRequest represents the request for quick GPU stats.
type GPUQuickStatsRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

// GPUQuickStatsResponse represents quick GPU statistics.
type GPUQuickStatsResponse struct {
	ClusterName      string  `json:"cluster_name"`
	TotalGPUs        int     `json:"total_gpus"`
	AllocatedGPUs    int     `json:"allocated_gpus"`
	AvailableGPUs    int     `json:"available_gpus"`
	AllocationRate   float64 `json:"allocation_rate"`
	AvgUtilization   float64 `json:"avg_utilization"`
	Timestamp        time.Time `json:"timestamp"`
}

// EchoRequest represents a simple echo request for testing.
type EchoRequest struct {
	Message string `json:"message" query:"message" mcp:"message,description=Message to echo back,required"`
	Count   int    `json:"count" query:"count" mcp:"count,description=Number of times to repeat (default 1)"`
}

// EchoResponse represents the echo response.
type EchoResponse struct {
	Echo      string    `json:"echo"`
	Count     int       `json:"count"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"` // "http" or "mcp"
}

// ===== Phase 2: Cluster Overview & GPU Allocation =====

// ClusterOverviewRequest represents the request for cluster overview.
type ClusterOverviewRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional - uses default if not specified)"`
}

// ClusterOverviewResponse wraps the GpuClusterOverview model.
type ClusterOverviewResponse struct {
	model.GpuClusterOverview
	ClusterName string    `json:"cluster_name"`
	Timestamp   time.Time `json:"timestamp"`
}

// ClusterConsumersRequest represents the request for cluster consumers.
type ClusterConsumersRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	PageNum  int    `json:"page_num" query:"page_num" mcp:"page_num,description=Page number for pagination (default 1)"`
	PageSize int    `json:"page_size" query:"page_size" mcp:"page_size,description=Number of items per page (default 10)"`
}

// ClusterConsumersResponse represents the cluster consumers response.
type ClusterConsumersResponse struct {
	Data        []model.TopLevelGpuResource `json:"data"`
	Total       int                         `json:"total"`
	ClusterName string                      `json:"cluster_name"`
}

// GPUNodeListRequest represents the request for GPU node list.
type GPUNodeListRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Name     string `json:"name" query:"name" mcp:"name,description=Filter by node name (partial match)"`
	GpuName  string `json:"gpu_name" query:"gpu_name" mcp:"gpu_name,description=Filter by GPU model name"`
	Status   string `json:"status" query:"status" mcp:"status,description=Filter by node status (comma-separated for multiple: ready,notready)"`
	PageNum  int    `json:"page_num" query:"page_num" mcp:"page_num,description=Page number for pagination (default 1)"`
	PageSize int    `json:"page_size" query:"page_size" mcp:"page_size,description=Number of items per page (default 10)"`
	OrderBy  string `json:"order_by" query:"order_by" mcp:"order_by,description=Field to order by"`
	Desc     bool   `json:"desc" query:"desc" mcp:"desc,description=Sort in descending order"`
}

// GPUNodeListResponse represents the GPU node list response.
type GPUNodeListResponse struct {
	Data        []model.GPUNode `json:"data"`
	Total       int             `json:"total"`
	ClusterName string          `json:"cluster_name"`
}

// GPUAllocationRequest represents the request for GPU allocation info.
type GPUAllocationRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional - uses default if not specified)"`
}

// GPUAllocationResponse represents the GPU allocation response.
type GPUAllocationResponse struct {
	Data        []model.GpuAllocation `json:"data"`
	ClusterName string                `json:"cluster_name"`
	Summary     GPUAllocationSummary  `json:"summary"`
	Timestamp   time.Time             `json:"timestamp"`
}

// GPUAllocationSummary provides aggregated stats.
type GPUAllocationSummary struct {
	TotalNodes     int     `json:"total_nodes"`
	TotalGPUs      int     `json:"total_gpus"`
	AllocatedGPUs  int     `json:"allocated_gpus"`
	AvailableGPUs  int     `json:"available_gpus"`
	AllocationRate float64 `json:"allocation_rate_percent"`
}

// ===== Phase 3: Core Query APIs =====

// GPUUtilizationRequest represents the request for GPU utilization.
type GPUUtilizationRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

// GPUUtilizationResponse represents GPU utilization response.
type GPUUtilizationResponse struct {
	ClusterName    string  `json:"cluster_name"`
	AllocationRate float64 `json:"allocation_rate"`
	Utilization    float64 `json:"utilization"`
	Timestamp      time.Time `json:"timestamp"`
}

// GPUUtilizationHistoryRequest represents the request for GPU utilization history.
type GPUUtilizationHistoryRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Start   string `json:"start" query:"start" mcp:"start,description=Start timestamp (Unix seconds),required"`
	End     string `json:"end" query:"end" mcp:"end,description=End timestamp (Unix seconds),required"`
	Step    string `json:"step" query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
}

// GPUUtilizationHistoryResponse represents GPU utilization history.
type GPUUtilizationHistoryResponse struct {
	ClusterName     string            `json:"cluster_name"`
	AllocationRate  []model.TimePoint `json:"allocation_rate"`
	Utilization     []model.TimePoint `json:"utilization"`
	VramUtilization []model.TimePoint `json:"vram_utilization"`
}

// NodeDetailRequest represents the request for node detail.
type NodeDetailRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name to get details for,required"`
}

// NodeDetailResponse represents the node detail response.
type NodeDetailResponse struct {
	model.GpuNodeDetail
	ClusterName string `json:"cluster_name"`
}

// NodeGPUDevicesRequest represents the request for node GPU devices.
type NodeGPUDevicesRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name to get GPU devices for,required"`
}

// NodeGPUDevicesResponse represents the node GPU devices response.
type NodeGPUDevicesResponse struct {
	NodeName    string              `json:"node_name"`
	ClusterName string              `json:"cluster_name"`
	Devices     []model.GpuDeviceInfo `json:"devices"`
}

// WorkloadListRequest represents the request for workload list.
type WorkloadListRequest struct {
	Cluster   string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Name      string `json:"name" query:"name" mcp:"name,description=Filter by workload name (partial match)"`
	Kind      string `json:"kind" query:"kind" mcp:"kind,description=Filter by workload kind (Job/PyTorchJob/etc)"`
	Namespace string `json:"namespace" query:"namespace" mcp:"namespace,description=Filter by namespace"`
	Status    string `json:"status" query:"status" mcp:"status,description=Filter by status (Running/Completed/Failed)"`
	PageNum   int    `json:"page_num" query:"page_num" mcp:"page_num,description=Page number (default 1)"`
	PageSize  int    `json:"page_size" query:"page_size" mcp:"page_size,description=Page size (default 10)"`
	OrderBy   string `json:"order_by" query:"order_by" mcp:"order_by,description=Order by field (start_at/end_at)"`
}

// WorkloadListResponse represents the workload list response.
type WorkloadListResponse struct {
	Data        []model.WorkloadListItem `json:"data"`
	Total       int                      `json:"total"`
	ClusterName string                   `json:"cluster_name"`
}

// WorkloadDetailRequest represents the request for workload detail.
type WorkloadDetailRequest struct {
	Cluster     string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	WorkloadUID string `json:"uid" param:"uid" mcp:"uid,description=Workload UID to get details for,required"`
}

// WorkloadDetailResponse represents the workload detail response.
type WorkloadDetailResponse struct {
	model.WorkloadInfo
	ClusterName string `json:"cluster_name"`
}

// PodStatsRequest represents the request for pod stats.
type PodStatsRequest struct {
	Cluster   string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name,required"`
	Namespace string `json:"namespace" query:"namespace" mcp:"namespace,description=Filter by namespace"`
	PodName   string `json:"pod_name" query:"pod_name" mcp:"pod_name,description=Filter by pod name (partial match)"`
	StartTime string `json:"start_time" query:"start_time" mcp:"start_time,description=Filter by start time (RFC3339)"`
	EndTime   string `json:"end_time" query:"end_time" mcp:"end_time,description=Filter by end time (RFC3339)"`
	Page      int    `json:"page" query:"page" mcp:"page,description=Page number (default 1)"`
	PageSize  int    `json:"page_size" query:"page_size" mcp:"page_size,description=Page size (default 20)"`
}

// UnifiedPodStatsResponse represents the pod stats response for unified API.
type UnifiedPodStatsResponse struct {
	Total       int        `json:"total"`
	Page        int        `json:"page"`
	Pods        []PodStats `json:"pods"`
	ClusterName string     `json:"cluster_name"`
}

// RealtimeStatusRequest represents the request for realtime status.
type RealtimeStatusRequest struct {
	Cluster string   `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name,required"`
	Include []string `json:"include" query:"include" mcp:"include,description=Fields to include (nodes/pods/workloads/alerts)"`
}

// RealtimeStatusResult represents the realtime status response.
type RealtimeStatusResult struct {
	Cluster            string                `json:"cluster"`
	Timestamp          time.Time             `json:"timestamp"`
	CurrentGPUUsage    RealtimeGPUUsage      `json:"current_gpu_usage"`
	RunningTasks       int                   `json:"running_tasks"`
	AvailableResources RealtimeAvailability  `json:"available_resources"`
}

// RealtimeGPUUsage represents GPU usage in realtime status.
type RealtimeGPUUsage struct {
	TotalGPUs       int     `json:"total_gpus"`
	AllocatedGPUs   int     `json:"allocated_gpus"`
	UtilizedGPUs    int     `json:"utilized_gpus"`
	AllocationRate  float64 `json:"allocation_rate"`
	UtilizationRate float64 `json:"utilization_rate"`
}

// RealtimeAvailability represents resource availability.
type RealtimeAvailability struct {
	AvailableGPUs    int `json:"available_gpus"`
	AvailableNodes   int `json:"available_nodes"`
	MaxContiguousGPU int `json:"max_contiguous_gpu"`
}

// ===== Phase 4: Node Analysis & Metrics Endpoints (10%) =====

// NodeGPUMetricsRequest represents the request for node GPU metrics history.
type NodeGPUMetricsRequest struct {
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name to get GPU metrics for,required"`
	Start    int64  `json:"start" query:"start" mcp:"start,description=Start timestamp (unix seconds),required"`
	End      int64  `json:"end" query:"end" mcp:"end,description=End timestamp (unix seconds),required"`
	Step     int    `json:"step" query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

// NodeGPUMetricsResponse represents the response for node GPU metrics.
type NodeGPUMetricsResponse struct {
	GpuUtilization    MetricsGraph `json:"gpu_utilization"`
	GpuAllocationRate MetricsGraph `json:"gpu_allocation_rate"`
}

// MetricsGraph represents a graph of metrics over time.
type MetricsGraph struct {
	Series []MetricsSeries    `json:"series"`
	Config MetricsGraphConfig `json:"config"`
}

// MetricsSeries represents a series of metric values.
type MetricsSeries struct {
	Name   string        `json:"name"`
	Values []MetricValue `json:"values"`
}

// MetricValue represents a single metric value at a point in time.
type MetricValue struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

// MetricsGraphConfig represents graph configuration.
type MetricsGraphConfig struct {
	YAxisUnit string `json:"y_axis_unit"`
}

// NodeUtilizationRequest represents the request for node utilization.
type NodeUtilizationRequest struct {
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name to get utilization for,required"`
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

// NodeUtilizationResponse represents the response for node utilization.
type NodeUtilizationResponse struct {
	NodeName       string  `json:"node_name"`
	CpuUtilization float64 `json:"cpu_utilization"`
	MemUtilization float64 `json:"mem_utilization"`
	GpuUtilization float64 `json:"gpu_utilization"`
	GpuAllocation  int     `json:"gpu_allocation"`
	Timestamp      int64   `json:"timestamp"`
}

// NodeUtilizationHistoryRequest represents the request for node utilization history.
type NodeUtilizationHistoryRequest struct {
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name to get utilization history for,required"`
	Start    int64  `json:"start" query:"start" mcp:"start,description=Start timestamp (unix seconds),required"`
	End      int64  `json:"end" query:"end" mcp:"end,description=End timestamp (unix seconds),required"`
	Step     int    `json:"step" query:"step" mcp:"step,description=Step interval in seconds (default 60)"`
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

// NodeUtilizationHistoryResponse represents the response for node utilization history.
type NodeUtilizationHistoryResponse struct {
	CpuUtilization MetricsGraph `json:"cpu_utilization"`
	MemUtilization MetricsGraph `json:"mem_utilization"`
	GpuUtilization MetricsGraph `json:"gpu_utilization"`
}

// NodeWorkloadsRequest represents the request for node workloads.
type NodeWorkloadsRequest struct {
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name to get workloads for,required"`
	PageNum  int    `json:"page_num" query:"page_num" mcp:"page_num,description=Page number (default 1)"`
	PageSize int    `json:"page_size" query:"page_size" mcp:"page_size,description=Page size (default 20)"`
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

// NodeWorkloadsResponse represents the response for node workloads.
type NodeWorkloadsResponse struct {
	Data  []NodeWorkloadInfo `json:"data"`
	Total int                `json:"total"`
}

// NodeWorkloadInfo represents a workload running on a node.
type NodeWorkloadInfo struct {
	UID          string    `json:"uid"`
	Name         string    `json:"name"`
	Namespace    string    `json:"namespace"`
	Kind         string    `json:"kind"`
	GPUAllocated int32     `json:"gpu_allocated"`
	StartedAt    time.Time `json:"started_at"`
	Status       string    `json:"status"`
}

// NodeWorkloadsHistoryRequest represents the request for node workloads history.
type NodeWorkloadsHistoryRequest struct {
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name to get workloads history for,required"`
	PageNum  int    `json:"page_num" query:"page_num" mcp:"page_num,description=Page number (default 1)"`
	PageSize int    `json:"page_size" query:"page_size" mcp:"page_size,description=Page size (default 20)"`
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

// NodeWorkloadsHistoryResponse represents the response for node workloads history.
type NodeWorkloadsHistoryResponse struct {
	Data  []NodeWorkloadHistoryInfo `json:"data"`
	Total int                       `json:"total"`
}

// NodeWorkloadHistoryInfo represents a historical workload on a node.
type NodeWorkloadHistoryInfo struct {
	UID          string    `json:"uid"`
	Name         string    `json:"name"`
	Namespace    string    `json:"namespace"`
	Kind         string    `json:"kind"`
	GPUAllocated int32     `json:"gpu_allocated"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at"`
	Duration     int64     `json:"duration_seconds"`
}

// UnifiedFragmentationRequest represents the request for fragmentation analysis.
type UnifiedFragmentationRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name,required"`
}

// UnifiedFragmentationResponse represents the response for fragmentation analysis.
type UnifiedFragmentationResponse struct {
	Cluster                   string                       `json:"cluster"`
	ClusterFragmentationScore float64                      `json:"cluster_fragmentation_score"`
	TotalNodes                int                          `json:"total_nodes"`
	NodeFragmentations        []UnifiedNodeFragInfo        `json:"node_fragmentations"`
	Recommendations           []string                     `json:"recommendations"`
	Summary                   UnifiedFragmentationSummary  `json:"summary"`
}

// UnifiedNodeFragInfo represents fragmentation info for a single node.
type UnifiedNodeFragInfo struct {
	NodeName           string  `json:"node_name"`
	TotalGPUs          int32   `json:"total_gpus"`
	AllocatedGPUs      int32   `json:"allocated_gpus"`
	AvailableGPUs      int32   `json:"available_gpus"`
	FragmentationScore float64 `json:"fragmentation_score"`
	Status             string  `json:"status"`
	Utilization        float64 `json:"utilization"`
}

// UnifiedFragmentationSummary represents the fragmentation summary.
type UnifiedFragmentationSummary struct {
	HealthyNodes    int     `json:"healthy_nodes"`
	FragmentedNodes int     `json:"fragmented_nodes"`
	CriticalNodes   int     `json:"critical_nodes"`
	TotalWastedGPUs int     `json:"total_wasted_gpus"`
	WastePercentage float64 `json:"waste_percentage"`
}

// UnifiedLoadBalanceRequest represents the request for load balance analysis.
type UnifiedLoadBalanceRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name,required"`
}

// UnifiedLoadBalanceResponse represents the response for load balance analysis.
type UnifiedLoadBalanceResponse struct {
	Cluster              string                   `json:"cluster"`
	LoadBalanceScore     float64                  `json:"load_balance_score"`
	NodeLoadDistribution []UnifiedNodeLoadInfo    `json:"node_load_distribution"`
	HotspotNodes         []string                 `json:"hotspot_nodes"`
	IdleNodes            []string                 `json:"idle_nodes"`
	Recommendations      []string                 `json:"recommendations"`
	Statistics           UnifiedLoadBalanceStats  `json:"statistics"`
}

// UnifiedNodeLoadInfo represents load info for a single node.
type UnifiedNodeLoadInfo struct {
	NodeName        string  `json:"node_name"`
	AllocationRate  float64 `json:"allocation_rate"`
	UtilizationRate float64 `json:"utilization_rate"`
	LoadScore       float64 `json:"load_score"`
}

// UnifiedLoadBalanceStats represents load balance statistics.
type UnifiedLoadBalanceStats struct {
	AvgAllocationRate float64 `json:"avg_allocation_rate"`
	StdDevAllocation  float64 `json:"stddev_allocation"`
	MaxAllocation     float64 `json:"max_allocation"`
	MinAllocation     float64 `json:"min_allocation"`
	Variance          float64 `json:"variance"`
}

// NodeFragmentationDetailRequest represents the request for node fragmentation detail.
type NodeFragmentationDetailRequest struct {
	NodeName string `json:"name" param:"name" mcp:"name,description=Node name to get fragmentation for,required"`
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name,required"`
}

// NodeFragmentationDetailResponse represents the response for node fragmentation detail.
type NodeFragmentationDetailResponse struct {
	NodeName           string                  `json:"node_name"`
	TotalGPUs          int32                   `json:"total_gpus"`
	AllocatedGPUs      int32                   `json:"allocated_gpus"`
	AvailableGPUs      int32                   `json:"available_gpus"`
	FragmentationScore float64                 `json:"fragmentation_score"`
	Status             string                  `json:"status"`
	AllocationPattern  AllocationPatternResult `json:"allocation_pattern"`
	RunningPods        []PodAllocationInfo     `json:"running_pods"`
	Recommendations    []string                `json:"recommendations"`
}

// AllocationPatternResult represents GPU allocation pattern.
type AllocationPatternResult struct {
	FullyAllocatedPods   int  `json:"fully_allocated_pods"`
	PartiallyAllocPods   int  `json:"partially_allocated_pods"`
	GPUSharing           bool `json:"gpu_sharing_enabled"`
	LargestContiguousGPU int  `json:"largest_contiguous_gpu"`
}

// PodAllocationInfo represents pod GPU allocation.
type PodAllocationInfo struct {
	PodName       string `json:"pod_name"`
	Namespace     string `json:"namespace"`
	AllocatedGPUs int32  `json:"allocated_gpus"`
}

// GPUHeatmapRequest represents the request for GPU heatmap.
type GPUHeatmapRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

// GPUHeatmapResponse represents the response for GPU heatmap.
type GPUHeatmapResponse struct {
	Power       UnifiedHeatmapData `json:"power"`
	Temperature UnifiedHeatmapData `json:"temperature"`
	Utilization UnifiedHeatmapData `json:"utilization"`
}

// UnifiedHeatmapData represents heatmap data for a metric.
type UnifiedHeatmapData struct {
	Serial   int                  `json:"serial"`
	Unit     string               `json:"unit"`
	YAxisMax int                  `json:"y_axis_max"`
	YAxisMin int                  `json:"y_axis_min"`
	Data     []UnifiedHeatmapPoint `json:"data"`
}

// UnifiedHeatmapPoint represents a point in the heatmap.
type UnifiedHeatmapPoint struct {
	NodeName string  `json:"node_name"`
	DeviceID int     `json:"device_id"`
	Value    float64 `json:"value"`
}

// UnifiedRunningTasksRequest represents the request for running tasks.
type UnifiedRunningTasksRequest struct {
	Cluster   string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name,required"`
	Namespace string `json:"namespace" query:"namespace" mcp:"namespace,description=Filter by namespace (optional)"`
}

// UnifiedRunningTasksResponse represents the response for running tasks.
type UnifiedRunningTasksResponse struct {
	Cluster    string                  `json:"cluster"`
	Timestamp  time.Time               `json:"timestamp"`
	TotalTasks int                     `json:"total_tasks"`
	Tasks      []UnifiedRunningTaskInfo `json:"tasks"`
}

// UnifiedRunningTaskInfo represents a running GPU task.
type UnifiedRunningTaskInfo struct {
	PodUID        string    `json:"pod_uid"`
	PodName       string    `json:"pod_name"`
	Namespace     string    `json:"namespace"`
	WorkloadType  string    `json:"workload_type"`
	WorkloadName  string    `json:"workload_name"`
	NodeName      string    `json:"node_name"`
	AllocatedGPUs int32     `json:"allocated_gpus"`
	RunningTime   int64     `json:"running_time_seconds"`
	StartedAt     time.Time `json:"started_at"`
	Owner         string    `json:"owner"`
}

// ===== Register Unified Endpoints =====

func init() {
	// Register cluster health endpoint
	unified.Register(&unified.EndpointDef[ClusterHealthRequest, ClusterHealthResponse]{
		Name:        "cluster_health",
		Description: "Get GPU cluster health status including node count and overall health",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/cluster/health",
		MCPToolName: "lens_cluster_health",
		Handler:     handleClusterHealth,
	})

	// Register GPU quick stats endpoint
	unified.Register(&unified.EndpointDef[GPUQuickStatsRequest, GPUQuickStatsResponse]{
		Name:        "gpu_quick_stats",
		Description: "Get quick GPU statistics including allocation and utilization",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/gpu/stats",
		MCPToolName: "lens_gpu_quick_stats",
		Handler:     handleGPUQuickStats,
	})

	// Register echo endpoint for simple testing
	unified.Register(&unified.EndpointDef[EchoRequest, EchoResponse]{
		Name:        "echo",
		Description: "Echo back the provided message - useful for testing MCP connectivity",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/echo",
		MCPToolName: "lens_echo",
		Handler:     handleEcho,
	})

	// ===== Phase 2: Cluster & Node APIs =====

	// Register cluster overview endpoint (mirrors /clusters/overview)
	unified.Register(&unified.EndpointDef[ClusterOverviewRequest, ClusterOverviewResponse]{
		Name:        "cluster_overview",
		Description: "Get comprehensive GPU cluster overview including node counts, health status, allocation rate, utilization, storage and RDMA statistics. Returns total/healthy/faulty nodes, idle/busy breakdown, and resource utilization metrics.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/clusters/overview",
		MCPToolName: "lens_cluster_overview",
		Handler:     handleClusterOverview,
	})

	// Register cluster consumers endpoint (mirrors /clusters/consumers)
	unified.Register(&unified.EndpointDef[ClusterConsumersRequest, ClusterConsumersResponse]{
		Name:        "cluster_consumers",
		Description: "List GPU resource consumers (workloads) in the cluster with their GPU allocation and utilization. Shows which workloads are using GPU resources and their current utilization percentage.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/clusters/consumers",
		MCPToolName: "lens_cluster_consumers",
		Handler:     handleClusterConsumers,
	})

	// Register GPU node list endpoint (mirrors /nodes)
	unified.Register(&unified.EndpointDef[GPUNodeListRequest, GPUNodeListResponse]{
		Name:        "node_list",
		Description: "List GPU nodes in the cluster with filtering and pagination. Returns node name, IP, GPU model, GPU count, allocation, utilization and status. Supports filtering by name, GPU model, and status.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes",
		MCPToolName: "lens_node_list",
		Handler:     handleGPUNodeList,
	})

	// Register GPU allocation endpoint (mirrors /nodes/gpuAllocation)
	unified.Register(&unified.EndpointDef[GPUAllocationRequest, GPUAllocationResponse]{
		Name:        "gpu_allocation",
		Description: "Get per-node GPU allocation information showing capacity, allocated count, and allocation rate for each node. Includes summary statistics for the entire cluster.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes/gpuAllocation",
		MCPToolName: "lens_gpu_allocation",
		Handler:     handleGPUAllocation,
	})

	// ===== Phase 3: Core Query APIs =====

	// Register GPU utilization endpoint (mirrors /nodes/gpuUtilization)
	unified.Register(&unified.EndpointDef[GPUUtilizationRequest, GPUUtilizationResponse]{
		Name:        "gpu_utilization",
		Description: "Get current cluster GPU utilization metrics including allocation rate and average utilization percentage across all GPUs.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes/gpuUtilization",
		MCPToolName: "lens_gpu_utilization",
		Handler:     handleGPUUtilization,
	})

	// Register GPU utilization history endpoint (mirrors /nodes/gpuUtilizationHistory)
	unified.Register(&unified.EndpointDef[GPUUtilizationHistoryRequest, GPUUtilizationHistoryResponse]{
		Name:        "gpu_utilization_history",
		Description: "Get historical GPU utilization data over a time range. Returns allocation rate, utilization, and VRAM utilization as time series data points.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes/gpuUtilizationHistory",
		MCPToolName: "lens_gpu_utilization_history",
		Handler:     handleGPUUtilizationHistory,
	})

	// Register node detail endpoint (mirrors /nodes/:name)
	unified.Register(&unified.EndpointDef[NodeDetailRequest, NodeDetailResponse]{
		Name:        "node_detail",
		Description: "Get detailed information about a specific GPU node including CPU, memory, OS, GPU driver version, kubelet version and health status.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes/:name",
		MCPToolName: "lens_node_detail",
		Handler:     handleNodeDetail,
	})

	// Register node GPU devices endpoint (mirrors /nodes/:name/gpuDevices)
	unified.Register(&unified.EndpointDef[NodeGPUDevicesRequest, NodeGPUDevicesResponse]{
		Name:        "node_gpu_devices",
		Description: "Get GPU device information for a specific node including device ID, model, memory, utilization, temperature and power for each GPU.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes/:name/gpuDevices",
		MCPToolName: "lens_node_gpu_devices",
		Handler:     handleNodeGPUDevices,
	})

	// Register workload list endpoint (mirrors /workloads)
	unified.Register(&unified.EndpointDef[WorkloadListRequest, WorkloadListResponse]{
		Name:        "workload_list",
		Description: "List GPU workloads in the cluster with filtering by name, kind, namespace, status. Returns workload metadata, GPU allocation, and utilization statistics.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/workloads",
		MCPToolName: "lens_workload_list",
		Handler:     handleWorkloadList,
	})

	// Register workload detail endpoint (mirrors /workloads/:uid)
	unified.Register(&unified.EndpointDef[WorkloadDetailRequest, WorkloadDetailResponse]{
		Name:        "workload_detail",
		Description: "Get detailed information about a specific workload by UID including pods, GPU allocation per node, start/end times, and status.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/workloads/:uid",
		MCPToolName: "lens_workload_detail",
		Handler:     handleWorkloadDetail,
	})

	// Register pod stats endpoint (mirrors /pods/stats)
	unified.Register(&unified.EndpointDef[PodStatsRequest, UnifiedPodStatsResponse]{
		Name:        "pod_stats",
		Description: "Query GPU pod statistics with filtering by namespace, pod name, and time range. Returns pod details, GPU allocation, and average utilization.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/pods/stats",
		MCPToolName: "lens_pod_stats",
		Handler:     handlePodStats,
	})

	// Register realtime status endpoint (mirrors /realtime/status)
	unified.Register(&unified.EndpointDef[RealtimeStatusRequest, RealtimeStatusResult]{
		Name:        "realtime_status",
		Description: "Get real-time cluster status including current GPU usage, running tasks count, and available resources. Provides a quick snapshot of cluster state.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/realtime/status",
		MCPToolName: "lens_realtime_status",
		Handler:     handleRealtimeStatus,
	})

	// ===== Phase 4: Node Analysis & Metrics (10%) =====

	// Register node GPU metrics endpoint (mirrors /nodes/:name/gpuMetrics)
	unified.Register(&unified.EndpointDef[NodeGPUMetricsRequest, NodeGPUMetricsResponse]{
		Name:        "node_gpu_metrics",
		Description: "Get GPU utilization and allocation rate history for a specific node over a time range. Returns time series data for charting GPU metrics.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes/:name/gpuMetrics",
		MCPToolName: "lens_node_gpu_metrics",
		Handler:     handleNodeGPUMetrics,
	})

	// Register node utilization endpoint (mirrors /nodes/:name/utilization)
	unified.Register(&unified.EndpointDef[NodeUtilizationRequest, NodeUtilizationResponse]{
		Name:        "node_utilization",
		Description: "Get current CPU, memory and GPU utilization for a specific node. Returns real-time resource usage metrics.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes/:name/utilization",
		MCPToolName: "lens_node_utilization",
		Handler:     handleNodeUtilization,
	})

	// Register node utilization history endpoint (mirrors /nodes/:name/utilizationHistory)
	unified.Register(&unified.EndpointDef[NodeUtilizationHistoryRequest, NodeUtilizationHistoryResponse]{
		Name:        "node_utilization_history",
		Description: "Get historical CPU, memory and GPU utilization for a specific node. Returns time series data for resource usage over time.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes/:name/utilizationHistory",
		MCPToolName: "lens_node_utilization_history",
		Handler:     handleNodeUtilizationHistory,
	})

	// Register node workloads endpoint (mirrors /nodes/:name/workloads)
	unified.Register(&unified.EndpointDef[NodeWorkloadsRequest, NodeWorkloadsResponse]{
		Name:        "node_workloads",
		Description: "Get currently running workloads on a specific node. Returns workload list with GPU allocation and status.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes/:name/workloads",
		MCPToolName: "lens_node_workloads",
		Handler:     handleNodeWorkloads,
	})

	// Register node workloads history endpoint (mirrors /nodes/:name/workloadsHistory)
	unified.Register(&unified.EndpointDef[NodeWorkloadsHistoryRequest, NodeWorkloadsHistoryResponse]{
		Name:        "node_workloads_history",
		Description: "Get historical workloads that ran on a specific node. Returns completed workloads with duration and GPU usage.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes/:name/workloadsHistory",
		MCPToolName: "lens_node_workloads_history",
		Handler:     handleNodeWorkloadsHistory,
	})

	// Register fragmentation analysis endpoint (mirrors /nodes/fragmentation-analysis)
	unified.Register(&unified.EndpointDef[UnifiedFragmentationRequest, UnifiedFragmentationResponse]{
		Name:        "fragmentation_analysis",
		Description: "Analyze GPU resource fragmentation across the cluster. Returns fragmentation score per node, recommendations, and summary statistics.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes/fragmentation-analysis",
		MCPToolName: "lens_fragmentation_analysis",
		Handler:     handleFragmentationAnalysis,
	})

	// Register load balance analysis endpoint (mirrors /nodes/load-balance-analysis)
	unified.Register(&unified.EndpointDef[UnifiedLoadBalanceRequest, UnifiedLoadBalanceResponse]{
		Name:        "load_balance_analysis",
		Description: "Analyze GPU workload distribution across nodes. Returns load balance score, hotspot/idle nodes, and optimization recommendations.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes/load-balance-analysis",
		MCPToolName: "lens_load_balance_analysis",
		Handler:     handleLoadBalanceAnalysis,
	})

	// Register node fragmentation detail endpoint (mirrors /nodes/:name/fragmentation)
	unified.Register(&unified.EndpointDef[NodeFragmentationDetailRequest, NodeFragmentationDetailResponse]{
		Name:        "node_fragmentation",
		Description: "Get detailed GPU fragmentation analysis for a specific node. Returns allocation pattern, running pods, and node-specific recommendations.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/nodes/:name/fragmentation",
		MCPToolName: "lens_node_fragmentation",
		Handler:     handleNodeFragmentationDetail,
	})

	// Register GPU heatmap endpoint (mirrors /clusters/gpuHeatmap)
	unified.Register(&unified.EndpointDef[GPUHeatmapRequest, GPUHeatmapResponse]{
		Name:        "gpu_heatmap",
		Description: "Get GPU heatmap data showing power, temperature and utilization for top K GPUs. Useful for visualizing cluster-wide GPU health.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/clusters/gpuHeatmap",
		MCPToolName: "lens_gpu_heatmap",
		Handler:     handleGPUHeatmap,
	})

	// Register running tasks endpoint (mirrors /realtime/running-tasks)
	unified.Register(&unified.EndpointDef[UnifiedRunningTasksRequest, UnifiedRunningTasksResponse]{
		Name:        "running_tasks",
		Description: "Get list of currently running GPU tasks in the cluster. Returns task details including pod name, workload, node, GPU allocation and runtime.",
		HTTPMethod:  "GET",
		HTTPPath:    "/unified/realtime/running-tasks",
		MCPToolName: "lens_running_tasks",
		Handler:     handleRunningTasks,
	})
}

// ===== Handler Implementations =====

// handleClusterHealth handles cluster health requests.
// Works for both HTTP and MCP.
func handleClusterHealth(ctx context.Context, req *ClusterHealthRequest) (*ClusterHealthResponse, error) {
	cm := clientsets.GetClusterManager()
	
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Get GPU node names
	gpuNodeNames, err := gpu.GetGpuNodes(ctx, clients.K8SClientSet, metadata.GpuVendorAMD)
	if err != nil {
		return nil, err
	}

	totalNodes := len(gpuNodeNames)
	healthyNodes := totalNodes // Simplified - real implementation would check node conditions

	// Get GPU allocation info for GPU count
	gpuCount := 0
	allocations, err := gpu.GetGpuNodesAllocation(ctx, clients.K8SClientSet, clients.ClusterName, metadata.GpuVendorAMD)
	if err == nil {
		for _, alloc := range allocations {
			gpuCount += alloc.Capacity
		}
	}

	// Determine health status
	status := "healthy"
	message := "All systems operational"
	if totalNodes == 0 {
		status = "critical"
		message = "No GPU nodes found in cluster"
	} else if healthyNodes < totalNodes {
		healthyRatio := float64(healthyNodes) / float64(totalNodes)
		if healthyRatio < 0.5 {
			status = "critical"
			message = "More than 50% of nodes are unhealthy"
		} else if healthyRatio < 0.9 {
			status = "degraded"
			message = "Some nodes are unhealthy"
		}
	}

	return &ClusterHealthResponse{
		ClusterName:  clients.ClusterName,
		Status:       status,
		TotalNodes:   totalNodes,
		HealthyNodes: healthyNodes,
		GPUCount:     gpuCount,
		Timestamp:    time.Now(),
		Message:      message,
	}, nil
}

// handleGPUQuickStats handles GPU quick stats requests.
func handleGPUQuickStats(ctx context.Context, req *GPUQuickStatsRequest) (*GPUQuickStatsResponse, error) {
	cm := clientsets.GetClusterManager()
	
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Get GPU allocation info
	allocations, err := gpu.GetGpuNodesAllocation(ctx, clients.K8SClientSet, clients.ClusterName, metadata.GpuVendorAMD)
	if err != nil {
		return nil, err
	}

	// Calculate stats
	totalGPUs := 0
	allocatedGPUs := 0
	for _, alloc := range allocations {
		totalGPUs += alloc.Capacity
		allocatedGPUs += alloc.Allocated
	}
	availableGPUs := totalGPUs - allocatedGPUs

	allocationRate := float64(0)
	if totalGPUs > 0 {
		allocationRate = float64(allocatedGPUs) / float64(totalGPUs) * 100
	}

	// Get utilization (simplified - real implementation would query metrics)
	avgUtilization := float64(0)
	usage, err := gpu.CalculateGpuUsage(ctx, clients.StorageClientSet, metadata.GpuVendorAMD)
	if err == nil {
		avgUtilization = usage
	}

	return &GPUQuickStatsResponse{
		ClusterName:    clients.ClusterName,
		TotalGPUs:      totalGPUs,
		AllocatedGPUs:  allocatedGPUs,
		AvailableGPUs:  availableGPUs,
		AllocationRate: allocationRate,
		AvgUtilization: avgUtilization,
		Timestamp:      time.Now(),
	}, nil
}

// handleEcho handles echo requests - simple test endpoint.
func handleEcho(ctx context.Context, req *EchoRequest) (*EchoResponse, error) {
	count := req.Count
	if count <= 0 {
		count = 1
	}
	if count > 100 {
		count = 100 // Limit to prevent abuse
	}

	// Build repeated message
	echo := ""
	for i := 0; i < count; i++ {
		if i > 0 {
			echo += " "
		}
		echo += req.Message
	}

	return &EchoResponse{
		Echo:      echo,
		Count:     count,
		Timestamp: time.Now(),
		Source:    "unified", // Handler doesn't know source, it's the same for both
	}, nil
}

// ===== Phase 2 Handler Implementations =====

// handleClusterOverview handles cluster overview requests.
// Mirrors the logic from getClusterOverview in cluster.go
func handleClusterOverview(ctx context.Context, req *ClusterOverviewRequest) (*ClusterOverviewResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Try to get cached data first
	result, err := cluster.GetClusterOverviewFromCache(ctx, clients.ClusterName)
	if err == nil && result != nil {
		return &ClusterOverviewResponse{
			GpuClusterOverview: *result,
			ClusterName:        clients.ClusterName,
			Timestamp:          time.Now(),
		}, nil
	}

	// Cache miss - fall back to real-time calculation
	gpuNodes, err := gpu.GetGpuNodes(ctx, clients.K8SClientSet, metadata.GpuVendorAMD)
	if err != nil {
		return nil, err
	}
	faultyNodes, err := fault.GetFaultyNodes(ctx, clients.K8SClientSet, gpuNodes)
	if err != nil {
		return nil, err
	}
	idle, partialIdle, busy, err := gpu.GetGpuNodeIdleInfo(ctx, clients.K8SClientSet, clients.ClusterName, metadata.GpuVendorAMD)
	if err != nil {
		return nil, err
	}
	usage, err := gpu.CalculateGpuUsage(ctx, clients.StorageClientSet, metadata.GpuVendorAMD)
	if err != nil {
		return nil, err
	}
	allocationRate, err := gpu.GetClusterGpuAllocationRateFromDB(ctx, database.GetFacade().GetPod(), database.GetFacade().GetNode())
	if err != nil {
		return nil, err
	}
	storageStat, err := storage.GetStorageStat(ctx)
	if err != nil {
		return nil, err
	}
	rdmaStat, err := rdma.GetRdmaClusterStat(ctx, clients.StorageClientSet)
	if err != nil {
		return nil, err
	}

	overview := model.GpuClusterOverview{
		RdmaClusterStat:    rdmaStat,
		StorageStat:        *storageStat,
		TotalNodes:         len(gpuNodes),
		HealthyNodes:       len(gpuNodes) - len(faultyNodes),
		FaultyNodes:        len(faultyNodes),
		FullyIdleNodes:     idle,
		PartiallyIdleNodes: partialIdle,
		BusyNodes:          busy,
		AllocationRate:     allocationRate,
		Utilization:        usage,
	}

	return &ClusterOverviewResponse{
		GpuClusterOverview: overview,
		ClusterName:        clients.ClusterName,
		Timestamp:          time.Now(),
	}, nil
}

// handleClusterConsumers handles cluster consumers requests.
// Mirrors the logic from getConsumerInfo in workload.go
func handleClusterConsumers(ctx context.Context, req *ClusterConsumersRequest) (*ClusterConsumersResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Set default pagination
	pageNum := req.PageNum
	pageSize := req.PageSize
	if pageNum <= 0 {
		pageNum = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	runningWorkload, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().ListRunningWorkload(ctx)
	if err != nil {
		return nil, err
	}

	result := []model.TopLevelGpuResource{}
	for _, dbWorkload := range runningWorkload {
		r := model.TopLevelGpuResource{
			Kind:      dbWorkload.Kind,
			Name:      dbWorkload.Name,
			Namespace: dbWorkload.Namespace,
			Uid:       dbWorkload.UID,
			Stat: model.GpuStat{
				GpuRequest:     int(dbWorkload.GpuRequest),
				GpuUtilization: 0,
			},
			Pods:   nil,
			Source: getSource(dbWorkload),
		}
		// Get GPU utilization
		r.Stat.GpuUtilization, _ = workload.GetCurrentWorkloadGpuUtilization(ctx, dbWorkload.UID, clients.StorageClientSet)
		result = append(result, r)
	}

	data, _, total, _ := sliceUtil.PaginateSlice(result, pageNum, pageSize)

	return &ClusterConsumersResponse{
		Data:        data,
		Total:       total,
		ClusterName: clients.ClusterName,
	}, nil
}

// handleGPUNodeList handles GPU node list requests.
// Mirrors the logic from getGPUNodeList in node.go
func handleGPUNodeList(ctx context.Context, req *GPUNodeListRequest) (*GPUNodeListResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Set default pagination
	pageNum := req.PageNum
	pageSize := req.PageSize
	if pageNum <= 0 {
		pageNum = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	// Build search request
	searchReq := model.SearchGpuNodeReq{
		Name:    req.Name,
		GpuName: req.GpuName,
		Status:  req.Status,
		OrderBy: req.OrderBy,
		Desc:    req.Desc,
	}
	searchReq.PageNum = pageNum
	searchReq.PageSize = pageSize

	filter := searchReq.ToNodeFilter()

	dbNodes, total, err := database.GetFacadeForCluster(clients.ClusterName).GetNode().SearchNode(ctx, filter)
	if err != nil {
		return nil, err
	}

	return &GPUNodeListResponse{
		Data:        batchCvtDbNode2GpuNodeListNode(dbNodes),
		Total:       total,
		ClusterName: clients.ClusterName,
	}, nil
}

// handleGPUAllocation handles GPU allocation info requests.
// Mirrors the logic from getClusterGpuAllocationInfo in node.go
func handleGPUAllocation(ctx context.Context, req *GPUAllocationRequest) (*GPUAllocationResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Try to get from cache first
	cacheFacade := database.GetFacadeForCluster(clients.ClusterName).GetGenericCache()
	cacheKey := "cluster:gpu:allocation_info"

	var result []model.GpuAllocation
	err = cacheFacade.Get(ctx, cacheKey, &result)
	if err != nil {
		// Cache miss, fallback to real-time calculation
		result, err = gpu.GetGpuNodesAllocation(ctx, clients.K8SClientSet, clients.ClusterName, metadata.GpuVendorAMD)
		if err != nil {
			return nil, err
		}
	}

	// Calculate summary
	summary := GPUAllocationSummary{
		TotalNodes: len(result),
	}
	for _, alloc := range result {
		summary.TotalGPUs += alloc.Capacity
		summary.AllocatedGPUs += alloc.Allocated
	}
	summary.AvailableGPUs = summary.TotalGPUs - summary.AllocatedGPUs
	if summary.TotalGPUs > 0 {
		summary.AllocationRate = float64(summary.AllocatedGPUs) / float64(summary.TotalGPUs) * 100
	}

	return &GPUAllocationResponse{
		Data:        result,
		ClusterName: clients.ClusterName,
		Summary:     summary,
		Timestamp:   time.Now(),
	}, nil
}

// ===== Phase 3 Handler Implementations =====

// handleGPUUtilization handles GPU utilization requests.
func handleGPUUtilization(ctx context.Context, req *GPUUtilizationRequest) (*GPUUtilizationResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Try to get from cache first
	cacheFacade := database.GetFacadeForCluster(clients.ClusterName).GetGenericCache()
	cacheKey := "cluster:gpu:utilization"

	var cachedResult model.GPUUtilization
	err = cacheFacade.Get(ctx, cacheKey, &cachedResult)
	if err == nil {
		return &GPUUtilizationResponse{
			ClusterName:    clients.ClusterName,
			AllocationRate: cachedResult.AllocationRate,
			Utilization:    cachedResult.Utilization,
			Timestamp:      time.Now(),
		}, nil
	}

	// Cache miss, fallback to real-time calculation
	usage, err := gpu.CalculateGpuUsage(ctx, clients.StorageClientSet, metadata.GpuVendorAMD)
	if err != nil {
		return nil, err
	}
	allocationRate, err := gpu.GetClusterGpuAllocationRateFromDB(ctx, database.GetFacade().GetPod(), database.GetFacade().GetNode())
	if err != nil {
		return nil, err
	}

	return &GPUUtilizationResponse{
		ClusterName:    clients.ClusterName,
		AllocationRate: allocationRate,
		Utilization:    usage,
		Timestamp:      time.Now(),
	}, nil
}

// handleGPUUtilizationHistory handles GPU utilization history requests.
func handleGPUUtilizationHistory(ctx context.Context, req *GPUUtilizationHistoryRequest) (*GPUUtilizationHistoryResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	startUnix, err := strconv.ParseInt(req.Start, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid start timestamp")
	}
	endUnix, err := strconv.ParseInt(req.End, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid end timestamp")
	}

	step := 60
	if req.Step != "" {
		step, err = strconv.Atoi(req.Step)
		if err != nil || step <= 0 {
			return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid step value")
		}
	}

	startTime := time.Unix(startUnix, 0)
	endTime := time.Unix(endUnix, 0)

	usageHistory, err := gpu.GetHistoryGpuUsage(ctx, clients.StorageClientSet, metadata.GpuVendorAMD, startTime, endTime, step)
	if err != nil {
		return nil, err
	}
	allocationHistory, err := gpu.GetHistoryGpuAllocationRate(ctx, clients.StorageClientSet, metadata.GpuVendorAMD, startTime, endTime, step)
	if err != nil {
		return nil, err
	}
	vramHistory, err := gpu.GetNodeGpuVramUsageHistory(ctx, clients.StorageClientSet, metadata.GpuVendorAMD, startTime, endTime, step)
	if err != nil {
		return nil, err
	}

	return &GPUUtilizationHistoryResponse{
		ClusterName:     clients.ClusterName,
		AllocationRate:  allocationHistory,
		Utilization:     usageHistory,
		VramUtilization: vramHistory,
	}, nil
}

// handleNodeDetail handles node detail requests.
func handleNodeDetail(ctx context.Context, req *NodeDetailRequest) (*NodeDetailResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	dbNode, err := database.GetFacadeForCluster(clients.ClusterName).GetNode().GetNodeByName(ctx, req.NodeName)
	if err != nil {
		return nil, err
	}
	if dbNode == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("node not found")
	}

	detail := model.GpuNodeDetail{
		Name:              dbNode.Name,
		Health:            dbNode.Status,
		Cpu:               fmt.Sprintf("%d X %s", dbNode.CPUCount, dbNode.CPU),
		Memory:            dbNode.Memory,
		OS:                dbNode.Os,
		StaticGpuDetails:  fmt.Sprintf("%d X %s", dbNode.GpuCount, dbNode.GpuName),
		KubeletVersion:    dbNode.KubeletVersion,
		ContainerdVersion: dbNode.ContainerdVersion,
		GPUDriverVersion:  dbNode.DriverVersion,
	}

	return &NodeDetailResponse{
		GpuNodeDetail: detail,
		ClusterName:   clients.ClusterName,
	}, nil
}

// handleNodeGPUDevices handles node GPU devices requests.
func handleNodeGPUDevices(ctx context.Context, req *NodeGPUDevicesRequest) (*NodeGPUDevicesResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	node, err := database.GetFacadeForCluster(clients.ClusterName).GetNode().GetNodeByName(ctx, req.NodeName)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("node not found")
	}

	devices, err := database.GetFacadeForCluster(clients.ClusterName).GetNode().ListGpuDeviceByNodeId(ctx, node.ID)
	if err != nil {
		return nil, err
	}

	deviceInfos := make([]model.GpuDeviceInfo, 0, len(devices))
	for _, d := range devices {
		deviceInfos = append(deviceInfos, model.GpuDeviceInfo{
			DeviceId:    int(d.GpuID),
			Model:       d.GpuModel,
			Memory:      fmt.Sprintf("%dGB", d.Memory/1024),
			Utilization: d.Utilization,
			Temperature: d.Temperature,
			Power:       d.Power,
		})
	}

	return &NodeGPUDevicesResponse{
		NodeName:    req.NodeName,
		ClusterName: clients.ClusterName,
		Devices:     deviceInfos,
	}, nil
}

// handleWorkloadList handles workload list requests.
func handleWorkloadList(ctx context.Context, req *WorkloadListRequest) (*WorkloadListResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	pageNum := req.PageNum
	pageSize := req.PageSize
	if pageNum <= 0 {
		pageNum = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	emptyParentUid := ""
	f := &filter.WorkloadFilter{
		Limit:     pageSize,
		Offset:    (pageNum - 1) * pageSize,
		ParentUid: &emptyParentUid,
	}
	if req.Name != "" {
		f.Name = &req.Name
	}
	if req.Kind != "" {
		f.Kind = &req.Kind
	}
	if req.Namespace != "" {
		f.Namespace = &req.Namespace
	}
	if req.Status != "" {
		f.Status = &req.Status
	}
	if req.OrderBy != "" {
		switch req.OrderBy {
		case "start_at":
			f.OrderBy = "created_at"
		case "end_at":
			f.OrderBy = "end_at"
		}
	}

	workloads, count, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().QueryWorkload(ctx, f)
	if err != nil {
		return nil, err
	}

	result := []model.WorkloadListItem{}
	for _, w := range workloads {
		item, _ := cvtDBWorkloadListItem(ctx, clients.ClusterName, w)
		result = append(result, item)
	}

	return &WorkloadListResponse{
		Data:        result,
		Total:       count,
		ClusterName: clients.ClusterName,
	}, nil
}

// handleWorkloadDetail handles workload detail requests.
func handleWorkloadDetail(ctx context.Context, req *WorkloadDetailRequest) (*WorkloadDetailResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	dbWorkload, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().GetGpuWorkloadByUid(ctx, req.WorkloadUID)
	if err != nil {
		return nil, err
	}
	if dbWorkload == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("workload not found")
	}

	workloadInfo := model.WorkloadInfo{
		ApiVersion:    dbWorkload.GroupVersion,
		Kind:          dbWorkload.Kind,
		Name:          dbWorkload.Name,
		Namespace:     dbWorkload.Namespace,
		Uid:           dbWorkload.UID,
		GpuAllocation: nil,
		Pods:          []model.WorkloadInfoPod{},
		ActivePods:    []model.WorkloadInfoPod{},
		StartTime:     dbWorkload.CreatedAt.Unix(),
		EndTime:       dbWorkload.EndAt.Unix(),
		Source:        getSource(dbWorkload),
	}
	if dbWorkload.EndAt.Unix() < int64(8*time.Hour) {
		workloadInfo.EndTime = 0
	}

	pods, err := workload.GetWorkloadPods(ctx, clients.ClusterName, dbWorkload.UID)
	if err != nil {
		return nil, err
	}
	for _, pod := range pods {
		podInfo := model.WorkloadInfoPod{
			PodUID:       pod.UID,
			PodNamespace: pod.Namespace,
			PodName:      pod.Name,
			NodeName:     pod.NodeName,
			Phase:        pod.Phase,
			Running:      pod.Running,
			IP:           pod.IP,
			GpuAllocated: int(pod.GpuAllocated),
			CreatedAt:    pod.CreatedAt.Unix(),
			UpdatedAt:    pod.UpdatedAt.Unix(),
		}
		workloadInfo.Pods = append(workloadInfo.Pods, podInfo)
		if pod.Running && !pod.Deleted {
			workloadInfo.ActivePods = append(workloadInfo.ActivePods, podInfo)
		}
	}

	gpuAllocation, err := workload.GetWorkloadResource(ctx, clients.ClusterName, dbWorkload.UID)
	if err == nil {
		workloadInfo.GpuAllocation = gpuAllocation
	}

	return &WorkloadDetailResponse{
		WorkloadInfo: workloadInfo,
		ClusterName:  clients.ClusterName,
	}, nil
}

// handlePodStats handles pod stats requests.
func handlePodStats(ctx context.Context, req *PodStatsRequest) (*UnifiedPodStatsResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(req.Cluster)
	if err != nil {
		return nil, err
	}

	page := req.Page
	pageSize := req.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	podFacade := database.GetFacadeForCluster(clients.ClusterName).GetPod()
	gpuPods, total, err := podFacade.QueryPodsWithFilters(
		ctx,
		req.Namespace,
		req.PodName,
		req.StartTime,
		req.EndTime,
		page,
		pageSize,
	)
	if err != nil {
		return nil, err
	}

	pods := make([]PodStats, 0, len(gpuPods))
	for _, pod := range gpuPods {
		avgUtil, _ := podFacade.GetAverageGPUUtilizationByNode(ctx, pod.NodeName)
		status := "Unknown"
		if pod.Running {
			status = "Running"
		} else {
			switch pod.Phase {
			case "Pending":
				status = "Pending"
			case "Succeeded":
				status = "Succeeded"
			case "Failed":
				status = "Failed"
			}
		}

		pods = append(pods, PodStats{
			PodUID:         pod.UID,
			PodName:        pod.Name,
			Namespace:      pod.Namespace,
			NodeName:       pod.NodeName,
			Status:         status,
			Phase:          pod.Phase,
			CreatedAt:      pod.CreatedAt,
			AllocatedGPUs:  pod.GpuAllocated,
			AvgUtilization: avgUtil,
			Running:        pod.Running,
			OwnerUID:       pod.OwnerUID,
			IP:             pod.IP,
		})
	}

	return &UnifiedPodStatsResponse{
		Total:       int(total),
		Page:        page,
		Pods:        pods,
		ClusterName: clients.ClusterName,
	}, nil
}

// handleRealtimeStatus handles realtime status requests.
func handleRealtimeStatus(ctx context.Context, req *RealtimeStatusRequest) (*RealtimeStatusResult, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clients.ClusterName)

	// Get GPU allocation info
	allocations, err := gpu.GetGpuNodesAllocation(ctx, clients.K8SClientSet, clients.ClusterName, metadata.GpuVendorAMD)
	if err != nil {
		return nil, err
	}

	totalGPUs := 0
	allocatedGPUs := 0
	for _, alloc := range allocations {
		totalGPUs += alloc.Capacity
		allocatedGPUs += alloc.Allocated
	}

	// Get utilization
	utilization, _ := gpu.CalculateGpuUsage(ctx, clients.StorageClientSet, metadata.GpuVendorAMD)

	// Estimate utilized GPUs (>50% utilization)
	utilizedGPUs := int(float64(allocatedGPUs) * utilization / 100)

	// Get running tasks count
	pods, _ := facade.GetPod().ListActiveGpuPods(ctx)

	// Calculate available nodes (nodes with available GPU slots)
	availableNodes := 0
	maxContiguous := 0
	for _, alloc := range allocations {
		available := alloc.Capacity - alloc.Allocated
		if available > 0 {
			availableNodes++
			if available > maxContiguous {
				maxContiguous = available
			}
		}
	}

	return &RealtimeStatusResult{
		Cluster:   clients.ClusterName,
		Timestamp: time.Now(),
		CurrentGPUUsage: RealtimeGPUUsage{
			TotalGPUs:       totalGPUs,
			AllocatedGPUs:   allocatedGPUs,
			UtilizedGPUs:    utilizedGPUs,
			AllocationRate:  float64(allocatedGPUs) / float64(totalGPUs) * 100,
			UtilizationRate: utilization,
		},
		RunningTasks: len(pods),
		AvailableResources: RealtimeAvailability{
			AvailableGPUs:    totalGPUs - allocatedGPUs,
			AvailableNodes:   availableNodes,
			MaxContiguousGPU: maxContiguous,
		},
	}, nil
}

// ===== Phase 4: Handler Implementations =====

// handleNodeGPUMetrics handles node GPU metrics requests.
func handleNodeGPUMetrics(ctx context.Context, req *NodeGPUMetricsRequest) (*NodeGPUMetricsResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	step := req.Step
	if step <= 0 {
		step = 60
	}

	startTime := time.Unix(req.Start, 0)
	endTime := time.Unix(req.End, 0)

	// Get GPU utilization history
	gpuUtil, err := node.GetNodeGpuUtilHistory(ctx, clients.StorageClientSet, metadata.GpuVendorAMD, req.NodeName, startTime, endTime, step)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithError(err).WithMessage("Failed to get GPU utilization history")
	}

	// Get GPU allocation rate history
	gpuAllocRate, err := node.GetNodeGpuAllocationHistory(ctx, clients.StorageClientSet, metadata.GpuVendorAMD, req.NodeName, startTime, endTime, step)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithError(err).WithMessage("Failed to get GPU allocation history")
	}

	// Convert to response format
	return &NodeGPUMetricsResponse{
		GpuUtilization: MetricsGraph{
			Series: convertToMetricsSeries(gpuUtil),
			Config: MetricsGraphConfig{YAxisUnit: "%"},
		},
		GpuAllocationRate: MetricsGraph{
			Series: convertToMetricsSeries(gpuAllocRate),
			Config: MetricsGraphConfig{YAxisUnit: "%"},
		},
	}, nil
}

// convertToMetricsSeries converts model.MetricsSeries to our MetricsSeries.
func convertToMetricsSeries(series []model.MetricsSeries) []MetricsSeries {
	result := make([]MetricsSeries, len(series))
	for i, s := range series {
		values := make([]MetricValue, len(s.Values))
		for j, v := range s.Values {
			values[j] = MetricValue{
				Timestamp: v.Timestamp,
				Value:     v.Value,
			}
		}
		// Extract name from labels if available
		name := ""
		if s.Labels != nil {
			if nodeName, ok := s.Labels["node"]; ok {
				name = string(nodeName)
			}
		}
		result[i] = MetricsSeries{
			Name:   name,
			Values: values,
		}
	}
	return result
}

// handleNodeUtilization handles node utilization requests.
func handleNodeUtilization(ctx context.Context, req *NodeUtilizationRequest) (*NodeUtilizationResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Get node from database
	dbNode, err := database.GetFacadeForCluster(clients.ClusterName).GetNode().GetNodeByName(ctx, req.NodeName)
	if err != nil {
		return nil, err
	}
	if dbNode == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("node not found")
	}

	// Get current utilization from recent metrics
	now := time.Now()
	oneMinuteAgo := now.Add(-1 * time.Minute)

	// Get CPU utilization
	cpuUtil, err := node.GetNodeCpuUtilHistory(ctx, clients.StorageClientSet, req.NodeName, oneMinuteAgo, now, 60)
	cpuUtilValue := 0.0
	if err == nil && len(cpuUtil) > 0 && len(cpuUtil[0].Values) > 0 {
		cpuUtilValue = cpuUtil[0].Values[len(cpuUtil[0].Values)-1].Value
	}

	// Get Memory utilization
	memUtil, err := node.GetNodeMemUtilHistory(ctx, clients.StorageClientSet, req.NodeName, oneMinuteAgo, now, 60)
	memUtilValue := 0.0
	if err == nil && len(memUtil) > 0 && len(memUtil[0].Values) > 0 {
		memUtilValue = memUtil[0].Values[len(memUtil[0].Values)-1].Value
	}

	return &NodeUtilizationResponse{
		NodeName:       dbNode.Name,
		CpuUtilization: cpuUtilValue,
		MemUtilization: memUtilValue,
		GpuUtilization: dbNode.GpuUtilization,
		GpuAllocation:  int(dbNode.GpuAllocation),
		Timestamp:      time.Now().Unix(),
	}, nil
}

// handleNodeUtilizationHistory handles node utilization history requests.
func handleNodeUtilizationHistory(ctx context.Context, req *NodeUtilizationHistoryRequest) (*NodeUtilizationHistoryResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	step := req.Step
	if step <= 0 {
		step = 60
	}

	startTime := time.Unix(req.Start, 0)
	endTime := time.Unix(req.End, 0)

	// Get CPU utilization history
	cpuUtil, err := node.GetNodeCpuUtilHistory(ctx, clients.StorageClientSet, req.NodeName, startTime, endTime, step)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithError(err).WithMessage("Failed to get CPU utilization history")
	}

	// Get Memory utilization history
	memUtil, err := node.GetNodeMemUtilHistory(ctx, clients.StorageClientSet, req.NodeName, startTime, endTime, step)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithError(err).WithMessage("Failed to get memory utilization history")
	}

	// Get GPU utilization history
	gpuUtil, err := node.GetNodeGpuUtilHistory(ctx, clients.StorageClientSet, metadata.GpuVendorAMD, req.NodeName, startTime, endTime, step)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithError(err).WithMessage("Failed to get GPU utilization history")
	}

	return &NodeUtilizationHistoryResponse{
		CpuUtilization: MetricsGraph{
			Series: convertToMetricsSeries(cpuUtil),
			Config: MetricsGraphConfig{YAxisUnit: "%"},
		},
		MemUtilization: MetricsGraph{
			Series: convertToMetricsSeries(memUtil),
			Config: MetricsGraphConfig{YAxisUnit: "%"},
		},
		GpuUtilization: MetricsGraph{
			Series: convertToMetricsSeries(gpuUtil),
			Config: MetricsGraphConfig{YAxisUnit: "%"},
		},
	}, nil
}

// handleNodeWorkloads handles node workloads requests.
func handleNodeWorkloads(ctx context.Context, req *NodeWorkloadsRequest) (*NodeWorkloadsResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	pageNum := req.PageNum
	pageSize := req.PageSize
	if pageNum <= 0 {
		pageNum = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	// Get running workloads on node
	workloads, err := workload.GetRunningTopLevelGpuWorkloadByNode(ctx, clients.ClusterName, req.NodeName)
	if err != nil {
		return nil, err
	}

	// Paginate
	total := len(workloads)
	start := (pageNum - 1) * pageSize
	end := start + pageSize
	if start >= total {
		return &NodeWorkloadsResponse{Data: []NodeWorkloadInfo{}, Total: total}, nil
	}
	if end > total {
		end = total
	}
	workloads = workloads[start:end]

	data := make([]NodeWorkloadInfo, 0, len(workloads))
	for _, w := range workloads {
		data = append(data, NodeWorkloadInfo{
			UID:          w.UID,
			Name:         w.Name,
			Namespace:    w.Namespace,
			Kind:         w.Kind,
			GPUAllocated: w.GpuRequest,
			StartedAt:    w.CreatedAt,
			Status:       getWorkloadStatus(w),
		})
	}

	return &NodeWorkloadsResponse{
		Data:  data,
		Total: total,
	}, nil
}

// getWorkloadStatus determines workload status from db model.
func getWorkloadStatus(w *dbModel.GpuWorkload) string {
	if w.DeletedAt.Valid {
		return "Completed"
	}
	return "Running"
}

// handleNodeWorkloadsHistory handles node workloads history requests.
func handleNodeWorkloadsHistory(ctx context.Context, req *NodeWorkloadsHistoryRequest) (*NodeWorkloadsHistoryResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	pageNum := req.PageNum
	pageSize := req.PageSize
	if pageNum <= 0 {
		pageNum = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	// Get history pods for the node
	pods, total, err := database.GetFacadeForCluster(clients.ClusterName).GetPod().GetHistoryGpuPodByNodeName(ctx, req.NodeName, pageNum, pageSize)
	if err != nil {
		return nil, err
	}

	// Get workload info for pods
	workloadInfos, err := workload.GetTopLevelWorkloadsByPods(ctx, clients.ClusterName, pods)
	if err != nil {
		return nil, err
	}

	// Build workload map
	workloadMap := make(map[string]*dbModel.GpuWorkload)
	for _, w := range workloadInfos {
		workloadMap[w.UID] = w
	}

	data := make([]NodeWorkloadHistoryInfo, 0)
	seenWorkloads := make(map[string]bool)
	for _, pod := range pods {
		if w, ok := workloadMap[pod.OwnerUID]; ok {
			if seenWorkloads[w.UID] {
				continue
			}
			seenWorkloads[w.UID] = true
			duration := int64(0)
			if !w.EndAt.IsZero() && w.EndAt.After(w.CreatedAt) {
				duration = int64(w.EndAt.Sub(w.CreatedAt).Seconds())
			}
			data = append(data, NodeWorkloadHistoryInfo{
				UID:          w.UID,
				Name:         w.Name,
				Namespace:    w.Namespace,
				Kind:         w.Kind,
				GPUAllocated: w.GpuRequest,
				StartedAt:    w.CreatedAt,
				EndedAt:      w.EndAt,
				Duration:     duration,
			})
		}
	}

	return &NodeWorkloadsHistoryResponse{
		Data:  data,
		Total: int(total),
	}, nil
}

// handleFragmentationAnalysis handles fragmentation analysis requests.
func handleFragmentationAnalysis(ctx context.Context, req *UnifiedFragmentationRequest) (*UnifiedFragmentationResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Get all GPU nodes
	nodeFacade := database.GetFacadeForCluster(clients.ClusterName).GetNode()
	nodes, err := nodeFacade.ListGpuNodes(ctx)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithError(err)
	}

	if len(nodes) == 0 {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("No GPU nodes found")
	}

	// Calculate fragmentation for each node
	nodeFrags := make([]UnifiedNodeFragInfo, 0, len(nodes))
	var totalScore float64
	var healthyNodes, fragmentedNodes, criticalNodes int
	var totalWasted int32

	for _, n := range nodes {
		availableGPUs := n.GpuCount - n.GpuAllocation
		allocationRate := float64(n.GpuAllocation) / float64(n.GpuCount)
		
		// Simplified fragmentation score calculation
		baseFragmentation := (1 - allocationRate) * 40
		utilizationGap := 0.0
		if n.GpuUtilization < allocationRate*100 {
			utilizationGap = (allocationRate*100 - n.GpuUtilization) / 100 * 40
		}
		score := baseFragmentation + utilizationGap
		if score > 100 {
			score = 100
		}

		status := "healthy"
		if score > 60 {
			status = "critical"
			criticalNodes++
		} else if score > 30 {
			status = "fragmented"
			fragmentedNodes++
		} else {
			healthyNodes++
		}

		// Calculate wasted GPUs (allocated but underutilized)
		if n.GpuUtilization < 30 && n.GpuAllocation > 0 {
			totalWasted += n.GpuAllocation
		}

		nodeFrags = append(nodeFrags, UnifiedNodeFragInfo{
			NodeName:           n.Name,
			TotalGPUs:          n.GpuCount,
			AllocatedGPUs:      n.GpuAllocation,
			AvailableGPUs:      availableGPUs,
			FragmentationScore: score,
			Status:             status,
			Utilization:        n.GpuUtilization,
		})
		totalScore += score
	}

	// Calculate cluster-wide score
	clusterScore := totalScore / float64(len(nodes))

	// Generate recommendations
	recommendations := generateFragmentationRecommendationsUnified(clusterScore, criticalNodes)

	// Calculate total GPUs for waste percentage
	var totalGPUs int32
	for _, n := range nodes {
		totalGPUs += n.GpuCount
	}
	wastePercentage := float64(totalWasted) / float64(totalGPUs) * 100

	return &UnifiedFragmentationResponse{
		Cluster:                   req.Cluster,
		ClusterFragmentationScore: clusterScore,
		TotalNodes:                len(nodes),
		NodeFragmentations:        nodeFrags,
		Recommendations:           recommendations,
		Summary: UnifiedFragmentationSummary{
			HealthyNodes:    healthyNodes,
			FragmentedNodes: fragmentedNodes,
			CriticalNodes:   criticalNodes,
			TotalWastedGPUs: int(totalWasted),
			WastePercentage: wastePercentage,
		},
	}, nil
}

// generateFragmentationRecommendationsUnified generates fragmentation recommendations.
func generateFragmentationRecommendationsUnified(clusterScore float64, criticalNodes int) []string {
	recommendations := make([]string, 0)
	if clusterScore > 50 {
		recommendations = append(recommendations, "High cluster fragmentation detected. Consider consolidating workloads to fewer nodes.")
	}
	if criticalNodes > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d critical nodes detected. Review GPU allocation patterns on these nodes.", criticalNodes))
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Cluster GPU allocation is healthy. No immediate action needed.")
	}
	return recommendations
}

// handleLoadBalanceAnalysis handles load balance analysis requests.
func handleLoadBalanceAnalysis(ctx context.Context, req *UnifiedLoadBalanceRequest) (*UnifiedLoadBalanceResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Get all GPU nodes
	nodeFacade := database.GetFacadeForCluster(clients.ClusterName).GetNode()
	nodes, err := nodeFacade.ListGpuNodes(ctx)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithError(err)
	}

	if len(nodes) == 0 {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("No GPU nodes found")
	}

	// Calculate load for each node
	nodeLoads := make([]UnifiedNodeLoadInfo, 0, len(nodes))
	var sum, max, min float64
	min = 100
	for _, n := range nodes {
		allocationRate := 0.0
		if n.GpuCount > 0 {
			allocationRate = float64(n.GpuAllocation) / float64(n.GpuCount) * 100
		}
		loadScore := allocationRate*0.6 + n.GpuUtilization*0.4

		nodeLoads = append(nodeLoads, UnifiedNodeLoadInfo{
			NodeName:        n.Name,
			AllocationRate:  allocationRate,
			UtilizationRate: n.GpuUtilization,
			LoadScore:       loadScore,
		})

		sum += allocationRate
		if allocationRate > max {
			max = allocationRate
		}
		if allocationRate < min {
			min = allocationRate
		}
	}

	// Calculate statistics
	mean := sum / float64(len(nodeLoads))
	var variance float64
	for _, load := range nodeLoads {
		variance += (load.AllocationRate - mean) * (load.AllocationRate - mean)
	}
	variance /= float64(len(nodeLoads))
	stddev := 0.0
	if variance > 0 {
		stddev = math.Sqrt(variance)
	}

	// Calculate load balance score
	cv := 0.0
	if mean > 0 {
		cv = stddev / mean
	}
	loadBalanceScore := 100 * (1 - math.Min(cv, 1))

	// Identify hotspot and idle nodes
	hotspotNodes := make([]string, 0)
	idleNodes := make([]string, 0)
	for _, load := range nodeLoads {
		if load.AllocationRate > 80 {
			hotspotNodes = append(hotspotNodes, load.NodeName)
		} else if load.AllocationRate < 20 {
			idleNodes = append(idleNodes, load.NodeName)
		}
	}

	// Generate recommendations
	recommendations := generateLoadBalanceRecommendationsUnified(loadBalanceScore, hotspotNodes, idleNodes)

	return &UnifiedLoadBalanceResponse{
		Cluster:              req.Cluster,
		LoadBalanceScore:     loadBalanceScore,
		NodeLoadDistribution: nodeLoads,
		HotspotNodes:         hotspotNodes,
		IdleNodes:            idleNodes,
		Recommendations:      recommendations,
		Statistics: UnifiedLoadBalanceStats{
			AvgAllocationRate: mean,
			StdDevAllocation:  stddev,
			MaxAllocation:     max,
			MinAllocation:     min,
			Variance:          variance,
		},
	}, nil
}

// generateLoadBalanceRecommendationsUnified generates load balance recommendations.
func generateLoadBalanceRecommendationsUnified(score float64, hotspotNodes, idleNodes []string) []string {
	recommendations := make([]string, 0)
	if score < 50 {
		recommendations = append(recommendations, "Poor load balance detected. Consider redistributing workloads.")
	}
	if len(hotspotNodes) > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d hotspot nodes detected (>80%% allocation). Consider migrating workloads.", len(hotspotNodes)))
	}
	if len(idleNodes) > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d idle nodes detected (<20%% allocation). Consider scheduling more workloads.", len(idleNodes)))
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Cluster workload distribution is balanced. No action needed.")
	}
	return recommendations
}

// handleNodeFragmentationDetail handles node fragmentation detail requests.
func handleNodeFragmentationDetail(ctx context.Context, req *NodeFragmentationDetailRequest) (*NodeFragmentationDetailResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Get node info
	nodeFacade := database.GetFacadeForCluster(clients.ClusterName).GetNode()
	dbNode, err := nodeFacade.GetNodeByName(ctx, req.NodeName)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithError(err)
	}
	if dbNode == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("Node not found")
	}

	// Get pods on this node
	podFacade := database.GetFacadeForCluster(clients.ClusterName).GetPod()
	pods, err := podFacade.GetActiveGpuPodByNodeName(ctx, req.NodeName)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithError(err)
	}

	// Calculate fragmentation
	availableGPUs := dbNode.GpuCount - dbNode.GpuAllocation
	allocationRate := float64(dbNode.GpuAllocation) / float64(dbNode.GpuCount)
	baseFragmentation := (1 - allocationRate) * 40
	utilizationGap := 0.0
	if dbNode.GpuUtilization < allocationRate*100 {
		utilizationGap = (allocationRate*100 - dbNode.GpuUtilization) / 100 * 40
	}
	score := baseFragmentation + utilizationGap
	if score > 100 {
		score = 100
	}

	status := "healthy"
	if score > 60 {
		status = "critical"
	} else if score > 30 {
		status = "fragmented"
	}

	// Build allocation pattern
	fullyAlloc := 0
	partialAlloc := 0
	for _, pod := range pods {
		if pod.GpuAllocated >= 4 {
			fullyAlloc++
		} else if pod.GpuAllocated > 0 {
			partialAlloc++
		}
	}

	// Build pod allocations
	podAllocations := make([]PodAllocationInfo, 0, len(pods))
	for _, pod := range pods {
		podAllocations = append(podAllocations, PodAllocationInfo{
			PodName:       pod.Name,
			Namespace:     pod.Namespace,
			AllocatedGPUs: pod.GpuAllocated,
		})
	}

	// Generate recommendations
	recommendations := make([]string, 0)
	if status == "critical" {
		recommendations = append(recommendations, "Critical fragmentation: Consider migrating some pods to other nodes")
	}
	if partialAlloc > 3 && dbNode.GpuCount >= 8 {
		recommendations = append(recommendations, "Many small GPU allocations detected. Consider consolidating workloads")
	}
	if availableGPUs > 0 && availableGPUs < 4 {
		recommendations = append(recommendations, "Limited contiguous GPU blocks. Difficult to schedule larger jobs")
	}
	if dbNode.GpuUtilization < 30 && dbNode.GpuAllocation > 0 {
		recommendations = append(recommendations, "Low GPU utilization despite allocation. Check if pods are idle or waiting")
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Node GPU allocation is healthy")
	}

	return &NodeFragmentationDetailResponse{
		NodeName:           req.NodeName,
		TotalGPUs:          dbNode.GpuCount,
		AllocatedGPUs:      dbNode.GpuAllocation,
		AvailableGPUs:      availableGPUs,
		FragmentationScore: score,
		Status:             status,
		AllocationPattern: AllocationPatternResult{
			FullyAllocatedPods:   fullyAlloc,
			PartiallyAllocPods:   partialAlloc,
			GPUSharing:           false,
			LargestContiguousGPU: int(availableGPUs),
		},
		RunningPods:     podAllocations,
		Recommendations: recommendations,
	}, nil
}

// handleGPUHeatmap handles GPU heatmap requests.
func handleGPUHeatmap(ctx context.Context, req *GPUHeatmapRequest) (*GPUHeatmapResponse, error) {
	k := 5 // Top K GPUs
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClusterClientsOrDefault(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Get top K GPU metrics
	power, err := gpu.TopKGpuPowerInstant(ctx, k, clients.StorageClientSet)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithError(err).WithMessage("Failed to get GPU power data")
	}

	util, err := gpu.TopKGpuUtilizationInstant(ctx, k, clients.StorageClientSet)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithError(err).WithMessage("Failed to get GPU utilization data")
	}

	temp, err := gpu.TopKGpuTemperatureInstant(ctx, k, clients.StorageClientSet)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithError(err).WithMessage("Failed to get GPU temperature data")
	}

	return &GPUHeatmapResponse{
		Power: UnifiedHeatmapData{
			Serial:   2,
			Unit:     "W",
			YAxisMax: 850,
			YAxisMin: 0,
			Data:     convertHeatmapData(power),
		},
		Temperature: UnifiedHeatmapData{
			Serial:   3,
			Unit:     "C",
			YAxisMax: 110,
			YAxisMin: 20,
			Data:     convertHeatmapData(temp),
		},
		Utilization: UnifiedHeatmapData{
			Serial:   1,
			Unit:     "%",
			YAxisMax: 100,
			YAxisMin: 0,
			Data:     convertHeatmapData(util),
		},
	}, nil
}

// convertHeatmapData converts model.ClusterOverviewHeatmapItem to our UnifiedHeatmapPoint slice.
func convertHeatmapData(data []model.ClusterOverviewHeatmapItem) []UnifiedHeatmapPoint {
	result := make([]UnifiedHeatmapPoint, len(data))
	for i, d := range data {
		result[i] = UnifiedHeatmapPoint{
			NodeName: d.NodeName,
			DeviceID: d.GpuId,
			Value:    d.Value,
		}
	}
	return result
}

// handleRunningTasks handles running tasks requests.
func handleRunningTasks(ctx context.Context, req *UnifiedRunningTasksRequest) (*UnifiedRunningTasksResponse, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Get active GPU pods
	podFacade := database.GetFacadeForCluster(clients.ClusterName).GetPod()
	pods, err := podFacade.ListActiveGpuPods(ctx)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.InternalError).WithError(err)
	}

	// Filter by namespace if specified
	if req.Namespace != "" {
		filtered := make([]*dbModel.GpuPods, 0)
		for _, pod := range pods {
			if pod.Namespace == req.Namespace {
				filtered = append(filtered, pod)
			}
		}
		pods = filtered
	}

	// Build task list
	tasks := make([]UnifiedRunningTaskInfo, 0, len(pods))
	now := time.Now()
	for _, pod := range pods {
		runningTime := int64(now.Sub(pod.CreatedAt).Seconds())
		workloadType := "Unknown"
		workloadName := "Unknown"
		// In real implementation, we would query workload info
		// Simplified for now

		tasks = append(tasks, UnifiedRunningTaskInfo{
			PodUID:        pod.UID,
			PodName:       pod.Name,
			Namespace:     pod.Namespace,
			WorkloadType:  workloadType,
			WorkloadName:  workloadName,
			NodeName:      pod.NodeName,
			AllocatedGPUs: pod.GpuAllocated,
			RunningTime:   runningTime,
			StartedAt:     pod.CreatedAt,
			Owner:         pod.Namespace, // Simplified - use namespace as owner
		})
	}

	return &UnifiedRunningTasksResponse{
		Cluster:    req.Cluster,
		Timestamp:  now,
		TotalTasks: len(tasks),
		Tasks:      tasks,
	}, nil
}
