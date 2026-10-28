/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package extauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/require"
)

const (
	testAudience = "safe-apiserver"
	testIssuerID = "alice-id"
)

// fakeIssuer is an OIDC issuer: a discovery document, a JWKS and a signer.
type fakeIssuer struct {
	t       *testing.T
	srv     *httptest.Server
	mu      sync.Mutex
	keys    map[string]*ecdsa.PrivateKey
	down    atomic.Bool
	jwksHit atomic.Int64
	discHit atomic.Int64
}

func newFakeIssuer(t *testing.T) *fakeIssuer {
	f := &fakeIssuer{t: t, keys: map[string]*ecdsa.PrivateKey{}}
	f.addKey("k1")
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		f.discHit.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": f.srv.URL, "jwks_uri": f.srv.URL + "/jwks"})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		f.jwksHit.Add(1)
		if f.down.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		set := jose.JSONWebKeySet{}
		for kid, k := range f.keys {
			set.Keys = append(set.Keys, jose.JSONWebKey{Key: &k.PublicKey, KeyID: kid, Algorithm: "ES256", Use: "sig"})
		}
		_ = json.NewEncoder(w).Encode(set)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeIssuer) addKey(kid string) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(f.t, err)
	f.mu.Lock()
	f.keys[kid] = k
	f.mu.Unlock()
}

// sign signs claims with the key kid. claims are merged over a valid default set.
func (f *fakeIssuer) sign(kid string, override map[string]any) string {
	f.t.Helper()
	f.mu.Lock()
	k := f.keys[kid]
	f.mu.Unlock()
	return signWith(f.t, k, kid, jose.ES256, f.claims(override))
}

func (f *fakeIssuer) claims(override map[string]any) map[string]any {
	now := time.Now()
	c := map[string]any{
		"iss": f.srv.URL, "sub": testIssuerID, "aud": []string{testAudience},
		"exp": now.Add(5 * time.Minute).Unix(), "iat": now.Unix(), "jti": "jti-1",
		"preferred_username": "alice", "email": "alice@example.com", "groups": []string{"platform-admins", "team-a"},
	}
	for k, v := range override {
		if v == nil {
			delete(c, k)
		} else {
			c[k] = v
		}
	}
	return c
}

func signWith(t *testing.T, key any, kid string, alg jose.SignatureAlgorithm, claims map[string]any) string {
	t.Helper()
	opts := (&jose.SignerOptions{}).WithType("JWT").WithHeader(jose.HeaderKey("kid"), kid)
	sg, err := jose.NewSigner(jose.SigningKey{Algorithm: alg, Key: key}, opts)
	require.NoError(t, err)
	raw, err := jwt.Signed(sg).Claims(claims).Serialize()
	require.NoError(t, err)
	return raw
}

func (f *fakeIssuer) authenticator(mod func(*OIDCConfig)) *OIDCAuthenticator {
	cfg := OIDCConfig{Issuer: f.srv.URL, Audience: testAudience}
	if mod != nil {
		mod(&cfg)
	}
	a, err := NewOIDCAuthenticator(cfg)
	require.NoError(f.t, err)
	return a
}

func auth(a Authenticator, token string) (*Identity, error) {
	return a.Authenticate(context.Background(), Credentials{Bearer: token})
}

func TestOIDCAcceptsValidToken(t *testing.T) {
	f := newFakeIssuer(t)
	a := f.authenticator(nil)
	id, err := auth(a, f.sign("k1", nil))
	require.NoError(t, err)
	require.Equal(t, testIssuerID, id.ID)
	require.Equal(t, "alice", id.Name)
	require.Equal(t, "alice@example.com", id.Email)
	require.Equal(t, []string{"platform-admins", "team-a"}, id.Groups)
	require.Equal(t, "jti-1", id.Jti)
	require.False(t, id.Delegated())
	require.EqualValues(t, 1, f.discHit.Load(), "the key set location comes from discovery")
}

func TestOIDCUsesConfiguredClaims(t *testing.T) {
	f := newFakeIssuer(t)
	a := f.authenticator(func(c *OIDCConfig) {
		c.IDClaim, c.UsernameClaim, c.EmailClaim, c.GroupsClaim = "uid", "login", "mail", "memberOf"
		c.JWKSURL = f.srv.URL + "/jwks"
	})
	id, err := auth(a, f.sign("k1", map[string]any{"uid": "u-7", "login": "bob", "mail": "b@example.com", "memberOf": "ops"}))
	require.NoError(t, err)
	require.Equal(t, "u-7", id.ID)
	require.Equal(t, "bob", id.Name)
	require.Equal(t, "b@example.com", id.Email)
	require.Equal(t, []string{"ops"}, id.Groups, "a single string is a list of one")
	require.Zero(t, f.discHit.Load(), "an explicit jwks_url skips discovery")
}

// The audience check is what keeps a token minted for another service out.
func TestOIDCRejectsWrongAudience(t *testing.T) {
	f := newFakeIssuer(t)
	a := f.authenticator(nil)
	for name, aud := range map[string]any{
		"other service": []string{"some-other-service"},
		"empty":         []string{},
		"missing":       nil,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := auth(a, f.sign("k1", map[string]any{"aud": aud}))
			require.ErrorIs(t, err, ErrUnauthenticated)
		})
	}
	// One of several audiences is enough.
	_, err := auth(a, f.sign("k1", map[string]any{"aud": []string{"x", testAudience}}))
	require.NoError(t, err)
}

func TestOIDCAudienceIsRequired(t *testing.T) {
	_, err := NewOIDCAuthenticator(OIDCConfig{Issuer: "https://issuer.example.com"})
	require.Error(t, err)
	_, err = NewOIDCAuthenticator(OIDCConfig{Audience: "a"})
	require.Error(t, err)
}

func TestOIDCClockSkew(t *testing.T) {
	f := newFakeIssuer(t)
	a := f.authenticator(func(c *OIDCConfig) { c.ClockSkew = 30 * time.Second })
	now := time.Now()
	t.Run("expired beyond the skew", func(t *testing.T) {
		_, err := auth(a, f.sign("k1", map[string]any{"exp": now.Add(-2 * time.Minute).Unix()}))
		require.ErrorIs(t, err, ErrUnauthenticated)
	})
	t.Run("expired within the skew", func(t *testing.T) {
		_, err := auth(a, f.sign("k1", map[string]any{"exp": now.Add(-10 * time.Second).Unix()}))
		require.NoError(t, err)
	})
	t.Run("not yet valid beyond the skew", func(t *testing.T) {
		_, err := auth(a, f.sign("k1", map[string]any{"nbf": now.Add(5 * time.Minute).Unix()}))
		require.ErrorIs(t, err, ErrUnauthenticated)
	})
	t.Run("not yet valid within the skew", func(t *testing.T) {
		_, err := auth(a, f.sign("k1", map[string]any{"nbf": now.Add(10 * time.Second).Unix()}))
		require.NoError(t, err)
	})
	t.Run("no expiry", func(t *testing.T) {
		_, err := auth(a, f.sign("k1", map[string]any{"exp": nil}))
		require.ErrorIs(t, err, ErrUnauthenticated)
	})
}

func TestOIDCSignatureAndIssuer(t *testing.T) {
	f := newFakeIssuer(t)
	a := f.authenticator(nil)

	t.Run("another issuer's token is not ours to judge", func(t *testing.T) {
		_, err := auth(a, f.sign("k1", map[string]any{"iss": "https://elsewhere.example.com"}))
		require.ErrorIs(t, err, ErrNotApplicable)
	})
	t.Run("not a JWT", func(t *testing.T) {
		_, err := auth(a, "ak-opaque")
		require.ErrorIs(t, err, ErrNotApplicable)
		_, err = a.Authenticate(context.Background(), Credentials{Cookie: "c"})
		require.ErrorIs(t, err, ErrNotApplicable)
	})
	t.Run("signed by a key the issuer does not publish", func(t *testing.T) {
		rogue, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		_, err := auth(a, signWith(t, rogue, "k1", jose.ES256, f.claims(nil)))
		require.ErrorIs(t, err, ErrUnauthenticated)
	})
	t.Run("HMAC signed with the public key bytes", func(t *testing.T) {
		_, err := auth(a, signWith(t, []byte("0123456789abcdef0123456789abcdef"), "k1", jose.HS256, f.claims(nil)))
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrBuiltin)
	})
	t.Run("alg none", func(t *testing.T) {
		raw := "eyJhbGciOiJub25lIiwia2lkIjoiazEifQ." + b64(t, f.claims(nil)) + "."
		_, err := auth(a, raw)
		require.Error(t, err)
	})
	t.Run("no subject", func(t *testing.T) {
		_, err := auth(a, f.sign("k1", map[string]any{"sub": nil}))
		require.ErrorIs(t, err, ErrUnauthenticated)
	})
}

func b64(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return base64URL(b)
}

func TestOIDCDelegatedClaims(t *testing.T) {
	f := newFakeIssuer(t)
	a := f.authenticator(nil)
	id, err := auth(a, f.sign("k1", map[string]any{"act": map[string]any{"sub": "svc-bot"}, "scope": "safe.read profile"}))
	require.NoError(t, err)
	require.True(t, id.Delegated())
	require.Equal(t, "svc-bot", id.Actor)
	require.Equal(t, []string{"safe.read", "profile"}, id.Scopes)

	for name, act := range map[string]any{"no sub": map[string]any{}, "a string": "svc-bot", "empty sub": map[string]any{"sub": ""}} {
		t.Run("malformed act: "+name, func(t *testing.T) {
			// A broken delegation must never be read as the user's own login.
			_, err := auth(a, f.sign("k1", map[string]any{"act": act}))
			require.ErrorIs(t, err, ErrUnauthenticated)
		})
	}
}

func TestOIDCKeyCachingAndRotation(t *testing.T) {
	f := newFakeIssuer(t)
	now := time.Now()
	var clock atomic.Pointer[time.Time]
	clock.Store(&now)
	a := f.authenticator(func(c *OIDCConfig) {
		c.JWKSURL = f.srv.URL + "/jwks"
		c.JWKSCacheTTL = time.Hour
		c.Now = func() time.Time { return *clock.Load() }
	})
	tok := f.sign("k1", nil)
	for i := 0; i < 5; i++ {
		_, err := auth(a, tok)
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, f.jwksHit.Load(), "keys are cached")

	// The issuer rotates: a token under a new key id triggers a refresh, but not
	// before the minimum refresh interval has passed.
	f.addKey("k2")
	tok2 := f.sign("k2", nil)
	_, err := auth(a, tok2)
	require.ErrorIs(t, err, ErrUnauthenticated, "inside the refresh interval the unknown key is refused, not fetched")
	require.EqualValues(t, 1, f.jwksHit.Load())
	later := now.Add(time.Minute)
	clock.Store(&later)
	// The token's own clock is real, the authenticator's is fake; keep exp valid.
	_, err = auth(a, tok2)
	require.NoError(t, err)
	require.EqualValues(t, 2, f.jwksHit.Load())
}

// A JWKS outage must not log everybody out: the cached keys keep working.
func TestOIDCKeepsServingWhenKeySetIsDown(t *testing.T) {
	f := newFakeIssuer(t)
	now := time.Now()
	var clock atomic.Pointer[time.Time]
	clock.Store(&now)
	a := f.authenticator(func(c *OIDCConfig) {
		c.JWKSURL = f.srv.URL + "/jwks"
		c.JWKSCacheTTL = time.Minute
		c.Now = func() time.Time { return *clock.Load() }
	})
	tok := f.sign("k1", nil)
	_, err := auth(a, tok)
	require.NoError(t, err)

	f.down.Store(true)
	later := now.Add(time.Hour) // the cache is stale now
	clock.Store(&later)
	require.NoError(t, func() error {
		_, e := auth(a, f.sign("k1", map[string]any{"exp": later.Add(time.Minute).Unix()}))
		return e
	}())
	require.EqualValues(t, 2, f.jwksHit.Load(), "a refresh was attempted and failed")
}

func TestOIDCUnavailableWhenNoKeysAtAll(t *testing.T) {
	f := newFakeIssuer(t)
	f.down.Store(true)
	a := f.authenticator(func(c *OIDCConfig) { c.JWKSURL = f.srv.URL + "/jwks" })
	_, err := auth(a, f.sign("k1", nil))
	var u *UnavailableError
	require.ErrorAs(t, err, &u, "cannot verify is not the same as invalid: the caller answers 503")
}

func base64URL(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}
