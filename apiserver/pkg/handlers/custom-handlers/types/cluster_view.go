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
	// The description for cluster
	Description string `json:"description,omitempty"`
	// The SSH secret name specified by the user, which must already exist
	SSHSecretId string `json:"sshSecretId,omitempty"`
	// The Image secret name specified by the user, which must already exist
	ImageSecretId string `json:"imageSecretId,omitempty"`
	// The labels for cluster
	Labels map[string]string `json:"labels,omitempty"`
	// Whether the cluster is under protection. When set to true, direct deletion is not allowed unless the label is first removed
	IsProtected bool `json:"isProtected,omitempty"`
}

type CreateClusterResponse struct {
	ClusterId string `json:"clusterId"`
}

type ListClusterResponse struct {
	TotalCount int                   `json:"totalCount"`
	Items      []ClusterResponseItem `json:"items"`
}

type ClusterResponseItem struct {
	ClusterId   string `json:"clusterId"`
	UserId      string `json:"userId"`
	Phase       string `json:"phase"`
	IsProtected bool   `json:"isProtected"`
	// cluster creation time
	CreationTime string `json:"creationTime"`
}

type GetClusterResponse struct {
	ClusterResponseItem
	Description string `json:"description"`
	Endpoint    string `json:"endpoint"`
	// The SSH secret name specified by the user, which must already exist
	SSHSecretId string `json:"sshSecretId"`
	// The Image secret name specified by the user, which must already exist
	ImageSecretId string `json:"imageSecretId"`
	// the nodes of control plane
	Nodes              []string `json:"nodes"`
	KubeSprayImage     *string  `json:"kubeSprayImage,omitempty"`
	KubePodsSubnet     *string  `json:"kubePodsSubnet,omitempty"`
	KubeServiceAddress *string  `json:"kubeServiceAddress,omitempty"`
	// default is cilium
	KubeNetworkPlugin *string           `json:"kubeNetworkPlugin,omitempty"`
	KubeVersion       *string           `json:"kubernetesVersion,omitempty"`
	KubeApiServerArgs map[string]string `json:"kubeApiServerArgs,omitempty"`
}

type ProcessNodesRequest struct {
	NodeIds []string `json:"nodeIds"`
	// add or remove
	Action string `json:"action"`
}

type ProcessNodesResponse struct {
	TotalCount   int `json:"totalCount"`
	SuccessCount int `json:"successCount"`
}

type GetClusterPodLogResponse struct {
	ClusterId string `json:"clusterId"`
	PodId     string `json:"podId"`
	// An array of log lines, returned in the same order as they appear in the original logs
	Logs []string `json:"logs"`
}

type PatchClusterRequest struct {
	IsProtected   *bool   `json:"isProtected,omitempty"`
	ImageSecretId *string `json:"imageSecretId,omitempty"`
}
