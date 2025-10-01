/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

type ListFaultRequest struct {
	// Starting offset for the results. dfault is 0
	Offset int `form:"offset" binding:"omitempty,min=0"`
	// Limit the number of returned results. default is 100
	Limit int `form:"limit" binding:"omitempty,min=1"`
	// Sort results by the specified field. default is create_time
	SortBy string `form:"sortBy" binding:"omitempty"`
	// default is desc
	Order string `form:"order" binding:"omitempty,oneof=desc asc"`
	// the node id
	NodeId string `form:"nodeId" binding:"omitempty,max=64"`
	// the ID used by NodeAgent for monitoring
	// If specifying multiple kind queries, separate them with commas
	MonitorId string `form:"monitorId" binding:"omitempty"`
	// the cluster id
	ClusterId string `form:"clusterId" binding:"omitempty"`
	// If set to true, only open faults are queried.
	OnlyOpen bool `form:"onlyOpen" binding:"omitempty"`
}

type ListFaultResponse struct {
	TotalCount int                 `json:"totalCount"`
	Items      []FaultResponseItem `json:"items"`
}

type FaultResponseItem struct {
	// the uniq id of response
	ID string `json:"id"`
	// the node ID related to this fault
	NodeId string `json:"nodeId"`
	// the ID used by NodeAgent for monitoring.
	MonitorId string `json:"monitorId"`
	// fault message
	Message string `json:"message"`
	// the action of fault. e.g. taint
	Action string `json:"action"`
	// the status of fault, including Succeeded/Failed
	Phase string `json:"phase"`
	// cluster id
	ClusterId string `json:"clusterId"`
	// the creation time of fault
	CreationTime string `json:"creationTime"`
	// the deletion time of fault
	DeletionTime string `json:"deletionTime"`
}
