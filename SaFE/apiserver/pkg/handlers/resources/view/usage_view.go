/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

import (
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/usage"
)

// The orders of the workspaces of a cluster.
const (
	UsageSortOccupancy            = "occupancy"
	UsageSortUtilization          = "utilization"
	UsageSortEffectiveUtilization = "effectiveUtilization"
	UsageSortAllocatedHours       = "allocatedHours"
)

// The tables a usage series is read from: mixed reads the hourly rollup for the older part of
// the range and the samples for the rest.
const (
	UsageSourceSample = "sample"
	UsageSourceHourly = "hourly"
	UsageSourceMixed  = "mixed"
)

// UsageQuery is the query of the usage APIs.
type UsageQuery struct {
	// Resource defaults to amd.com/gpu.
	Resource string `form:"resource"`
	// Start and End are RFC3339. End defaults to now and Start to a day before End.
	Start string `form:"start"`
	End   string `form:"end"`
	// Step is the width of a series bucket, e.g. 15m or 1h; it is never finer than the source.
	Step string `form:"step"`
	// SortBy orders the workspaces of a cluster: occupancy, utilization, effectiveUtilization or
	// allocatedHours, highest first unless Order is asc. The default orders them by name.
	SortBy string `form:"sortBy"`
	Order  string `form:"order"`
	// Limit is the length of the workload lists of a workspace, 10 by default.
	Limit int `form:"limit"`
}

// UsageRange is the range a usage response covers.
type UsageRange struct {
	Resource string    `json:"resource"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Step     string    `json:"step"`
	Source   string    `json:"source"`
}

// UsageClusterItem is the usage of the workspaces of one cluster the caller may see.
type UsageClusterItem struct {
	Cluster    string `json:"cluster"`
	Workspaces int    `json:"workspaces"`
	// Current is the latest sample, null when the cluster has none recently.
	Current *usage.Point  `json:"current"`
	Summary usage.Summary `json:"summary"`
	Series  []usage.Point `json:"series"`
}

// ListUsageClustersResponse is the usage of every cluster.
type ListUsageClustersResponse struct {
	UsageRange
	Items []UsageClusterItem `json:"items"`
}

// UsageWorkspaceItem is the usage of one workspace over the range.
type UsageWorkspaceItem struct {
	Workspace string        `json:"workspace"`
	Current   *usage.Point  `json:"current"`
	Summary   usage.Summary `json:"summary"`
}

// ListUsageWorkspacesResponse is the usage of the workspaces of one cluster.
type ListUsageWorkspacesResponse struct {
	UsageRange
	Cluster string `json:"cluster"`
	// Total is over the listed workspaces only.
	Total usage.Summary        `json:"total"`
	Items []UsageWorkspaceItem `json:"items"`
}

// UsageWorkloadItem is a running workload of a workspace.
type UsageWorkloadItem struct {
	WorkloadId  string `json:"workloadId"`
	DisplayName string `json:"displayName"`
	UserName    string `json:"userName"`
	Phase       string `json:"phase"`
	// Allocated is the amount of the resource on nodes.
	Allocated float64 `json:"allocated"`
	// GpuUtilization is the average GPU utilization in percent over the last sample interval,
	// null when its window is not covered or Robust cannot be asked.
	GpuUtilization *float64 `json:"gpuUtilization"`
}

// GetUsageWorkspaceResponse is the usage of one workspace.
type GetUsageWorkspaceResponse struct {
	UsageRange
	Workspace string        `json:"workspace"`
	Cluster   string        `json:"cluster"`
	Current   *usage.Point  `json:"current"`
	Summary   usage.Summary `json:"summary"`
	Series    []usage.Point `json:"series"`
	// TopAllocated are the running workloads holding the most of the resource.
	TopAllocated []UsageWorkloadItem `json:"topAllocated"`
	// LowestUtilization are the running workloads with the lowest GPU utilization, empty for a
	// resource that is not a GPU.
	LowestUtilization []UsageWorkloadItem `json:"lowestUtilization"`
}
