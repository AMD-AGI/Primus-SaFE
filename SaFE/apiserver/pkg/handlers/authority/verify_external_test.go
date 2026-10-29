/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// POST /auth/verify in external mode runs the configured chain, as ParseToken does, so the
// credential of an identity provider's user gets the roles its groups map to: the services that
// authorize through the endpoint can see an administrator there. Each case is one credential
// presented one way; wantRoles is nil when the request must be refused.
func TestVerifyRunsTheExternalChain(t *testing.T) {
	defer setupInternalAuth(t)()
	admin := []string{"platform-admins"}
	cases := []struct {
		name      string
		stored    []v1.UserRole // the roles on the user object before the request, if any
		prepare   func(e *extEnv) VerifyTokenRequest
		wantCode  int
		wantRoles []v1.UserRole
	}{
		{
			name: "introspected session cookie, administrator group",
			prepare: func(e *extEnv) VerifyTokenRequest {
				e.revUsers["session-1"] = map[string]any{"sub": extUser, "username": "alice", "email": "alice@example.com", "groups": admin}
				return VerifyTokenRequest{Cookie: "Token=session-1; UserType=sso"}
			},
			wantCode: http.StatusOK, wantRoles: []v1.UserRole{v1.SystemAdminRole},
		},
		{
			name: "oidc bearer token, administrator group",
			prepare: func(e *extEnv) VerifyTokenRequest {
				return VerifyTokenRequest{Authorization: "Bearer " + e.jwt(nil)}
			},
			wantCode: http.StatusOK, wantRoles: []v1.UserRole{v1.SystemAdminRole},
		},
		{
			name: "oidc bearer token, no administrator group",
			prepare: func(e *extEnv) VerifyTokenRequest {
				return VerifyTokenRequest{Authorization: "Bearer " + e.jwt(map[string]any{"groups": []string{"team-a"}})}
			},
			wantCode: http.StatusOK, wantRoles: []v1.UserRole{v1.DefaultRole},
		},
		{
			name: "introspected API key in the apiKey field, administrator group",
			prepare: func(e *extEnv) VerifyTokenRequest {
				e.revUsers["ak-ext-1"] = map[string]any{"sub": extUser, "username": "alice", "groups": admin}
				return VerifyTokenRequest{ApiKey: "ak-ext-1", ClientIP: "10.0.0.1"}
			},
			wantCode: http.StatusOK, wantRoles: []v1.UserRole{v1.SystemAdminRole},
		},
		{
			name: "introspected API key in the authorization field, administrator group",
			prepare: func(e *extEnv) VerifyTokenRequest {
				e.revUsers["ak-ext-2"] = map[string]any{"sub": extUser, "username": "alice", "groups": admin}
				return VerifyTokenRequest{Authorization: "Bearer ak-ext-2"}
			},
			wantCode: http.StatusOK, wantRoles: []v1.UserRole{v1.SystemAdminRole},
		},
		{
			name:   "a stored platform role does not count for an identity without the group",
			stored: []v1.UserRole{v1.SystemAdminRole},
			prepare: func(e *extEnv) VerifyTokenRequest {
				return VerifyTokenRequest{Authorization: "Bearer " + e.jwt(map[string]any{"groups": []string{}})}
			},
			wantCode: http.StatusOK, wantRoles: []v1.UserRole{v1.DefaultRole},
		},
		{
			name:   "other stored roles are kept",
			stored: []v1.UserRole{"data-scientist", v1.SystemAdminReadonlyRole},
			prepare: func(e *extEnv) VerifyTokenRequest {
				return VerifyTokenRequest{Authorization: "Bearer " + e.jwt(map[string]any{"groups": []string{}})}
			},
			wantCode: http.StatusOK, wantRoles: []v1.UserRole{v1.DefaultRole, "data-scientist"},
		},
		{
			name: "an introspection answer that is not active",
			prepare: func(e *extEnv) VerifyTokenRequest {
				return VerifyTokenRequest{ApiKey: "ak-revoked"}
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "an expired oidc token",
			prepare: func(e *extEnv) VerifyTokenRequest {
				return VerifyTokenRequest{Authorization: "Bearer " + e.jwt(map[string]any{"exp": time.Now().Add(-time.Hour).Unix()})}
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "the introspection endpoint is down for a credential it never accepted",
			prepare: func(e *extEnv) VerifyTokenRequest {
				e.revMode.Store("503")
				return VerifyTokenRequest{ApiKey: "ak-never-seen"}
			},
			wantCode: http.StatusServiceUnavailable,
		},
		{
			name: "a delegated credential never verifies",
			prepare: func(e *extEnv) VerifyTokenRequest {
				return VerifyTokenRequest{Authorization: "Bearer " + e.delegated("safe.api")}
			},
			wantCode: http.StatusForbidden,
		},
		{
			name: "an authorization header that is not a bearer token",
			prepare: func(e *extEnv) VerifyTokenRequest {
				return VerifyTokenRequest{Authorization: "Digest abc"}
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "no credential at all",
			prepare:  func(e *extEnv) VerifyTokenRequest { return VerifyTokenRequest{} },
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "a cookie without a token",
			prepare:  func(e *extEnv) VerifyTokenRequest { return VerifyTokenRequest{Cookie: "UserType=sso"} },
			wantCode: http.StatusBadRequest,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newExtEnv(t, adminMapping, false)
			if tc.stored != nil {
				e.storeUser(tc.stored...)
			}
			w := callVerifyToken(t, tc.prepare(e), testInternalToken)
			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			if tc.wantCode != http.StatusOK {
				list := &v1.UserList{}
				require.NoError(t, e.cli.List(context.Background(), list))
				require.Len(t, list.Items, 0, "a refused credential provisions nobody")
				return
			}
			var resp verifyResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, extUser, resp.Data.Id)
			require.Equal(t, UserTypeExternal, resp.Data.Type)
			require.ElementsMatch(t, tc.wantRoles, resp.Data.Roles)
			require.Equal(t, v1.SSOUserType, e.user(extUser).Spec.Type, "provisioned as a request provisions")
		})
	}
}

// The verdict of POST /auth/verify is the verdict of the chain, whatever the chain holds: for each
// credential, verify and ParseToken answer alike, with and without a built-in entry. It
// pins that verify is not a second chain that can drift from the first.
func TestVerifyAgreesWithParseToken(t *testing.T) {
	defer setupInternalAuth(t)()
	for _, withBuiltin := range []bool{false, true} {
		creds := map[string]func(e *extEnv) string{
			"oidc token":            func(e *extEnv) string { return e.jwt(nil) },
			"oidc token, expired":   func(e *extEnv) string { return e.jwt(map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}) },
			"oidc token, wrong aud": func(e *extEnv) string { return e.jwt(map[string]any{"aud": []string{"other"}}) },
			"delegated token":       func(e *extEnv) string { return e.delegated("safe.api") },
			"introspected, active":  func(e *extEnv) string { return "opaque-active" },
			"introspected, unknown": func(e *extEnv) string { return "opaque-unknown" },
			"built-in session":      func(e *extEnv) string { return generateTestToken(t, "local-admin", "admin") },
			"garbage":               func(e *extEnv) string { return "garbage-credential" },
			"introspected, down":    func(e *extEnv) string { e.revMode.Store("503"); return "opaque-down" },
		}
		for name, token := range creds {
			t.Run(map[bool]string{false: "no builtin entry", true: "builtin entry"}[withBuiltin]+"/"+name, func(t *testing.T) {
				e := newExtEnv(t, adminMapping, withBuiltin)
				defer setupDefaultToken(t)()
				e.revUsers["opaque-active"] = map[string]any{"sub": extUser, "username": "alice", "groups": []string{"platform-admins"}}
				tok := token(e)

				want := http.StatusOK
				if name == "delegated token" {
					// ParseToken judges the route it is asked for; verify has none, and never serves a delegated credential.
					want = http.StatusForbidden
				} else if _, err := extParse(t, http.MethodGet, "/api/v1/workloads", tok); err != nil {
					st, ok := err.(apierrors.APIStatus)
					require.True(t, ok, "%v", err)
					want = int(st.Status().Code)
				}
				w := callVerifyToken(t, VerifyTokenRequest{Authorization: "Bearer " + tok}, testInternalToken)
				require.Equal(t, want, w.Code, w.Body.String())
			})
		}
	}
}

// A user that exists only as a record is provisioned by verify exactly as by a request, and the
// record is not changed: roles are computed per call, never stored.
func TestVerifyProvisionsLikeARequestAndWritesNoRole(t *testing.T) {
	defer setupInternalAuth(t)()
	e := newExtEnv(t, adminMapping, false)
	w := callVerifyToken(t, VerifyTokenRequest{Authorization: "Bearer " + e.jwt(nil)}, testInternalToken)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	u := &v1.User{}
	require.NoError(t, e.cli.Get(context.Background(), client.ObjectKey{Name: extUser}, u))
	require.Equal(t, v1.SSOUserType, u.Spec.Type)
	require.Equal(t, "alice@example.com", v1.GetUserEmail(u))
	require.NotContains(t, u.Spec.Roles, v1.SystemAdminRole, "the role the groups grant is not written to the record")

	// Revoking the group takes effect on the next verify, although the record is the same.
	w = callVerifyToken(t, VerifyTokenRequest{Authorization: "Bearer " + e.jwt(map[string]any{"groups": []string{}, "jti": "jti-2"})}, testInternalToken)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp verifyResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, []v1.UserRole{v1.DefaultRole}, resp.Data.Roles)
}
