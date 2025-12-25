/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

import (
	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

type CreateWorkspaceRequest struct {
	// Workspace name(display only). Used for generate workspace ID.
	// The final ID is clusterId + "-" + name.
	Name string `json:"name"`
	// The cluster which workspace belongs to
	ClusterId string `json:"clusterId"`
	// Workspace description
	Description string `json:"description,omitempty"`
	// Queuing policy for workloads submitted in this workspace.
	// All workloads currently share the same policy, supports fifo (default) and balance.
	// 1. "fifo" means first-in, first-out: the workload that enters the queue first is served first.
	//    If the front workload does not meet the conditions for dispatch, it will wait indefinitely,
	//    and other tasks in the queue will also be blocked waiting.
	// 2. "balance" allows any workload that meets the resource conditions to be dispatched,
	//    avoiding blockage by the front workload in the queue. However, it is still subject to priority constraints.
	//    If a higher-priority task cannot be dispatched, lower-priority tasks will wait.
	QueuePolicy string `json:"queuePolicy,omitempty"`
	// The node flavor ID of workspace, A workspace supports only one node flavor
	FlavorId string `json:"flavorId,omitempty"`
	// The expected number of nodes in the workspace
	Replica int `json:"replica,omitempty"`
	// Service modules available in this space. support: Train/Infer/Authoring, No limitation if not specified
	Scopes []v1.WorkspaceScope `json:"scopes,omitempty"`
	// Volumes used in this workspace
	Volumes []v1.WorkspaceVolume `json:"volumes,omitempty"`
	// Whether preemption is enabled. If enabled, higher-priority workload will preempt the lower-priority one
	EnablePreempt bool `json:"enablePreempt"`
	// Set the workspace as the default workspace (i.e., all users can access it)
	IsDefault bool `json:"isDefault,omitempty"`
	// Workspace image secret ID, used for downloading images
	ImageSecretIds []string `json:"imageSecretIds,omitempty"`
}

type CreateWorkspaceResponse struct {
	// Workspace ID
	WorkspaceId string `json:"workspaceId"`
}

type ListWorkspaceRequest struct {
	// Filter results by cluster ID
	ClusterId string `form:"clusterId" binding:"omitempty,max=64"`
}

type ListWorkspaceResponse struct {
	// The total number of workspaces, not limited by pagination
	TotalCount int                     `json:"totalCount"`
	Items      []WorkspaceResponseItem `json:"items"`
}

type WorkspaceResponseItem struct {
	// Workspace ID
	WorkspaceId string `json:"workspaceId"`
	// Workspace name
	WorkspaceName string `json:"workspaceName"`
	// The cluster which workspace belongs to
	ClusterId string `json:"clusterId"`
	// The node flavor ID used by workspace
	FlavorId string `json:"flavorId"`
	// User ID of workspace creator
	UserId string `json:"userId"`
	// The target expected number of nodes of workspace
	TargetNodeCount int `json:"targetNodeCount"`
	// The current total number of nodes
	CurrentNodeCount int `json:"currentNodeCount"`
	// The current total number of abnormal nodes
	AbnormalNodeCount int `json:"abnormalNodeCount"`
	// The status of workspace, e.g. Creating, Running, Abnormal, Deleting
	Phase string `json:"phase"`
	// Workspace creation time (RFC3339Short, e.g. "2025-07-08T10:31:46")
	CreationTime string `json:"creationTime"`
	// Workspace description
	Description string `json:"description"`
	// Queuing policy for workload submitted in this workspace
	// Refer to the explanation of the same-named parameter in CreateWorkspaceRequest
	QueuePolicy v1.WorkspaceQueuePolicy `json:"queuePolicy"`
	// Support service module: Train/Infer/Authoring, No limitation if not specified
	Scopes []v1.WorkspaceScope `json:"scopes"`
	// The store volumes used by workspace
	Volumes []v1.WorkspaceVolume `json:"volumes"`
	// Whether preemption is enabled. If enabled, higher-priority workload will preempt the lower-priority one
	EnablePreempt bool `json:"enablePreempt"`
	// User ID of the workspace administrator
	Managers []UserEntity `json:"managers"`
	// Set the workspace as the default workspace (i.e., all users can access it).
	IsDefault bool `json:"isDefault"`
}

type GetWorkspaceResponse struct {
	WorkspaceResponseItem
	// The total resource of workspace
	TotalQuota ResourceList `json:"totalQuota"`
	// The available resource of workspace
	AvailQuota ResourceList `json:"availQuota"`
	// The abnormal resources of workspace
	AbnormalQuota ResourceList `json:"abnormalQuota"`
	// The used resources of workspace
	UsedQuota ResourceList `json:"usedQuota"`
	// The node currently in use has workloads running on it
	UsedNodeCount int `json:"usedNodeCount"`
	// Workspace image secret ID, used for downloading images
	ImageSecretIds []string `json:"imageSecretIds"`
}

type PatchWorkspaceRequest struct {
	// The node flavor ID used by workspace
	FlavorId *string `json:"flavorId,omitempty"`
	// The expected total node count
	Replica *int `json:"replica,omitempty"`
	// Queuing policy for tasks submitted in this workspace. e.g. fifo, balance
	// Refer to the explanation of the same-named parameter in CreateWorkspaceRequest
	QueuePolicy *v1.WorkspaceQueuePolicy `json:"queuePolicy,omitempty"`
	// Support service module: Train/Infer/Authoring, No limitation if not specified
	Scopes *[]v1.WorkspaceScope `json:"scopes,omitempty"`
	// The store volumes used by workspace
	Volumes *[]v1.WorkspaceVolume `json:"volumes,omitempty"`
	// Workspace description
	Description *string `json:"description,omitempty"`
	// Whether preemption is enabled
	EnablePreempt *bool `json:"enablePreempt,omitempty"`
	// User ID of the workspace administrator
	Managers *[]string `json:"managers,omitempty"`
	// Set the workspace as the default workspace (i.e., all users can access it).
	IsDefault *bool `json:"isDefault,omitempty"`
	// Workspace image secret ID, used for downloading images
	ImageSecretIds *[]string `json:"imageSecretIds,omitempty"`
}

type WorkspaceEntry struct {
	// workspace ID
	Id string `json:"id"`
	// workspace name
	Name string `json:"name"`
}
