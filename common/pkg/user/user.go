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

// IsRolesEqual compares two slices of user roles for equality
// Parameters:
//
//	roles1: First slice of UserRole to compare
//	roles2: Second slice of UserRole to compare
//
// Returns:
//
//	true if both slices contain the same roles regardless of order, false otherwise
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

// GetWorkspace retrieves the list of workspaces that a user has access to
// Parameters:
//
//	user: Pointer to User object
//
// Returns:
//
//	Slice of workspace names the user can access
func GetWorkspace(user *v1.User) []string {
	return getWorkspace(user, common.UserWorkspaces)
}

// GetManagedWorkspace retrieves the list of workspaces that a user can manage
// Parameters:
//
//	user: Pointer to User object
//
// Returns:
//
//	Slice of workspace names the user can manage
func GetManagedWorkspace(user *v1.User) []string {
	return getWorkspace(user, common.UserManagedWorkspaces)
}

// getWorkspace retrieves workspace names from user spec based on the provided key
// Parameters:
//
//	user: Pointer to User object
//	key: Key to look up in user's resources map
//
// Returns:
//
//	Slice of workspace names or nil if user is nil or has no resources
func getWorkspace(user *v1.User, key string) []string {
	if user == nil || len(user.Spec.Resources) == 0 {
		return nil
	}
	return user.Spec.Resources[key]
}

// AddWorkspace adds workspace names to user's accessible workspaces list
// Parameters:
//
//	user: Pointer to User object
//	workspaceNames: Variable number of workspace names to add
//
// Returns:
//
//	true if workspaces were successfully added, false otherwise
func AddWorkspace(user *v1.User, workspaceNames ...string) bool {
	return addWorkspace(user, common.UserWorkspaces, workspaceNames...)
}

// AddManagedWorkspace adds workspace names to user's manageable workspaces list
// Parameters:
//
//	user: Pointer to User object
//	workspaceNames: Variable number of workspace names to add
//
// Returns:
//
//	true if workspaces were successfully added, false otherwis
func AddManagedWorkspace(user *v1.User, workspaceNames ...string) bool {
	return addWorkspace(user, common.UserManagedWorkspaces, workspaceNames...)
}

// addWorkspace adds workspace names to user's resources map under specified key
// Parameters:
//
//	user: Pointer to User object
//	key: Key in resources map where workspaces should be stored
//	workspaceNames: Variable number of workspace names to add
//
// Returns:
//
//	true if workspaces were successfully added, false otherwise
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

// RemoveWorkspace removes a workspace from user's accessible workspaces list
// Parameters:
//
//	user: Pointer to User object
//	workspaceName: Name of workspace to remove
//
// Returns:
//
//	true if workspace was successfully removed, false otherwis
func RemoveWorkspace(user *v1.User, workspaceName string) bool {
	return removeWorkspace(user, workspaceName, common.UserWorkspaces)
}

// RemoveManagedWorkspace removes a workspace from user's manageable workspaces list
// Parameters:
//
//	user: Pointer to User object
//	workspaceName: Name of workspace to remove
//
// Returns:
//
//	true if workspace was successfully removed, false otherwise
func RemoveManagedWorkspace(user *v1.User, workspaceName string) bool {
	return removeWorkspace(user, workspaceName, common.UserManagedWorkspaces)
}

// removeWorkspace removes a workspace from user's resources map under specified key
// Parameters:
//
//	user: Pointer to User object
//	workspaceName: Name of workspace to remove
//	key: Key in resources map where workspace is stored
//
// Returns:
//
//	true if workspace was successfully removed, false otherwise
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

// HasWorkspaceRight checks if user has access rights to specified workspaces
// Parameters:
//
//	user: Pointer to User object
//	workspaces: Variable number of workspace names to check
//
// Returns:
//
//	true if user has access to all specified workspaces, false otherwis
func HasWorkspaceRight(user *v1.User, workspaces ...string) bool {
	return hasWorkspaceRight(user, common.UserWorkspaces, workspaces...)
}

// HasWorkspaceManagedRight checks if user has management rights to specified workspaces
// Parameters:
//
//	user: Pointer to User object
//	workspaces: Variable number of workspace names to check
//
// Returns:
//
//	true if user can manage all specified workspaces, false otherwi
func HasWorkspaceManagedRight(user *v1.User, workspaces ...string) bool {
	return hasWorkspaceRight(user, common.UserManagedWorkspaces, workspaces...)
}

// hasWorkspaceRight checks if user has rights to specified workspaces under given key
// Parameters:
//
//	user: Pointer to User object
//	key: Key in resources map to check
//	workspaces: Variable number of workspace names to check
//
// Returns:
//
//	true if user has rights to all specified workspaces, false otherwis
func hasWorkspaceRight(user *v1.User, key string, workspaces ...string) bool {
	userWorkspaces := getWorkspace(user, key)
	return slice.ContainsStrings(userWorkspaces, workspaces)
}

// AssignWorkspace assigns workspaces to user, replacing any existing workspace assignments
// Parameters:
//
//	user: Pointer to User object
//	workspaces: Variable number of workspace names to assign
func AssignWorkspace(user *v1.User, workspaces ...string) {
	DelWorkspace(user)
	AddWorkspace(user, workspaces...)
}

// AssignManagedWorkspace assigns manageable workspaces to user, replacing any existing assignments
// Parameters:
//
//	user: Pointer to User object
//	workspaces: Variable number of workspace names to assign
func AssignManagedWorkspace(user *v1.User, workspaces ...string) {
	DelManagedWorkspace(user)
	AddManagedWorkspace(user, workspaces...)
}

// DelManagedWorkspace removes all manageable workspaces from user
// Parameters:
//
//	user: Pointer to User object
func DelWorkspace(user *v1.User) {
	delWorkspace(user, common.UserWorkspaces)
}

// DelManagedWorkspace removes all manageable workspaces from user
// Parameters:
//
//	user: Pointer to User object
func DelManagedWorkspace(user *v1.User) {
	delWorkspace(user, common.UserManagedWorkspaces)
}

// delWorkspace removes all workspaces under specified key from user's resources
// Parameters:
//
//	user: Pointer to User object
//	key: Key in resources map to delete
func delWorkspace(user *v1.User, key string) {
	if user == nil || len(user.Spec.Resources) == 0 {
		return
	}
	delete(user.Spec.Resources, key)
}

// GenerateUserIdByName generates a user ID by applying MD5 hash to the username
// Parameters:
//
//	name: Username string
//
// Returns:
//
//	MD5 hash of the username as user I
func GenerateUserIdByName(name string) string {
	return stringutil.MD5(name)
}
