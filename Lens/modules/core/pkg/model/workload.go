// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"

type SearchWorkloadReq struct {
	rest.Page
	Name      string `form:"name"`
	Kind      string `form:"kind"`
	Namespace string `form:"namespace"`
	Status    string `form:"status"`
	OrderBy   string `form:"order_by"`
	Order     string `form:"order"`
}

type WorkloadListItem struct {
	Kind                  string            `json:"kind"`
	Name                  string            `json:"name"`
	Namespace             string            `json:"namespace"`
	Uid                   string            `json:"uid"`
	GpuAllocated          int               `json:"gpu_allocated"`
	GpuAllocation         GpuAllocationInfo `json:"gpu_allocation"`
	Status                string            `json:"status"`
	StatusColor           string            `json:"status_color"`
	StartAt               int64             `json:"start_at"`
	EndAt                 int64             `json:"end_at"`
	Source                string            `json:"source"`
	InstantGpuUtilization *float64          `json:"instant_gpu_utilization"`
	AvgGpuUtilization     float64           `json:"avg_gpu_utilization"`
	P50GpuUtilization     float64           `json:"p50_gpu_utilization"`
	P90GpuUtilization     float64           `json:"p90_gpu_utilization"`
	P95GpuUtilization     float64           `json:"p95_gpu_utilization"`
}

type GpuAllocationInfo map[string]float64

type WorkloadHierarchyItem struct {
	Kind      string                  `json:"kind"`
	Name      string                  `json:"name"`
	Namespace string                  `json:"namespace"`
	Uid       string                  `json:"uid"`
	Children  []WorkloadHierarchyItem `json:"children"`
}

type WorkloadInfo struct {
	ApiVersion    string            `json:"apiVersion"`
	Kind          string            `json:"kind"`
	Name          string            `json:"name"`
	Namespace     string            `json:"namespace"`
	Uid           string            `json:"uid"`
	GpuAllocation GpuAllocationInfo `json:"gpu_allocation"`
	Pods          []WorkloadInfoPod `json:"pods"`
	StartTime     int64             `json:"startTime"`
	EndTime       int64             `json:"endTime"`
	Source        string            `json:"source"`
}

type WorkloadInfoPod struct {
	NodeName     string `json:"nodeName"`
	PodNamespace string `json:"podNamespace"`
	PodName      string `json:"podName"`
}

type WorkloadStatisticResp struct {
	RunningWorkloadsCount        int     `json:"running_workloads_count"`
	AvgGpuAllocated              float64 `json:"avg_gpu_allocated"`
	AvgGpuUtilization            float64 `json:"avg_gpu_utilization"`
	LowUtilizationWorkloadsCount int     `json:"low_utilization_workloads_count"`
}
