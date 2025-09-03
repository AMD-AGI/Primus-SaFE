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
	// Is preemption enabled
	EnablePreempt bool `json:"enablePreempt"`
	// the manager's user_id of workspace
	Managers []string `json:"managers,omitempty"`
}

type CreateWorkspaceResponse struct {
	WorkspaceId string `json:"workspaceId"`
}

type ListWorkspaceRequest struct {
	ClusterId string `form:"clusterId" binding:"omitempty,max=64"`
}

type ListWorkspaceResponse struct {
	TotalCount int                     `json:"totalCount"`
	Items      []WorkspaceResponseItem `json:"items"`
}

type WorkspaceResponseItem struct {
	// workspace id
	WorkspaceId string `json:"workspaceId"`
	// workspace name
	WorkspaceName string `json:"workspaceName"`
	// workspace's cluster
	ClusterId string `json:"clusterId"`
	// node flavor id
	NodeFlavor string `json:"nodeFlavor"`
	// the creator's id
	UserId string `json:"userId"`
	// total node count
	TotalNode int `json:"totalNode"`
	// abnormal node count
	AbnormalNode int `json:"abnormalNode"`
	// the status of workspace
	Phase string `json:"phase"`
	// creation time
	CreateTime string `json:"createTime"`
	// description of workspace
	Description string `json:"description"`
	// Queuing policy for tasks submitted in this workspace.
	QueuePolicy v1.WorkspaceQueuePolicy `json:"queuePolicy"`
	// support service module: Train/Infer/Authoring, No limitation if not specified
	Scopes []v1.WorkspaceScope `json:"scopes"`
	// the store volumes used by workspace
	Volumes []v1.WorkspaceVolume `json:"volumes"`
	// Is preemption enabled
	EnablePreempt bool `json:"enablePreempt"`
	// the manager's user_id
	Managers []string `json:"managers"`
}

type GetWorkspaceResponse struct {
	WorkspaceResponseItem
	// the total resource of workspace
	TotalQuota ResourceList `json:"totalQuota"`
	// the available resource of workspace
	AvailQuota ResourceList `json:"availQuota"`
	// the faulty resources of workspace
	AbnormalQuota ResourceList `json:"abnormalQuota"`
	// the used resources of workspace
	UsedQuota ResourceList `json:"usedQuota"`
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
	// the managers for workspace
	Managers *[]string `json:"managers,omitempty"`
}

type WorkspaceEntry struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}
