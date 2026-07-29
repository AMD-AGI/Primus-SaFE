/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package github

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRSAPrivateKeyPKCS1(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	got, err := parseRSAPrivateKey(string(pemBytes))
	assert.NoError(t, err)
	assert.NotNil(t, got)
}

func TestParseRSAPrivateKeyPKCS8WithEscapedNewlines(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	assert.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	// Simulate a single-line key with escaped newlines (as stored in env/config).
	escaped := strings.ReplaceAll(string(pemBytes), "\n", `\n`)
	got, err := parseRSAPrivateKey(escaped)
	assert.NoError(t, err)
	assert.NotNil(t, got)
}

func TestParseRSAPrivateKeyInvalid(t *testing.T) {
	_, err := parseRSAPrivateKey("not-a-pem")
	assert.Error(t, err)
}

func TestNewMetricsCollector(t *testing.T) {
	c := NewMetricsCollector(nil)
	assert.NotNil(t, c)
}
