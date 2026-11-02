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
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

// legacyApplyOverlay is ApplyOverlay as it was before users.base_roles existed, kept as the reference: with
// the defaults the overlay must give every request the user it always did. It is a frozen copy, not a call
// into the code under test.
func legacyApplyOverlay(ctx context.Context, user *v1.User) *v1.User {
	if ctx == nil {
		return user
	}
	o, _ := ctx.Value(overlayKey{}).(*Overlay)
	if o == nil || user == nil || user.Name != o.UserId {
		return user
	}
	u := user.DeepCopy()
	kept := make([]v1.UserRole, 0, len(u.Spec.Roles)+len(o.Roles))
	for _, r := range u.Spec.Roles {
		if !v1.IsContainRole(PlatformRoles, r) {
			kept = append(kept, r)
		}
	}
	for _, r := range o.Roles {
		if !v1.IsContainRole(kept, r) {
			kept = append(kept, r)
		}
	}
	u.Spec.Roles = kept
	switch {
	case u.IsSystemAdmin():
		u.Spec.Roles = []v1.UserRole{v1.SystemAdminRole}
	case !v1.IsContainRole(u.Spec.Roles, v1.DefaultRole):
		u.Spec.Roles = append([]v1.UserRole{v1.DefaultRole}, u.Spec.Roles...)
	}
	commonuser.AddWorkspace(u, o.Workspaces...)
	commonuser.AddManagedWorkspace(u, o.Managed...)
	if o.Restricted {
		u.Spec.RestrictedType = v1.UserFrozen
	}
	return u
}

func setBaseRoles(t *testing.T, value string) {
	t.Helper()
	if value != "unset" {
		commonconfig.SetValue("users.base_roles", value)
	}
	t.Cleanup(func() { commonconfig.UnsetValue("users.base_roles") })
}

var overlayStored = [][]v1.UserRole{
	nil,
	{},
	{v1.DefaultRole},
	{v1.SystemAdminRole},
	{v1.SystemAdminReadonlyRole, v1.DefaultRole},
	{"data-scientist"},
	{"data-scientist", v1.DefaultRole},
	{v1.SystemAdminRole, "data-scientist"},
	{v1.SelfReadRole},
}

var overlayGroups = [][]string{nil, {"platform-admins"}, {"auditors"}, {"team-a"}, {"team-a-leads", "team-a"}, {"platform-admins", "auditors", "team-a"}}

// With the defaults (unset, written out, empty), every request sees the user it always saw.
func TestApplyOverlayUnderTheDefaultsIsWhatItAlwaysWas(t *testing.T) {
	for _, base := range []string{"unset", "default", ""} {
		t.Run("base roles "+base, func(t *testing.T) {
			setBaseRoles(t, base)
			for _, stored := range overlayStored {
				for _, groups := range overlayGroups {
					ctx := WithOverlay(context.Background(), MapGroups("alice-id", groups, overlayMappings, false))
					require.Equal(t, legacyApplyOverlay(ctx, overlayUser(stored)), ApplyOverlay(ctx, overlayUser(stored)),
						"stored %v, groups %v", stored, groups)
				}
			}
		})
	}
}

// With users.base_roles set to the minimal role, the user a request sees holds it, in place of the default role
// that used to be given to everyone: a new user, a user of today, an administrator whom the identity provider no
// longer vouches for, and one it does.
func TestApplyOverlayWithTheMinimalBaseRole(t *testing.T) {
	setBaseRoles(t, "self-read")
	for _, tt := range []struct {
		name   string
		stored []v1.UserRole
		groups []string
		want   []v1.UserRole
	}{
		{"a new user holds only the minimal role", nil, nil, []v1.UserRole{v1.SelfReadRole}},
		{"so does one with an empty list", []v1.UserRole{}, nil, []v1.UserRole{v1.SelfReadRole}},
		{"a user of today keeps what it has and gains the minimal role", []v1.UserRole{v1.DefaultRole}, nil, []v1.UserRole{v1.SelfReadRole, v1.DefaultRole}},
		{"the default role is not given back to one that does not hold it", []v1.UserRole{"data-scientist"}, nil, []v1.UserRole{v1.SelfReadRole, "data-scientist"}},
		{"already held", []v1.UserRole{v1.SelfReadRole}, nil, []v1.UserRole{v1.SelfReadRole}},
		{"an administrator the provider vouches for holds that role alone", []v1.UserRole{v1.DefaultRole}, []string{"platform-admins"}, []v1.UserRole{v1.SystemAdminRole}},
		{"an administrator it no longer vouches for is an ordinary user, with the minimal role", []v1.UserRole{v1.SystemAdminRole}, nil, []v1.UserRole{v1.SelfReadRole}},
		{"a read-only administrator is an ordinary user too", []v1.UserRole{v1.SystemAdminReadonlyRole}, []string{"auditors"}, []v1.UserRole{v1.SelfReadRole, v1.SystemAdminReadonlyRole}},
		{"a role the identity's group maps to is held next to it", nil, []string{"team-a"}, []v1.UserRole{v1.SelfReadRole, v1.DefaultRole}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := WithOverlay(context.Background(), MapGroups("alice-id", tt.groups, overlayMappings, false))
			stored := overlayUser(tt.stored)
			got := ApplyOverlay(ctx, stored)
			require.Equal(t, tt.want, got.Spec.Roles)
			require.Equal(t, overlayUser(tt.stored), stored, "the record is never modified")
			require.Equal(t, got, ApplyOverlay(ctx, got), "applying it twice gives the same user")
		})
	}
}

// A user who is not the one the overlay is for, and a request with no overlay, are returned as they are: the
// base roles are a property of an authenticated request, not of a record.
func TestApplyOverlayLeavesOtherUsersAlone(t *testing.T) {
	setBaseRoles(t, "self-read")
	u := overlayUser([]v1.UserRole{v1.DefaultRole})
	require.Same(t, u, ApplyOverlay(context.Background(), u))
	ctx := WithOverlay(context.Background(), MapGroups("someone-else", nil, overlayMappings, false))
	require.Same(t, u, ApplyOverlay(ctx, u))
}
