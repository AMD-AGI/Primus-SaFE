/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

const (
	guardAudience = "safe-apiserver"
	// refusal is part of every message the external-mode guards answer with, which is how a
	// test tells them from an authorization 403 or any other refusal.
	refusal = "external identity provider"

	localName     = "local-user"
	localPassword = "local-account-password"
	// idpUserName is the name of a user the identity provider created. Such a user has no
	// password, which is what makes an empty password match it.
	idpUserName = "idp-user@example.com"
)

var (
	guardClusterOnce sync.Once
	guardCluster     client.Client
)

// guardTargets are the users the patch tests change, one per test so that they do not meet.
var guardTargets = []string{"t-grant", "t-revoke", "t-echo", "t-local", "t-local-rev", "t-workspaces", "t-builtin"}

func guardUser(name string, userType v1.UserType, roles ...v1.UserRole) *v1.User {
	return &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      map[string]string{v1.UserIdLabel: name},
			Annotations: map[string]string{v1.UserNameAnnotation: name},
		},
		Spec: v1.UserSpec{Type: userType, Roles: roles},
	}
}

// sharedGuardCluster returns the fake cluster of this file. The apiserver's local-account
// sign-in keeps one process-wide instance bound to the first client it is given, so every
// test here uses the same cluster and the targets it changes are told apart by name.
func sharedGuardCluster(t *testing.T) client.Client {
	t.Helper()
	guardClusterOnce.Do(func() {
		scheme := runtime.NewScheme()
		require.NoError(t, v1.AddToScheme(scheme))
		local := guardUser(commonuser.GenerateUserIdByName(localName), v1.DefaultUserType, v1.SystemAdminRole)
		local.Annotations[v1.UserNameAnnotation] = localName
		local.Spec.Password = stringutil.Base64Encode(localPassword)
		idp := guardUser(commonuser.GenerateUserIdByName(idpUserName), v1.SSOUserType, v1.DefaultRole)
		idp.Annotations[v1.UserNameAnnotation] = idpUserName
		objs := []client.Object{
			genMockRole(), local, idp,
			guardUser("idp-admin-id", v1.SSOUserType, v1.DefaultRole, v1.SystemAdminRole),
		}
		for _, name := range guardTargets {
			switch name {
			case "t-local":
				objs = append(objs, guardUser(name, v1.DefaultUserType, v1.DefaultRole))
			case "t-local-rev":
				objs = append(objs, guardUser(name, v1.DefaultUserType, v1.DefaultRole, v1.SystemAdminRole))
			case "t-revoke":
				objs = append(objs, guardUser(name, v1.SSOUserType, v1.DefaultRole, v1.SystemAdminRole))
			default:
				objs = append(objs, guardUser(name, v1.SSOUserType, v1.DefaultRole))
			}
		}
		guardCluster = fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
		require.NotNil(t, authority.NewDefaultToken(guardCluster))
		// Another test that got there first would leave the sign-in reading a different cluster.
		require.True(t, authority.DefaultTokenInstance().Client == guardCluster, "the local sign-in is bound to another cluster")
	})
	return guardCluster
}

// guardEngine serves the real route table in external mode. The authenticator chain is a
// fake RFC 7662 endpoint ("idp-admin" is the platform administrator the identity provider
// vouches for) and, when builtin is set, the built-in authenticator: the most permissive
// configuration.
func guardEngine(t *testing.T, builtin bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cli := sharedGuardCluster(t)

	rev := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		answer := map[string]any{"active": false}
		if r.PostForm.Get("token") == "idp-admin" {
			answer = map[string]any{"active": true, "sub": "idp-admin-id", "username": "idp-admin", "aud": guardAudience}
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(rev.Close)
	secretFile := filepath.Join(t.TempDir(), "client-secret")
	require.NoError(t, os.WriteFile(secretFile, []byte("secret"), 0o600))
	chain := []map[string]any{{
		"type": "introspection", "url": rev.URL, "client_id": "svc-safe",
		"client_secret_file": secretFile, "audience": guardAudience,
	}}
	if builtin {
		chain = append(chain, map[string]any{"type": "builtin"})
	}
	commonconfig.SetValue("crypto.enable", "false")
	viper.Set("auth.mode", "external")
	viper.Set("auth.authenticators", chain)
	require.NoError(t, authority.InitExternalAuth(cli))
	t.Cleanup(func() {
		commonconfig.SetValue("crypto.enable", "")
		viper.Set("auth.mode", nil)
		viper.Set("auth.authenticators", nil)
		require.NoError(t, authority.InitExternalAuth(nil))
	})

	e := gin.New()
	InitCustomRouters(e, &Handler{Client: cli, accessController: &authority.AccessController{Client: cli}})
	return e
}

type guardRequest struct {
	method, path string
	body         string
	contentType  string // JSON when empty
	bearer       string
}

func (r guardRequest) serve(e *gin.Engine) *httptest.ResponseRecorder {
	req := httptest.NewRequest(r.method, r.path, strings.NewReader(r.body))
	if r.contentType == "" {
		req.Header.Set("Content-Type", "application/json")
	} else {
		req.Header.Set("Content-Type", r.contentType)
	}
	if r.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+r.bearer)
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w
}

func loginBody(userType, name, password string) string {
	b, _ := json.Marshal(map[string]string{"type": userType, "name": name, "password": password})
	return string(b)
}

func storedRoles(t *testing.T, name string) []v1.UserRole {
	t.Helper()
	u := &v1.User{}
	require.NoError(t, guardCluster.Get(context.Background(), client.ObjectKey{Name: name}, u))
	return u.Spec.Roles
}

// With auth.mode external, every way of making a credential or an account that the
// identity provider owns is refused on the real route table, whoever asks, local accounts
// included, in the most permissive configuration: a chain that still contains the built-in
// authenticator.
func TestExternalModeRefusesCredentialAndRoleWrites(t *testing.T) {
	e := guardEngine(t, true)
	tests := []struct {
		name  string
		req   guardRequest
		after func(t *testing.T, w *httptest.ResponseRecorder)
	}{
		{
			name: "login with an SSO authorization code",
			req:  guardRequest{method: http.MethodPost, path: "/api/v1/login", body: `{"type":"sso","code":"c","name":"x"}`},
		},
		{
			name: "login as a local account with the right password (JSON)",
			req:  guardRequest{method: http.MethodPost, path: "/api/v1/login", body: loginBody("default", localName, localPassword)},
			after: func(t *testing.T, w *httptest.ResponseRecorder) {
				assert.NotContains(t, w.Body.String(), `"token"`, "no session may be issued")
			},
		},
		{
			name: "login as a local account with the right password (console form)",
			req: guardRequest{method: http.MethodPost, path: "/api/v1/login", contentType: ContentTypeForm,
				body: url.Values{"type": {"default"}, "name": {localName}, "password": {localPassword}}.Encode()},
			after: func(t *testing.T, w *httptest.ResponseRecorder) {
				assert.Empty(t, w.Header().Values("Set-Cookie"), "no cookie may be set")
			},
		},
		{
			name: "login as an identity-provider user with an empty password (JSON)",
			req:  guardRequest{method: http.MethodPost, path: "/api/v1/login", body: loginBody("default", idpUserName, "")},
			after: func(t *testing.T, w *httptest.ResponseRecorder) {
				assert.NotContains(t, w.Body.String(), `"token"`, "no session may be issued")
			},
		},
		{
			name: "login as an identity-provider user with an empty password (console form)",
			req: guardRequest{method: http.MethodPost, path: "/api/v1/login", contentType: ContentTypeForm,
				body: url.Values{"type": {"default"}, "name": {idpUserName}, "password": {""}}.Encode()},
			after: func(t *testing.T, w *httptest.ResponseRecorder) {
				assert.Empty(t, w.Header().Values("Set-Cookie"), "no cookie may be set")
			},
		},
		{
			name: "register an account",
			req:  guardRequest{method: http.MethodPost, path: "/api/v1/users", body: `{"name":"intruder","password":"pw"}`},
			after: func(t *testing.T, _ *httptest.ResponseRecorder) {
				err := guardCluster.Get(context.Background(),
					client.ObjectKey{Name: commonuser.GenerateUserIdByName("intruder")}, &v1.User{})
				assert.True(t, apierrors.IsNotFound(err), "the account must not exist, got %v", err)
			},
		},
		{
			name: "create an API key as an administrator",
			req:  guardRequest{method: http.MethodPost, path: "/api/v1/apikeys", body: `{"name":"k","ttlDays":7}`, bearer: "idp-admin"},
		},
		{
			name: "delete an API key as an administrator",
			req:  guardRequest{method: http.MethodDelete, path: "/api/v1/apikeys/1", bearer: "idp-admin"},
		},
		{
			name: "grant a platform role to an identity-provider user",
			req: guardRequest{method: http.MethodPatch, path: "/api/v1/users/t-grant",
				body: `{"roles":["default","system-admin"]}`, bearer: "idp-admin"},
			after: func(t *testing.T, _ *httptest.ResponseRecorder) {
				assert.Equal(t, []v1.UserRole{v1.DefaultRole}, storedRoles(t, "t-grant"))
			},
		},
		{
			name: "grant a platform role to a local account",
			req: guardRequest{method: http.MethodPatch, path: "/api/v1/users/t-local",
				body: `{"roles":["default","system-admin"]}`, bearer: "idp-admin"},
			after: func(t *testing.T, _ *httptest.ResponseRecorder) {
				assert.Equal(t, []v1.UserRole{v1.DefaultRole}, storedRoles(t, "t-local"))
			},
		},
		{
			name: "revoke a platform role of a local account",
			req: guardRequest{method: http.MethodPatch, path: "/api/v1/users/t-local-rev",
				body: `{"roles":["default"]}`, bearer: "idp-admin"},
			after: func(t *testing.T, _ *httptest.ResponseRecorder) {
				assert.Equal(t, []v1.UserRole{v1.DefaultRole, v1.SystemAdminRole}, storedRoles(t, "t-local-rev"))
			},
		},
		{
			name: "revoke a platform role of an identity-provider user",
			req: guardRequest{method: http.MethodPatch, path: "/api/v1/users/t-revoke",
				body: `{"roles":["default"]}`, bearer: "idp-admin"},
			after: func(t *testing.T, _ *httptest.ResponseRecorder) {
				assert.Equal(t, []v1.UserRole{v1.DefaultRole, v1.SystemAdminRole}, storedRoles(t, "t-revoke"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := tt.req.serve(e)
			require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), refusal, "refused by the external-mode guard, not by something else")
			if tt.after != nil {
				tt.after(t, w)
			}
		})
	}
}

// What stays available in external mode: a patch that leaves the platform roles alone,
// and role names that are not platform roles, are not role changes.
func TestExternalModeAllowsEditsThatLeavePlatformRolesAlone(t *testing.T) {
	e := guardEngine(t, true)

	t.Run("a patch that echoes the roles back goes through", func(t *testing.T) {
		w := guardRequest{method: http.MethodPatch, path: "/api/v1/users/t-echo",
			body: `{"roles":["default"],"avatarUrl":"https://example.com/a.png"}`, bearer: "idp-admin"}.serve(e)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		u := &v1.User{}
		require.NoError(t, guardCluster.Get(context.Background(), client.ObjectKey{Name: "t-echo"}, u))
		assert.Equal(t, "https://example.com/a.png", v1.GetUserAvatarUrl(u))
	})

	t.Run("workspace-admin is not a platform role", func(t *testing.T) {
		w := guardRequest{method: http.MethodPatch, path: "/api/v1/users/t-workspaces",
			body: `{"roles":["default","workspace-admin"]}`, bearer: "idp-admin"}.serve(e)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})
}

// The refusal does not depend on the shape of the chain: without the built-in authenticator
// in it, sign-in is refused all the same.
func TestExternalModeRefusesLoginWhateverTheChain(t *testing.T) {
	e := guardEngine(t, false)
	for _, body := range []string{loginBody("default", localName, localPassword), loginBody("sso", "x", "")} {
		w := guardRequest{method: http.MethodPost, path: "/api/v1/login", body: body}.serve(e)
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), refusal)
		assert.NotContains(t, w.Body.String(), `"token"`)
	}
}

// Builtin mode, the default, is untouched: registration, local sign-in, role edits and the
// API key handlers all behave as they always did.
func TestBuiltinModeKeepsAccountEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cli := sharedGuardCluster(t)
	commonconfig.SetValue("crypto.enable", "false")
	t.Cleanup(func() { commonconfig.SetValue("crypto.enable", "") })
	h := &Handler{Client: cli, accessController: &authority.AccessController{Client: cli}}
	e := gin.New()
	InitCustomRouters(e, h)

	t.Run("registration creates an account", func(t *testing.T) {
		w := guardRequest{method: http.MethodPost, path: "/api/v1/users", body: `{"name":"newcomer","password":"pw"}`}.serve(e)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.NoError(t, cli.Get(context.Background(),
			client.ObjectKey{Name: commonuser.GenerateUserIdByName("newcomer")}, &v1.User{}))
	})

	t.Run("a local account signs in", func(t *testing.T) {
		w := guardRequest{method: http.MethodPost, path: "/api/v1/login", body: loginBody("default", localName, localPassword)}.serve(e)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("an administrator grants a platform role", func(t *testing.T) {
		body, _ := json.Marshal(view.PatchUserRequest{Roles: &[]v1.UserRole{v1.DefaultRole, v1.SystemAdminRole}})
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(string(body)))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, commonuser.GenerateUserIdByName(localName))
		c.Set(common.Name, "t-builtin")
		h.PatchUser(c)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.ElementsMatch(t, []v1.UserRole{v1.DefaultRole, v1.SystemAdminRole}, storedRoles(t, "t-builtin"))
	})

	t.Run("the API key handlers reach their own logic", func(t *testing.T) {
		for name, handler := range map[string]gin.HandlerFunc{"create": h.CreateApiKey, "delete": h.DeleteApiKey} {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
			c.Set(common.UserId, "someone")
			handler(c)
			// There is no database in this test: that answer comes from the handler's own
			// first check, which a refusal in front of it would have pre-empted.
			assert.Equal(t, http.StatusInternalServerError, w.Code, name)
			assert.Contains(t, w.Body.String(), "database is not enabled", name)
		}
	})
}

// GET /envs tells the console which mode it is talking to.
func TestEnvsReportsAuthMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	get := func() view.GetEnvResponse {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/envs", nil)
		(&Handler{}).GetEnvs(c)
		require.Equal(t, http.StatusOK, w.Code)
		var resp view.GetEnvResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp
	}
	t.Cleanup(func() { viper.Set("auth.mode", nil) })

	assert.Equal(t, "builtin", get().AuthMode, "the default")
	viper.Set("auth.mode", "external")
	assert.Equal(t, "external", get().AuthMode)
	viper.Set("auth.mode", "externl")
	assert.Equal(t, "builtin", get().AuthMode, "a typo is builtin, as everywhere else")
}

// Which role changes count: only adding or removing a platform role of a user the identity
// provider governs.
func TestRefuseExternalRoleChange(t *testing.T) {
	roles := func(r ...v1.UserRole) *[]v1.UserRole { return &r }
	tests := []struct {
		name     string
		external bool
		target   *v1.User
		request  *[]v1.UserRole
		refused  bool
	}{
		{"builtin mode never refuses", false, guardUser("u", v1.SSOUserType, v1.DefaultRole), roles(v1.SystemAdminRole), false},
		{"no roles in the patch", true, guardUser("u", v1.SSOUserType, v1.DefaultRole), nil, false},
		{"grant system-admin", true, guardUser("u", v1.SSOUserType, v1.DefaultRole), roles(v1.DefaultRole, v1.SystemAdminRole), true},
		{"grant system-admin-readonly", true, guardUser("u", v1.SSOUserType, v1.DefaultRole), roles(v1.DefaultRole, v1.SystemAdminReadonlyRole), true},
		{"revoke system-admin", true, guardUser("u", v1.SSOUserType, v1.DefaultRole, v1.SystemAdminRole), roles(v1.DefaultRole), true},
		{"swap system-admin for readonly", true, guardUser("u", v1.SSOUserType, v1.SystemAdminRole), roles(v1.SystemAdminReadonlyRole), true},
		{"same roles in another order", true, guardUser("u", v1.SSOUserType, v1.DefaultRole, v1.SystemAdminRole), roles(v1.SystemAdminRole, v1.DefaultRole), false},
		{"only a non-platform role differs", true, guardUser("u", v1.SSOUserType, v1.DefaultRole), roles(v1.DefaultRole, v1.WorkspaceAdminRole), false},
		{"a local account is refused too", true, guardUser("u", v1.DefaultUserType, v1.DefaultRole), roles(v1.DefaultRole, v1.SystemAdminRole), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.external {
				viper.Set("auth.mode", "external")
				t.Cleanup(func() { viper.Set("auth.mode", nil) })
			}
			err := refuseExternalRoleChange(tt.target, &view.PatchUserRequest{Roles: tt.request})
			if tt.refused {
				require.Error(t, err)
				assert.Contains(t, err.Error(), refusal)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
