/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package secure

import (
	"strings"
	"testing"

	"gotest.tools/assert"
)

// TestGenerateKey verifies an RSA key pair is generated.
func TestGenerateKey(t *testing.T) {
	private, public, err := GenerateKey(2048)
	assert.NilError(t, err)
	assert.Assert(t, private != nil)
	assert.Assert(t, public != nil)
}

// TestGenerateKeyError verifies an error is returned for an invalid key size.
func TestGenerateKeyError(t *testing.T) {
	_, _, err := GenerateKey(16)
	assert.Assert(t, err != nil)
}

// TestEncodePrivateKey verifies the private key is PEM encoded.
func TestEncodePrivateKey(t *testing.T) {
	private, _, err := GenerateKey(2048)
	assert.NilError(t, err)
	data := EncodePrivateKey(private)
	assert.Assert(t, strings.Contains(string(data), "RSA PRIVATE KEY"))
}

// TestEncodeSSHKey verifies the public key is encoded in SSH format.
func TestEncodeSSHKey(t *testing.T) {
	_, public, err := GenerateKey(2048)
	assert.NilError(t, err)
	data, err := EncodeSSHKey(public)
	assert.NilError(t, err)
	assert.Assert(t, strings.HasPrefix(string(data), "ssh-rsa"))
}

// TestMakeSSHKeyPair verifies a full SSH key pair is produced.
func TestMakeSSHKeyPair(t *testing.T) {
	private, public, err := MakeSSHKeyPair()
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(string(private), "RSA PRIVATE KEY"))
	assert.Assert(t, strings.HasPrefix(string(public), "ssh-rsa"))
}
