/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	"strings"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

type CreateUserRequest struct {
	// The username, required
	Name string `json:"name,omitempty"`
	// The user mail
	Email string `json:"email,omitempty"`
	// The user type, such as default, teams, required
	Type v1.UserType `json:"type,omitempty"`
	// The user password
	Password string `json:"password,omitempty"`
	// The workspaces which user can access
	Workspaces []string `json:"workspaces,omitempty"`
	// The user avatar URL
	AvatarUrl string `json:"avatarUrl,omitempty"`
}

type CreateUserResponse struct {
	// The user id
	Id string `json:"id"`
}

type ListUserRequest struct {
	// The username
	Name string `form:"name" binding:"omitempty"`
	// The user mail
	Email string `form:"email" binding:"omitempty"`
	// Workspace id accessible to the user.
	WorkspaceId string `form:"workspaceId" binding:"omitempty,max=64"`
}

type ListUserResponse struct {
	// The total number of node templates, not limited by pagination
	TotalCount int                `json:"totalCount"`
	Items      []UserResponseItem `json:"items,omitempty"`
}

type UserResponseItem struct {
	// The user id
	Id string `json:"id"`
	// The username
	Name string `json:"name"`
	// The user mail
	Email string `json:"email"`
	// The user type, such as default, teams
	Type v1.UserType `json:"type"`
	// The user's role, such as system-admin, default
	Roles []v1.UserRole `json:"roles"`
	// The workspace's id which user can access
	Workspaces []WorkspaceEntry `json:"workspaces"`
	// The workspace's id which user can manage
	ManagedWorkspaces []WorkspaceEntry `json:"managedWorkspaces"`
	// The user creation time
	CreationTime string `json:"creationTime"`
	// The User restriction type, 0: normal; 1 frozen
	RestrictedType v1.UserRestrictedType `json:"restrictedType"`
	// The user avatar URL
	AvatarUrl string `json:"avatarUrl,omitempty"`
}

type PatchUserRequest struct {
	// The user role, such as system-admin, default
	Roles *[]v1.UserRole `json:"roles,omitempty"`
	// The workspaces which user can access
	Workspaces *[]string `json:"workspaces,omitempty"`
	// The user avatar URL
	AvatarUrl *string `json:"avatarUrl,omitempty"`
	// The user password
	Password *string `json:"password,omitempty"`
	// The User restriction type, 0: normal; 1 frozen
	RestrictedType *v1.UserRestrictedType `json:"restrictedType,omitempty"`
	// The user email
	Email *string `json:"email,omitempty"`
}

type UserLoginRequest struct {
	// The user type, such as default, teams
	Type v1.UserType `json:"type,omitempty"`
	// The username
	Name string `json:"name,omitempty"`
	// The user password
	Password string `json:"password,omitempty"`
	// Whether the request is from console
	IsFromConsole bool `json:"-"`
}

type UserLoginResponse struct {
	UserResponseItem `json:",inline"`
	// The timestamp when the user's token expires, in seconds.
	Expire int64 `json:"expire"`
	// The user's token
	Token string `json:"token"`
}

type UserEntity struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type UserSlice []v1.User

func (users UserSlice) Less(i, j int) bool {
	if users[i].CreationTimestamp.Time.Equal(users[j].CreationTimestamp.Time) {
		return strings.Compare(users[i].Name, users[j].Name) < 0
	}
	return users[i].CreationTimestamp.Time.Before(users[j].CreationTimestamp.Time)
}

func (users UserSlice) Len() int {
	return len(users)
}

func (users UserSlice) Swap(i, j int) {
	users[i], users[j] = users[j], users[i]
}
