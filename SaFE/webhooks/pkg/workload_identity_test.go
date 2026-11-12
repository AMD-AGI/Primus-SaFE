/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	testifyassert "github.com/stretchr/testify/assert"
	testifyrequire "github.com/stretchr/testify/require"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// admissionFixture is a signed assertion and the workload it names.
func admissionFixture(t *testing.T, key *ecdsa.PrivateKey, kid string) (commonconfig.SandboxAdmissionConfig, *v1.Workload, string) {
	t.Helper()
	wl := &v1.Workload{}
	wl.Name = "sb-1"
	wl.Spec.Workspace = "ws-1"
	wl.Spec.Kind = sandboxDetailKind
	wl.Labels = map[string]string{v1.UserIdLabel: "user-1"}
	claims := identityClaims{}
	claims.Issuer = "https://site.example.com"
	claims.Subject = "user-1"
	claims.Audience = []string{"agent-sandbox-admission"}
	claims.Expiry = jwt.NewNumericDate(time.Now().Add(time.Hour))
	claims.Act.Sub = "svc-actor"
	claims.AuthorizationDetails = []map[string]any{{
		"type": "agent-sandbox-workload", "workspace": "ws-1", "kind": sandboxDetailKind, "name": "sb-1", "gpus": 0.0,
	}}
	raw, err := json.Marshal(claims)
	testifyrequire.NoError(t, err)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: key},
		(&jose.SignerOptions{}).WithType("assertion+jwt").WithHeader("kid", kid))
	testifyrequire.NoError(t, err)
	signed, err := signer.Sign(raw)
	testifyrequire.NoError(t, err)
	token, err := signed.CompactSerialize()
	testifyrequire.NoError(t, err)
	return commonconfig.SandboxAdmissionConfig{
		Issuer: "https://site.example.com", Audience: "agent-sandbox-admission",
		TokenType: "assertion+jwt", Algorithm: "ES256", Actor: "svc-actor", DetailType: "agent-sandbox-workload",
	}, wl, token
}

func publicJWKS(key *ecdsa.PrivateKey, kid string) []byte {
	set := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key: &key.PublicKey, KeyID: kid, Algorithm: string(jose.ES256), Use: "sig",
	}}}
	body, err := json.Marshal(set)
	if err != nil {
		panic(err)
	}
	return body
}

func useJWKSServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	jwksMu.Lock()
	prev := jwksClient
	jwksCached = cachedJWKS{}
	jwksClient = srv.Client()
	jwksClient.Timeout = 5 * time.Second
	jwksMu.Unlock()
	t.Cleanup(func() {
		srv.Close()
		jwksMu.Lock()
		jwksCached = cachedJWKS{}
		jwksClient = prev
		jwksMu.Unlock()
	})
	return srv
}

// TestVerifyIdentityAssertionFetchesAndCachesJWKS pins that the key set is read from the URL and then kept.
func TestVerifyIdentityAssertionFetchesAndCachesJWKS(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	testifyrequire.NoError(t, err)
	var hits atomic.Int32
	body := publicJWKS(key, "k1")
	srv := useJWKSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write(body)
	}))
	cfg, wl, token := admissionFixture(t, key, "k1")
	cfg.JWKSURL = srv.URL + "/.well-known/jwks.json"
	if _, err = verifyIdentityAssertion(context.Background(), cfg, token, false, wl); err != nil {
		t.Fatal(err)
	}
	if _, err = verifyIdentityAssertion(context.Background(), cfg, token, false, wl); err != nil {
		t.Fatal(err)
	}
	testifyassert.Equal(t, int32(1), hits.Load())
}

// TestVerifyIdentityAssertionRefetchesAnUnknownKey pins that a key id the cached set does not contain is fetched again.
func TestVerifyIdentityAssertionRefetchesAnUnknownKey(t *testing.T) {
	oldKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	testifyrequire.NoError(t, err)
	newKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	testifyrequire.NoError(t, err)
	var hits atomic.Int32
	srv := useJWKSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write(publicJWKS(newKey, "k2"))
	}))
	cfg, wl, token := admissionFixture(t, newKey, "k2")
	cfg.JWKSURL = srv.URL + "/.well-known/jwks.json"
	jwksCached = cachedJWKS{url: cfg.JWKSURL, set: mustParseJWKS(t, publicJWKS(oldKey, "k1")), at: time.Now()}
	if _, err = verifyIdentityAssertion(context.Background(), cfg, token, false, wl); err != nil {
		t.Fatal(err)
	}
	testifyassert.Equal(t, int32(1), hits.Load())
}

func mustParseJWKS(t *testing.T, body []byte) jose.JSONWebKeySet {
	t.Helper()
	var set jose.JSONWebKeySet
	testifyrequire.NoError(t, json.Unmarshal(body, &set))
	return set
}

// TestFetchJWKSRejectsPlaintext pins that a key set is not read over http.
func TestFetchJWKSRejectsPlaintext(t *testing.T) {
	_, err := fetchJWKS(context.Background(), "http://identity.example/.well-known/jwks.json", "")
	testifyassert.ErrorContains(t, err, "https")
}
