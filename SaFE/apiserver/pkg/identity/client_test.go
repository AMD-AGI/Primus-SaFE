/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identity

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// Registration sends the certificate the public URL presents.
func TestRegisterTargetSendsThePresentedCertificate(t *testing.T) {
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
	pub := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(pub.Close)
	secret := writeFile(t, "client_secret", "secret\n")
	viper.Set("auth.scim.identity_url", idn.URL)
	viper.Set("auth.authenticators", []map[string]any{{
		"type": "introspection", "client_id": "svc", "client_secret_file": secret,
	}})

	require.NoError(t, RegisterTarget(context.Background(), pub.URL, "tok"))
	var got scimTarget
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, strings.TrimRight(pub.URL, "/"), got.URL)
	require.Equal(t, "tok", got.BearerToken)
	block, _ := pem.Decode([]byte(got.Certificate))
	require.NotNil(t, block)
	parsed, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	require.True(t, parsed.Equal(pub.Certificate()))
}
