/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority/authtest"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

// rawNoAccess returns what the response carries under "noAccess" as JSON; it fails when the field is absent,
// because a service reads it without a nil check: it is always there, false for a user who holds access.
func rawNoAccess(t *testing.T, body []byte) string {
	t.Helper()
	var resp struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	v, present := resp.Data["noAccess"]
	require.True(t, present, "the response has no noAccess field: %s", body)
	return string(v)
}

// POST /auth/verify tells a service whether the user holds no access, by the rule every route of the apiserver
// applies (AuthenticateRequest), so a service that trusts it does not serve a user the apiserver refuses
// everywhere else. It is checked against that gate, not against a list of its own: the two must give the
// same answer for the same user whatever the credential.
func TestVerifyReportsWhetherTheUserHoldsNoAccess(t *testing.T) {
	defer setupInternalAuth(t)()

	selfRead := accessUser("self-read-user", v1.SelfReadRole)
	plain := accessUser("plain-user", v1.DefaultRole)
	both := accessUser("both-user", v1.SelfReadRole, v1.DefaultRole)
	admin := accessUser("admin-user", v1.SystemAdminRole)
	readonly := accessUser("readonly-user", v1.SystemAdminReadonlyRole)
	readonlyAndSelfRead := accessUser("readonly-self-read-user", v1.SystemAdminReadonlyRole, v1.SelfReadRole)
	norole := accessUser("no-role-user")
	ghostRole := accessUser("ghost-role-user", "a-role-nobody-installed")
	manager := accessUser("manager-user", v1.SelfReadRole)
	commonuser.AddManagedWorkspace(manager, "ws-a")
	env := newAccessEnv(t, selfRead, plain, both, admin, readonly, readonlyAndSelfRead, norole, ghostRole, manager)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	for _, tt := range []struct {
		user *v1.User
		want string
	}{
		{selfRead, "true"},
		{plain, "false"},
		{both, "false"},
		{admin, "false"},
		{readonly, "false"},
		{readonlyAndSelfRead, "false"},
		{norole, "true"},
		{ghostRole, "true"},
		{manager, "false"},
	} {
		t.Run(tt.user.Name, func(t *testing.T) {
			token := generateTestToken(t, tt.user.Name, tt.user.Name)
			testKey := syntheticAPIKey("noaccess-" + tt.user.Name)
			mockDB := mock_client.NewMockInterface(ctrl)
			mockDB.EXPECT().GetApiKeyByKey(gomock.Any(), HashApiKey(testKey, nil)).Return(&dbclient.ApiKey{
				Id: 7, UserId: tt.user.Name, UserName: tt.user.Name, Whitelist: "[]",
				ExpirationTime: pq.NullTime{Time: time.Now().UTC().Add(time.Hour), Valid: true},
			}, nil).AnyTimes()
			oldKey := apiKeyTokenInstance
			apiKeyTokenInstance = &ApiKeyToken{dbClient: mockDB, Client: env.cli}
			defer func() { apiKeyTokenInstance = oldKey }()

			for name, req := range map[string]VerifyTokenRequest{
				"cookie":  {Cookie: "Token=" + token},
				"bearer":  {Authorization: "Bearer " + token},
				"api key": verifyRequestWithAuthKey(VerifyTokenRequest{ClientIP: "10.0.0.1"}, testKey),
			} {
				w := callVerifyToken(t, req, testInternalToken)
				require.Equal(t, http.StatusOK, w.Code, "%s: %s", name, w.Body.String())
				require.Equal(t, tt.want, rawNoAccess(t, w.Body.Bytes()), name)
			}

			// The gate refuses on a route that is not one of the own-record routes exactly when verify says so.
			gate := env.call(tt.user, http.MethodGet, "/api/v1/clusters")
			require.Equal(t, tt.want == "true", gate.Code == http.StatusForbidden && containsNoAccessCode(gate), gate.Body.String())
		})
	}

	t.Run("a user whose record cannot be read is reported as holding none", func(t *testing.T) {
		token := generateTestToken(t, "nobody-knows-me", "nobody")
		w := callVerifyToken(t, VerifyTokenRequest{Cookie: "Token=" + token}, testInternalToken)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Equal(t, "true", rawNoAccess(t, w.Body.Bytes()))
	})

	t.Run("system administrators hold access when the platform Roles are missing", func(t *testing.T) {
		for _, name := range []v1.UserRole{v1.SystemAdminRole, v1.SystemAdminReadonlyRole} {
			require.NoError(t, env.cli.Delete(t.Context(), &v1.Role{ObjectMeta: metav1.ObjectMeta{Name: string(name)}}))
		}
		for _, u := range []*v1.User{admin, readonly} {
			token := generateTestToken(t, u.Name, u.Name)
			w := callVerifyToken(t, VerifyTokenRequest{Cookie: "Token=" + token}, testInternalToken)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.Equal(t, "false", rawNoAccess(t, w.Body.Bytes()), u.Name)
		}
		// An ordinary user whose Role is missing is judged as ever.
		require.NoError(t, env.cli.Delete(t.Context(), &v1.Role{ObjectMeta: metav1.ObjectMeta{Name: string(v1.DefaultRole)}}))
		w := callVerifyToken(t, VerifyTokenRequest{Cookie: "Token=" + generateTestToken(t, plain.Name, plain.Name)}, testInternalToken)
		require.Equal(t, "true", rawNoAccess(t, w.Body.Bytes()))
	})
}

func containsNoAccessCode(w *httptest.ResponseRecorder) bool {
	var body struct {
		ErrorCode string `json:"errorCode"`
	}
	return json.Unmarshal(w.Body.Bytes(), &body) == nil && body.ErrorCode == commonerrors.NoAccess
}

// In external mode the user is judged as the request sees it: the base roles and the roles the identity's
// groups map to are applied, nothing is stored. Verify and the gate agree for the same credential, for a new
// identity of a minimal deployment (no access), of the default one (access), and for an administrator the
// provider vouches for, who holds access whatever the base roles are and whether or not the platform Roles
// can be read.
func TestVerifyAgreesWithTheGateInExternalMode(t *testing.T) {
	defer setupInternalAuth(t)()
	for _, tt := range []struct {
		name         string
		base         any
		groups       []string
		withoutRoles bool
		noAccess     bool
	}{
		{"the default base role", nil, []string{"team-a"}, false, false},
		{"the minimal base role", []string{"self-read"}, []string{"some-group"}, false, true},
		{"the minimal base role and no groups", []string{"self-read"}, nil, false, true},
		{"the minimal and the default base role", []string{"self-read", "default"}, []string{"some-group"}, false, false},
		{"managing a workspace through a group is access", []string{"self-read"}, []string{"team-a"}, false, false},
		{"an administrator under the minimal base role", []string{"self-read"}, []string{"platform-admins"}, false, false},
		{"an administrator when the Role objects are missing", []string{"self-read"}, []string{"platform-admins"}, true, false},
		{"a new identity when the Role objects are missing", []string{"self-read"}, []string{"some-group"}, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			viper.Set("users.base_roles", tt.base)
			t.Cleanup(func() { viper.Set("users.base_roles", nil) })
			e := newExtEnv(t, adminMapping, false)
			if !tt.withoutRoles {
				for _, r := range authtest.ChartRoles(t) {
					require.NoError(t, e.cli.Create(t.Context(), r))
				}
			}
			bearer := e.jwt(map[string]any{"groups": tt.groups})

			w := callVerifyToken(t, VerifyTokenRequest{Authorization: "Bearer " + bearer}, testInternalToken)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			want := "false"
			if tt.noAccess {
				want = "true"
			}
			require.Equal(t, want, rawNoAccess(t, w.Body.Bytes()))

			gin.SetMode(gin.TestMode)
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				if err := AuthenticateRequest(c); err != nil {
					abortAs(c, err)
					return
				}
				c.Next()
			})
			engine.GET("/api/v1/users", func(c *gin.Context) { c.String(http.StatusOK, "reached") })
			req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
			req.Header.Set("Authorization", "Bearer "+bearer)
			gate := httptest.NewRecorder()
			engine.ServeHTTP(gate, req)
			if tt.noAccess {
				require.Equal(t, http.StatusForbidden, gate.Code, gate.Body.String())
				require.True(t, containsNoAccessCode(gate), gate.Body.String())
			} else {
				require.Equal(t, http.StatusOK, gate.Code, gate.Body.String())
			}
		})
	}
}

// A built-in credential (a session a local account already holds) in external mode is an ordinary user,
// whatever the record stores: no identity provider vouched for it, so a stored platform role does not count,
// for verify as for the gate. Under the minimal base role such an account holds no access, the system-admin
// its record lists notwithstanding.
func TestVerifyAgreesWithTheGateForABuiltinCredentialInExternalMode(t *testing.T) {
	defer setupInternalAuth(t)()
	viper.Set("users.base_roles", []string{"self-read"})
	t.Cleanup(func() { viper.Set("users.base_roles", nil) })
	e := newExtEnv(t, adminMapping, true)
	defer setupDefaultToken(t)()
	for _, r := range authtest.ChartRoles(t) {
		require.NoError(t, e.cli.Create(t.Context(), r))
	}
	e.storeUser(v1.SystemAdminRole)
	token := generateTestToken(t, extUser, "alice")

	w := callVerifyToken(t, VerifyTokenRequest{Cookie: "Token=" + token}, testInternalToken)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.JSONEq(t, `["self-read"]`, string(rawRoles(t, w.Body.Bytes())), "the stored platform role does not count")
	require.Equal(t, "true", rawNoAccess(t, w.Body.Bytes()))

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		if err := AuthenticateRequest(c); err != nil {
			abortAs(c, err)
			return
		}
		c.Next()
	})
	engine.GET("/api/v1/users", func(c *gin.Context) { c.String(http.StatusOK, "reached") })
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	gate := httptest.NewRecorder()
	engine.ServeHTTP(gate, req)
	require.Equal(t, http.StatusForbidden, gate.Code, gate.Body.String())
	require.True(t, containsNoAccessCode(gate), gate.Body.String())
}

// A delegated token of a key actor (auth.key_actors) is verified with the identity it acts for, so what the
// answer says about access is the subject's, judged with the roles the subject's groups map to, as the gate
// judges a request that carries the same token. The type of the answer is still "delegated".
func TestVerifyOfAKeyActorReportsNoAccessAsTheGateDoes(t *testing.T) {
	defer setupInternalAuth(t)()
	for _, tt := range []struct {
		name     string
		groups   []string
		noAccess bool
	}{
		{"a subject who holds no access under the minimal base role", []string{"some-group"}, true},
		{"a subject who is an administrator by group", []string{"platform-admins"}, false},
		{"a subject who manages a workspace by group", []string{"team-a"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			viper.Set("users.base_roles", []string{"self-read"})
			t.Cleanup(func() { viper.Set("users.base_roles", nil) })
			withKeyActors(t, keyActorClient)
			e := newExtEnv(t, adminMapping, false)
			for _, r := range authtest.ChartRoles(t) {
				require.NoError(t, e.cli.Create(t.Context(), r))
			}
			bearer := e.jwt(map[string]any{
				"act": map[string]any{"sub": keyActorClient}, "client_id": keyActorClient,
				"scope": "safe.api", "groups": tt.groups,
			})

			w := callVerifyToken(t, VerifyTokenRequest{Authorization: "Bearer " + bearer}, testInternalToken)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var resp struct {
				Data struct {
					Type string `json:"type"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, UserTypeDelegated, resp.Data.Type)
			want := "false"
			if tt.noAccess {
				want = "true"
			}
			require.Equal(t, want, rawNoAccess(t, w.Body.Bytes()))

			gin.SetMode(gin.TestMode)
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				if err := AuthenticateRequest(c); err != nil {
					abortAs(c, err)
					return
				}
				c.Next()
			})
			engine.GET("/api/v1/clusters", func(c *gin.Context) { c.String(http.StatusOK, "reached") })
			req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
			req.Header.Set("Authorization", "Bearer "+bearer)
			gate := httptest.NewRecorder()
			engine.ServeHTTP(gate, req)
			if tt.noAccess {
				require.Equal(t, http.StatusForbidden, gate.Code, gate.Body.String())
				require.True(t, containsNoAccessCode(gate), gate.Body.String())
			} else {
				require.Equal(t, http.StatusOK, gate.Code, gate.Body.String())
			}
		})
	}
}
