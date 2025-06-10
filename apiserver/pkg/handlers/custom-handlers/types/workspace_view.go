/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

type CreateWorkspaceRequest struct {
	Name        string `json:"name"`
	Cluster     string `json:"cluster"`
	Description string `json:"description,omitempty"`
	// Queuing policy for tasks submitted in this workspace.
	// All tasks currently share the same policy (no per-task customization). Default: fifo.
	QueuePolicy string `json:"queuePolicy,omitempty"`
	// node flavor id
	NodeFlavor string `json:"nodeFlavor,omitempty"`
	// node count
	Replica int `json:"replica,omitempty"`
	// Service modules available in this space. support: Train/Infer/Authoring, No limitation if not specified
	Scopes []v1.WorkspaceScope `json:"scopes,omitempty"`
	// volumes used in this space
	Volumes []v1.WorkspaceVolume `json:"volumes,omitempty"`
}

type CreateWorkspaceResponse struct {
	WorkspaceId string `json:"workspaceId"`
}

type GetWorkspaceRequest struct {
	ClusterId string `form:"clusterId" binding:"omitempty,max=64"`
}

type GetWorkspaceResponseItem struct {
	// workspace id
	WorkspaceId string `json:"workspaceId"`
	// workspace name
	WorkspaceName string `json:"workspaceName"`
	// workspace's cluster
	ClusterId string `json:"clusterId"`
	// the total resource of workspace
	TotalQuota ResourceList `json:"totalQuota,omitempty"`
	// the available resource of workspace
	AvailQuota ResourceList `json:"availQuota,omitempty"`
	// the faulty resources of workspace
	AbnormalQuota ResourceList `json:"abnormalQuota,omitempty"`
	// the used resources of workspace
	UsedQuota ResourceList `json:"usedQuota,omitempty"`
	// node flavor id
	NodeFlavor string `json:"nodeFlavor,omitempty"`
	// total node count
	TotalReplica int `json:"totalReplica,omitempty"`
	// available node count
	AvailableReplica int `json:"availableReplica,omitempty"`
	// abnormal node count
	AbnormalReplica int `json:"abnormalReplica,omitempty"`
	// the status of workspace
	Phase string `json:"phase,omitempty"`
	// creation time
	CreatedTime string `json:"createdTime"`
	// description of workspace
	Description string `json:"description,omitempty"`
	// Queuing policy for tasks submitted in this workspace.
	QueuePolicy v1.WorkspaceQueuePolicy `json:"queuePolicy"`
	// support service module: Train/Infer/Authoring, No limitation if not specified
	Scopes []v1.WorkspaceScope `json:"scopes"`
	// the store volumes used by workspace
	Volumes []v1.WorkspaceVolume `json:"volumes,omitempty"`
	// Is preemption enabled
	EnablePreempt bool `json:"enablePreempt,omitempty"`
}

type GetWorkspaceResponse struct {
	TotalCount int                        `json:"totalCount"`
	Items      []GetWorkspaceResponseItem `json:"items,omitempty"`
}

type PatchWorkspaceRequest struct {
	// node flavor id
	NodeFlavor *string `json:"nodeFlavor,omitempty"`
	// total node count
	Replica *int `json:"replica,omitempty"`
	// Queuing policy for tasks submitted in this workspace. fifo/balance
	QueuePolicy *v1.WorkspaceQueuePolicy `json:"queuePolicy,omitempty"`
	// support service module: Train/Infer/Authoring, No limitation if not specified
	Scopes *[]v1.WorkspaceScope `json:"scopes,omitempty"`
	// the store volumes used by workspace
	Volumes *[]v1.WorkspaceVolume `json:"volumes,omitempty"`
	// description
	Description *string `json:"description,omitempty"`
	// EnablePreempt
	EnablePreempt *bool `json:"enablePreempt,omitempty"`
}
