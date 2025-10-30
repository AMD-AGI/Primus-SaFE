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
	// User mail
	Email string `json:"email,omitempty"`
	// User type, e.g. default, teams
	Type v1.UserType `json:"type,omitempty"`
	// User password
	Password string `json:"password,omitempty"`
	// The workspaces which user can access
	Workspaces []string `json:"workspaces,omitempty"`
	// User avatar URL
	AvatarUrl string `json:"avatarUrl,omitempty"`
}

type CreateUserResponse struct {
	// User ID
	Id string `json:"id"`
}

type ListUserRequest struct {
	// The username, will be QueryEscape processed
	Name string `form:"name" binding:"omitempty"`
	// User mail, will be QueryEscape processed
	Email string `form:"email" binding:"omitempty"`
	// Workspace ID accessible to the user.
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
	// User type, e.g. default, teams
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
}

type PatchUserRequest struct {
	// User role, e.g. system-admin, default
	Roles *[]v1.UserRole `json:"roles,omitempty"`
	// The workspaces which user can access
	Workspaces *[]string `json:"workspaces,omitempty"`
	// User avatar URL
	AvatarUrl *string `json:"avatarUrl,omitempty"`
	// User password
	Password *string `json:"password,omitempty"`
	// User restriction type, 0: normal; 1 frozen
	RestrictedType *v1.UserRestrictedType `json:"restrictedType,omitempty"`
	// User email
	Email *string `json:"email,omitempty"`
}

type UserLoginRequest struct {
	// User type, e.g. default, teams
	Type v1.UserType `json:"type,omitempty"`
	// Username
	Name string `json:"name,omitempty"`
	// User password
	Password string `json:"password,omitempty"`
	// Whether the request is from console
	IsFromConsole bool `json:"-"`
}

type UserLoginResponse struct {
	UserResponseItem `json:",inline"`
	// The timestamp when the user token expires, in seconds.
	Expire int64 `json:"expire"`
	// User token
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
