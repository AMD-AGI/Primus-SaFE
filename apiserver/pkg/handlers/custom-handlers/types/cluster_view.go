/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

type CreateClusterRequest struct {
	v1.ControlPlane
	// The cluster name specified by the user
	Name string `json:"name"`
	// The cluster's description
	Description string `json:"description,omitempty"`
	// The SSH secret id specified by the user, which must exist, used for node SSH login.
	SSHSecretId string `json:"sshSecretId,omitempty"`
	// The labels for cluster
	Labels map[string]string `json:"labels,omitempty"`
	// Whether the cluster is under protection. When set to true, direct deletion is not allowed unless the label is removed
	IsProtected bool `json:"isProtected,omitempty"`
}

type CreateClusterResponse struct {
	// The cluster's id
	ClusterId string `json:"clusterId"`
}

type ListClusterResponse struct {
	// The total number of clusters, not limited by pagination
	TotalCount int                   `json:"totalCount"`
	Items      []ClusterResponseItem `json:"items"`
}

type ClusterResponseItem struct {
	// The cluster's id
	ClusterId string `json:"clusterId"`
	// User id who created the cluster.
	UserId string `json:"userId"`
	// The cluster's status
	Phase string `json:"phase"`
	// Whether the cluster is under protection
	IsProtected bool `json:"isProtected"`
	// cluster's creation time
	CreationTime string `json:"creationTime"`
}

type GetClusterResponse struct {
	ClusterResponseItem
	// The cluster's description
	Description string `json:"description"`
	// The Cluster access address, usually the service address
	Endpoint string `json:"endpoint"`
	// The SSH secret id specified by the user, which must exist
	SSHSecretId string `json:"sshSecretId"`
	// The Image secret id specified by the user, which must exist
	ImageSecretId string `json:"imageSecretId"`
	// The nodes of control plane
	Nodes []string `json:"nodes"`
	// KubeSpray image name used for installation
	KubeSprayImage *string `json:"kubeSprayImage,omitempty"`
	// Subnet configuration
	KubePodsSubnet *string `json:"kubePodsSubnet,omitempty"`
	// Service Address configuration
	KubeServiceAddress *string `json:"kubeServiceAddress,omitempty"`
	// Network plugin, default is cilium
	KubeNetworkPlugin *string `json:"kubeNetworkPlugin,omitempty"`
	// Kubernetes version
	KubeVersion *string `json:"kubernetesVersion,omitempty"`
	// Some parameter settings for Kubernetes
	KubeApiServerArgs map[string]string `json:"kubeApiServerArgs,omitempty"`
}

type ProcessNodesRequest struct {
	// List of node ids to operate on.
	NodeIds []string `json:"nodeIds"`
	// The action taken on the node of cluster, such as add or remove
	Action string `json:"action"`
}

type ProcessNodesResponse struct {
	// Total number of nodes to be processed
	TotalCount int `json:"totalCount"`
	// Number of nodes processed successfully
	SuccessCount int `json:"successCount"`
}

type GetClusterPodLogResponse struct {
	// The cluster's id
	ClusterId string `json:"clusterId"`
	// Pod id used to create the cluster.
	PodId string `json:"podId"`
	// An array of log lines, returned in the same order as they appear in the original logs
	Logs []string `json:"logs"`
}

type PatchClusterRequest struct {
	// Whether the cluster is under protection, empty means do nothing
	IsProtected *bool `json:"isProtected,omitempty"`
}
