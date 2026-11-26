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
	// Sources lists, per workspace, the group:<name> and team:<id> sources of the access the
	// overlay grants. ManagedSources does the same for the management the overlay grants.
	Sources        map[string][]string
	ManagedSources map[string][]string
}

type overlayKey struct{}

// WithOverlay returns a context carrying the overlay.
func WithOverlay(ctx context.Context, o *Overlay) context.Context {
	return context.WithValue(ctx, overlayKey{}, o)
}

// PlatformRoles are the roles an identity grants platform-wide. In external mode the
// authenticators are their only source: the value stored on the user object is not
// consulted for them (see ApplyOverlay). It is the one list of them: whatever needs to
// know which roles the identity provider owns, such as the guard on editing them, reads it
// from here.
var PlatformRoles = []v1.UserRole{v1.SystemAdminRole, v1.SystemAdminReadonlyRole}

// ApplyOverlay returns the user as this request sees it. For a request that has an overlay
// (every request the external mode authenticates):
//   - the platform roles (system-admin, system-admin-readonly) are those the overlay grants,
//     and only those: what the user object stores for them is ignored, so revoking one at the
//     identity provider removes it from the next request that is authenticated afresh, even
//     if the object still lists it. A user left without one is an ordinary user, who holds
//     the base roles (the default role unless users.base_roles says otherwise);
//   - every other role, the workspaces and the restriction are the record's united with the
//     overlay's.
//
// The record passed in is not modified; when there is no overlay for this user, it is
// returned as is. Applying it twice gives the same user as applying it once, so a caller
// that is handed an already merged user may apply it again.
func ApplyOverlay(ctx context.Context, user *v1.User) *v1.User {
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
		// As the admission webhook keeps it: a system administrator holds that role alone.
		u.Spec.Roles = []v1.UserRole{v1.SystemAdminRole}
	default:
		// As the admission webhook keeps it: everyone else holds the base roles (the default
		// role unless users.base_roles says otherwise). They were not stored for an
		// administrator, who is an ordinary user once the identity revokes the role.
		if missing := commonuser.MissingBaseRoles(u.Spec.Roles); len(missing) > 0 {
			u.Spec.Roles = append(missing, u.Spec.Roles...)
		}
	}
	commonuser.AddWorkspace(u, o.Workspaces...)
	commonuser.AddManagedWorkspace(u, o.Managed...)
	if o.Restricted {
		u.Spec.RestrictedType = v1.UserFrozen
	}
	return u
}

// RequestOverlay returns the overlay of the request when it belongs to user, and nil otherwise.
// The overlay is read only: it must not be modified.
func RequestOverlay(ctx context.Context, user *v1.User) *Overlay {
	if ctx == nil || user == nil {
		return nil
	}
	o, _ := ctx.Value(overlayKey{}).(*Overlay)
	if o == nil || user.Name != o.UserId {
		return nil
	}
	return o
}

// addSource records source as one of the sources of the workspace in the map.
func addSource(m *map[string][]string, workspace, source string) {
	if *m == nil {
		*m = map[string][]string{}
	}
	for _, s := range (*m)[workspace] {
		if s == source {
			return
		}
	}
	(*m)[workspace] = append((*m)[workspace], source)
}

// GrantedWorkspaces returns the workspaces this request's identity grants user, which the
// record does not have to store: they are the overlay's, and exist only while the groups do.
// It is empty for a request without an overlay and for any user but the one the request is
// from, because the overlay is that one user's.
func GrantedWorkspaces(ctx context.Context, user *v1.User) []string {
	if ctx == nil || user == nil {
		return nil
	}
	o, _ := ctx.Value(overlayKey{}).(*Overlay)
	if o == nil || user.Name != o.UserId {
		return nil
	}
	return append([]string(nil), o.Workspaces...)
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
		for _, ws := range m.Workspaces {
			addSource(&o.Sources, ws, GroupSourcePrefix+m.Group)
		}
		for _, r := range m.Roles {
			if r == string(v1.WorkspaceAdminRole) {
				o.Managed = append(o.Managed, m.Workspaces...)
				for _, ws := range m.Workspaces {
					addSource(&o.ManagedSources, ws, GroupSourcePrefix+m.Group)
				}
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

// Prefixes of the sources an overlay records for a workspace.
const (
	GroupSourcePrefix = "group:"
	TeamSourcePrefix  = "team:"
)

// TeamWorkspaceLookup returns the workspaces bound to a team and the role of its members in each.
type TeamWorkspaceLookup func(team string) map[string]v1.WorkspaceTeamRole

// AddTeams appends the workspaces bound to the given teams to the overlay.
// A manager binding adds the workspace to both the accessible and the managed list.
// When a team grants any workspace, the overlay also grants the default role, so that a user
// whose base roles do not include it can use the workspace. Teams never grant a platform role.
func AddTeams(o *Overlay, teams []string, lookup TeamWorkspaceLookup) {
	if o == nil || lookup == nil {
		return
	}
	granted := false
	for _, t := range dedupe(teams) {
		bound := lookup(t)
		names := make([]string, 0, len(bound))
		for ws := range bound {
			if ws != "" {
				names = append(names, ws)
			}
		}
		sort.Strings(names)
		for _, ws := range names {
			granted = true
			o.Workspaces = append(o.Workspaces, ws)
			addSource(&o.Sources, ws, TeamSourcePrefix+t)
			if bound[ws] == v1.WorkspaceTeamManager {
				o.Managed = append(o.Managed, ws)
				addSource(&o.ManagedSources, ws, TeamSourcePrefix+t)
			}
		}
	}
	if !granted {
		return
	}
	if !v1.IsContainRole(o.Roles, v1.DefaultRole) {
		o.Roles = append(o.Roles, v1.DefaultRole)
		sort.Slice(o.Roles, func(i, j int) bool { return o.Roles[i] < o.Roles[j] })
	}
	o.Workspaces, o.Managed = dedupe(o.Workspaces), dedupe(o.Managed)
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
