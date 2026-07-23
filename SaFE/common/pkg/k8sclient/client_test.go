/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package k8sclient

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

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

func TestNewClientSetWithRestConfig(t *testing.T) {
	cs, err := NewClientSetWithRestConfig(&rest.Config{Host: "http://127.0.0.1:60999"})
	assert.NoError(t, err)
	assert.NotNil(t, cs)
}

func TestHostPortFromEndpoint(t *testing.T) {
	assert.Equal(t, "10.0.0.1:6443", hostPortFromEndpoint("https://10.0.0.1:6443"))
	assert.Equal(t, "10.0.0.1:6443", hostPortFromEndpoint("http://10.0.0.1:6443/"))
	assert.Equal(t, "10.0.0.1:6443", hostPortFromEndpoint("10.0.0.1:6443"))
}

// TestClusterDialerFailsOver verifies the dialer reaches a healthy endpoint even
// when another endpoint is dead, regardless of the rotated start position.
func TestClusterDialerFailsOver(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	go func() {
		for {
			c, e := ln.Accept()
			if e != nil {
				return
			}
			_ = c.Close()
		}
	}()
	healthy := ln.Addr().String()

	// A closed port gives a fast connection-refused, i.e. a "dead" endpoint.
	tmp, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	dead := tmp.Addr().String()
	require.NoError(t, tmp.Close())

	dial := clusterDialer([]string{"https://" + dead, "https://" + healthy})
	for i := 0; i < 8; i++ {
		conn, err := dial(context.Background(), "tcp", dead)
		require.NoError(t, err, "dialer should fail over to the healthy endpoint")
		_ = conn.Close()
	}
}

// TestClusterDialerSingleEndpoint verifies a single-endpoint dialer just dials
// the requested address (keepalive path, no failover).
func TestClusterDialerSingleEndpoint(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	go func() {
		for {
			c, e := ln.Accept()
			if e != nil {
				return
			}
			_ = c.Close()
		}
	}()
	dial := clusterDialer([]string{"https://" + ln.Addr().String()})
	conn, err := dial(context.Background(), "tcp", ln.Addr().String())
	require.NoError(t, err)
	_ = conn.Close()
}
