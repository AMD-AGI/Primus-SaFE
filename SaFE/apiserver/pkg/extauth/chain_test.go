/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package extauth

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

type stubAuth struct {
	name string
	id   *Identity
	err  error
	n    int
}

func (s *stubAuth) Name() string { return s.name }
func (s *stubAuth) Authenticate(context.Context, Credentials) (*Identity, error) {
	s.n++
	return s.id, s.err
}

func TestChainOrderAndFallthrough(t *testing.T) {
	skip := &stubAuth{name: "a", err: ErrNotApplicable}
	rejected := &stubAuth{name: "b", err: ErrUnauthenticated}
	ok := &stubAuth{name: "c", id: &Identity{ID: "u1"}}
	late := &stubAuth{name: "d", id: &Identity{ID: "u2"}}
	id, err := NewChain(skip, rejected, ok, late).Authenticate(context.Background(), Credentials{Bearer: "x"})
	require.NoError(t, err)
	require.Equal(t, "u1", id.ID, "the first authenticator that accepts decides")
	require.Zero(t, late.n, "later authenticators are not asked")
}

func TestChainAllRefuse(t *testing.T) {
	_, err := NewChain(&stubAuth{err: ErrNotApplicable}, &stubAuth{err: ErrUnauthenticated}).Authenticate(context.Background(), Credentials{Bearer: "x"})
	require.ErrorIs(t, err, ErrUnauthenticated)
	_, err = NewChain().Authenticate(context.Background(), Credentials{Bearer: "x"})
	require.ErrorIs(t, err, ErrUnauthenticated, "an empty chain refuses everything")
}

func TestChainUnavailableIsNotUnauthenticated(t *testing.T) {
	down := &stubAuth{name: "w", err: &UnavailableError{Authenticator: "w", Err: errors.New("boom")}}
	_, err := NewChain(down, &stubAuth{err: ErrUnauthenticated}).Authenticate(context.Background(), Credentials{Bearer: "x"})
	var u *UnavailableError
	require.ErrorAs(t, err, &u)
	// Another authenticator can still accept it.
	id, err := NewChain(down, &stubAuth{id: &Identity{ID: "u"}}).Authenticate(context.Background(), Credentials{Bearer: "x"})
	require.NoError(t, err)
	require.Equal(t, "u", id.ID)
}

func TestChainBuiltinMarker(t *testing.T) {
	down := &stubAuth{err: &UnavailableError{Authenticator: "w", Err: errors.New("boom")}}
	_, err := NewChain(&stubAuth{err: ErrNotApplicable}, down, Builtin{}).Authenticate(context.Background(), Credentials{Bearer: "x"})
	require.ErrorIs(t, err, ErrBuiltin, "a local administrator survives an outage of the external authenticator")
}

func TestChainIgnoresAnIdentityWithoutId(t *testing.T) {
	_, err := NewChain(&stubAuth{id: &Identity{}}).Authenticate(context.Background(), Credentials{Bearer: "x"})
	require.ErrorIs(t, err, ErrUnauthenticated, "no id is no identity, never a pass")
}

func TestMapGroupsAndOverlay(t *testing.T) {
	mappings := []commonconfig.AuthGroupMapping{
		{Group: "platform-admins", Roles: []string{"system-admin"}},
		{Group: "auditors", Roles: []string{"system-admin-readonly"}},
		{Group: "team-a-leads", Workspaces: []string{"ws-a"}, Roles: []string{"workspace-admin"}},
		{Group: "team-a", Workspaces: []string{"ws-a", "ws-b"}, Roles: []string{"default"}},
		{Group: "", Roles: []string{"system-admin"}},
	}
	stored := &v1.User{}
	stored.Name = "alice-id"
	stored.Spec.Roles = []v1.UserRole{v1.DefaultRole}

	t.Run("system admin by group", func(t *testing.T) {
		o := MapGroups("alice-id", []string{"platform-admins"}, mappings, false)
		u := ApplyOverlay(WithOverlay(context.Background(), o), stored)
		require.True(t, u.IsSystemAdmin())
		require.Equal(t, []v1.UserRole{v1.SystemAdminRole}, u.Spec.Roles, "an admin holds that role alone, as the admission webhook keeps it")
		require.False(t, stored.IsSystemAdmin(), "the stored record is never changed")
		require.Equal(t, []v1.UserRole{v1.DefaultRole}, stored.Spec.Roles)
	})
	t.Run("removing the group removes the role on the next request", func(t *testing.T) {
		o := MapGroups("alice-id", nil, mappings, false)
		u := ApplyOverlay(WithOverlay(context.Background(), o), stored)
		require.False(t, u.IsSystemAdmin())
	})
	t.Run("workspace scoped mapping", func(t *testing.T) {
		o := MapGroups("alice-id", []string{"team-a-leads", "team-a"}, mappings, false)
		u := ApplyOverlay(WithOverlay(context.Background(), o), stored)
		require.ElementsMatch(t, []string{"ws-a", "ws-b"}, u.Spec.Resources["workspaces"])
		require.Equal(t, []string{"ws-a"}, u.Spec.Resources["managedWorkspaces"])
		require.NotContains(t, u.Spec.Roles, v1.WorkspaceAdminRole, "workspace-admin is not a user role")
		require.Empty(t, stored.Spec.Resources)
	})
	t.Run("an unmapped group gives nothing, an empty mapping group matches nothing", func(t *testing.T) {
		o := MapGroups("alice-id", []string{"unknown", ""}, mappings, false)
		require.Empty(t, o.Roles)
		require.Empty(t, o.Workspaces)
	})
	t.Run("restricted", func(t *testing.T) {
		o := MapGroups("alice-id", nil, nil, true)
		require.True(t, ApplyOverlay(WithOverlay(context.Background(), o), stored).IsRestricted())
		require.False(t, stored.IsRestricted())
	})
	t.Run("another user is untouched", func(t *testing.T) {
		o := MapGroups("someone-else", []string{"platform-admins"}, mappings, false)
		u := ApplyOverlay(WithOverlay(context.Background(), o), stored)
		require.False(t, u.IsSystemAdmin())
	})
	t.Run("no overlay, nil context", func(t *testing.T) {
		require.Same(t, stored, ApplyOverlay(context.Background(), stored))
		require.Same(t, stored, ApplyOverlay(nil, stored)) //nolint:staticcheck
	})
}

func TestForbiddenDelegatedCapabilities(t *testing.T) {
	for _, tt := range []struct {
		method, path, capability string
	}{
		{http.MethodPost, "/api/v1/apikeys", CapabilityApiKey},
		{http.MethodGet, "/api/v1/apikeys", CapabilityApiKey},
		{http.MethodDelete, "/api/v1/apikeys/1", CapabilityApiKey},
		{http.MethodGet, "/api/v1/secrets/s1", CapabilitySecret},
		{http.MethodPost, "/api/v1/secrets", CapabilitySecret},
		{http.MethodGet, "/api/v1/users", CapabilityUserManagement},
		{http.MethodGet, "/api/v1/users/u1", CapabilityUserManagement},
		{http.MethodGet, "/api/v1/users/u1/settings", CapabilityUserManagement},
		{http.MethodPatch, "/api/v1/users/self", CapabilityUserManagement},
		{http.MethodPatch, "/api/v1/users/u1", CapabilityUserManagement},
		{http.MethodDelete, "/api/v1/users/u1", CapabilityUserManagement},
		{http.MethodPost, "/api/v1/publickeys", CapabilityCredentialMint},
		{http.MethodGet, "/api/v1/llm-gateway/binding", CapabilityCredentialRead},
		{http.MethodPost, "/api/v1/auth/verify", CapabilityPlatformKey},
		// Path tricks do not get around it.
		{http.MethodGet, "/api/v1//apikeys", CapabilityApiKey},
		{http.MethodGet, "/api/v1/./secrets/s1", CapabilitySecret},
		{http.MethodGet, "/API/V1/Secrets/s1", CapabilitySecret},
		{http.MethodGet, "/api/v1/x/../secrets/s1", CapabilitySecret},
	} {
		c, forbidden := Forbidden(tt.method, tt.path)
		require.True(t, forbidden, "%s %s", tt.method, tt.path)
		require.Equal(t, tt.capability, c)
	}
	for _, tt := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/users/self"},
		{http.MethodGet, "/api/v1/users/self/workspaces"},
		{http.MethodGet, "/api/v1/workloads"},
		{http.MethodPost, "/api/v1/workloads"},
		{http.MethodGet, "/api/v1/publickeys"},
		{http.MethodGet, "/api/v1/llm-gateway/usage"},
		{http.MethodGet, "/healthz"},
	} {
		_, forbidden := Forbidden(tt.method, tt.path)
		require.False(t, forbidden, "%s %s", tt.method, tt.path)
	}
}

func TestScopeAllows(t *testing.T) {
	require.True(t, ScopeAllows([]string{ScopeAPI}, http.MethodPost))
	require.True(t, ScopeAllows([]string{ScopeRead}, http.MethodGet))
	require.False(t, ScopeAllows([]string{ScopeRead}, http.MethodPost))
	require.False(t, ScopeAllows(nil, http.MethodGet), "a delegated token without a scope allows nothing")
	require.False(t, ScopeAllows([]string{"profile"}, http.MethodGet))
}
