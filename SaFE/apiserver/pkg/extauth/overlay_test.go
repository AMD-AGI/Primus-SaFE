/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package extauth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

func overlayUser(roles []v1.UserRole, workspaces ...string) *v1.User {
	u := &v1.User{}
	u.Name = "alice-id"
	u.Spec.Roles = roles
	if len(workspaces) > 0 {
		u.Spec.Resources = map[string][]string{common.UserWorkspaces: workspaces}
	}
	return u
}

var overlayMappings = []commonconfig.AuthGroupMapping{
	{Group: "platform-admins", Roles: []string{"system-admin"}},
	{Group: "auditors", Roles: []string{"system-admin-readonly"}},
	{Group: "team-a-leads", Workspaces: []string{"ws-a"}, Roles: []string{"workspace-admin"}},
	{Group: "team-a", Workspaces: []string{"ws-a", "ws-b"}, Roles: []string{"default"}},
}

// GetRequestUser applies the overlay to a user that Authorize applies it to again, so applying
// it must be idempotent, whatever the stored roles are.
func TestApplyOverlayIsIdempotent(t *testing.T) {
	var (
		admin    = []v1.UserRole{v1.SystemAdminRole}
		readonly = []v1.UserRole{v1.SystemAdminReadonlyRole, v1.DefaultRole}
		plain    = []v1.UserRole{v1.DefaultRole}
		custom   = []v1.UserRole{"data-scientist", v1.DefaultRole}
		bare     = []v1.UserRole{}
	)
	for _, tt := range []struct {
		name   string
		stored []v1.UserRole
		groups []string
	}{
		{"plain, admin group", plain, []string{"platform-admins"}},
		{"plain, no group", plain, nil},
		{"readonly record, no group", readonly, nil},
		{"readonly record, same group", readonly, []string{"auditors"}},
		{"readonly record, admin group", readonly, []string{"platform-admins"}},
		{"admin record, no group", admin, nil},
		{"admin record, admin group", admin, []string{"platform-admins"}},
		{"admin record, readonly group", admin, []string{"auditors"}},
		{"custom role kept", custom, []string{"auditors"}},
		{"no roles stored", bare, []string{"auditors"}},
		{"workspaces and managed workspaces", plain, []string{"team-a-leads", "team-a"}},
		{"every group", plain, []string{"platform-admins", "auditors", "team-a"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := WithOverlay(context.Background(), MapGroups("alice-id", tt.groups, overlayMappings, false))
			stored := overlayUser(tt.stored)
			once := ApplyOverlay(ctx, stored)
			twice := ApplyOverlay(ctx, once)
			require.Equal(t, once, twice)
			require.Equal(t, overlayUser(tt.stored), stored, "the record is never modified")
		})
	}
}

// In external mode the platform roles are those the identity grants, and only those: the
// record's value is ignored, so a role revoked upstream is not kept alive by a stale one on
// the user object. Roles that are not platform roles are still a union.
func TestApplyOverlayPlatformRolesComeOnlyFromTheIdentity(t *testing.T) {
	var (
		admin    = []v1.UserRole{v1.SystemAdminRole}
		readonly = []v1.UserRole{v1.SystemAdminReadonlyRole, v1.DefaultRole}
		plain    = []v1.UserRole{v1.DefaultRole}
	)
	for _, tt := range []struct {
		name   string
		stored []v1.UserRole
		groups []string
		want   []v1.UserRole
	}{
		{"stored admin, group gone: revoked", admin, nil, plain},
		{"stored admin, still in the group", admin, []string{"platform-admins"}, admin},
		{"stored plain, in the group: granted by the group alone", plain, []string{"platform-admins"}, admin},
		{"stored admin, moved to the readonly group: downgraded", admin, []string{"auditors"}, []v1.UserRole{v1.DefaultRole, v1.SystemAdminReadonlyRole}},
		{"stored readonly, group gone: revoked", readonly, nil, plain},
		{"stored readonly, still in the group", readonly, []string{"auditors"}, []v1.UserRole{v1.DefaultRole, v1.SystemAdminReadonlyRole}},
		{"stored readonly, promoted to admin", readonly, []string{"platform-admins"}, admin},
		{"stored admin with nothing else: revoked admin is an ordinary user", admin, []string{"unrelated"}, plain},
		{"a role that is not a platform role stays a union", []v1.UserRole{"data-scientist", v1.SystemAdminRole}, nil, []v1.UserRole{v1.DefaultRole, "data-scientist"}},
		{"an ordinary user holds the default role", []v1.UserRole{"data-scientist"}, nil, []v1.UserRole{"data-scientist", v1.DefaultRole}},
		{"a mapped default role is not duplicated", plain, []string{"team-a"}, plain},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := WithOverlay(context.Background(), MapGroups("alice-id", tt.groups, overlayMappings, false))
			u := ApplyOverlay(ctx, overlayUser(tt.stored))
			require.ElementsMatch(t, tt.want, u.Spec.Roles)
			require.Equal(t, v1.IsContainRole(tt.want, v1.SystemAdminRole), u.IsSystemAdmin())
			require.Equal(t, v1.IsContainRole(tt.want, v1.SystemAdminReadonlyRole), u.IsSystemAdminReadonly())
		})
	}

	t.Run("workspaces and restriction are still a union", func(t *testing.T) {
		o := MapGroups("alice-id", []string{"team-a"}, overlayMappings, true)
		u := ApplyOverlay(WithOverlay(context.Background(), o), overlayUser(admin, "ws-z"))
		require.ElementsMatch(t, []string{"ws-z", "ws-a", "ws-b"}, u.Spec.Resources[common.UserWorkspaces])
		require.True(t, u.IsRestricted())
		require.False(t, u.IsSystemAdmin())
	})
	t.Run("another user is untouched", func(t *testing.T) {
		o := MapGroups("someone-else", nil, overlayMappings, false)
		stored := overlayUser(admin)
		require.Same(t, stored, ApplyOverlay(WithOverlay(context.Background(), o), stored))
	})
	t.Run("a context with no overlay keeps the record", func(t *testing.T) {
		// builtin mode, and callers that are not requests (an SSH session, a controller).
		stored := overlayUser(admin)
		require.Same(t, stored, ApplyOverlay(context.Background(), stored))
	})
}

// GrantedWorkspaces is what the identity grants the caller, and nothing for anyone else.
func TestGrantedWorkspaces(t *testing.T) {
	o := &Overlay{UserId: "alice-id", Workspaces: []string{"ws-a", "ws-b"}}
	withOverlay := WithOverlay(context.Background(), o)

	require.Equal(t, []string{"ws-a", "ws-b"}, GrantedWorkspaces(withOverlay, overlayUser(nil)))
	require.Nil(t, GrantedWorkspaces(context.Background(), overlayUser(nil)), "no overlay")
	require.Nil(t, GrantedWorkspaces(withOverlay, nil), "no user")

	other := overlayUser(nil)
	other.Name = "bob-id"
	require.Nil(t, GrantedWorkspaces(withOverlay, other), "the overlay is the caller's alone")

	// A copy: a caller that edits the answer does not edit the request's overlay.
	got := GrantedWorkspaces(withOverlay, overlayUser(nil))
	got[0] = "changed"
	require.Equal(t, "ws-a", o.Workspaces[0])
}
