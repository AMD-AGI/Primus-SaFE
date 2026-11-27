/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package main

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/s3"
)

// The downloader verifies the endpoint's certificate as usual unless its secret carries a CA bundle or turns the
// verification off.
func TestTLSOptionsFromSecret(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	write("access_key", "ak")
	write("secret_key", "sk")
	assert.Equal(t, s3.TLSOptions{}, tlsOptionsFromSecret(dir), "a secret with only the keys verifies as usual")

	write("ca_bundle", "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----\n")
	write("insecure_skip_verify", "false")
	assert.Equal(t, s3.TLSOptions{CABundle: "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----"},
		tlsOptionsFromSecret(dir))

	write("insecure_skip_verify", "TRUE\n")
	assert.True(t, tlsOptionsFromSecret(dir).InsecureSkipVerify)
	assert.Equal(t, s3.TLSOptions{}, tlsOptionsFromSecret(filepath.Join(dir, "missing")))
}

// The downloader refuses an endpoint whose certificate no trusted authority signed, and reaches it once its secret
// carries that authority as ca_bundle.
func TestRunVerifiesTheEndpointCertificate(t *testing.T) {
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && r.URL.Path == "/bucket" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	secret := t.TempDir()
	write := func(name, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(secret, name), []byte(body), 0o600))
	}
	write("access_key", "ak")
	write("secret_key", "sk")
	t.Setenv("SECRET_PATH", secret)
	t.Setenv("INPUT_URL", server.URL+"/bucket/models/m1/")
	t.Setenv("DEST_PATH", t.TempDir())

	err := run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create S3 client")
	assert.Contains(t, err.Error(), "certificate", "the unknown authority is refused by default")

	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	write("ca_bundle", string(cert))
	err = run()
	if err != nil {
		assert.NotContains(t, err.Error(), "failed to create S3 client", "the client is created with the CA bundle")
		assert.NotContains(t, err.Error(), "certificate")
	}

	require.NoError(t, os.Remove(filepath.Join(secret, "ca_bundle")))
	write("insecure_skip_verify", "true")
	err = run()
	if err != nil {
		assert.NotContains(t, err.Error(), "failed to create S3 client", "the opt-out skips the verification")
	}
}
