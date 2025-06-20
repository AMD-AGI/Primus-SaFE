/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	corev1 "k8s.io/api/core/v1"
)

type CreateNodeRequest struct {
	// The name of the cluster that the node belongs to
	Cluster *string `json:"cluster,omitempty"`
	// The name of the workspace that the node belongs to
	Workspace *string `json:"workspace,omitempty"`
	// node hostname. If not specified, it will be assigned the value of PrivateIP.
	Hostname *string `json:"hostname,omitempty"`
	// required
	PrivateIP string `json:"privateIP"`
	// optional
	PublicIP string `json:"publicIP,omitempty"`
	// SSH port，default 22
	Port *int32 `json:"port,omitempty"`
	// node labels
	Labels map[string]string `json:"labels,omitempty"`
	// the name of node flavor
	FlavorName string `json:"flavorName"`
	// the name of ssh secret
	SSHSecretName string `json:"sshSecretName,omitempty"`
}

type CreateNodeResponse struct {
	NodeId string `json:"nodeId"`
}

type ListNodeRequest struct {
	WorkspaceId *string `form:"workspaceId" binding:"omitempty,max=64"`
	ClusterId   *string `form:"clusterId" binding:"omitempty,max=64"`
	NodeFlavor  *string `form:"nodeFlavor" binding:"omitempty,max=64"`
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

type WorkloadInfo struct {
	// workload id
	Id string `json:"id"`
	// workload submitter
	User string `json:"user,omitempty"`
	// Workspace that the workload belongs to
	Workspace string `json:"workspace"`
}

type GetNodeResponseItem struct {
	// node id
	NodeId string `json:"nodeId"`
	// node display name
	DisplayName string `json:"displayName,omitempty"`
	// the node's cluster
	Cluster string `json:"cluster,omitempty"`
	// the node's workspace
	Workspace string `json:"workspace,omitempty"`
	// the node's phase
	Phase string `json:"phase,omitempty"`
	// the internal ip of k8s cluster
	InternalIP string `json:"internalIP,omitempty"`
	// the nodes' flavor
	NodeFlavor string `json:"nodeFlavor,omitempty"`
	// Indicates whether the node can be scheduled in the Kubernetes cluster.
	Unschedulable bool `json:"unschedulable,omitempty"`
	// the taints on node
	Taints []corev1.Taint `json:"taints,omitempty"`
	// total resource of node
	TotalResources ResourceList `json:"totalResources"`
	// available resource of node
	AvailResources ResourceList `json:"availResources,omitempty"`
	// Creation timestamp of the node
	CreateTime string `json:"createTime,omitempty"`
	// Running workloads information on the node
	Workloads []WorkloadInfo `json:"workloads,omitempty"`
	// the labels by customer
	CustomerLabels map[string]string `json:"customerLabels,omitempty"`
	// the last startup time
	LastStartupTime string `json:"lastStartupTime,omitempty"`
	// Indicates whether the node is the control plane node in the Kubernetes cluster.
	IsControlPlane bool `json:"isControlPlane,omitempty"`
}

type GetNodeResponse struct {
	TotalCount int                   `json:"totalCount"`
	Items      []GetNodeResponseItem `json:"items,omitempty"`
}

type PatchNodeRequest struct {
	Taints     *[]corev1.Taint    `json:"taints,omitempty"`
	Labels     *map[string]string `json:"labels,omitempty"`
	NodeFlavor *string            `json:"nodeFlavor,omitempty"`
}

type GetNodePodLogResponse struct {
	ClusterId string `json:"clusterId"`
	// node id
	NodeId string `json:"nodeId,omitempty"`
	// pod id
	PodId string `json:"podId"`
	// An array of log lines, returned in the same order as they appear in the original logs
	Logs []string `json:"logs,omitempty"`
}
