/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package k8sclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func TestNewClientSetWithRestConfig(t *testing.T) {
	cs, err := NewClientSetWithRestConfig(&rest.Config{Host: "http://127.0.0.1:60999"})
	assert.NoError(t, err)
	assert.NotNil(t, cs)
}

func TestNormalizeEndpointHost(t *testing.T) {
	assert.Equal(t, "https://10.0.0.1:6443", NormalizeEndpointHost("10.0.0.1:6443"))
	assert.Equal(t, "https://c1.primus-safe.svc:443", NormalizeEndpointHost("c1.primus-safe.svc:443"))
	assert.Equal(t, "http://127.0.0.1:8080", NormalizeEndpointHost("http://127.0.0.1:8080"))
}

func TestUniqueEndpoints(t *testing.T) {
	out := uniqueEndpoints([]string{"10.0.0.1:6443", "https://10.0.0.1:6443", ""})
	assert.Len(t, out, 1)
}

func TestNewClientSetWithProbeNoCandidates(t *testing.T) {
	_, _, _, err := NewClientSetWithProbe(context.Background(), "", nil, "", "", "", true)
	assert.Error(t, err)
}

func TestProbeRESTConfigNil(t *testing.T) {
	assert.Error(t, ProbeRESTConfig(nil))
}

func TestProbeRESTConfigUnreachable(t *testing.T) {
	assert.Error(t, ProbeRESTConfig(&rest.Config{
		Host:    "https://127.0.0.1:1",
		Timeout: time.Millisecond * 200,
	}))
}

func TestProbeAPIServer(t *testing.T) {
	ctx := context.Background()
	assert.Error(t, ProbeAPIServer(ctx, nil, nil))
	assert.Error(t, ProbeAPIServer(ctx, nil, &rest.Config{
		Host:    "https://127.0.0.1:1",
		Timeout: time.Millisecond * 200,
	}))
	// Without a REST config the probe falls back to the supplied clientset.
	assert.NoError(t, ProbeAPIServer(ctx, k8sfake.NewSimpleClientset(), nil))
}

func TestNewClientSetInsecureAndWithCA(t *testing.T) {
	certData, keyData := testClientCert(t)

	_, restCfg, err := NewClientSet("10.96.1.1:6443", certData, keyData, "", true)
	assert.NoError(t, err)
	assert.Equal(t, "https://10.96.1.1:6443", restCfg.Host)
	assert.True(t, restCfg.TLSClientConfig.Insecure)

	// Secure mode requires CA data.
	_, _, err = NewClientSet("10.96.1.1:6443", certData, keyData, "", false)
	assert.Error(t, err)

	_, restCfg, err = NewClientSet("10.96.1.1:6443", certData, keyData, certData, false)
	assert.NoError(t, err)
	assert.NotEmpty(t, restCfg.TLSClientConfig.CAData)
}

func TestNewClientSetWithProbeSkipsUnbuildableEndpoint(t *testing.T) {
	// An empty endpoint cannot build a client, so the probe moves on and still fails overall.
	_, _, _, err := NewClientSetWithProbe(context.Background(), "https://127.0.0.1:1", []string{""},
		"", "", "", true)
	assert.ErrorContains(t, err, "no reachable apiserver endpoint")
}
