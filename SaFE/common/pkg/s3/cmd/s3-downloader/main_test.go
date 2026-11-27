/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package main

import (
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
