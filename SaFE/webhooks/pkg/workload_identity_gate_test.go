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
	"os"
	"path/filepath"
	"testing"

	"github.com/go-jose/go-jose/v4"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

func keySet(key *ecdsa.PrivateKey, kid string) jose.JSONWebKeySet {
	return jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key: &key.PublicKey, KeyID: kid, Algorithm: string(jose.ES256), Use: "sig",
	}}}
}

// Admission stays quiet until it is enabled for the sandbox creator, and then it requires a readable assertion.
func TestValidateSandboxIdentityGates(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	v := &WorkloadValidator{}
	wl := &v1.Workload{}
	wl.Name = "sb-1"
	require.NoError(t, v.validateSandboxIdentity(context.Background(), admission.Request{}, wl, nil))

	viper.Set("sandbox.admission.enabled", true)
	viper.Set("sandbox.admission.creator", "sandbox-controller")
	other := admission.Request{}
	other.UserInfo.Username = "someone"
	require.NoError(t, v.validateSandboxIdentity(context.Background(), other, wl, nil))

	req := admission.Request{}
	req.UserInfo.Username = "sandbox-controller"
	require.ErrorContains(t, v.validateSandboxIdentity(context.Background(), req, wl, nil), "annotation")

	viper.Set("sandbox.admission.annotation", "agents.example/assertion")
	require.ErrorContains(t, v.validateSandboxIdentity(context.Background(), req, wl, nil), "required")

	v1.SetAnnotation(wl, "agents.example/assertion", "not-a-jwt")
	require.ErrorContains(t, v.validateSandboxIdentity(context.Background(), req, wl, nil), "not configured")
}

// A signed assertion is refused when a claim, the signature, or the workload detail does not match.
func TestCheckAssertionRejects(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	cfg, wl, token := admissionFixture(t, key, "k1")
	keys := keySet(key, "k1")

	_, err = checkAssertion(keys, cfg, "not-a-jwt", false, wl)
	require.ErrorContains(t, err, "signed JWT")

	badType := cfg
	badType.TokenType = "other"
	_, err = checkAssertion(keys, badType, token, false, wl)
	require.ErrorContains(t, err, "token type")

	_, _, missing := admissionFixture(t, key, "missing")
	_, err = checkAssertion(keys, cfg, missing, false, wl)
	require.ErrorIs(t, err, errUnknownKID)

	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	_, err = checkAssertion(keySet(other, "k1"), cfg, token, false, wl)
	require.ErrorContains(t, err, "signature")

	cfg.Audience = "other"
	_, err = checkAssertion(keys, cfg, token, true, wl)
	require.ErrorContains(t, err, "claims")
	cfg.Audience = "agent-sandbox-admission"

	wl.Labels = map[string]string{v1.UserIdLabel: "other"}
	_, err = checkAssertion(keys, cfg, token, false, wl)
	require.ErrorContains(t, err, "sub")
	wl.Labels = map[string]string{v1.UserIdLabel: "user-1"}

	cfg.Actor = "other"
	_, err = checkAssertion(keys, cfg, token, false, wl)
	require.ErrorContains(t, err, "act.sub")
	cfg.Actor = "svc-actor"

	wl.Spec.Resources = []v1.WorkloadResource{{GPU: "nope"}}
	_, err = checkAssertion(keys, cfg, token, false, wl)
	require.ErrorContains(t, err, "gpu")

	wl.Spec.Resources = []v1.WorkloadResource{{GPU: "2", Replica: 2}}
	_, err = checkAssertion(keys, cfg, token, false, wl)
	require.ErrorContains(t, err, "authorization detail")

	cfg.Kind = "Other"
	require.False(t, hasMatchingDetail([]map[string]any{{
		"type": cfg.DetailType, "workspace": wl.Spec.Workspace, "kind": sandboxDetailKind, "name": wl.Name, "gpus": 0.0,
	}}, cfg, wl, 0))

	_, _, err = admissionKeys(context.Background(), commonconfig.SandboxAdmissionConfig{JWKS: "{"}, false)
	require.ErrorContains(t, err, "not readable")
	_, _, err = admissionKeys(context.Background(), commonconfig.SandboxAdmissionConfig{JWKS: `{"keys":[]}`}, false)
	require.ErrorContains(t, err, "not readable")
	raw, err := json.Marshal(keys)
	require.NoError(t, err)
	got, fromURL, err := admissionKeys(context.Background(), commonconfig.SandboxAdmissionConfig{JWKS: string(raw)}, false)
	require.NoError(t, err)
	require.False(t, fromURL)
	require.NotEmpty(t, got.Keys)
}

// A failed key-set fetch keeps the previous set. The CA file must be a PEM bundle.
func TestJWKSFetchFallbackAndCA(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	body := publicJWKS(key, "k1")
	hits := 0
	srv := useJWKSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			_, _ = w.Write(body)
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	cfg := commonconfig.SandboxAdmissionConfig{JWKSURL: srv.URL + "/jwks"}
	set, fromURL, err := admissionKeys(context.Background(), cfg, false)
	require.NoError(t, err)
	require.True(t, fromURL)
	require.NotEmpty(t, set.Keys)

	jwksMu.Lock()
	jwksCached.at = jwksCached.at.Add(-jwksTTL)
	jwksMu.Unlock()
	set, _, err = admissionKeys(context.Background(), cfg, false)
	require.NoError(t, err)
	require.NotEmpty(t, set.Keys)

	plain := useJWKSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{"))
	}))
	_, err = fetchJWKS(context.Background(), plain.URL+"/jwks", "")
	require.ErrorContains(t, err, "not readable")

	_, err = jwksHTTPClient(filepath.Join(t.TempDir(), "missing.pem"))
	require.ErrorContains(t, err, "ca")
	bad := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(bad, []byte("not a cert"), 0o600))
	_, err = jwksHTTPClient(bad)
	require.ErrorContains(t, err, "not readable")
}

// Membership is the SubjectAccessReview answer. A failed review is an internal error.
func TestCheckWorkspaceAccess(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, authorizationv1.AddToScheme(scheme))
	allowed := interceptor.NewClient(fake.NewClientBuilder().WithScheme(scheme).Build(), interceptor.Funcs{
		Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
			obj.(*authorizationv1.SubjectAccessReview).Status.Allowed = true
			return nil
		},
	})
	v := &WorkloadValidator{Client: allowed}
	require.NoError(t, v.checkWorkspaceAccess(context.Background(), &identityClaims{Groups: []string{"lab"}}, "lab"))

	denied := interceptor.NewClient(fake.NewClientBuilder().WithScheme(scheme).Build(), interceptor.Funcs{
		Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
			return nil
		},
	})
	v.Client = denied
	require.ErrorContains(t, v.checkWorkspaceAccess(context.Background(), &identityClaims{}, "lab"), "not a member")

	v.Client = interceptor.NewClient(fake.NewClientBuilder().WithScheme(scheme).Build(), interceptor.Funcs{
		Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
			return context.Canceled
		},
	})
	require.ErrorContains(t, v.checkWorkspaceAccess(context.Background(), &identityClaims{}, "lab"), "could not be checked")
}
