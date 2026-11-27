/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

import (
	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// The values of WorkspaceCatalogItem.MyStatus.
const (
	CatalogStatusMember  = "member"
	CatalogStatusPending = "pending"
	CatalogStatusNone    = "none"
)

// The values of the view query parameter of the access request list.
const (
	AccessRequestViewMine     = "mine"
	AccessRequestViewApprover = "approver"
)

// CreateWorkspaceAccessRequest is the body of POST /workspace-access-requests.
type CreateWorkspaceAccessRequest struct {
	Workspace string `json:"workspace"`
	Reason    string `json:"reason"`
}

// DecideWorkspaceAccessRequest is the body of the approve and reject requests.
type DecideWorkspaceAccessRequest struct {
	Comment string `json:"comment"`
}

// WorkspaceAccessRequestItem is a workspace access request as the API returns it.
type WorkspaceAccessRequestItem struct {
	Id              int64  `json:"id"`
	Workspace       string `json:"workspace"`
	WorkspaceName   string `json:"workspaceName"`
	UserId          string `json:"userId"`
	UserName        string `json:"userName"`
	UserEmail       string `json:"userEmail"`
	Reason          string `json:"reason"`
	Status          string `json:"status"`
	DecidedBy       string `json:"decidedBy"`
	DecidedByName   string `json:"decidedByName"`
	DecisionComment string `json:"decisionComment"`
	DecidedAt       string `json:"decidedAt"`
	CreationTime    string `json:"creationTime"`
	UpdateTime      string `json:"updateTime"`
	// ApprovalUrl is the console page of the request.
	ApprovalUrl string `json:"approvalUrl"`
}

// WorkspaceAccessRequestDetail is a request with what the caller may do with it.
type WorkspaceAccessRequestDetail struct {
	WorkspaceAccessRequestItem
	// CanDecide is true when the caller is an approver of the pending request and not its requester.
	CanDecide   bool `json:"canDecide"`
	IsRequester bool `json:"isRequester"`
}

// ListWorkspaceAccessRequestResponse is the response of GET /workspace-access-requests.
type ListWorkspaceAccessRequestResponse struct {
	TotalCount int                          `json:"totalCount"`
	Items      []WorkspaceAccessRequestItem `json:"items"`
}

// WorkspaceCatalogManager is a direct manager of a catalog workspace.
type WorkspaceCatalogManager struct {
	Id    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// WorkspaceCatalogTeam is a team bound to a catalog workspace.
type WorkspaceCatalogTeam struct {
	Id          string               `json:"id"`
	DisplayName string               `json:"displayName"`
	Role        v1.WorkspaceTeamRole `json:"role"`
}

// WorkspaceCatalogItem is a workspace that accepts access requests.
type WorkspaceCatalogItem struct {
	WorkspaceId   string                    `json:"workspaceId"`
	WorkspaceName string                    `json:"workspaceName"`
	Description   string                    `json:"description"`
	Managers      []WorkspaceCatalogManager `json:"managers"`
	Teams         []WorkspaceCatalogTeam    `json:"teams"`
	// MyStatus is member, pending or none.
	MyStatus string `json:"myStatus"`
	// PendingRequestId is the caller's pending request when MyStatus is pending.
	PendingRequestId *int64 `json:"pendingRequestId,omitempty"`
}

// WorkspaceCatalogResponse is the response of GET /workspaces/catalog.
type WorkspaceCatalogResponse struct {
	TotalCount int                    `json:"totalCount"`
	Items      []WorkspaceCatalogItem `json:"items"`
}
