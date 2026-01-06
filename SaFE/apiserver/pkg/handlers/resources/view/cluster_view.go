/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

import (
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

type CreateClusterRequest struct {
	v1.ControlPlane
	// Cluster name specified by the user
	Name string `json:"name"`
	// Cluster description
	Description string `json:"description,omitempty"`
	// The SSH secret ID specified by the user, which must exist, used for node SSH login.
	SSHSecretId string `json:"sshSecretId,omitempty"`
	// User-defined labels. Keys cannot start with "primus-safe."
	Labels map[string]string `json:"labels,omitempty"`
	// Whether the cluster is under protection. When set to true, direct deletion is not allowed unless the label is removed
	IsProtected bool `json:"isProtected,omitempty"`
}

type CreateClusterResponse struct {
	// Cluster ID
	ClusterId string `json:"clusterId"`
}

type ListClusterResponse struct {
	// The total number of clusters
	TotalCount int                   `json:"totalCount"`
	Items      []ClusterResponseItem `json:"items"`
}

type ClusterResponseItem struct {
	// Cluster ID
	ClusterId string `json:"clusterId"`
	// User ID who created the cluster.
	UserId string `json:"userId"`
	// Cluster status, e.g. Ready,Creating,Failed,Deleting
	Phase string `json:"phase"`
	// Whether the cluster is under protection
	IsProtected bool `json:"isProtected"`
	// Cluster creation time(RFC3339Short), e.g. "2025-07-08T10:31:46"
	CreationTime string `json:"creationTime"`
}

type GetClusterResponse struct {
	ClusterResponseItem
	// Cluster description
	Description string `json:"description"`
	// The endpoint of cluster control plane. e.g. "10.0.0.1:443"
	Endpoint string `json:"endpoint"`
	// The secret ID for node ssh specified by the user
	SSHSecretId string `json:"sshSecretId"`
	// The secret ID for pulling image specified by the user
	ImageSecretId string `json:"imageSecretId"`
	// The nodes of control plane
	Nodes []string `json:"nodes"`
	// KubeSpray image name used for installation. e.g. "docker.io/your-group/kubespray:20200530"
	KubeSprayImage *string `json:"kubeSprayImage,omitempty"`
	// Subnet configuration, e.g. "10.0.0.0/16"
	KubePodsSubnet *string `json:"kubePodsSubnet,omitempty"`
	// Service Address configuration, e.g. "10.254.0.0/16"
	KubeServiceAddress *string `json:"kubeServiceAddress,omitempty"`
	// Network plugin, default flannel
	KubeNetworkPlugin *string `json:"kubeNetworkPlugin,omitempty"`
	// Kubernetes version, e.g. "1.32.5"
	KubeVersion *string `json:"kubernetesVersion,omitempty"`
	// Some settings for Kubernetes
	KubeApiServerArgs map[string]string `json:"kubeApiServerArgs,omitempty"`
	// User-defined labels
	Labels map[string]string `json:"labels,omitempty"`
}

type ProcessNodesRequest struct {
	// List of node IDs to operate on.
	NodeIds []string `json:"nodeIds"`
	// Action type: add/remove
	Action string `json:"action"`
}

type ProcessNodesResponse struct {
	// Total number of nodes to be processed
	TotalCount int `json:"totalCount"`
	// Number of nodes processed successfully
	SuccessCount int `json:"successCount"`
}

type GetClusterPodLogResponse struct {
	// Cluster ID
	ClusterId string `json:"clusterId"`
	// Pod ID used to create the cluster.
	PodId string `json:"podId"`
	// An array of log lines, returned in the same order as they appear in the original logs
	Logs []string `json:"logs"`
}

type PatchClusterRequest struct {
	// Whether Cluster is under protection, empty means do nothing
	IsProtected *bool `json:"isProtected,omitempty"`
	// User-defined labels. Keys cannot start with "primus-safe."
	Labels *map[string]string `json:"labels,omitempty"`
}
