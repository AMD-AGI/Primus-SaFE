/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

type CreateNodeRequest struct {
	// Node hostname, uses privateIP if not specified
	Hostname *string `json:"hostname,omitempty"`
	// Node private ip, required
	PrivateIP string `json:"privateIP"`
	// Node public IP, accessible from external networks
	PublicIP string `json:"publicIP,omitempty"`
	// SSH port, default 22
	Port *int32 `json:"port,omitempty"`
	// Node labels
	Labels map[string]string `json:"labels,omitempty"`
	// Associated node flavor ID
	FlavorId string `json:"flavorId"`
	// Associated node template ID (for addon installation)
	TemplateId string `json:"templateId"`
	// The secret ID for ssh
	SSHSecretId string `json:"sshSecretId,omitempty"`
}

type CreateNodeResponse struct {
	// Node ID
	NodeId string `json:"nodeId"`
}

type ListNodeRequest struct {
	// Filter results by workspace ID
	WorkspaceId *string `form:"workspaceId" binding:"omitempty,max=64"`
	// Filter results by cluster ID
	ClusterId *string `form:"clusterId" binding:"omitempty,max=64"`
	// Filter results by node flavor ID
	FlavorId *string `form:"flavorId" binding:"omitempty,max=64"`
	// Filter results by node ID
	NodeId *string `form:"nodeId" binding:"omitempty,max=64"`
	// Filter results based on node availability
	Available *bool `form:"available" binding:"omitempty"`
	// Filter by status (comma-separated),  e.g. Ready, SSHFailed, HostnameFailed, Managing, ManagedFailed, Unmanaging, UnmanagedFailed
	Phase *v1.NodePhase `form:"phase" binding:"omitempty"`
	// Filter results based on whether the node has the addon installed
	IsAddonsInstalled *bool `form:"isAddonsInstalled" binding:"omitempty"`
	// If enabled, only node ID, name, IP, availability, and unavailability reason (if any) will be returned
	Brief bool `form:"brief" binding:"omitempty"`
	// Starting offset for the results. default: 0
	Offset int `form:"offset" binding:"omitempty,min=0"`
	// Limit the number of returned results. default: 100, -1 for all
	Limit int `form:"limit" binding:"omitempty"`
}

// GetWorkspaceId returns the workspace ID from the request.
func (req *ListNodeRequest) GetWorkspaceId() string {
	if req == nil || req.WorkspaceId == nil {
		return ""
	}
	return *req.WorkspaceId
}

// GetClusterId returns the cluster ID from the request.
func (req *ListNodeRequest) GetClusterId() string {
	if req == nil || req.ClusterId == nil {
		return ""
	}
	return *req.ClusterId
}

type ListNodeBriefResponse struct {
	// TotalCount indicates the total number of nodes, not limited by pagination
	TotalCount int                     `json:"totalCount"`
	Items      []NodeBriefResponseItem `json:"items"`
}

type NodeBriefResponseItem struct {
	// Node ID
	NodeId string `json:"nodeId"`
	// Node name
	NodeName string `json:"nodeName"`
	// The internal ip of k8s cluster
	InternalIP string `json:"internalIP"`
	// Indicates whether the node can be scheduled in the Kubernetes cluster.
	Available bool `json:"available"`
	// If a node is unavailable, provide the reason
	Message string `json:"message,omitempty"`
}

type ListNodeResponse struct {
	// TotalCount indicates the total number of nodes, not limited by pagination
	TotalCount int                `json:"totalCount"`
	Items      []NodeResponseItem `json:"items"`
}

type NodeResponseItem struct {
	NodeBriefResponseItem
	// The cluster ID of node
	ClusterId string `json:"clusterId"`
	// The workspace ID and name of node
	Workspace WorkspaceEntry `json:"workspace"`
	// Node phase, e.g. Ready, SSHFailed, HostnameFailed, Managing, ManagedFailed, Unmanaging, UnmanagedFailed
	Phase string `json:"phase"`
	// Total resource of node
	TotalResources ResourceList `json:"totalResources"`
	// Available resource of node
	AvailResources ResourceList `json:"availResources"`
	// Creation timestamp of the node (RFC3339Short)
	CreationTime string `json:"creationTime"`
	// Running workloads information on the node
	Workloads []WorkloadInfo `json:"workloads"`
	// Indicates whether the node is the control plane node in the Kubernetes cluster.
	IsControlPlane bool `json:"isControlPlane"`
	// Indicates whether the addons of node-template are installed.
	IsAddonsInstalled bool `json:"isAddonsInstalled"`
	// GPU utilization percentage from node statistics (0-100)
	GpuUtilization *float64 `json:"gpuUtilization,omitempty"`
}

type GetNodeResponse struct {
	NodeResponseItem
	// Node flavor ID
	FlavorId string `json:"flavorId"`
	// Node template ID
	TemplateId string `json:"templateId"`
	// The taints on node
	Taints []corev1.Taint `json:"taints"`
	// The labels by customer
	Labels map[string]string `json:"labels"`
	// The last startup time on node (RFC3339Short)
	LastStartupTime string `json:"lastStartupTime"`
}

type WorkloadInfo struct {
	// Workload ID
	Id string `json:"id"`
	// Workload Kind
	Kind string `json:"kind"`
	// User ID of the workload submitter
	UserId string `json:"userId"`
	// Workspace that the workload belongs to
	WorkspaceId string `json:"workspaceId"`
}

type PatchNodeRequest struct {
	// Taints to modify on the node
	Taints *[]corev1.Taint `json:"taints,omitempty"`
	// Labels to modify on the node.
	Labels *map[string]string `json:"labels,omitempty"`
	// Node Flavor ID to modify on the node.
	FlavorId *string `json:"flavorId,omitempty"`
	// Node Template ID to modify on the node.
	TemplateId *string `json:"templateId,omitempty"`
	// Node port for ssh
	Port *int32 `json:"port,omitempty"`
	// Node Private ip
	PrivateIP *string `json:"privateIP,omitempty"`
}

type GetNodePodLogResponse struct {
	// The cluster that the node belongs to
	ClusterId string `json:"clusterId"`
	// Node ID
	NodeId string `json:"nodeId"`
	// Pod id used to create the node
	PodId string `json:"podId"`
	// An array of log lines, returned in the same order as they appear in the original logs
	Logs []string `json:"logs"`
}

type ListNodeRebootLogRequest struct {
	// Start timestamp of the query (RFC3339)
	SinceTime time.Time `form:"sinceTime" binding:"omitempty"`
	// End timestamp of the query (RFC3339)
	UntilTime time.Time `form:"untilTime" binding:"omitempty"`
	// Starting offset for the results. default 0
	Offset int `form:"offset" binding:"omitempty,min=0"`
	// Limit the number of returned results. default 100
	// If set to -1, all results will be returned.
	Limit int `form:"limit" binding:"omitempty"`
	// Sort results by the specified field. default creation_time
	SortBy string `form:"sortBy" binding:"omitempty"`
	// The sorting order. Valid values are "desc" (default) or "asc"
	Order string `form:"order" binding:"omitempty,oneof=desc asc"`
}

type ListNodeRebootLogResponse struct {
	// TotalCount indicates the total number of nodes, not limited by pagination
	TotalCount int                         `json:"totalCount"`
	Items      []NodeRebootLogResponseItem `json:"items"`
}
type NodeRebootLogResponseItem struct {
	UserId       string `json:"userId"`
	UserName     string `json:"userName"`
	CreationTime string `json:"creationTime"`
}

type BatchNodesRequest struct {
	// List of node IDs to be processed
	NodeIds []string `json:"nodeIds"`
}

// RetryNodesRequest represents the request for retrying node operations (single or batch)
// For a single node, pass an array with one element
type RetryNodesRequest struct {
	// List of node IDs to retry
	NodeIds []string `json:"nodeIds" binding:"required,min=1"`
}

// RetryNodesResponse represents the response for batch retrying node operations
type RetryNodesResponse struct {
	// Total number of nodes requested
	TotalCount int `json:"totalCount"`
	// Number of nodes successfully processed
	SuccessCount int `json:"successCount"`
	// Details of successfully processed nodes (optional)
	SuccessNodes []RetrySuccessNode `json:"successNodes,omitempty"`
	// Details of failed nodes (optional)
	FailedNodes []RetryFailedNode `json:"failedNodes,omitempty"`
}

// RetrySuccessNode represents a node that was successfully processed
type RetrySuccessNode struct {
	NodeId      string   `json:"nodeId"`
	PodsDeleted []string `json:"podsDeleted,omitempty"`
}

// RetryFailedNode represents a node that failed to retry
type RetryFailedNode struct {
	NodeId string `json:"nodeId"`
	Error  string `json:"error"`
}
