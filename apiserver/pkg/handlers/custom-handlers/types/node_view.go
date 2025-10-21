/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	corev1 "k8s.io/api/core/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

type CreateNodeRequest struct {
	// Node hostname. If not specified, it will be assigned the value of PrivateIP
	Hostname *string `json:"hostname,omitempty"`
	// Node private ip, required
	PrivateIP string `json:"privateIP"`
	// Node public IP, accessible from external networks
	PublicIP string `json:"publicIP,omitempty"`
	// SSH port，default is 22
	Port *int32 `json:"port,omitempty"`
	// Node labels
	Labels map[string]string `json:"labels,omitempty"`
	// Associated node flavor id
	FlavorId string `json:"flavorId"`
	// Associated node template id
	TemplateId string `json:"templateId"`
	// The secret id for ssh
	SSHSecretId string `json:"sshSecretId,omitempty"`
}

type CreateNodeResponse struct {
	// The node's id
	NodeId string `json:"nodeId"`
}

type ListNodeRequest struct {
	// Filter results by workspace id
	WorkspaceId *string `form:"workspaceId" binding:"omitempty,max=64"`
	// Filter results by cluster id
	ClusterId *string `form:"clusterId" binding:"omitempty,max=64"`
	// Filter results by node flavor id
	FlavorId *string `form:"flavorId" binding:"omitempty,max=64"`
	// Filter results by node id
	NodeId *string `form:"nodeId" binding:"omitempty,max=64"`
	// Filter results based on node availability
	Available *bool `form:"available" binding:"omitempty"`
	// Filter results based on node phase, such as Ready, SSHFailed, HostnameFailed, Managing, ManagedFailed, Unmanaging, UnmanagedFailed
	// If specifying multiple kind queries, separate them with commas
	Phase *v1.NodePhase `form:"phase" binding:"omitempty"`
	// Filter results based on whether the node has the addon installed
	IsAddonsInstalled *bool `form:"isAddonsInstalled" binding:"omitempty"`
	// If enabled, only the node id, node Name and node IP will be returned.
	Brief bool `form:"brief" binding:"omitempty"`
	// Starting offset for the results. dfault is 0
	Offset int `form:"offset" binding:"omitempty,min=0"`
	// Limit the number of returned results. default is 100
	// If set to -1, all results will be returned.
	Limit int `form:"limit" binding:"omitempty"`
}

func (req *ListNodeRequest) GetWorkspaceId() string {
	if req == nil || req.WorkspaceId == nil {
		return ""
	}
	return *req.WorkspaceId
}

func (req *ListNodeRequest) GetClusterId() string {
	if req == nil || req.ClusterId == nil {
		return ""
	}
	return *req.ClusterId
}

type ListNodeBriefResponse struct {
	// TotalCount indicates the total number of faults, not limited by pagination
	TotalCount int                     `json:"totalCount"`
	Items      []NodeBriefResponseItem `json:"items"`
}

type NodeBriefResponseItem struct {
	// node id
	NodeId string `json:"nodeId"`
	// node display name
	NodeName string `json:"nodeName"`
	// the internal ip of k8s cluster
	InternalIP string `json:"internalIP"`
}

type ListNodeResponse struct {
	// TotalCount indicates the total number of faults, not limited by pagination
	TotalCount int                `json:"totalCount"`
	Items      []NodeResponseItem `json:"items"`
}

type NodeResponseItem struct {
	NodeBriefResponseItem
	// The node's cluster id
	ClusterId string `json:"clusterId"`
	// The node's workspace id
	Workspace WorkspaceEntry `json:"workspace"`
	// The node's phase, such as Ready, SSHFailed, HostnameFailed, Managing, ManagedFailed, Unmanaging, UnmanagedFailed
	Phase string `json:"phase"`
	// Indicates whether the node can be scheduled in the Kubernetes cluster.
	Available bool `json:"available"`
	// If a node is unavailable, provide the reason
	Message string `json:"message,omitempty"`
	// Total resource of node
	TotalResources ResourceList `json:"totalResources"`
	// Available resource of node
	AvailResources ResourceList `json:"availResources"`
	// Creation timestamp of the node
	CreationTime string `json:"creationTime"`
	// Running workloads information on the node
	Workloads []WorkloadInfo `json:"workloads"`
	// Indicates whether the node is the control plane node in the Kubernetes cluster.
	IsControlPlane bool `json:"isControlPlane"`
	// Indicates whether the addons of node template are installed.
	IsAddonsInstalled bool `json:"isAddonsInstalled"`
}

type GetNodeResponse struct {
	NodeResponseItem
	// The node flavor id
	FlavorId string `json:"flavorId"`
	// The node template id
	TemplateId string `json:"templateId"`
	// The taints on node
	Taints []corev1.Taint `json:"taints"`
	// The labels by customer
	CustomerLabels map[string]string `json:"customerLabels"`
	// The last startup time on node
	LastStartupTime string `json:"lastStartupTime"`
}

type WorkloadInfo struct {
	// Workload id
	Id string `json:"id"`
	// User id of the workload submitter
	UserId string `json:"userId"`
	// Workspace that the workload belongs to
	WorkspaceId string `json:"workspaceId"`
}

type PatchNodeRequest struct {
	// Taints to modify on the node
	Taints *[]corev1.Taint `json:"taints,omitempty"`
	// Labels to modify on the node.
	Labels *map[string]string `json:"labels,omitempty"`
	// Node Flavor id to modify on the node.
	FlavorId *string `json:"flavorId,omitempty"`
	// Node Template id to modify on the node.
	TemplateId *string `json:"templateId,omitempty"`
	// Node port for ssh
	Port *int32 `json:"port,omitempty"`
}

type GetNodePodLogResponse struct {
	// The cluster that the node belongs to
	ClusterId string `json:"clusterId"`
	// The Node id
	NodeId string `json:"nodeId"`
	// Pod id used to create the node
	PodId string `json:"podId"`
	// An array of log lines, returned in the same order as they appear in the original logs
	Logs []string `json:"logs"`
}
