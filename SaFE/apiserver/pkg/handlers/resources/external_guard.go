/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// With auth.mode external the identity provider owns sign-in, accounts, API keys and
// platform roles, so the apiserver stops offering the ways of creating them itself:
// password and SSO sign-in (POST /login), registration (POST /users), API key creation,
// changes to the platform roles of a user and setting a password. There is no exception
// for local accounts; backing out of the external mode is a configuration change
// (auth.mode back to builtin), not a local sign-in. Deleting an API key is not refused:
// the keys issued before the switch keep authenticating through the built-in entry, and
// revoking one must stay possible. Nothing here applies in builtin mode, the default.

// refuseExternal returns the 403 for an operation that is not available in external
// mode, or nil in builtin mode.
func refuseExternal(operation string) error {
	if commonconfig.GetAuthMode() != commonconfig.AuthModeExternal {
		return nil
	}
	return commonerrors.NewForbidden(operation + " is not available: authentication is managed by an external identity provider")
}

// refuseExternalPasswordChange refuses a patch that carries a password. There are no
// passwords in external mode: the console no longer offers one, and a password set here would
// be one its owner does not know about, waiting for the day auth.mode goes back to builtin.
func refuseExternalPasswordChange(req *view.PatchUserRequest) error {
	if req.Password == nil {
		return nil
	}
	return refuseExternal("changing a password")
}

// refuseExternalRoleChange refuses a patch that adds or removes a platform role of a
// user. A patch that leaves the platform roles as they are, such as one that only echoes
// the roles back or changes the workspaces, is not a role change.
func refuseExternalRoleChange(target *v1.User, req *view.PatchUserRequest) error {
	if req.Roles == nil {
		return nil
	}
	for _, role := range extauth.PlatformRoles {
		if v1.IsContainRole(target.Spec.Roles, role) != v1.IsContainRole(*req.Roles, role) {
			return refuseExternal("changing platform roles")
		}
	}
	return nil
}
