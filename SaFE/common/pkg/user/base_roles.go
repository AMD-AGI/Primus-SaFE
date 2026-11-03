/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package user

import (
	"strings"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// BaseRoles returns the roles every user who is not a system administrator holds, from
// users.base_roles. It is [default] unless the deployment says otherwise, which is what the
// platform has always done, so an unconfigured deployment sees no change.
//
// A name that is a platform role (system-admin, system-admin-readonly) or the workspace-admin
// marker is ignored: a base role is held by everyone, and these are granted to people one by
// one (the identity provider owns the platform roles; the admission webhook strips
// workspace-admin). Blank names and repeats are dropped. A list left empty by that, or by
// the operator, is the default: a user cannot be created without a role.
//
// The list is read on every call, so a reloaded configuration applies to the next request.
func BaseRoles() []v1.UserRole {
	var roles []v1.UserRole
	seen := map[v1.UserRole]bool{}
	for _, name := range commonconfig.GetUserBaseRoles() {
		role := v1.UserRole(strings.TrimSpace(name))
		if role == "" || seen[role] || role == v1.SystemAdminRole ||
			role == v1.SystemAdminReadonlyRole || role == v1.WorkspaceAdminRole {
			continue
		}
		seen[role] = true
		roles = append(roles, role)
	}
	if len(roles) == 0 {
		return []v1.UserRole{v1.DefaultRole}
	}
	return roles
}

// MissingBaseRoles returns the base roles that roles does not contain, in the order of
// BaseRoles. It is empty when the user already holds them all.
func MissingBaseRoles(roles []v1.UserRole) []v1.UserRole {
	var missing []v1.UserRole
	for _, base := range BaseRoles() {
		if !v1.IsContainRole(roles, base) {
			missing = append(missing, base)
		}
	}
	return missing
}
