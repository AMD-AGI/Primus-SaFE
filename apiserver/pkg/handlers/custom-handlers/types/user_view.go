/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	"strings"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

const (
	Authorization = "Authorization"
	AuthCode      = "authorization_code"
)

type CreateUserRequest struct {
	// user's id
	Id string `json:"id,omitempty"`
	// user's name
	Name string `json:"name,omitempty"`
	// user's mail
	Email string `json:"email,omitempty"`
	// user's type. includes: default, teams
	Type v1.UserType `json:"type,omitempty"`
	// system-admin, queue-admin, default
	Roles []v1.UserRole `json:"roles,omitempty"`
	// the workspaces which user can access
	Workspaces []string `json:"workspaces,omitempty"`
	// password, base64 encode
	Password string `json:"password,omitempty"`
	// user's avatar URL
	AvatarUrl string `json:"avatarUrl,omitempty"`
}

type CreateUserResponse struct {
	UserId string `json:"userId"`
}

type ListUserRequest struct {
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
	Id string `json:"id,omitempty"`
	// user's name
	Name string `json:"name,omitempty"`
	// user's mail
	Email string `json:"email,omitempty"`
	// user's type. value includes: default, teams
	Type v1.UserType `json:"type,omitempty"`
	// system-admin, queue-admin, default
	Roles []v1.UserRole `json:"roles,omitempty"`
	// the workspace's name which user can access
	Workspaces []string `json:"workspaces,omitempty"`
	// the workspace's name which user can manage
	ManagedWorkspaces []string `json:"managedWorkspaces,omitempty"`
	// user's creation time
	CreatedTime string `json:"createdTime,omitempty"`
	// 0: normal; 1 frozen
	RestrictedType v1.UserRestrictedType `json:"restrictedType,omitempty"`
	// user's avatar URL
	AvatarUrl string `json:"avatarUrl,omitempty"`
}

type PatchUserRequest struct {
	// user's name
	Name *string `json:"name,omitempty"`
	// system-admin, queue-admin
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

type ListWorkspaceUsersResponse struct {
	TotalCount int                  `json:"totalCount"`
	Items      []WorkspaceUsersItem `json:"items"`
}

type WorkspaceUsersItem struct {
	UserId   string `json:"userId"`
	UserName string `json:"userName"`
}

type UserLoginRequest struct {
	// teams, default
	Type v1.UserType `json:"userType,omitempty"`
	// user's id
	Id string `json:"userId,omitempty"`
	// user's password
	Password string `json:"password,omitempty"`
	// whether the request is from console
	IsFromConsole bool `json:"-"`
	// the response url
	ReturnUrl string `json:"returnUrl,omitempty"`
}

type UserLoginResponse struct {
	UserResponseItem `json:",inline"`
	// the timestamp when the user's token expires, in seconds.
	Expire int64 `json:"expire"`
	// user's token
	Token string `json:"token"`
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
