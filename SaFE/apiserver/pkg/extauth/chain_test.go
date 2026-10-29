/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package extauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
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

// A scope is judged by what the request can do, not by its HTTP method: a GET that
// upgrades into a shell, exec or port-forward session is not a read.
func TestScopeAllows(t *testing.T) {
	upgrade := http.Header{"Connection": {"Upgrade"}, "Upgrade": {"websocket"}}
	for _, tt := range []struct {
		name    string
		scopes  []string
		method  string
		path    string
		headers http.Header
		want    bool
	}{
		{"api: any write", []string{ScopeAPI}, http.MethodPost, "/api/v1/workloads", nil, true},
		{"read: a read", []string{ScopeRead}, http.MethodGet, "/api/v1/workloads", nil, true},
		{"read: a HEAD", []string{ScopeRead}, http.MethodHead, "/api/v1/workloads", nil, true},
		{"read: a write", []string{ScopeRead}, http.MethodPost, "/api/v1/workloads", nil, false},
		{"no scope allows nothing", nil, http.MethodGet, "/api/v1/workloads", nil, false},
		{"unknown scope allows nothing", []string{"profile"}, http.MethodGet, "/api/v1/workloads", nil, false},

		// The web shell is a GET that upgrades into an interactive exec session.
		{"read: the web shell", []string{ScopeRead}, http.MethodGet, "/api/v1/workloads/w1/pods/p1/webshell", nil, false},
		{"read: the web shell, upgrade headers", []string{ScopeRead}, http.MethodGet, "/api/v1/workloads/w1/pods/p1/webshell", upgrade, false},
		{"api: the web shell", []string{ScopeAPI}, http.MethodGet, "/api/v1/workloads/w1/pods/p1/webshell", upgrade, true},
		{"read+api: the web shell", []string{ScopeRead, ScopeAPI}, http.MethodGet, "/api/v1/workloads/w1/pods/p1/webshell", upgrade, true},
		// The same capability under other names and on routes that do not exist yet.
		{"read: exec", []string{ScopeRead}, http.MethodGet, "/api/v1/workloads/w1/pods/p1/exec", nil, false},
		{"read: attach", []string{ScopeRead}, http.MethodGet, "/api/v1/workloads/w1/pods/p1/attach", nil, false},
		{"read: port-forward", []string{ScopeRead}, http.MethodGet, "/api/v1/workloads/w1/pods/p1/port-forward", nil, false},
		{"read: portforward", []string{ScopeRead}, http.MethodGet, "/api/v1/workloads/w1/pods/p1/portforward", nil, false},
		{"read: a new route that upgrades", []string{ScopeRead}, http.MethodGet, "/api/v1/clusters/c1/stream", upgrade, false},
		{"read: a route outside the api prefix that upgrades", []string{ScopeRead}, http.MethodGet, "/proxy/svc/ws", upgrade, false},
		{"read: SPDY upgrade", []string{ScopeRead}, http.MethodGet, "/api/v1/workloads/w1/stream", http.Header{"Connection": {"Upgrade"}, "Upgrade": {"SPDY/3.1"}}, false},
		{"read: upgrade token among others", []string{ScopeRead}, http.MethodGet, "/api/v1/workloads/w1/stream", http.Header{"Connection": {"keep-alive, Upgrade"}}, false},
		{"read: upgrade token, lower case, second header", []string{ScopeRead}, http.MethodGet, "/api/v1/workloads/w1/stream", http.Header{"Connection": {"keep-alive", "upgrade"}}, false},
		{"read: Upgrade header alone", []string{ScopeRead}, http.MethodGet, "/api/v1/workloads/w1/stream", http.Header{"Upgrade": {"websocket"}}, false},
		{"read: path tricks", []string{ScopeRead}, http.MethodGet, "/API/V1/Workloads/w1/Pods/p1/WebShell/", nil, false},
		{"read: path tricks, dot segments", []string{ScopeRead}, http.MethodGet, "/api/v1/workloads/w1/../w1/pods/p1/webshell", nil, false},

		// What stays a read.
		{"read: logs", []string{ScopeRead}, http.MethodGet, "/api/v1/workloads/w1/pods/p1/logs", nil, true},
		{"read: a workload that is called exec", []string{ScopeRead}, http.MethodGet, "/api/v1/workloads/exec", nil, true},
		{"read: keep-alive", []string{ScopeRead}, http.MethodGet, "/api/v1/workloads", http.Header{"Connection": {"keep-alive"}}, true},
		{"read: upgrade-insecure-requests is not an upgrade", []string{ScopeRead}, http.MethodGet, "/api/v1/workloads", http.Header{"Upgrade-Insecure-Requests": {"1"}}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.path, nil)
			for k, v := range tt.headers {
				r.Header[k] = v
			}
			require.Equal(t, tt.want, ScopeAllows(tt.scopes, r))
		})
	}
}

func TestInteractive(t *testing.T) {
	for _, tt := range []struct {
		path string
		want bool
	}{
		{"/api/v1/workloads/w1/pods/p1/webshell", true},
		{"/api/v1/workloads/w1/pods/p1/exec", true},
		{"/api/v1/nodes/n1/shell", true},
		{"/api/v1/workloads/w1/pods/p1/logs", false},
		{"/api/v1/workloads/exec", false},
		{"/api/v1/exec", false},
		{"/other/workloads/w1/pods/p1/exec", false},
	} {
		require.Equal(t, tt.want, Interactive(httptest.NewRequest(http.MethodGet, tt.path, nil)), tt.path)
	}
}

// A route whose handler upgrades must be classified by its path, not only by the
// headers a client happens to send.
func TestInteractiveRouteGaps(t *testing.T) {
	routes := []gin.RouteInfo{
		{Method: http.MethodGet, Path: "/api/v1/workloads/:name/pods/:podId/webshell", Handler: "pkg.(*H).WebShell-fm"},
		{Method: http.MethodGet, Path: "/api/v1/workloads/:name/pods/:podId/logs", Handler: "pkg.(*H).Logs-fm"},
		{Method: http.MethodGet, Path: "/api/v1/clusters/:name/stream", Handler: "pkg.(*H).Stream-fm"},
	}
	upgrading := func(h string) bool { return strings.Contains(h, "WebShell") || strings.Contains(h, "Stream") }
	require.Equal(t, []string{"GET /api/v1/clusters/:name/stream"}, InteractiveRouteGaps(routes, upgrading),
		"an upgrading route whose path is not classified is reported; a plain read route is not")
}
