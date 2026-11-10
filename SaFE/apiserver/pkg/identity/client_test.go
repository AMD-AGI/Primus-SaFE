/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identity

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

// The introspection client id is the configured one, or the content of client_id_file when client_id is empty.
func TestIntrospectionSecretClientID(t *testing.T) {
	t.Cleanup(viper.Reset)
	secret := writeFile(t, "client_secret", "s3cret\n")
	idFile := writeFile(t, "client_id", "svc-from-file\n")

	viper.Set("auth.authenticators", []map[string]any{{"type": "introspection", "client_id": "svc-inline",
		"client_id_file": idFile, "client_secret_file": secret}})
	id, got, err := introspectionSecret()
	require.NoError(t, err)
	require.Equal(t, "svc-inline", id)
	require.Equal(t, "s3cret", got)

	viper.Set("auth.authenticators", []map[string]any{{"type": "introspection", "client_id_file": idFile, "client_secret_file": secret}})
	id, got, err = introspectionSecret()
	require.NoError(t, err)
	require.Equal(t, "svc-from-file", id)
	require.Equal(t, "s3cret", got)

	viper.Set("auth.authenticators", []map[string]any{{"type": "introspection", "client_secret_file": secret}})
	_, _, err = introspectionSecret()
	var cfg *ConfigError
	require.ErrorAs(t, err, &cfg, "no id and no id file is a configuration error")
}

// Registration sends the certificate mounted from the ingress TLS secret, and none when that file is absent.
func TestRegisterTargetSendsTheMountedCertificate(t *testing.T) {
	t.Cleanup(viper.Reset)
	var body []byte
	idn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		user, pass, ok := r.BasicAuth()
		if r.Method != http.MethodPut || r.URL.Path != "/oauth/scim-target" || !ok || user != "svc" || pass != "secret" {
			t.Errorf("request: %s %s auth=%v user=%s", r.Method, r.URL.Path, ok, user)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(idn.Close)
	leaf := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(leaf.Close)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Certificate().Raw})
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tls.crt"), pemBytes, 0o600))
	scimCertDir = dir
	t.Cleanup(func() { scimCertDir = "/etc/secrets/scim-tls" })
	secret := writeFile(t, "client_secret", "secret\n")
	viper.Set("auth.scim.identity_url", idn.URL)
	viper.Set("auth.scim.public_url", "https://apiserver.example.com/scim/v2")
	viper.Set("auth.authenticators", []map[string]any{{
		"type": "introspection", "client_id": "svc", "client_secret_file": secret,
	}})

	require.NoError(t, RegisterTarget(context.Background(), "https://apiserver.example.com/scim/v2", "tok"))
	var got scimTarget
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, "https://apiserver.example.com/scim/v2", got.URL)
	require.Equal(t, "tok", got.BearerToken)
	require.Equal(t, string(pemBytes), got.Certificate)

	scimCertDir = t.TempDir()
	require.NoError(t, RegisterTarget(context.Background(), "https://apiserver.example.com/scim/v2", "tok"))
	got = scimTarget{}
	require.NoError(t, json.Unmarshal(body, &got))
	require.Empty(t, got.Certificate)
}

// Repeated calls share one client, including its connections.
func TestHTTPClientIsShared(t *testing.T) {
	t.Cleanup(func() {
		viper.Reset()
		httpClientsMu.Lock()
		httpClients = map[string]*http.Client{}
		httpClientsMu.Unlock()
	})
	viper.Set("auth.authenticators", []map[string]any{{"type": "introspection"}})
	first, err := httpClient()
	require.NoError(t, err)
	second, err := httpClient()
	require.NoError(t, err)
	require.Same(t, first, second)

	viper.Set("auth.authenticators", []map[string]any{{"type": "introspection", "ca_bundle": "not a certificate"}})
	_, err = httpClient()
	require.Error(t, err)
}
