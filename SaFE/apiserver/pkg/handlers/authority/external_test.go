/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

const (
	extAudience     = "safe-apiserver"
	extUser         = "alice-id"
	extClientID     = "svc-safe"
	extClientSecret = "s3cret-for-the-introspection-endpoint"
)

// extEnv is a fake issuer (discovery + JWKS), a fake RFC 7662 introspection server and a
// fake cluster, wired to the external mode through configuration.
type extEnv struct {
	t        *testing.T
	issuer   *httptest.Server
	key      *ecdsa.PrivateKey
	reviewer *httptest.Server
	revMode  atomic.Value              // "ok" | "503"
	revUsers map[string]map[string]any // token -> the introspection answer for it
	cli      client.Client
}

func newExtEnv(t *testing.T, mapping []map[string]any, builtin bool) *extEnv {
	t.Helper()
	e := &extEnv{t: t, revUsers: map[string]map[string]any{}}
	e.revMode.Store("ok")
	var err error
	e.key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &e.key.PublicKey, KeyID: "k1", Algorithm: "ES256"}}})
	})
	e.issuer = httptest.NewServer(mux)
	t.Cleanup(e.issuer.Close)

	secretFile := filepath.Join(t.TempDir(), "client-secret")
	require.NoError(t, os.WriteFile(secretFile, []byte(extClientSecret+"\n"), 0o600))
	e.reviewer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if e.revMode.Load() == "503" {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		if id, secret, ok := r.BasicAuth(); !ok || id != extClientID || secret != extClientSecret {
			http.Error(w, "who are you", http.StatusUnauthorized)
			return
		}
		require.NoError(t, r.ParseForm())
		answer := map[string]any{"active": false}
		if user, ok := e.revUsers[r.PostForm.Get("token")]; ok {
			answer = map[string]any{"active": true}
			for k, v := range user {
				answer[k] = v
			}
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(e.reviewer.Close)

	chain := []map[string]any{
		{"type": "oidc", "issuer": e.issuer.URL, "audience": extAudience, "jwks_url": e.issuer.URL + "/jwks"},
		{"type": "introspection", "url": e.reviewer.URL, "client_id": extClientID, "client_secret_file": secretFile, "cache_ttl": "50ms", "stale_if_error": "30m"},
	}
	if builtin {
		chain = append(chain, map[string]any{"type": "builtin"})
	}
	viper.Set("auth.mode", "external")
	viper.Set("auth.authenticators", chain)
	viper.Set("auth.group_mappings", mapping)
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	e.cli = fake.NewClientBuilder().WithScheme(scheme).Build()
	require.NoError(t, InitExternalAuth(e.cli))
	t.Cleanup(func() {
		viper.Set("auth.mode", nil)
		viper.Set("auth.authenticators", nil)
		viper.Set("auth.group_mappings", nil)
		require.NoError(t, InitExternalAuth(nil))
	})
	return e
}

func (e *extEnv) jwt(override map[string]any) string {
	now := time.Now()
	claims := map[string]any{
		"iss": e.issuer.URL, "sub": extUser, "aud": []string{extAudience}, "exp": now.Add(5 * time.Minute).Unix(),
		"iat": now.Unix(), "jti": "jti-1", "preferred_username": "alice", "email": "alice@example.com",
		"groups": []string{"platform-admins"},
	}
	for k, v := range override {
		if v == nil {
			delete(claims, k)
		} else {
			claims[k] = v
		}
	}
	sg, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: e.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader(jose.HeaderKey("kid"), "k1"))
	require.NoError(e.t, err)
	raw, err := jwt.Signed(sg).Claims(claims).Serialize()
	require.NoError(e.t, err)
	return raw
}

func (e *extEnv) delegated(scope string) string {
	return e.jwt(map[string]any{"act": map[string]any{"sub": "svc-bot"}, "scope": scope})
}

func (e *extEnv) user(id string) *v1.User {
	u := &v1.User{}
	require.NoError(e.t, e.cli.Get(context.Background(), client.ObjectKey{Name: id}, u))
	return u
}

func extParse(t *testing.T, method, path, bearer string) (*gin.Context, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(method, path, nil)
	if bearer != "" {
		c.Request.Header.Set("Authorization", "Bearer "+bearer)
	}
	return c, ParseToken(c)
}

var adminMapping = []map[string]any{
	{"group": "platform-admins", "roles": []string{"system-admin"}},
	{"group": "team-a", "roles": []string{"workspace-admin"}, "workspaces": []string{"ws-a"}},
}

func TestBuiltinModeIsUnchanged(t *testing.T) {
	// The default: no configuration, no chain, ParseToken runs only the built-in path.
	viper.Set("auth.mode", nil)
	require.NoError(t, InitExternalAuth(nil))
	require.Nil(t, externalState.Load())

	e := &extEnv{t: t}
	var err error
	e.key, _ = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	e.issuer = srv
	// A well-formed JWT is just an unknown credential to the built-in path, as before.
	c, err := extParse(t, http.MethodGet, "/api/v1/workloads", e.jwt(nil))
	require.Error(t, err)
	require.Empty(t, c.GetString(common.UserId))

	// An unrecognised value of auth.mode is builtin, never "no authentication".
	viper.Set("auth.mode", "externl")
	t.Cleanup(func() { viper.Set("auth.mode", nil) })
	require.NoError(t, InitExternalAuth(nil))
	require.Nil(t, externalState.Load())

	// Built-in tokens work, as before.
	defer setupDefaultToken(t)()
	viper.Set("auth.mode", nil)
	require.NoError(t, InitExternalAuth(nil))
	c, err = extParse(t, http.MethodGet, "/api/v1/users/self", generateTestToken(t, "u1", "name1"))
	require.NoError(t, err)
	require.Equal(t, "u1", c.GetString(common.UserId))
}

func TestExternalModeRefusesAChainItCannotBuild(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	goodSecret := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(goodSecret, []byte("x"), 0o600))
	t.Cleanup(func() {
		viper.Set("auth.mode", nil)
		viper.Set("auth.authenticators", nil)
		require.NoError(t, InitExternalAuth(nil))
	})
	for name, chain := range map[string]any{
		"empty":                   nil,
		"unknown type":            []map[string]any{{"type": "ldap"}},
		"oidc no aud":             []map[string]any{{"type": "oidc", "issuer": "https://issuer.example.com"}},
		"introspection no url":    []map[string]any{{"type": "introspection", "client_id": "c", "client_secret_file": goodSecret}},
		"introspection no client": []map[string]any{{"type": "introspection", "url": "https://x.example.com"}},
		"unreadable secret file":  []map[string]any{{"type": "introspection", "url": "https://x.example.com", "client_id": "c", "client_secret_file": goodSecret + ".missing"}},
		"bad duration":            []map[string]any{{"type": "introspection", "url": "https://x.example.com", "client_id": "c", "client_secret_file": goodSecret, "cache_ttl": "soon"}},
		"the retired webhook":     []map[string]any{{"type": "webhook", "url": "https://x.example.com"}},
	} {
		t.Run(name, func(t *testing.T) {
			viper.Set("auth.mode", "external")
			viper.Set("auth.authenticators", chain)
			require.Error(t, InitExternalAuth(cli), "external mode must not silently start as something else")
		})
	}
}

func TestExternalJITCreatesAndRefreshesUser(t *testing.T) {
	e := newExtEnv(t, nil, false)
	c, err := extParse(t, http.MethodGet, "/api/v1/workloads", e.jwt(nil))
	require.NoError(t, err)
	require.Equal(t, extUser, c.GetString(common.UserId))
	require.Equal(t, "alice", c.GetString(common.UserName))
	require.Equal(t, UserTypeExternal, c.GetString(common.UserType))
	require.Empty(t, c.GetString(common.ActorClientId))

	u := e.user(extUser)
	require.Equal(t, v1.SSOUserType, u.Spec.Type)
	require.Equal(t, "alice", v1.GetUserName(u))
	require.Equal(t, "alice@example.com", v1.GetUserEmail(u))
	require.Empty(t, u.Spec.Roles, "roles are left to the admission webhook; nothing is written from groups")

	// The same user again: no second record, annotations follow the claims.
	_, err = extParse(t, http.MethodGet, "/api/v1/workloads", e.jwt(map[string]any{"email": "alice@new.example.com"}))
	require.NoError(t, err)
	require.Equal(t, "alice@new.example.com", v1.GetUserEmail(e.user(extUser)))
	list := &v1.UserList{}
	require.NoError(t, e.cli.List(context.Background(), list))
	require.Len(t, list.Items, 1)
}

func TestExternalRejectsAnIdThatIsNotAResourceName(t *testing.T) {
	e := newExtEnv(t, nil, false)
	_, err := extParse(t, http.MethodGet, "/api/v1/workloads", e.jwt(map[string]any{"sub": "Alice/../Admin"}))
	require.True(t, apierrors.IsUnauthorized(err), "%v", err)
	list := &v1.UserList{}
	require.NoError(t, e.cli.List(context.Background(), list))
	require.Empty(t, list.Items)
}

func TestExternalRejectsBadCredentials(t *testing.T) {
	e := newExtEnv(t, nil, false)
	for name, tok := range map[string]string{
		"wrong audience": e.jwt(map[string]any{"aud": []string{"another-service"}}),
		"expired":        e.jwt(map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}),
		"unknown opaque": "ak-nobody",
		"none":           "",
	} {
		t.Run(name, func(t *testing.T) {
			c, err := extParse(t, http.MethodGet, "/api/v1/workloads", tok)
			require.True(t, apierrors.IsUnauthorized(err), "%v", err)
			require.Empty(t, c.GetString(common.UserId))
		})
	}
	list := &v1.UserList{}
	require.NoError(t, e.cli.List(context.Background(), list))
	require.Empty(t, list.Items, "a refused credential creates no user")
}

// Groups become roles when the request is authorized, and are never stored.
func TestExternalGroupsMapToRolesPerRequest(t *testing.T) {
	e := newExtEnv(t, adminMapping, false)
	ac := &AccessController{Client: e.cli}

	check := func(groups []string) error {
		c, err := extParse(t, http.MethodGet, "/api/v1/workloads", e.jwt(map[string]any{"groups": groups}))
		require.NoError(t, err)
		return ac.AuthorizeSystemAdmin(AccessInput{Context: c.Request.Context(), UserId: c.GetString(common.UserId)}, false)
	}
	require.NoError(t, check([]string{"platform-admins"}))
	require.Error(t, check([]string{"team-a"}), "a group that maps to no admin role gives none")
	require.NoError(t, check([]string{"platform-admins", "team-a"}))
	require.Error(t, check(nil), "the next request without the group has no admin rights: nothing was remembered")

	u := e.user(extUser)
	require.Empty(t, u.Spec.Roles, "the record never gained a role")
	require.Empty(t, u.Spec.Resources)

	// GetRoles sees the mapped roles too.
	require.NoError(t, e.cli.Create(context.Background(), &v1.Role{ObjectMeta: roleMeta(string(v1.SystemAdminRole))}))
	c, err := extParse(t, http.MethodGet, "/api/v1/workloads", e.jwt(nil))
	require.NoError(t, err)
	roles := ac.GetRoles(c.Request.Context(), e.user(extUser))
	require.Len(t, roles, 1)
	require.Equal(t, string(v1.SystemAdminRole), roles[0].Name)
}

func TestExternalFrozenByTheAuthenticator(t *testing.T) {
	e := newExtEnv(t, adminMapping, false)
	e.revUsers["tok"] = map[string]any{"sub": "frozen-id", "username": "f", "restricted": true}
	c, err := extParse(t, http.MethodGet, "/api/v1/workloads", "tok")
	require.NoError(t, err)
	ac := &AccessController{Client: e.cli}
	err = ac.Authorize(AccessInput{Context: c.Request.Context(), UserId: "frozen-id", ResourceKind: "workload", Verb: v1.GetVerb})
	require.Error(t, err)
	require.False(t, e.user("frozen-id").IsRestricted(), "frozen for this request only; the record is the user's to keep")
}

// Delegated credentials may not reach credentials, users or secrets, through the real
// authentication entry point.
func TestExternalDelegatedTokenExclusions(t *testing.T) {
	e := newExtEnv(t, adminMapping, false)
	tok := e.delegated("safe.api")
	for _, tt := range []struct{ name, method, path string }{
		{"create api key", http.MethodPost, "/api/v1/apikeys"},
		{"delete api key", http.MethodDelete, "/api/v1/apikeys/1"},
		{"read secret", http.MethodGet, "/api/v1/secrets/s1"},
		{"create secret", http.MethodPost, "/api/v1/secrets"},
		{"list users", http.MethodGet, "/api/v1/users"},
		{"read another user", http.MethodGet, "/api/v1/users/someone-else"},
		{"read another user's settings", http.MethodGet, "/api/v1/users/someone-else/settings"},
		{"change a user's roles", http.MethodPatch, "/api/v1/users/u1"},
		{"add an ssh key", http.MethodPost, "/api/v1/publickeys"},
		{"reveal the gateway key", http.MethodGet, "/api/v1/llm-gateway/binding/reveal"},
		{"platform key", http.MethodPost, "/api/v1/auth/verify"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := extParse(t, tt.method, tt.path, tok)
			require.True(t, apierrors.IsForbidden(err), "%v", err)
			require.Contains(t, err.Error(), "not available with a delegated token")
			require.Empty(t, c.GetString(common.UserId), "no identity is established for a refused request")
		})
	}
	list := &v1.UserList{}
	require.NoError(t, e.cli.List(context.Background(), list))
	require.Empty(t, list.Items, "a refused request provisions nothing")

	// The same token works for the user's ordinary access, and the actor is recorded.
	c, err := extParse(t, http.MethodGet, "/api/v1/users/self", tok)
	require.NoError(t, err)
	require.Equal(t, extUser, c.GetString(common.UserId))
	require.Equal(t, UserTypeDelegated, c.GetString(common.UserType))
	require.Equal(t, "svc-bot", c.GetString(common.ActorClientId))
	require.Equal(t, "jti-1", c.GetString(common.TokenJti))
	require.True(t, IsDelegatedRequest(c))

	// The same user without delegation is not subject to the exclusions.
	_, err = extParse(t, http.MethodPost, "/api/v1/apikeys", e.jwt(nil))
	require.NoError(t, err)
}

func TestExternalDelegatedScope(t *testing.T) {
	e := newExtEnv(t, nil, false)
	read := e.delegated("safe.read")
	_, err := extParse(t, http.MethodGet, "/api/v1/workloads", read)
	require.NoError(t, err)
	_, err = extParse(t, http.MethodPost, "/api/v1/workloads", read)
	require.True(t, apierrors.IsForbidden(err), "%v", err)
	_, err = extParse(t, http.MethodGet, "/api/v1/workloads", e.delegated("profile"))
	require.True(t, apierrors.IsForbidden(err), "no recognised scope allows nothing: %v", err)
}

func TestExternalIntrospectionCookieAndOutage(t *testing.T) {
	e := newExtEnv(t, nil, false)
	e.revUsers["session"] = map[string]any{"sub": "bob-id", "username": "bob", "groups": []string{"g"}}
	gin.SetMode(gin.TestMode)
	call := func() (*gin.Context, error) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/workloads", nil)
		c.Request.AddCookie(&http.Cookie{Name: CookieToken, Value: "session"})
		return c, ParseToken(c)
	}
	c, err := call()
	require.NoError(t, err)
	require.Equal(t, "bob-id", c.GetString(common.UserId))

	// The authenticator goes down after the cache entry has aged past its TTL: the
	// credential seen a moment ago keeps working (stale_if_error)...
	time.Sleep(80 * time.Millisecond)
	e.revMode.Store("503")
	c, err = call()
	require.NoError(t, err)
	require.Equal(t, "bob-id", c.GetString(common.UserId))

	// ...but one never seen gets 503: not a pass, and not a 401 either.
	_, err = extParse(t, http.MethodGet, "/api/v1/workloads", "ak-never-seen")
	require.True(t, apierrors.IsServiceUnavailable(err), "%v", err)
	require.EqualValues(t, http.StatusServiceUnavailable, err.(apierrors.APIStatus).Status().Code)
}

func TestExternalFallsThroughToBuiltin(t *testing.T) {
	e := newExtEnv(t, nil, true)
	defer setupDefaultToken(t)()
	// The introspection endpoint does not know this credential; the built-in mode at the end of the chain does.
	c, err := extParse(t, http.MethodGet, "/api/v1/users/self", generateTestToken(t, "local-admin", "admin"))
	require.NoError(t, err)
	require.Equal(t, "local-admin", c.GetString(common.UserId))
	list := &v1.UserList{}
	require.NoError(t, e.cli.List(context.Background(), list))
	require.Empty(t, list.Items, "the built-in path provisions nothing")

	// Through an outage of the introspection endpoint too: a local administrator still gets in.
	e.revMode.Store("503")
	c, err = extParse(t, http.MethodGet, "/api/v1/users/self", generateTestToken(t, "local-admin2", "admin"))
	require.NoError(t, err)
	require.Equal(t, "local-admin2", c.GetString(common.UserId))

	// And an unknown credential is still refused.
	_, err = extParse(t, http.MethodGet, "/api/v1/workloads", "garbage-credential")
	require.Error(t, err)
}

func roleMeta(name string) metav1.ObjectMeta { return metav1.ObjectMeta{Name: name} }
