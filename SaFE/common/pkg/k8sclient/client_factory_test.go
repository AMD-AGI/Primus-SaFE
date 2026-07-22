/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package k8sclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// newFactoryForServer builds a client factory whose clientSet targets the given
// base URL, so Probe exercises a real HTTP round-trip.
func newFactoryForServer(t *testing.T, host string) *ClientFactory {
	t.Helper()
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: host})
	require.NoError(t, err)
	return &ClientFactory{name: "test", clientSet: cs}
}

func TestProbeTreatsAnyHTTPStatusAsAlive(t *testing.T) {
	// A 500 still means the connection is alive; Probe must not report an error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := newFactoryForServer(t, srv.URL)
	assert.NoError(t, f.Probe(context.Background()))
}

func TestProbeHealthyOn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	f := newFactoryForServer(t, srv.URL)
	assert.NoError(t, f.Probe(context.Background()))
}

func TestProbeFailsOnTransportError(t *testing.T) {
	// Point at a closed server so no HTTP status is ever received.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	f := newFactoryForServer(t, url)
	assert.Error(t, f.Probe(context.Background()))
}

func TestProbeNoClient(t *testing.T) {
	f := &ClientFactory{name: "test"}
	assert.Error(t, f.Probe(context.Background()))
}
