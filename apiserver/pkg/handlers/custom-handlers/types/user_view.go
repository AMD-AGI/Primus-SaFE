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
	// user's name
	Name string `json:"name,omitempty"`
	// user's mail
	Email string `json:"email,omitempty"`
	// user's type. includes: default, teams
	Type v1.UserType `json:"type,omitempty"`
	// the workspaces which user can access
	Workspaces []string `json:"workspaces,omitempty"`
	// password
	Password string `json:"password,omitempty"`
	// user's avatar URL
	AvatarUrl string `json:"avatarUrl,omitempty"`
}

type CreateUserResponse struct {
	Id string `json:"id"`
}

type ListUserRequest struct {
	// user's name
	Name        string `form:"name" binding:"omitempty"`
	Email       string `form:"email" binding:"omitempty"`
	WorkspaceId string `form:"workspaceId" binding:"omitempty,max=64"`
}

type ListUserResponse struct {
	TotalCount int                `json:"totalCount"`
	Items      []UserResponseItem `json:"items,omitempty"`
}

type UserResponseItem struct {
	// user's id
	Id string `json:"id"`
	// user's name
	Name string `json:"name"`
	// user's mail
	Email string `json:"email"`
	// user's type. value includes: default, teams
	Type v1.UserType `json:"type"`
	// system-admin, default
	Roles []v1.UserRole `json:"roles"`
	// the workspace's name which user can access
	Workspaces []WorkspaceEntry `json:"workspaces"`
	// the workspace's name which user can manage
	ManagedWorkspaces []WorkspaceEntry `json:"managedWorkspaces"`
	// user's creation time
	CreationTime string `json:"creationTime"`
	// 0: normal; 1 frozen
	RestrictedType v1.UserRestrictedType `json:"restrictedType"`
	// user's avatar URL
	AvatarUrl string `json:"avatarUrl,omitempty"`
}

type PatchUserRequest struct {
	// system-admin, default
	Roles *[]v1.UserRole `json:"roles,omitempty"`
	// the workspaces which user can access
	Workspaces *[]string `json:"workspaces,omitempty"`
	// user's avatar URL
	AvatarUrl *string `json:"avatarUrl,omitempty"`
	// user's password
	Password *string `json:"password,omitempty"`
	// 0: normal; 1 frozen
	RestrictedType *v1.UserRestrictedType `json:"restrictedType,omitempty"`
	// user's email
	Email *string `json:"email,omitempty"`
}

type UserLoginRequest struct {
	// teams or default
	Type v1.UserType `json:"type,omitempty"`
	// user's name
	Name string `json:"name,omitempty"`
	// user's password
	Password string `json:"password,omitempty"`
	// whether the request is from console
	IsFromConsole bool `json:"-"`
}

type UserLoginResponse struct {
	UserResponseItem `json:",inline"`
	// the timestamp when the user's token expires, in seconds.
	Expire int64 `json:"expire"`
	// user's token
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
