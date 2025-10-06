/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	corev1 "k8s.io/api/core/v1"
)

type CreateNodeRequest struct {
	// node hostname. If not specified, it will be assigned the value of PrivateIP.
	Hostname *string `json:"hostname,omitempty"`
	// required
	PrivateIP string `json:"privateIP"`
	// optional
	PublicIP string `json:"publicIP,omitempty"`
	// optional, the ip of bmc
	BMCIp string `json:"bmcIp,omitempty"`
	// optional, the password of bmc
	BMCPassword string `json:"bmcPassword,omitempty"`
	// SSH port，default 22
	Port *int32 `json:"port,omitempty"`
	// node labels
	Labels map[string]string `json:"labels,omitempty"`
	// the id of node flavor
	FlavorId string `json:"flavorId"`
	// the id of node template
	TemplateId string `json:"templateId"`
	// the id of ssh secret
	SSHSecretId string `json:"sshSecretId,omitempty"`
}

type CreateNodeResponse struct {
	NodeId string `json:"nodeId"`
}

type ListNodeRequest struct {
	WorkspaceId *string `form:"workspaceId" binding:"omitempty,max=64"`
	ClusterId   *string `form:"clusterId" binding:"omitempty,max=64"`
	FlavorId    *string `form:"flavorId" binding:"omitempty,max=64"`
	// If enabled, only the node ID and node IP will be returned.
	Brief bool `form:"brief" binding:"omitempty"`
	// Starting offset for the results. dfault is 0
	Offset int `form:"offset" binding:"omitempty,min=0"`
	// Limit the number of returned results. default is 100
	// If set to -1, all results will be returned.
	Limit int `form:"limit" binding:"omitempty,min=1"`
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

type ListNodeResponse struct {
	TotalCount int                `json:"totalCount"`
	Items      []NodeResponseItem `json:"items"`
}

type NodeResponseItem struct {
	// node id
	NodeId string `json:"nodeId"`
	// node display name
	DisplayName string `json:"displayName"`
	// the node's cluster id
	ClusterId string `json:"clusterId"`
	// the node's workspace
	Workspace WorkspaceEntry `json:"workspace"`
	// the node's phase
	Phase string `json:"phase"`
	// the internal ip of k8s cluster
	InternalIP string `json:"internalIP"`
	// the bmc ip of node
	BMCIP string `json:"bmcIP"`
	// the nodes' flavor id
	FlavorId string `json:"flavorId"`
	// the nodes' template id
	TemplateId string `json:"templateId"`
	// Indicates whether the node can be scheduled in the Kubernetes cluster.
	Available bool `json:"available"`
	// If a node is unavailable, provide the reason
	Message string `json:"message,omitempty"`
	// the taints on node
	Taints []corev1.Taint `json:"taints"`
	// total resource of node
	TotalResources ResourceList `json:"totalResources"`
	// available resource of node
	AvailResources ResourceList `json:"availResources"`
	// Creation timestamp of the node
	CreationTime string `json:"creationTime"`
	// Running workloads information on the node
	Workloads []WorkloadInfo `json:"workloads"`
	// the labels by customer
	CustomerLabels map[string]string `json:"customerLabels"`
	// the last startup time
	LastStartupTime string `json:"lastStartupTime"`
	// Indicates whether the node is the control plane node in the Kubernetes cluster.
	IsControlPlane bool `json:"isControlPlane"`
	// Indicates whether the addons of node template are installed.
	IsAddonsInstalled bool `json:"isAddonsInstalled"`
}

type WorkloadInfo struct {
	// workload id
	Id string `json:"id"`
	// workload submitter
	UserId string `json:"userId"`
	// Workspace that the workload belongs to
	WorkspaceId string `json:"workspaceId"`
}

type PatchNodeRequest struct {
	Taints       *[]corev1.Taint    `json:"taints,omitempty"`
	Labels       *map[string]string `json:"labels,omitempty"`
	NodeFlavor   *string            `json:"nodeFlavor,omitempty"`
	NodeTemplate *string            `json:"nodeTemplate,omitempty"`
	Port         *int32             `json:"port,omitempty"`
	BMCIp        *string            `json:"bmcIp,omitempty"`
	BMCPassword  *string            `json:"bmcPassword,omitempty"`
}

type GetNodePodLogResponse struct {
	ClusterId string `json:"clusterId"`
	// node id
	NodeId string `json:"nodeId"`
	// pod id
	PodId string `json:"podId"`
	// An array of log lines, returned in the same order as they appear in the original logs
	Logs []string `json:"logs"`
}

type RebootNodeRequest struct {
	// force: Boolean, optional, default is false.
	// true: Force restart (e.g., power off and then power on)
	// false: Graceful restart (attempt a clean shutdown before restarting)
	Force *bool `json:"force,omitempty"`
}
