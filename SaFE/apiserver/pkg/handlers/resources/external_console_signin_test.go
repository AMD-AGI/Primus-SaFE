/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// The console's sign-in in external mode, end to end over HTTP: a browser (a cookie jar) that the identity
// service has signed in, the identity service (its authorization code flow for the console's public client,
// and the RFC 7662 introspection of its own sessions), and this apiserver's real login and users/self routes.

const (
	consoleClient   = "safe-console"
	consoleCallback = "https://console.example.com/sso/callback"
	consoleVerifier = "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJ"
)

// fakeIdentity is the identity service as the apiserver meets it. Its sessions are opaque ids; its codes are
// bound to a user and a PKCE challenge; its ID tokens are for the console client.
type fakeIdentity struct {
	t   *testing.T
	srv *httptest.Server
	key *ecdsa.PrivateKey

	mu       sync.Mutex
	sessions map[string]string    // session id -> user id
	codes    map[string][2]string // code -> user id, S256 challenge
	exp      time.Time
}

func newFakeIdentity(t *testing.T) *fakeIdentity {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	f := &fakeIdentity{t: t, key: k, sessions: map[string]string{}, codes: map[string][2]string{},
		exp: time.Now().Add(12 * time.Hour).Truncate(time.Second)}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": f.srv.URL, "authorization_endpoint": f.srv.URL + "/oauth/authorize",
			"token_endpoint": f.srv.URL + "/oauth/token", "jwks_uri": f.srv.URL + "/.well-known/jwks.json",
			"introspection_endpoint": f.srv.URL + "/oauth/introspect", "id_token_signing_alg_values_supported": []string{"ES256"},
		})
	})
	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &f.key.PublicKey, KeyID: "k1", Algorithm: "ES256", Use: "sig"}}})
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		f.mu.Lock()
		grant, ok := f.codes[r.PostForm.Get("code")]
		delete(f.codes, r.PostForm.Get("code"))
		f.mu.Unlock()
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if !ok || r.PostForm.Get("client_id") != consoleClient || r.PostForm.Get("redirect_uri") != consoleCallback ||
			base64.RawURLEncoding.EncodeToString(sum[:]) != grant[1] {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 300,
			"id_token": f.idToken(grant[0])})
	})
	mux.HandleFunc("/oauth/introspect", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		if id, secret, ok := r.BasicAuth(); !ok || id != "svc-safe" || secret != "secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		sub, ok := f.sessions[r.PostForm.Get("token")]
		f.mu.Unlock()
		answer := map[string]any{"active": false}
		if ok {
			// As the identity service answers for a session: no audience, its user, its end.
			answer = map[string]any{"active": true, "iss": f.srv.URL, "sub": sub, "username": sub + "@example.com",
				"email": sub + "@example.com", "user_type": "sso", "token_type": "session", "exp": f.exp.Unix()}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(answer)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// signIn is the identity service's own sign-in: a new session for the user, whose id the browser gets in its cookie.
func (f *fakeIdentity) signIn(userID string) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	sid := base64.RawURLEncoding.EncodeToString(b)
	f.mu.Lock()
	f.sessions[sid] = userID
	f.mu.Unlock()
	return sid
}

// authorize is the authorization endpoint, after it accepted the browser's session of userID: a code for the console.
func (f *fakeIdentity) authorize(userID string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	code := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(consoleVerifier))
	f.mu.Lock()
	f.codes[code] = [2]string{userID, base64.RawURLEncoding.EncodeToString(sum[:])}
	f.mu.Unlock()
	return code
}

// idToken is the console client's ID token for the user: a JWT that lives an hour.
func (f *fakeIdentity) idToken(userID string) string {
	now := time.Now()
	sg, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: f.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader(jose.HeaderKey("kid"), "k1"))
	require.NoError(f.t, err)
	raw, err := jwt.Signed(sg).Claims(map[string]any{"iss": f.srv.URL, "sub": userID, "aud": consoleClient,
		"exp": now.Add(time.Hour).Unix(), "iat": now.Unix(), "email": userID + "@example.com", "name": userID}).Serialize()
	require.NoError(f.t, err)
	return raw
}

// consoleSignInEngine is this apiserver in external mode with the chain the identity service is deployed
// with, and sso.login at the fake identity service, served over HTTP.
func consoleSignInEngine(t *testing.T, f *fakeIdentity) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cli := sharedGuardCluster(t)
	set := func(k, v string) {
		commonconfig.SetValue(k, v)
		t.Cleanup(func() { commonconfig.SetValue(k, "") })
	}
	set("db.enable", "false")
	set("crypto.enable", "false")
	set("sso.secret_path", t.TempDir())
	set("sso.login.enable", "true")
	set("sso.login.issuer", f.srv.URL)
	set("sso.login.client_id", consoleClient)
	set("sso.login.redirect_uri", consoleCallback)
	secretFile := filepath.Join(t.TempDir(), "client-secret")
	require.NoError(t, os.WriteFile(secretFile, []byte("secret"), 0o600))
	viper.Set("auth.mode", "external")
	viper.Set("auth.authenticators", []map[string]any{
		{"type": "oidc", "name": "primus-identity", "issuer": f.srv.URL, "audience": "primus-safe", "jwks_url": f.srv.URL + "/.well-known/jwks.json"},
		{"type": "introspection", "name": "primus-identity-introspect", "url": f.srv.URL + "/oauth/introspect", "issuer": f.srv.URL,
			"audience": "primus-safe", "allow_missing_audience": true, "client_id": "svc-safe", "client_secret_file": secretFile},
	})
	viper.Set("auth.group_mappings", []map[string]any{{"group": "platform-admins", "roles": []string{"system-admin"}}})
	t.Cleanup(func() {
		viper.Set("auth.mode", "external")
		// Authentication stays external, so the process-wide chain keeps a builtin entry.
		viper.Set("auth.authenticators", []map[string]any{{"type": "builtin"}})
		viper.Set("auth.group_mappings", nil)
		require.NoError(t, authority.InitExternalAuth(nil))
	})
	require.NoError(t, authority.InitExternalAuth(cli))
	inst, err := authority.NewSSOTokenFromConfig(cli)
	require.NoError(t, err)
	prev := ssoTokenInstance
	ssoTokenInstance = func() authority.TokenInterface { return inst }
	t.Cleanup(func() { ssoTokenInstance = prev })

	e := gin.New()
	InitCustomRouters(e, &Handler{Client: cli, accessController: &authority.AccessController{Client: cli}})
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	return srv
}

// browser is a cookie jar on the console's host, holding what the identity service's /sign-in callback set.
type browser struct {
	t    *testing.T
	base *url.URL
	hc   *http.Client
}

func newBrowser(t *testing.T, srv *httptest.Server) *browser {
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	base, err := url.Parse(srv.URL)
	require.NoError(t, err)
	return &browser{t: t, base: base, hc: &http.Client{Jar: jar, Timeout: 10 * time.Second}}
}

// holdSession puts the cookies the identity service writes at sign-in: Token, userId and userType, Path /.
func (b *browser) holdSession(token, userID string) {
	b.hc.Jar.SetCookies(b.base, []*http.Cookie{
		{Name: authority.CookieToken, Value: token, Path: "/", HttpOnly: true},
		{Name: "userId", Value: userID, Path: "/", HttpOnly: true},
		{Name: "userType", Value: "sso", Path: "/", HttpOnly: true},
	})
}

func (b *browser) cookie(name string) string {
	for _, c := range b.hc.Jar.Cookies(b.base) {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

// login is the console's callback: the code and the verifier, posted as a form with the browser's cookies.
func (b *browser) login(code string) (*http.Response, map[string]any) {
	form := url.Values{"type": {"sso"}, "code": {code}, "code_verifier": {consoleVerifier}}
	resp, err := b.hc.Post(b.base.String()+"/api/v1/login", ContentTypeForm, strings.NewReader(form.Encode()))
	require.NoError(b.t, err)
	return resp, decodeObject(b.t, resp)
}

func (b *browser) self() (*http.Response, map[string]any) {
	resp, err := b.hc.Get(b.base.String() + "/api/v1/users/self")
	require.NoError(b.t, err)
	return resp, decodeObject(b.t, resp)
}

func decodeObject(t *testing.T, resp *http.Response) map[string]any {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	m := map[string]any{}
	_ = json.Unmarshal(body, &m)
	return m
}

// admitted is the user record the admission webhook leaves for a new user: the default role, which lets
// the user read itself. The fake cluster has no webhook.
func admitted(t *testing.T, name string) {
	require.NoError(t, sharedGuardCluster(t).Create(context.Background(), guardUser(name, v1.SSOUserType, v1.DefaultRole)))
}

func isJWT(v string) bool { return len(strings.Split(v, ".")) == 3 }

// After the console's sign-in the browser still holds the identity service's session: the login response
// sets no cookie and carries no token, and that session is what the apiserver accepts afterwards.
func TestExternalConsoleSignInKeepsTheIdentitySession(t *testing.T) {
	f := newFakeIdentity(t)
	srv := consoleSignInEngine(t, f)
	admitted(t, "u-alice")
	b := newBrowser(t, srv)
	sid := f.signIn("u-alice")
	b.holdSession(sid, "u-alice")

	resp, body := b.login(f.authorize("u-alice"))
	require.Equal(t, http.StatusOK, resp.StatusCode, body)
	assert.Empty(t, resp.Header.Values("Set-Cookie"), "the login must not replace the identity service's cookies")
	assert.Equal(t, "u-alice", body["id"])
	assert.Empty(t, body["token"], "the login hands out no credential")
	assert.EqualValues(t, f.exp.Unix(), body["expire"], "the session's end, not the ID token's hour")

	assert.Equal(t, sid, b.cookie(authority.CookieToken), "the browser's Token is the identity service's session")
	assert.False(t, isJWT(b.cookie(authority.CookieToken)))
	assert.Equal(t, "sso", b.cookie("userType"))

	resp, body = b.self()
	require.Equal(t, http.StatusOK, resp.StatusCode, body)
	assert.Equal(t, "u-alice", body["id"])
}

// The code is honoured only for the browser whose session belongs to the user the code was issued to.
func TestExternalConsoleSignInNeedsTheBrowsersOwnSession(t *testing.T) {
	f := newFakeIdentity(t)
	srv := consoleSignInEngine(t, f)

	// Another user's session.
	b := newBrowser(t, srv)
	b.holdSession(f.signIn("u-bob"), "u-bob")
	resp, body := b.login(f.authorize("u-carol"))
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, body)
	assert.Contains(t, body["errorMessage"], "does not match")
	assert.Empty(t, resp.Header.Values("Set-Cookie"))

	// No session at all.
	b = newBrowser(t, srv)
	resp, body = b.login(f.authorize("u-carol"))
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, body)
	assert.Empty(t, resp.Header.Values("Set-Cookie"))

	// A session the identity service no longer holds.
	b = newBrowser(t, srv)
	b.holdSession("ended-session", "u-carol")
	resp, body = b.login(f.authorize("u-carol"))
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, body)
	assert.Empty(t, resp.Header.Values("Set-Cookie"))

	err := sharedGuardCluster(t).Get(context.Background(), client.ObjectKey{Name: "u-carol"}, &v1.User{})
	assert.True(t, apierrors.IsNotFound(err), "a refused sign-in created the user: %v", err)
}

// A browser that still holds the console's ID token from an earlier release is refused by the apiserver, as it
// is by the identity service, so the console starts one sign-in; the identity service's sign-in replaces the
// cookie with its session, and from then on both accept it. No second round, no loop.
func TestExternalConsoleSignInAfterAnIDTokenCookie(t *testing.T) {
	f := newFakeIdentity(t)
	srv := consoleSignInEngine(t, f)
	admitted(t, "u-dave")
	b := newBrowser(t, srv)
	legacy := f.idToken("u-dave")
	require.True(t, isJWT(legacy))
	b.holdSession(legacy, "u-dave")

	resp, body := b.self()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "the ID token is not a credential: %v", body)
	resp, body = b.login(f.authorize("u-dave"))
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "nor is it a sign-in session: %v", body)

	// The identity service's /sign-in callback writes its own cookies over the old ones (same name, path, host).
	sid := f.signIn("u-dave")
	b.holdSession(sid, "u-dave")
	resp, body = b.login(f.authorize("u-dave"))
	require.Equal(t, http.StatusOK, resp.StatusCode, body)
	assert.Empty(t, resp.Header.Values("Set-Cookie"))
	resp, body = b.self()
	require.Equal(t, http.StatusOK, resp.StatusCode, body)
	assert.Equal(t, "u-dave", body["id"])
	assert.Equal(t, sid, b.cookie(authority.CookieToken))
}
