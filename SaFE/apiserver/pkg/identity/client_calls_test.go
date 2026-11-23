/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identity

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func resetIdentityConfig(t *testing.T) {
	t.Helper()
	viper.Reset()
	t.Cleanup(func() {
		viper.Reset()
		httpClientsMu.Lock()
		httpClients = map[string]*http.Client{}
		httpClientsMu.Unlock()
		scimCertDir = "/etc/secrets/scim-tls"
	})
}

// The identity base comes from auth.scim.identity_url, or from an introspection URL with its /oauth path removed.
func TestResolveBase(t *testing.T) {
	resetIdentityConfig(t)
	_, err := ResolveBase()
	var cfg *ConfigError
	require.ErrorAs(t, err, &cfg)

	viper.Set("auth.scim.identity_url", "ftp://idp.example")
	_, err = ResolveBase()
	require.ErrorAs(t, err, &cfg)

	viper.Set("auth.scim.identity_url", "https://idp.example/root/")
	base, err := ResolveBase()
	require.NoError(t, err)
	require.Equal(t, "https://idp.example/root", base)

	viper.Set("auth.scim.identity_url", "")
	viper.Set("auth.authenticators", []map[string]any{
		{"type": "oidc", "issuer": "https://idp.example"},
		{"type": "introspection", "url": "https://idp.example/oauth/introspect"},
	})
	base, err = ResolveBase()
	require.NoError(t, err)
	require.Equal(t, "https://idp.example", base)

	viper.Set("auth.authenticators", []map[string]any{{"type": "introspection", "issuer": "https://idp.example"}})
	base, err = ResolveBase()
	require.NoError(t, err)
	require.Equal(t, "https://idp.example", base)

	viper.Set("auth.authenticators", []map[string]any{{"type": "introspection", "url": "https://idp.example/custom"}})
	_, err = ResolveBase()
	require.ErrorAs(t, err, &cfg)

	_, err = deriveBase("://")
	require.Error(t, err)
	_, err = normalizePublic("  ")
	require.ErrorAs(t, err, &cfg)
	got, err := normalizePublic("https://safe.example/scim/")
	require.NoError(t, err)
	require.Equal(t, "https://safe.example/scim", got)
	require.False(t, absoluteHTTP(nil))

	viper.Set("auth.scim.identity_url", "https://idp.example")
	url, err := userURL("u-1", "status")
	require.NoError(t, err)
	require.Equal(t, "https://idp.example/api/v1/identities/u-1/status", url)
}

// A status call forwards the bearer. Transport and HTTP failures become 503 or 502, and a missing bearer is 502.
func TestSetStatus(t *testing.T) {
	resetIdentityConfig(t)
	err := SetStatus(context.Background(), "u-1", "disabled", "")
	require.Error(t, err)
	var status apierrors.APIStatus
	require.ErrorAs(t, err, &status)
	require.Equal(t, int32(http.StatusBadGateway), status.Status().Code)

	err = SetStatus(context.Background(), "u-1", "disabled", "bearer")
	require.ErrorAs(t, err, &status)
	require.Equal(t, int32(http.StatusServiceUnavailable), status.Status().Code)

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/api/v1/identities/u-1/status" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	viper.Set("auth.scim.identity_url", srv.URL)
	require.NoError(t, SetStatus(context.Background(), "u-1", "disabled", "tok"))
	require.Equal(t, "Bearer tok", gotAuth)

	require.Error(t, SetStatus(context.Background(), "other", "active", "tok"))

	viper.Set("auth.scim.identity_url", "http://127.0.0.1:1")
	err = SetStatus(context.Background(), "u-1", "disabled", "tok")
	require.ErrorAs(t, err, &status)
	require.Equal(t, int32(http.StatusServiceUnavailable), status.Status().Code)

	require.Error(t, mapCall("op", http.StatusBadRequest, nil))
	require.Error(t, mapCall("op", http.StatusBadGateway, nil))
}

// The mounted certificate is sent as stored, with a trailing newline added when the file has none.
func TestTargetCertificateAndRegistrationStatus(t *testing.T) {
	resetIdentityConfig(t)
	dir := t.TempDir()
	scimCertDir = dir
	got, err := targetCertificate()
	require.NoError(t, err)
	require.Empty(t, got)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "tls.crt"), []byte("  \n"), 0o600))
	got, err = targetCertificate()
	require.NoError(t, err)
	require.Empty(t, got)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "tls.crt"), []byte("CERT"), 0o600))
	got, err = targetCertificate()
	require.NoError(t, err)
	require.Equal(t, "CERT\n", got)

	scimCertDir = filepath.Join(dir, "tls.crt")
	_, err = targetCertificate()
	require.Error(t, err)
	scimCertDir = dir

	leaf := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(leaf.Close)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Certificate().Raw})
	viper.Set("auth.authenticators", []map[string]any{{"type": "introspection", "ca_bundle": string(pemBytes)}})
	key, pool, err := clientCA()
	require.NoError(t, err)
	require.NotEmpty(t, key)
	require.NotNil(t, pool)
	cli, err := httpClient()
	require.NoError(t, err)
	require.NotNil(t, cli)

	viper.Set("auth.authenticators", []map[string]any{{"type": "introspection", "ca_bundle": filepath.Join(dir, "missing.pem")}})
	httpClients = map[string]*http.Client{}
	_, _, err = clientCA()
	require.Error(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	secret := writeFile(t, "client_secret", "secret\n")
	viper.Set("auth.scim.identity_url", srv.URL)
	viper.Set("auth.scim.public_url", "https://safe.example/scim")
	viper.Set("auth.authenticators", []map[string]any{{
		"type": "introspection", "client_id": "svc", "client_secret_file": secret,
	}})
	err = RegisterTarget(context.Background(), "https://safe.example/scim", "tok")
	require.ErrorContains(t, err, "answered 502")
	require.Error(t, RegisterTarget(context.Background(), "", "tok"))
}
