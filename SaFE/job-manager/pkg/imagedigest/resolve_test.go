/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package imagedigest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestIsPinned(t *testing.T) {
	digest := "repo/img@sha256:" + strings.Repeat("a", 64)
	if !IsPinned(digest) {
		t.Fatalf("expected pinned: %s", digest)
	}
	if IsPinned("repo/img:tag") {
		t.Fatal("tag must not be pinned")
	}
	if IsPinned("repo/img@sha256:short") {
		t.Fatal("short digest must not be pinned")
	}
}

func TestKeychainFromDockerConfigJSON(t *testing.T) {
	raw := []byte(`{"auths":{"example.com":{"username":"u","password":"p"}}}`)
	kc, err := KeychainFromDockerConfigJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if kc == nil {
		t.Fatal("nil keychain")
	}
}

func TestRegistryTransportDefault(t *testing.T) {
	// Default skip-verify is true when the key is unset.
	viper.Set("external_execution.registry_ca_path", "")
	viper.Set("external_execution.registry_insecure_skip_verify", true)
	t.Cleanup(func() {
		viper.Set("external_execution.registry_ca_path", "")
		viper.Set("external_execution.registry_insecure_skip_verify", false)
	})
	tr, err := registryTransport()
	if err != nil {
		t.Fatal(err)
	}
	if tr == nil || tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("expected default insecure transport")
	}
}

func TestRegistryTransportStrictNoCA(t *testing.T) {
	viper.Set("external_execution.registry_ca_path", "")
	viper.Set("external_execution.registry_insecure_skip_verify", false)
	t.Cleanup(func() {
		viper.Set("external_execution.registry_ca_path", "")
		viper.Set("external_execution.registry_insecure_skip_verify", false)
	})
	tr, err := registryTransport()
	if err != nil {
		t.Fatal(err)
	}
	if tr != nil {
		t.Fatal("expected nil transport when verify is on and no CA path is set")
	}
}

func TestRegistryTransportWithCA(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(caPath, mustTestCAPEM(t), 0o600); err != nil {
		t.Fatal(err)
	}
	viper.Set("external_execution.registry_ca_path", caPath)
	viper.Set("external_execution.registry_insecure_skip_verify", false)
	t.Cleanup(func() {
		viper.Set("external_execution.registry_ca_path", "")
		viper.Set("external_execution.registry_insecure_skip_verify", false)
	})
	tr, err := registryTransport()
	if err != nil {
		t.Fatal(err)
	}
	if tr == nil || tr.TLSClientConfig == nil || tr.TLSClientConfig.RootCAs == nil {
		t.Fatal("expected transport with custom RootCAs")
	}
	if tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify must stay false when only CA path is set")
	}
}

func TestRegistryTransportMissingCA(t *testing.T) {
	viper.Set("external_execution.registry_ca_path", "/no/such/registry-ca.pem")
	viper.Set("external_execution.registry_insecure_skip_verify", false)
	t.Cleanup(func() {
		viper.Set("external_execution.registry_ca_path", "")
		viper.Set("external_execution.registry_insecure_skip_verify", false)
	})
	_, err := registryTransport()
	if err == nil {
		t.Fatal("expected error for missing CA file")
	}
}

func TestRegistryTransportInsecure(t *testing.T) {
	viper.Set("external_execution.registry_ca_path", "")
	viper.Set("external_execution.registry_insecure_skip_verify", true)
	t.Cleanup(func() {
		viper.Set("external_execution.registry_ca_path", "")
		viper.Set("external_execution.registry_insecure_skip_verify", false)
	})
	tr, err := registryTransport()
	if err != nil {
		t.Fatal(err)
	}
	if tr == nil || tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("expected InsecureSkipVerify transport")
	}
}

func TestRegistryTransportCAWinsOverInsecureSkip(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(caPath, mustTestCAPEM(t), 0o600); err != nil {
		t.Fatal(err)
	}
	viper.Set("external_execution.registry_ca_path", caPath)
	viper.Set("external_execution.registry_insecure_skip_verify", true)
	t.Cleanup(func() {
		viper.Set("external_execution.registry_ca_path", "")
		viper.Set("external_execution.registry_insecure_skip_verify", false)
	})
	tr, err := registryTransport()
	if err != nil {
		t.Fatal(err)
	}
	if tr == nil || tr.TLSClientConfig == nil || tr.TLSClientConfig.RootCAs == nil {
		t.Fatal("expected transport with custom RootCAs")
	}
	if tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("configured CA must not be overridden by insecure skip")
	}
}

func mustTestCAPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "imagedigest-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
