// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

type WorkloadHistoryNodeView struct {
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Uid          string `json:"uid"`
	GpuAllocated int    `json:"gpu_allocated"`
	PodName      string `json:"pod_name"`
	PodNamespace string `json:"pod_namespace"`
	StartTime    int64  `json:"start_time"`
	EndTime      int64  `json:"end_time"`
}

type WorkloadNodeView struct {
	Kind             string `json:"kind"`
	Name             string `json:"name"`
	Namespace        string `json:"namespace"`
	Uid              string `json:"uid"`
	GpuAllocated     int    `json:"gpu_allocated"`
	GpuAllocatedNode int    `json:"gpu_allocated_node"`
	NodeName         string `json:"node_name"`
	Status           string `json:"status"`
}
type TopLevelGpuResource struct {
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Uid       string   `json:"uid"`
	Stat      GpuStat  `json:"stat"`
	Pods      []GpuPod `json:"pods"`
	Source    string   `json:"source"`
}

func (t *TopLevelGpuResource) CalculateGpuUsage() {
	totalUsage := 0.0
	totalRequest := 0
	for _, pod := range t.Pods {
		totalUsage += pod.Stat.GpuUtilization
		totalRequest += pod.Stat.GpuRequest
	}
	t.Stat.GpuUtilization = totalUsage / float64(totalRequest)
}

type GpuStat struct {
	GpuRequest     int     `json:"gpu_request"`
	GpuUtilization float64 `json:"gpu_utilization"`
}

type GpuPod struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Node      string   `json:"node"`
	Devices   []string `json:"devices"`
	Stat      GpuStat  `json:"stat"`
}
