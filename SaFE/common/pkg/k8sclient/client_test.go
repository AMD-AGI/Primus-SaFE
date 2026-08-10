/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package k8sclient

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
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
