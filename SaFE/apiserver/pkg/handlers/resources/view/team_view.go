/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

import (
	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// UpdateWorkspaceTeamsRequest changes the team bindings of a workspace incrementally.
type UpdateWorkspaceTeamsRequest struct {
	// Bindings to add. A team that is already bound gets the given role
	Add []v1.WorkspaceTeam `json:"add,omitempty"`
	// Ids of the teams to unbind
	Remove []string `json:"remove,omitempty"`
}

// WorkspaceTeamsResponse is the team bindings of a workspace after an update.
type WorkspaceTeamsResponse struct {
	Teams []v1.WorkspaceTeam `json:"teams"`
}

// TeamWorkspaceBinding is the binding of a team to one workspace, as a request entry.
type TeamWorkspaceBinding struct {
	// Workspace ID
	WorkspaceId string `json:"workspace"`
	// Role of the team members in the workspace, member by default
	Role v1.WorkspaceTeamRole `json:"role,omitempty"`
}

// TeamWorkspaceItem is a workspace a team is bound to.
type TeamWorkspaceItem struct {
	// Workspace ID
	WorkspaceId string `json:"workspaceId"`
	// Workspace name
	WorkspaceName string `json:"workspaceName"`
	// Role of the team members in the workspace
	Role v1.WorkspaceTeamRole `json:"role"`
}

// TeamWorkspacesResponse lists the workspaces a team is bound to.
type TeamWorkspacesResponse struct {
	TeamId     string              `json:"teamId"`
	TotalCount int                 `json:"totalCount"`
	Items      []TeamWorkspaceItem `json:"items"`
}

// WorkspaceMemberUser is a user with direct access to a workspace.
type WorkspaceMemberUser struct {
	// User ID
	Id string `json:"id"`
	// Username
	Name string `json:"name"`
	// User mail
	Email string `json:"email"`
	// Preferred username from the SSO identity provider
	PreferredName string `json:"preferredName,omitempty"`
	// member or manager
	Role v1.WorkspaceTeamRole `json:"role"`
	// Always direct
	Source string `json:"source"`
}

// WorkspaceMemberManager is a manager of a team, as the identity service describes it.
type WorkspaceMemberManager struct {
	Id            string `json:"id"`
	Name          string `json:"name,omitempty"`
	Email         string `json:"email,omitempty"`
	PreferredName string `json:"preferredName,omitempty"`
}

// WorkspaceMemberTeam is a team bound to a workspace. The display name, the member count and the
// managers come from the identity service and are left out when it cannot answer.
type WorkspaceMemberTeam struct {
	// Team ID
	Id string `json:"id"`
	// Team display name
	DisplayName string `json:"displayName,omitempty"`
	// Role of the team members in the workspace
	Role v1.WorkspaceTeamRole `json:"role"`
	// Number of team members
	MemberCount *int `json:"memberCount,omitempty"`
	// Managers of the team
	Managers []WorkspaceMemberManager `json:"managers,omitempty"`
	// team:<id>
	Source string `json:"source"`
}

// WorkspaceMemberGroup is a platform group whose mapping reaches a workspace.
type WorkspaceMemberGroup struct {
	// Platform group name
	Name string `json:"name"`
	// manager when the mapping holds the workspace-admin role, member otherwise
	Role v1.WorkspaceTeamRole `json:"role"`
	// group:<name>
	Source string `json:"source"`
}

// WorkspaceMembersResponse is the effective member view of a workspace.
type WorkspaceMembersResponse struct {
	Users  []WorkspaceMemberUser  `json:"users"`
	Teams  []WorkspaceMemberTeam  `json:"teams"`
	Groups []WorkspaceMemberGroup `json:"groups"`
}

// TeamMemberItem is a member of a team, as the identity service describes it.
type TeamMemberItem struct {
	UserId        string `json:"userId"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	PreferredName string `json:"preferredName"`
	Role          string `json:"role"`
}

// TeamItem is a team of the identity service in the team selector.
type TeamItem struct {
	Id          string           `json:"id"`
	DisplayName string           `json:"displayName"`
	Description string           `json:"description"`
	MemberCount int              `json:"memberCount"`
	Managers    []TeamMemberItem `json:"managers"`
}

// ListTeamResponse lists the teams of the identity service, sorted by id.
type ListTeamResponse struct {
	TotalCount int        `json:"totalCount"`
	Items      []TeamItem `json:"items"`
}

// TeamMembersResponse lists the members of one team.
type TeamMembersResponse struct {
	TeamId     string           `json:"teamId"`
	TotalCount int              `json:"totalCount"`
	Items      []TeamMemberItem `json:"items"`
}
