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
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

const (
	secretClientID    = "secret-client"
	secretRedirectURI = "https://console.example.com/sign-in"
	loginClientID     = "console"
	loginRedirectURI  = "https://console.example.com/sso/callback"
	loginVerifier     = "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJ" // 46 characters
)

// fakeLoginProvider is an OpenID Connect provider that serves the authorization code flow for a public
// client with PKCE, as the sso.login provider does: discovery, keys, and a token endpoint that redeems
// one code, issued for one challenge, with no client secret.
type fakeLoginProvider struct {
	t   *testing.T
	srv *httptest.Server
	key *ecdsa.PrivateKey

	down        atomic.Bool
	discoveries atomic.Int64
	// confidential makes it the provider of the sso Secret instead: a client with a secret, no PKCE.
	confidential bool

	mu        sync.Mutex
	codes     map[string]string // code -> S256 challenge
	lastForm  url.Values
	lastBasic bool
	idClaims  map[string]any
}

func newFakeLoginProvider(t *testing.T) *fakeLoginProvider {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	f := &fakeLoginProvider{t: t, key: k, codes: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		f.discoveries.Add(1)
		if f.down.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": f.srv.URL, "authorization_endpoint": f.srv.URL + "/oauth/authorize",
			"token_endpoint": f.srv.URL + "/oauth/token", "jwks_uri": f.srv.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"ES256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
			{Key: &f.key.PublicKey, KeyID: "k1", Algorithm: "ES256", Use: "sig"}}})
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		_, _, basic := r.BasicAuth()
		f.mu.Lock()
		f.lastForm, f.lastBasic = r.PostForm, basic
		challenge, ok := f.codes[r.PostForm.Get("code")]
		delete(f.codes, r.PostForm.Get("code"))
		f.mu.Unlock()
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		client, redirectURI, pkceOK := loginClientID, loginRedirectURI, base64.RawURLEncoding.EncodeToString(sum[:]) == challenge
		if f.confidential {
			id, secret, _ := r.BasicAuth()
			if id == "" {
				id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
			}
			client, redirectURI, pkceOK = secretClientID, secretRedirectURI, id == secretClientID && secret == "s3cret"
			r.PostForm.Set("client_id", id)
		}
		switch {
		case !ok, r.PostForm.Get("grant_type") != "authorization_code",
			r.PostForm.Get("client_id") != client,
			r.PostForm.Get("redirect_uri") != redirectURI,
			!pkceOK:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer",
			"expires_in": 300, "id_token": f.idToken(nil)})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// issue makes a code for the challenge of verifier, as the authorization endpoint would.
func (f *fakeLoginProvider) issue(code, verifier string) {
	sum := sha256.Sum256([]byte(verifier))
	f.mu.Lock()
	f.codes[code] = base64.RawURLEncoding.EncodeToString(sum[:])
	f.mu.Unlock()
}

func (f *fakeLoginProvider) idToken(override map[string]any) string {
	now := time.Now()
	aud := loginClientID
	if f.confidential {
		aud = secretClientID
	}
	c := map[string]any{"iss": f.srv.URL, "sub": "u-1", "aud": aud, "exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(), "email": "alice@example.com", "name": "Alice", "preferred_username": "alice"}
	for k, v := range f.idClaims {
		c[k] = v
	}
	for k, v := range override {
		c[k] = v
	}
	sg, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: f.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader(jose.HeaderKey("kid"), "k1"))
	require.NoError(f.t, err)
	raw, err := jwt.Signed(sg).Claims(c).Serialize()
	require.NoError(f.t, err)
	return raw
}

// withLoginConfig sets sso.login to the fake provider and points sso.secret_path at an empty directory,
// so that no sso.secret_path provider is configured, for the duration of the test.
func withLoginConfig(t *testing.T, issuer string) {
	set := func(k, v string) {
		commonconfig.SetValue(k, v)
		t.Cleanup(func() { commonconfig.SetValue(k, "") })
	}
	set("db.enable", "false")
	set("sso.secret_path", t.TempDir())
	set("sso.login.enable", "true")
	set("sso.login.issuer", issuer)
	set("sso.login.client_id", loginClientID)
	set("sso.login.redirect_uri", loginRedirectURI)
}

func newLoginTestClient(t *testing.T) client.Client {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).Build()
}

func TestSSOLoginInitWithoutSecretProvider(t *testing.T) {
	f := newFakeLoginProvider(t)
	withLoginConfig(t, f.srv.URL)

	inst, err := initializeSSOToken(newLoginTestClient(t))
	require.NoError(t, err)
	assert.Nil(t, inst.provider, "no sso.secret_path provider is configured")
	assert.Equal(t, "", inst.AuthURL(), "no URL of the sso.secret_path provider is advertised without it")

	// The sign-in URL is the provider's, for the console's client and its own callback, and asks for PKCE.
	u, err := url.Parse(inst.LoginAuthURL())
	require.NoError(t, err)
	assert.Equal(t, f.srv.URL+"/oauth/authorize", u.Scheme+"://"+u.Host+u.Path)
	q := u.Query()
	assert.Equal(t, loginClientID, q.Get("client_id"))
	assert.Equal(t, loginRedirectURI, q.Get("redirect_uri"))
	assert.Equal(t, "code", q.Get("response_type"))
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	assert.Equal(t, "openid profile email", q.Get("scope"))
}

func TestSSOLoginInitNeedsAllSettings(t *testing.T) {
	f := newFakeLoginProvider(t)
	for _, key := range []string{"sso.login.issuer", "sso.login.client_id", "sso.login.redirect_uri"} {
		t.Run(key, func(t *testing.T) {
			withLoginConfig(t, f.srv.URL)
			commonconfig.SetValue(key, "")
			_, err := initializeSSOToken(newLoginTestClient(t))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "sso.login.enable needs")
		})
	}
	t.Run("relative redirect", func(t *testing.T) {
		withLoginConfig(t, f.srv.URL)
		commonconfig.SetValue("sso.login.redirect_uri", "/sso/callback")
		_, err := initializeSSOToken(newLoginTestClient(t))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be an absolute URL")
	})
}

func TestSSOLoginOffKeepsTodaysBehaviour(t *testing.T) {
	commonconfig.SetValue("db.enable", "false")
	commonconfig.SetValue("sso.secret_path", t.TempDir())
	defer commonconfig.SetValue("db.enable", "")
	defer commonconfig.SetValue("sso.secret_path", "")
	// sso.login is not enabled: the sso.secret_path provider is required, exactly as before.
	_, err := initializeSSOToken(newLoginTestClient(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find sso config")
}

func TestSSOLoginProviderDownAtStart(t *testing.T) {
	f := newFakeLoginProvider(t)
	withLoginConfig(t, f.srv.URL)
	f.down.Store(true)

	// The apiserver starts; the sign-in is unavailable until the provider answers.
	inst, err := initializeSSOToken(newLoginTestClient(t))
	require.NoError(t, err)
	assert.True(t, inst.LoginEnabled(), "the console must not fall back to the other sign-in while the provider is down")
	// Within the retry interval the provider is not asked again: requests fail at once.
	hits := f.discoveries.Load()
	assert.Equal(t, "", inst.LoginAuthURL())
	assert.Equal(t, hits, f.discoveries.Load(), "discovery was retried within the retry interval")
	inst.login.retryInterval = 0
	f.issue("code-1", loginVerifier)
	_, _, err = inst.Login(context.Background(), TokenInput{Code: "code-1", CodeVerifier: loginVerifier})
	require.Error(t, err)

	f.down.Store(false)
	assert.NotEqual(t, "", inst.LoginAuthURL())
	user, resp, err := inst.Login(context.Background(), TokenInput{Code: "code-1", CodeVerifier: loginVerifier})
	require.NoError(t, err)
	assert.Equal(t, commonuser.GenerateUserIdByName("alice@example.com"), user.Name)
	assert.NotEmpty(t, resp.Token)
}

func TestSSOLoginExchangesWithPKCEAsPublicClient(t *testing.T) {
	f := newFakeLoginProvider(t)
	withLoginConfig(t, f.srv.URL)
	cli := newLoginTestClient(t)
	inst, err := initializeSSOToken(cli)
	require.NoError(t, err)

	f.issue("code-1", loginVerifier)
	user, resp, err := inst.Login(context.Background(), TokenInput{Code: "code-1", CodeVerifier: loginVerifier})
	require.NoError(t, err)

	// The exchange is a public client's: the client id in the form, no secret, the verifier.
	f.mu.Lock()
	form, basic := f.lastForm, f.lastBasic
	f.mu.Unlock()
	assert.False(t, basic, "no client authentication header")
	assert.False(t, form.Has("client_secret"))
	assert.Equal(t, loginVerifier, form.Get("code_verifier"))

	// The user is the one SaFE's rule names for this e-mail, whichever provider signs it in.
	wantID := commonuser.GenerateUserIdByName("alice@example.com")
	assert.Equal(t, wantID, user.Name)
	stored := &v1.User{}
	require.NoError(t, cli.Get(context.Background(), client.ObjectKey{Name: wantID}, stored))
	assert.Equal(t, v1.SSOUserType, stored.Spec.Type)
	assert.Equal(t, "alice@example.com", v1.GetUserEmail(stored))
	assert.Equal(t, "alice", v1.GetAnnotation(stored, v1.UserPreferredNameAnnotation))

	// Without a database the session token is the ID token, and it validates as a session.
	assert.Greater(t, resp.Expire, time.Now().Add(50*time.Minute).Unix())
	info, err := inst.Validate(context.Background(), resp.Token)
	require.NoError(t, err)
	assert.Equal(t, wantID, info.Id)
}

func TestSSOLoginRefusals(t *testing.T) {
	f := newFakeLoginProvider(t)
	withLoginConfig(t, f.srv.URL)
	inst, err := initializeSSOToken(newLoginTestClient(t))
	require.NoError(t, err)

	t.Run("wrong verifier", func(t *testing.T) {
		f.issue("code-2", loginVerifier)
		other := strings.Repeat("z", 43)
		_, _, err := inst.Login(context.Background(), TokenInput{Code: "code-2", CodeVerifier: other})
		require.Error(t, err)
	})
	t.Run("malformed verifier is not sent", func(t *testing.T) {
		f.issue("code-3", "short")
		_, _, err := inst.Login(context.Background(), TokenInput{Code: "code-3", CodeVerifier: "short"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid code verifier")
	})
	t.Run("a code without a verifier needs the sso.secret_path provider", func(t *testing.T) {
		f.issue("code-4", loginVerifier)
		_, _, err := inst.Login(context.Background(), TokenInput{Code: "code-4"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not configured")
	})
	t.Run("an ID token for another client", func(t *testing.T) {
		f.idClaims = map[string]any{"aud": "another-app"}
		defer func() { f.idClaims = nil }()
		f.issue("code-5", loginVerifier)
		_, _, err := inst.Login(context.Background(), TokenInput{Code: "code-5", CodeVerifier: loginVerifier})
		require.Error(t, err)
	})
}

func TestSSOLoginValidateChoosesTheIssuersVerifier(t *testing.T) {
	f := newFakeLoginProvider(t)
	withLoginConfig(t, f.srv.URL)
	inst, err := initializeSSOToken(newLoginTestClient(t))
	require.NoError(t, err)

	_, err = inst.Validate(context.Background(), f.idToken(nil))
	require.NoError(t, err)

	// Expired, for another client, or signed by a key the provider does not publish: refused.
	_, err = inst.Validate(context.Background(), f.idToken(map[string]any{"exp": time.Now().Add(-time.Minute).Unix()}))
	assert.Error(t, err)
	_, err = inst.Validate(context.Background(), f.idToken(map[string]any{"aud": "another-app"}))
	assert.Error(t, err)
	other := newFakeLoginProvider(t)
	other.srv.Close()
	forged := other.idToken(map[string]any{"iss": f.srv.URL})
	_, err = inst.Validate(context.Background(), forged)
	assert.Error(t, err)

	// A token of another issuer goes to the sso.secret_path provider, which is not configured here.
	_, err = inst.Validate(context.Background(), other.idToken(nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no provider is configured")
	_, err = inst.Validate(context.Background(), "not-a-jwt")
	require.Error(t, err)
}

func TestUnverifiedIssuerAudience(t *testing.T) {
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	iss, aud := unverifiedIssuerAudience(enc(`{}`) + "." + enc(`{"iss":"https://idp","aud":"a"}`) + ".sig")
	assert.Equal(t, "https://idp", iss)
	assert.Equal(t, []string{"a"}, aud)
	_, aud = unverifiedIssuerAudience(enc(`{}`) + "." + enc(`{"iss":"https://idp","aud":["a","b"]}`) + ".sig")
	assert.Equal(t, []string{"a", "b"}, aud)
	for _, raw := range []string{"a.b", "a.!!!.c", enc(`{}`) + "." + enc(`[1]`) + ".sig"} {
		iss, aud = unverifiedIssuerAudience(raw)
		assert.Equal(t, "", iss)
		assert.Empty(t, aud)
	}
}

// One issuer, two clients: the sso Secret's client (bridged web apps) and the console's (sso.login). Each session is
// judged by the verifier of its own client.
func TestSSOLoginSharedIssuerTwoClients(t *testing.T) {
	login := newFakeLoginProvider(t)
	withLoginConfig(t, login.srv.URL)
	dir := t.TempDir()
	for name, v := range map[string]string{"id": secretClientID, "secret": "s3cret", "endpoint": login.srv.URL, "redirect_uri": secretRedirectURI} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(v+"\n"), 0o600))
	}
	commonconfig.SetValue("sso.secret_path", dir)
	inst, err := initializeSSOToken(newLoginTestClient(t))
	require.NoError(t, err)
	for name, aud := range map[string]any{"console": loginClientID, "bridged": secretClientID, "bridged, list": []string{secretClientID}} {
		_, err = inst.Validate(context.Background(), login.idToken(map[string]any{"aud": aud}))
		assert.NoError(t, err, name)
	}
	_, err = inst.Validate(context.Background(), login.idToken(map[string]any{"aud": "a-third-app"}))
	assert.Error(t, err)
	assert.True(t, inst.LoginEnabled())
}

// With both providers, the sso Secret's keeps serving what it served: its authorization URL stays that provider's, a code
// without a verifier is exchanged there with the client secret (the web apps that bridge through the console), and its
// sessions stay valid. The console's sign-in is the sso.login provider's.
func TestSSOLoginKeepsTheSecretProvider(t *testing.T) {
	login := newFakeLoginProvider(t)
	secret := newFakeLoginProvider(t)
	secret.confidential = true
	withLoginConfig(t, login.srv.URL)
	dir := t.TempDir()
	for name, v := range map[string]string{"id": secretClientID, "secret": "s3cret", "endpoint": secret.srv.URL, "redirect_uri": secretRedirectURI} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(v+"\n"), 0o600))
	}
	commonconfig.SetValue("sso.secret_path", dir)
	cli := newLoginTestClient(t)
	inst, err := initializeSSOToken(cli)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(inst.AuthURL(), secret.srv.URL+"/oauth/authorize?"), inst.AuthURL())
	assert.True(t, strings.HasPrefix(inst.LoginAuthURL(), login.srv.URL+"/oauth/authorize?"), inst.LoginAuthURL())

	secret.issue("bridged", "")
	_, bridged, err := inst.Login(context.Background(), TokenInput{Code: "bridged"})
	require.NoError(t, err, "a code without a verifier is the sso Secret provider's")
	login.issue("console", loginVerifier)
	_, console, err := inst.Login(context.Background(), TokenInput{Code: "console", CodeVerifier: loginVerifier})
	require.NoError(t, err)

	// Each session validates with the verifier of its own issuer, and with no other.
	for name, token := range map[string]string{"bridged": bridged.Token, "console": console.Token} {
		info, err := inst.Validate(context.Background(), token)
		require.NoError(t, err, name)
		assert.Equal(t, commonuser.GenerateUserIdByName("alice@example.com"), info.Id, name)
	}
	_, err = inst.Validate(context.Background(), secret.idToken(map[string]any{"aud": loginClientID}))
	assert.Error(t, err, "a token of the sso Secret provider for the console's client id")
	_, err = inst.Validate(context.Background(), login.idToken(map[string]any{"aud": secretClientID}))
	assert.Error(t, err, "a token of the sso.login provider for the other client id")
}

// With sso.login on, the sso Secret's provider being down at start-up does not stop the console's sign-in: it is
// discovered on first use, and serves again once it answers.
func TestSSOLoginSecretProviderDownAtStart(t *testing.T) {
	login := newFakeLoginProvider(t)
	secret := newFakeLoginProvider(t)
	secret.confidential = true
	secret.down.Store(true)
	withLoginConfig(t, login.srv.URL)
	dir := t.TempDir()
	for name, v := range map[string]string{"id": secretClientID, "secret": "s3cret", "endpoint": secret.srv.URL, "redirect_uri": secretRedirectURI} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(v+"\n"), 0o600))
	}
	commonconfig.SetValue("sso.secret_path", dir)
	inst, err := initializeSSOToken(newLoginTestClient(t))
	require.NoError(t, err, "a provider of the sso Secret that is down must not stop the apiserver")
	inst.secretLazy.retryInterval = 0

	assert.NotEqual(t, "", inst.LoginAuthURL())
	login.issue("console", loginVerifier)
	_, _, err = inst.Login(context.Background(), TokenInput{Code: "console", CodeVerifier: loginVerifier})
	require.NoError(t, err, "the console signs in while the other provider is down")
	assert.Equal(t, "", inst.AuthURL())
	secret.issue("bridged", "")
	_, _, err = inst.Login(context.Background(), TokenInput{Code: "bridged"})
	require.Error(t, err)

	secret.down.Store(false)
	assert.True(t, strings.HasPrefix(inst.AuthURL(), secret.srv.URL+"/oauth/authorize?"), inst.AuthURL())
	secret.issue("bridged-2", "")
	_, resp, err := inst.Login(context.Background(), TokenInput{Code: "bridged-2"})
	require.NoError(t, err)
	_, err = inst.Validate(context.Background(), resp.Token)
	require.NoError(t, err)
}

// With sso.login off nothing is deferred: a provider of the sso Secret that cannot be discovered stops the start, as before.
func TestSSOSecretProviderDownWithoutLogin(t *testing.T) {
	secret := newFakeLoginProvider(t)
	secret.down.Store(true)
	commonconfig.SetValue("db.enable", "false")
	defer commonconfig.SetValue("db.enable", "")
	dir := t.TempDir()
	for name, v := range map[string]string{"id": secretClientID, "secret": "s3cret", "endpoint": secret.srv.URL, "redirect_uri": secretRedirectURI} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(v+"\n"), 0o600))
	}
	commonconfig.SetValue("sso.secret_path", dir)
	defer commonconfig.SetValue("sso.secret_path", "")
	_, err := initializeSSOToken(newLoginTestClient(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to new provider")
}
