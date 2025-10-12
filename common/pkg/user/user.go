/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package user

import (
	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

func IsRolesEqual(roles1, roles2 []v1.UserRole) bool {
	if len(roles1) != len(roles2) {
		return false
	}
	if len(roles1) == 0 {
		return true
	}
	currentRolesSet := sets.NewSet()
	for i := range roles1 {
		currentRolesSet.Insert(string(roles1[i]))
	}
	for _, r := range roles2 {
		if !currentRolesSet.Has(string(r)) {
			return false
		}
	}
	return true
}

func GetWorkspace(u *v1.User) []string {
	return getWorkspace(u, common.UserWorkspaces)
}

func GetManagedWorkspace(u *v1.User) []string {
	return getWorkspace(u, common.UserManagedWorkspaces)
}

func getWorkspace(u *v1.User, key string) []string {
	if u == nil || len(u.Spec.Resources) == 0 {
		return nil
	}
	return u.Spec.Resources[key]
}

func AddWorkspace(u *v1.User, workspaceNames ...string) bool {
	return addWorkspace(u, common.UserWorkspaces, workspaceNames...)
}

func AddManagedWorkspace(u *v1.User, workspaceNames ...string) bool {
	return addWorkspace(u, common.UserManagedWorkspaces, workspaceNames...)
}

func addWorkspace(u *v1.User, key string, workspaceNames ...string) bool {
	if u == nil || len(workspaceNames) == 0 {
		return false
	}
	if u.Spec.Resources == nil {
		u.Spec.Resources = make(map[string][]string)
	}
	userWorkspaces, ok := u.Spec.Resources[key]
	if !ok {
		u.Spec.Resources[key] = slice.Copy(workspaceNames, len(workspaceNames))
		return true
	}

	userWorkspaces, ok = slice.AddAndDelDuplicates(userWorkspaces, workspaceNames)
	if !ok {
		return false
	}
	u.Spec.Resources[key] = userWorkspaces
	return true
}

func RemoveWorkspace(u *v1.User, workspaceName string) bool {
	return removeWorkspace(u, workspaceName, common.UserWorkspaces)
}

func RemoveManagedWorkspace(u *v1.User, workspaceName string) bool {
	return removeWorkspace(u, workspaceName, common.UserManagedWorkspaces)
}

func removeWorkspace(u *v1.User, workspaceName, key string) bool {
	if u == nil || len(u.Spec.Resources) == 0 {
		return false
	}
	userWorkspaces, ok := u.Spec.Resources[key]
	if !ok {
		return false
	}
	userWorkspaces, ok = slice.RemoveString(userWorkspaces, workspaceName)
	if ok {
		u.Spec.Resources[key] = userWorkspaces
		return true
	}
	return false
}

func HasWorkspaceRight(u *v1.User, workspaces ...string) bool {
	return hasWorkspaceRight(u, common.UserWorkspaces, workspaces...)
}

func HasWorkspaceManagedRight(u *v1.User, workspaces ...string) bool {
	return hasWorkspaceRight(u, common.UserManagedWorkspaces, workspaces...)
}

func hasWorkspaceRight(u *v1.User, key string, workspaces ...string) bool {
	userWorkspaces := getWorkspace(u, key)
	return slice.ContainsStrings(userWorkspaces, workspaces)
}

func AssignWorkspace(u *v1.User, workspaces ...string) {
	DelWorkspace(u)
	AddWorkspace(u, workspaces...)
}

func AssignManagedWorkspace(u *v1.User, workspaces ...string) {
	DelManagedWorkspace(u)
	AddManagedWorkspace(u, workspaces...)
}

func DelWorkspace(u *v1.User) {
	delWorkspace(u, common.UserWorkspaces)
}

func DelManagedWorkspace(u *v1.User) {
	delWorkspace(u, common.UserManagedWorkspaces)
}

func delWorkspace(u *v1.User, key string) {
	if u == nil || len(u.Spec.Resources) == 0 {
		return
	}
	delete(u.Spec.Resources, key)
}

func GenerateUserIdByName(name string) string {
	return stringutil.MD5(name)
}
