/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package extauth

import (
	"context"
	"sort"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

// Overlay is what the identity of one request adds to the user record. It lives in
// the request context only: nothing here is written to the user object, so removing
// a group takes effect on the next request and cannot drift from the record.
type Overlay struct {
	UserId     string
	Roles      []v1.UserRole
	Workspaces []string
	Managed    []string
	Restricted bool
}

type overlayKey struct{}

// WithOverlay returns a context carrying the overlay.
func WithOverlay(ctx context.Context, o *Overlay) context.Context {
	return context.WithValue(ctx, overlayKey{}, o)
}

// ApplyOverlay returns the user as this request sees it: the record's roles and
// workspaces united with the overlay's. The record passed in is not modified; when
// there is no overlay for this user, it is returned as is.
func ApplyOverlay(ctx context.Context, user *v1.User) *v1.User {
	if ctx == nil {
		return user
	}
	o, _ := ctx.Value(overlayKey{}).(*Overlay)
	if o == nil || user == nil || user.Name != o.UserId {
		return user
	}
	u := user.DeepCopy()
	for _, r := range o.Roles {
		if !v1.IsContainRole(u.Spec.Roles, r) {
			u.Spec.Roles = append(u.Spec.Roles, r)
		}
	}
	if u.IsSystemAdmin() {
		// As the admission webhook keeps it: a system administrator holds that role alone.
		u.Spec.Roles = []v1.UserRole{v1.SystemAdminRole}
	}
	commonuser.AddWorkspace(u, o.Workspaces...)
	commonuser.AddManagedWorkspace(u, o.Managed...)
	if o.Restricted {
		u.Spec.RestrictedType = v1.UserFrozen
	}
	return u
}

// MapGroups turns the groups of an identity into an overlay, by the configured mappings.
// A mapping with workspaces scopes the roles to them; the workspace-admin role is not
// a user role (the admission webhook strips it), it means managing those workspaces.
func MapGroups(userId string, groups []string, mappings []commonconfig.AuthGroupMapping, restricted bool) *Overlay {
	o := &Overlay{UserId: userId, Restricted: restricted}
	have := map[string]bool{}
	for _, g := range groups {
		have[g] = true
	}
	roles := map[string]bool{}
	for _, m := range mappings {
		if m.Group == "" || !have[m.Group] {
			continue
		}
		for _, r := range m.Roles {
			if r == string(v1.WorkspaceAdminRole) {
				o.Managed = append(o.Managed, m.Workspaces...)
				continue
			}
			roles[r] = true
		}
		o.Workspaces = append(o.Workspaces, m.Workspaces...)
	}
	for r := range roles {
		o.Roles = append(o.Roles, v1.UserRole(r))
	}
	o.Workspaces, o.Managed = dedupe(o.Workspaces), dedupe(o.Managed)
	sort.Slice(o.Roles, func(i, j int) bool { return o.Roles[i] < o.Roles[j] })
	return o
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
