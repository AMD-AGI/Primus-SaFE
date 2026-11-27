/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package s3

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tlsBucketServer serves HeadBucket for "bucket" over TLS with a certificate no system authority signed, and
// returns it with that certificate in PEM.
func tlsBucketServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && r.URL.Path == "/bucket" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	return server, string(cert)
}

// selfSignedCert returns the PEM certificate of an authority that signed nothing else.
func selfSignedCert(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "another authority"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true,
		KeyUsage: x509.KeyUsageCertSign, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// s3SecretDir points the S3 secret path at a directory holding items and returns it.
func s3SecretDir(t *testing.T, items map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range items {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	viper.Set("s3.secret_path", dir)
	t.Cleanup(func() { viper.Set("s3.secret_path", nil) })
	return dir
}

// An S3 client verifies its endpoint's certificate by default, trusts the authorities of a CA bundle in addition
// to the system's, and skips the verification only when asked to.
func TestClientVerifiesTheEndpointCertificate(t *testing.T) {
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	server, cert := tlsBucketServer(t)
	newClient := func(opts TLSOptions) error {
		conf, err := newConfigFromCredentials("ak", "sk", server.URL, "bucket", opts)
		if err != nil {
			return err
		}
		_, err = NewClientFromConfig(context.Background(), conf, Option{})
		return err
	}

	err := newClient(TLSOptions{})
	require.Error(t, err, "a certificate of an unknown authority is refused by default")
	assert.Contains(t, err.Error(), "certificate")

	assert.NoError(t, newClient(TLSOptions{CABundle: cert}), "the authority of the CA bundle is trusted")
	assert.NoError(t, newClient(TLSOptions{InsecureSkipVerify: true}), "the verification is skipped on request")

	err = newClient(TLSOptions{CABundle: selfSignedCert(t)})
	require.Error(t, err, "a CA bundle of another authority does not make the certificate trusted")
	assert.Contains(t, err.Error(), "certificate")

	err = newClient(TLSOptions{CABundle: "not a certificate"})
	assert.ErrorContains(t, err, "holds no PEM certificate")
}

// A transport that has no TLS config yet gets one.
func TestSetTLSOnABareTransport(t *testing.T) {
	bare := &http.Transport{}
	setTLS(bare, nil, true)
	assert.True(t, bare.TLSClientConfig.InsecureSkipVerify)
	assert.Nil(t, bare.TLSClientConfig.RootCAs)
}

// The TLS options of the platform's S3 endpoint and what the AWS CLI jobs get come from the optional ca_bundle and
// insecure_skip_verify items of the S3 secret; without them everything verifies as usual.
func TestPlatformTLSOptionsComeFromTheS3Secret(t *testing.T) {
	s3SecretDir(t, map[string]string{"endpoint": "https://s3.local"})
	assert.Equal(t, TLSOptions{}, PlatformTLSOptions())
	assert.Empty(t, AWSCLIEnv())
	assert.Equal(t, "aws", AWSCLICommand())

	_, cert := tlsBucketServer(t)
	s3SecretDir(t, map[string]string{"ca_bundle": cert, "insecure_skip_verify": "false\n"})
	assert.Equal(t, TLSOptions{CABundle: strings.TrimRight(cert, "\n")}, PlatformTLSOptions())
	assert.Equal(t, map[string]string{AWSCLICABundleEnv: strings.TrimRight(cert, "\n")}, AWSCLIEnv())
	assert.Equal(t, "aws", AWSCLICommand(), "a CA bundle keeps the verification on")

	s3SecretDir(t, map[string]string{"insecure_skip_verify": "True\n"})
	assert.Equal(t, TLSOptions{InsecureSkipVerify: true}, PlatformTLSOptions())
	assert.Equal(t, "aws --no-verify-ssl", AWSCLICommand())
}

// The shell a job runs before its aws commands writes the CA bundle after the image's authorities and points the
// AWS CLI at the result; without a CA bundle it leaves the AWS CLI alone.
func TestAWSCLITLSSetup(t *testing.T) {
	run := func(bundle string) string {
		cmd := exec.Command("sh", "-c", "set -e; "+AWSCLITLSSetup+`; echo "bundle=${AWS_CA_BUNDLE:-}"; `+
			`if [ -n "${AWS_CA_BUNDLE:-}" ]; then tail -n 2 "$AWS_CA_BUNDLE"; fi`)
		cmd.Env = append(os.Environ(), AWSCLICABundleEnv+"="+bundle)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return string(out)
	}
	assert.Equal(t, "bundle=\n", run(""))
	out := run("-----BEGIN CERTIFICATE-----\nline\n-----END CERTIFICATE-----")
	assert.Equal(t, "bundle=/tmp/s3-ca-bundle.pem\nline\n-----END CERTIFICATE-----\n", out)
}

// The platform's S3 client is configured from the S3 secret, its optional CA bundle included.
func TestNewConfigReadsThePlatformSecret(t *testing.T) {
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	server, cert := tlsBucketServer(t)
	viper.Set("s3.enable", true)
	t.Cleanup(func() { viper.Set("s3.enable", nil) })
	connect := func() error {
		conf, err := NewConfig()
		require.NoError(t, err)
		_, err = NewClientFromConfig(context.Background(), conf, Option{})
		return err
	}

	keys := map[string]string{"endpoint": server.URL, "bucket": "bucket", "access_key": "ak", "secret_key": "sk"}
	s3SecretDir(t, keys)
	require.Error(t, connect(), "without a CA bundle the unknown authority is refused")

	keys["ca_bundle"] = cert
	s3SecretDir(t, keys)
	assert.NoError(t, connect(), "the CA bundle of the secret is trusted")
}

// A CA bundle is still trusted where the system's authorities cannot be read.
func TestCABundleWithoutASystemCertPool(t *testing.T) {
	t.Setenv("AWS_MAX_ATTEMPTS", "1")
	server, cert := tlsBucketServer(t)
	original := systemCertPool
	systemCertPool = func() (*x509.CertPool, error) { return nil, errors.New("no system pool") }
	t.Cleanup(func() { systemCertPool = original })

	conf, err := newConfigFromCredentials("ak", "sk", server.URL, "bucket", TLSOptions{CABundle: cert})
	require.NoError(t, err)
	_, err = NewClientFromConfig(context.Background(), conf, Option{})
	assert.NoError(t, err)
}
