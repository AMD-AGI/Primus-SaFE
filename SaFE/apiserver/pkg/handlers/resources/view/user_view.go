/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

import (
	"fmt"
	"strings"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// MaxUserLowUtilizationWorkspaces caps the workspace entries of a user's low utilization settings.
const MaxUserLowUtilizationWorkspaces = 200

type CreateUserRequest struct {
	// The username, required
	Name string `json:"name,omitempty"`
	// User mail
	Email string `json:"email,omitempty"`
	// User password
	Password string `json:"password,omitempty"`
	// User avatar URL
	AvatarUrl string `json:"avatarUrl,omitempty"`
}

type CreateUserResponse struct {
	// User ID
	Id string `json:"id"`
}

type ListUserRequest struct {
	// Filter by username (URL encoded)
	Name string `form:"name" binding:"omitempty"`
	// Filter by email (URL encoded)
	Email string `form:"email" binding:"omitempty"`
	// Filter by workspace (returns users with access to this workspace)
	WorkspaceId string `form:"workspaceId" binding:"omitempty,max=64"`
}

type ListUserResponse struct {
	// The total number of users, not limited by pagination
	TotalCount int                `json:"totalCount"`
	Items      []UserResponseItem `json:"items,omitempty"`
}

type UserResponseItem struct {
	// User ID
	Id string `json:"id"`
	// Username
	Name string `json:"name"`
	// User mail
	Email string `json:"email"`
	// Preferred username from the SSO identity provider, recorded at SSO sign-in
	PreferredName string `json:"preferredName,omitempty"`
	// User type, e.g. default
	Type v1.UserType `json:"type"`
	// User role, e.g. system-admin, default
	Roles []v1.UserRole `json:"roles"`
	// The workspaces which user can access
	Workspaces []WorkspaceEntry `json:"workspaces"`
	// The workspaces which user can manage
	ManagedWorkspaces []WorkspaceEntry `json:"managedWorkspaces"`
	// User creation time
	CreationTime string `json:"creationTime"`
	// User restriction type, 0: normal; 1 frozen
	RestrictedType v1.UserRestrictedType `json:"restrictedType"`
	// User avatar URL
	AvatarUrl string `json:"avatarUrl,omitempty"`
	// Per-user resource and runtime ceilings.
	Qos *v1.UserQos `json:"qos,omitempty"`
	// Per-user low utilization settings per workspace.
	LowUtilization *v1.UserLowUtilization `json:"lowUtilization,omitempty"`
	// How the user reaches the queried workspace: direct and team:<id>. Only set when the list is
	// filtered by workspace.
	Sources []string `json:"sources,omitempty"`
}

// UserWorkspacesResponse is the answer of GET /users/self/workspaces: the same two lists
// the user record carries, served on their own so a client that learns who the user is
// from another service can still ask SaFE which workspaces the user can reach.
type UserWorkspacesResponse struct {
	// The workspaces which user can access
	Workspaces []WorkspaceEntry `json:"workspaces"`
	// The workspaces which user can manage
	ManagedWorkspaces []WorkspaceEntry `json:"managedWorkspaces"`
}

type PatchUserRequest struct {
	// User role, e.g. system-admin, default
	Roles *[]v1.UserRole `json:"roles,omitempty"`
	// The workspaces which user can access
	Workspaces *[]string `json:"workspaces,omitempty"`
	// The workspaces to add to the user's accessible workspaces, exclusive with workspaces
	AddWorkspaces *[]string `json:"addWorkspaces,omitempty"`
	// The workspaces to remove from the user's accessible workspaces, exclusive with workspaces
	RemoveWorkspaces *[]string `json:"removeWorkspaces,omitempty"`
	// User avatar URL
	AvatarUrl *string `json:"avatarUrl,omitempty"`
	// User password
	Password *string `json:"password,omitempty"`
	// User restriction type, 0: normal; 1 frozen
	RestrictedType *v1.UserRestrictedType `json:"restrictedType,omitempty"`
	// User email
	Email *string `json:"email,omitempty"`
	// Per-user resource and runtime ceilings. System administrators only.
	Qos *v1.UserQos `json:"qos,omitempty"`
	// Per-user low utilization settings per workspace, replacing the stored ones. System administrators only.
	LowUtilization *v1.UserLowUtilization `json:"lowUtilization,omitempty"`
}

// ValidateUserLowUtilization checks the workspace ids of a user's low utilization settings.
// A workspace does not have to exist.
func ValidateUserLowUtilization(settings *v1.UserLowUtilization) error {
	if settings == nil {
		return nil
	}
	if len(settings.Workspaces) > MaxUserLowUtilizationWorkspaces {
		return commonerrors.NewBadRequest(fmt.Sprintf("lowUtilization holds at most %d workspaces",
			MaxUserLowUtilizationWorkspaces))
	}
	for workspaceID := range settings.Workspaces {
		if strings.TrimSpace(workspaceID) == "" {
			return commonerrors.NewBadRequest("lowUtilization workspace id is required")
		}
	}
	return nil
}

type UserSettingsResponse struct {
	// Whether email notification is enabled for this user
	EnableNotification bool `json:"enableNotification"`
}

type UpdateUserSettingsRequest struct {
	// Whether to enable email notification for workload status changes
	EnableNotification *bool `json:"enableNotification,omitempty"`
}

type UserLoginRequest struct {
	// User type, e.g. default
	Type v1.UserType `json:"type,omitempty"`
	// Username
	Name string `json:"name,omitempty"`
	// User password
	Password string `json:"password,omitempty"`
	// code generated by sso
	Code string `json:"code,omitempty"`
	// The PKCE code verifier of a code the console asked for at the sso.login provider
	CodeVerifier string `json:"codeVerifier,omitempty"`
	// Whether the request is from console
	IsFromConsole bool `json:"-"`
}

type UserLoginResponse struct {
	UserResponseItem `json:",inline"`
	// The timestamp when the user token expires, in seconds.
	Expire int64 `json:"expire"`
	// User token, encrypted internally.
	Token string `json:"token"`
}

type UserEntity struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type UserSlice []v1.User

// Less implements sort.Interface for sorting.
func (users UserSlice) Less(i, j int) bool {
	if users[i].CreationTimestamp.Time.Equal(users[j].CreationTimestamp.Time) {
		return strings.Compare(users[i].Name, users[j].Name) < 0
	}
	return users[i].CreationTimestamp.Time.Before(users[j].CreationTimestamp.Time)
}

// Len implements sort.Interface by returning the length of the slice.
func (users UserSlice) Len() int {
	return len(users)
}

// Swap implements sort.Interface by swapping elements at the given indices.
func (users UserSlice) Swap(i, j int) {
	users[i], users[j] = users[j], users[i]
}
