/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
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

// IsRolesEqual compares two role lists for equality.
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

// GetWorkspace returns the list of workspaces the user has access to.
func GetWorkspace(user *v1.User) []string {
	return getWorkspace(user, common.UserWorkspaces)
}

// GetManagedWorkspace returns the list of workspaces the user manages.
func GetManagedWorkspace(user *v1.User) []string {
	return getWorkspace(user, common.UserManagedWorkspaces)
}

// getWorkspace retrieves workspace names from user spec based on the provided key.
func getWorkspace(user *v1.User, key string) []string {
	if user == nil || len(user.Spec.Resources) == 0 {
		return nil
	}
	return user.Spec.Resources[key]
}

// AddWorkspace adds workspaces to the user's accessible workspace list.
func AddWorkspace(user *v1.User, workspaceNames ...string) bool {
	return addWorkspace(user, common.UserWorkspaces, workspaceNames...)
}

// AddManagedWorkspace adds workspaces to the user's managed workspace list.
func AddManagedWorkspace(user *v1.User, workspaceNames ...string) bool {
	return addWorkspace(user, common.UserManagedWorkspaces, workspaceNames...)
}

// addWorkspace adds workspace names to user's resources map under specified key.
func addWorkspace(user *v1.User, key string, workspaceNames ...string) bool {
	if user == nil || len(workspaceNames) == 0 {
		return false
	}
	if user.Spec.Resources == nil {
		user.Spec.Resources = make(map[string][]string)
	}
	userWorkspaces, ok := user.Spec.Resources[key]
	if !ok {
		user.Spec.Resources[key] = slice.Copy(workspaceNames, len(workspaceNames))
		return true
	}

	userWorkspaces, ok = slice.AddAndDelDuplicates(userWorkspaces, workspaceNames)
	if !ok {
		return false
	}
	user.Spec.Resources[key] = userWorkspaces
	return true
}

// RemoveWorkspace removes a workspace from the user's accessible list.
func RemoveWorkspace(user *v1.User, workspaceName string) bool {
	return removeWorkspace(user, workspaceName, common.UserWorkspaces)
}

// RemoveManagedWorkspace removes a workspace from the user's managed list.
func RemoveManagedWorkspace(user *v1.User, workspaceName string) bool {
	return removeWorkspace(user, workspaceName, common.UserManagedWorkspaces)
}

// removeWorkspace removes a workspace from user's resources map under specified key.
func removeWorkspace(user *v1.User, workspaceName, key string) bool {
	if user == nil || len(user.Spec.Resources) == 0 {
		return false
	}
	userWorkspaces, ok := user.Spec.Resources[key]
	if !ok {
		return false
	}
	userWorkspaces, ok = slice.RemoveString(userWorkspaces, workspaceName)
	if ok {
		user.Spec.Resources[key] = userWorkspaces
		return true
	}
	return false
}

// HasWorkspaceRight checks if the user has access to specified workspaces.
func HasWorkspaceRight(user *v1.User, workspaces ...string) bool {
	return hasWorkspaceRight(user, common.UserWorkspaces, workspaces...)
}

// HasWorkspaceManagedRight checks if the user has management rights.
func HasWorkspaceManagedRight(user *v1.User, workspaces ...string) bool {
	return hasWorkspaceRight(user, common.UserManagedWorkspaces, workspaces...)
}

// hasWorkspaceRight checks if user has rights to specified workspaces under given key.
func hasWorkspaceRight(user *v1.User, key string, workspaces ...string) bool {
	userWorkspaces := getWorkspace(user, key)
	return slice.ContainsStrings(userWorkspaces, workspaces)
}

// AssignWorkspace replaces the user's accessible workspace list.
func AssignWorkspace(user *v1.User, workspaces ...string) {
	DelWorkspace(user)
	AddWorkspace(user, workspaces...)
}

// AssignManagedWorkspace replaces the user's managed workspace list.
func AssignManagedWorkspace(user *v1.User, workspaces ...string) {
	DelManagedWorkspace(user)
	AddManagedWorkspace(user, workspaces...)
}

// DelWorkspace clears all accessible workspaces from the user.
func DelWorkspace(user *v1.User) {
	delWorkspace(user, common.UserWorkspaces)
}

// DelManagedWorkspace clears all managed workspaces from the user.
func DelManagedWorkspace(user *v1.User) {
	delWorkspace(user, common.UserManagedWorkspaces)
}

// delWorkspace removes all workspaces under specified key from user's resources.
func delWorkspace(user *v1.User, key string) {
	if user == nil || len(user.Spec.Resources) == 0 {
		return
	}
	delete(user.Spec.Resources, key)
}

// GenerateUserIdByName generates a unique user ID from a username.
func GenerateUserIdByName(name string) string {
	return stringutil.MD5(name)
}
