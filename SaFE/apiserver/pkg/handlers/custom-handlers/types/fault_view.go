/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

type ListFaultRequest struct {
	// The starting offset for the results. default 0
	Offset int `form:"offset" binding:"omitempty,min=0"`
	// The maximum number of returned results. default 100
	Limit int `form:"limit" binding:"omitempty,min=1"`
	// The field to sort results by. default "creation_time"
	SortBy string `form:"sortBy" binding:"omitempty"`
	// Sort order: desc/asc, default desc
	Order string `form:"order" binding:"omitempty,oneof=desc asc"`
	// Filter by node ID (fuzzy match)
	NodeId string `form:"nodeId" binding:"omitempty,max=64"`
	// Filter by monitor ID(from node-agent); multiple IDs comma-separated
	MonitorId string `form:"monitorId" binding:"omitempty"`
	// Filter results by cluster ID
	ClusterId string `form:"clusterId" binding:"omitempty"`
	// If set to true, only return faults that are currently open
	OnlyOpen bool `form:"onlyOpen" binding:"omitempty"`
}

type ListFaultResponse struct {
	// TotalCount indicates the total number of faults, not limited by pagination.
	TotalCount int                 `json:"totalCount"`
	Items      []FaultResponseItem `json:"items"`
}

type FaultResponseItem struct {
	// The uniq ID of fault
	ID string `json:"id"`
	// The node ID related to this fault
	NodeId string `json:"nodeId"`
	// The ID used by NodeAgent for monitoring.
	MonitorId string `json:"monitorId"`
	// Fault message
	Message string `json:"message"`
	// The action taken on the fault, e.g. taint
	Action string `json:"action"`
	// The status of fault, including Succeeded/Failed
	Phase string `json:"phase"`
	// The cluster ID
	ClusterId string `json:"clusterId"`
	// The creation time of fault. (RFC3339Short, e.g. "2025-07-08T10:31:46")
	CreationTime string `json:"creationTime"`
	// The deletion time of fault. (RFC3339Short, e.g. "2025-07-08T10:31:46")
	DeletionTime string `json:"deletionTime"`
}
