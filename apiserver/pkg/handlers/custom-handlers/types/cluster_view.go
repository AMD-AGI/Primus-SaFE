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
	SSHSecretName string `json:"sshSecretName,omitempty"`
	// The Image secret name specified by the user, which must already exist
	ImageSecretName string `json:"imageSecretName,omitempty"`
	// The labels for cluster
	Labels map[string]string `json:"labels,omitempty"`
	// The maximum number of pods supported per node. It must be a power of two, with a maximum value of 256
	MaxPodCount uint32 `json:"maxPodCount,omitempty"`
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
	ClusterId     string `json:"clusterId"`
	UserId        string `json:"userId"`
	Phase         string `json:"phase"`
	ImageSecretId string `json:"imageSecretId"`
	IsProtected   bool   `json:"isProtected"`
}

type GetClusterResponse struct {
	ClusterResponseItem
	Endpoint string                       `json:"endpoint"`
	Storages []BindingStorageResponseItem `json:"storage"`
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
