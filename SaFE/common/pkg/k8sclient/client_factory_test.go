/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package k8sclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

// testClientCert returns a base64-encoded self-signed cert/key pair accepted by tls.X509KeyPair,
// which lets factory construction run without reaching an apiserver.
func testClientCert(t *testing.T) (certData, keyData string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	assert.NoError(t, err)
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "safe-unit-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	assert.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	assert.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return base64.StdEncoding.EncodeToString(certPEM), base64.StdEncoding.EncodeToString(keyPEM)
}

func TestClientFactoryWithOnlyClient(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	f := NewClientFactoryWithOnlyClient(context.Background(), "c1", cs)
	assert.Equal(t, "c1", f.Name())
	assert.NotNil(t, f.ClientSet())

	f.SetValid(false, "down")
	assert.False(t, f.IsValid())
	assert.Equal(t, "down", f.GetInvalidReason())
	f.SetValid(true, "")
	assert.True(t, f.IsValid())

	// Release on a factory without informers should not error.
	assert.NoError(t, f.Release())
}

func TestNewClientFactoryWithFallbacksInvalidInput(t *testing.T) {
	_, err := NewClientFactoryWithFallbacks(context.Background(), "c1", "", []string{"https://10.0.0.2:6443"},
		"", "", "", DisableInformer)
	assert.Error(t, err)
}

func TestNewClientFactoryDisableInformer(t *testing.T) {
	certData, keyData := testClientCert(t)
	f, err := NewClientFactory(context.Background(), "c1", "10.96.1.1:6443", certData, keyData, "", DisableInformer)
	assert.NoError(t, err)
	assert.Equal(t, "c1", f.Name())
	// Service mode keeps the address it was given, normalized with a scheme.
	assert.Equal(t, "https://10.96.1.1:6443", f.Endpoint())
	assert.NotNil(t, f.RestConfig())
	assert.NotNil(t, f.DynamicClient())
	assert.Nil(t, f.SharedInformerFactory())
	assert.Nil(t, f.DynamicSharedInformerFactory())
	assert.Nil(t, f.Mapper())

	assert.Equal(t, "", f.BackendFingerprint())
	f.SetBackendFingerprint("10.0.0.1,10.0.0.2")
	assert.Equal(t, "10.0.0.1,10.0.0.2", f.BackendFingerprint())
	assert.NoError(t, f.Release())
}

func TestNewClientFactoryEnableInformer(t *testing.T) {
	certData, keyData := testClientCert(t)
	f, err := NewClientFactory(context.Background(), "c1", "10.96.1.1:6443", certData, keyData, "", EnableInformer)
	assert.NoError(t, err)
	assert.NotNil(t, f.SharedInformerFactory())
	assert.Nil(t, f.DynamicSharedInformerFactory())
	// Release stops the informer factory and is safe to call twice.
	assert.NoError(t, f.Release())
	assert.NoError(t, f.Release())
}

func TestNewClientFactoryEnableDynamicInformer(t *testing.T) {
	certData, keyData := testClientCert(t)
	f, err := NewClientFactory(context.Background(), "c1", "10.96.1.1:6443", certData, keyData, "",
		EnableDynamicInformer)
	assert.NoError(t, err)
	assert.NotNil(t, f.DynamicSharedInformerFactory())
	// The REST mapper resolves lazily, so no apiserver call happens during construction.
	assert.NotNil(t, f.Mapper())
	assert.Nil(t, f.SharedInformerFactory())
	assert.NoError(t, f.Release())
}

func TestNewClientFactoryWithFallbacksAllUnreachable(t *testing.T) {
	certData, keyData := testClientCert(t)
	_, err := NewClientFactoryWithFallbacks(context.Background(), "c1", "https://127.0.0.1:1",
		[]string{"https://127.0.0.1:2"}, certData, keyData, "", DisableInformer)
	assert.ErrorContains(t, err, "no reachable apiserver endpoint")
}

// TestClientFactoryValidityUnderConcurrency mirrors production access: watch error handlers write
// validity from reflector goroutines while reconcilers and API request handlers read it. The common
// module runs with -race in CI, so this guards the accessors from losing their synchronization.
func TestClientFactoryValidityUnderConcurrency(t *testing.T) {
	factory := NewClientFactoryWithOnlyClient(context.Background(), "c1", k8sfake.NewSimpleClientset())

	const iterations = 200
	var wg sync.WaitGroup
	for writer := 0; writer < 4; writer++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				factory.SetValid(false, fmt.Sprintf("connection refused on watch %d-%d", id, i))
				factory.SetValid(true, "")
			}
		}(writer)
	}
	for reader := 0; reader < 4; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if !factory.IsValid() {
					_ = factory.GetInvalidReason()
				}
			}
		}()
	}
	wg.Wait()

	factory.SetValid(false, "final")
	assert.False(t, factory.IsValid())
	assert.Equal(t, "final", factory.GetInvalidReason())
}

func TestClientFactoryTestHelpers(t *testing.T) {
	f := NewClientFactoryForTest("c1", "10.96.1.1:6443")
	assert.Equal(t, "https://10.96.1.1:6443", f.Endpoint())
	assert.True(t, f.IsValid())
	assert.Nil(t, f.RestConfig())

	cfg := &rest.Config{Host: "https://127.0.0.1:1"}
	f.AttachRestConfigForTest(cfg)
	assert.Same(t, cfg, f.RestConfig())

	informerFactory := NewClientFactoryForTestWithInformer("c1", k8sfake.NewSimpleClientset())
	assert.NotNil(t, informerFactory.SharedInformerFactory())
	informerFactory.StartInformer()
	assert.NoError(t, informerFactory.Release())
}
