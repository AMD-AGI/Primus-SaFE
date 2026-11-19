/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package robustclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func statusServer(t *testing.T, code int) *ClusterClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte("nope"))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(ClientConfig{})
	c.RegisterCluster("c1", srv.URL)
	return c.ForCluster("c1")
}

func TestNotSupportedError(t *testing.T) {
	ctx := context.Background()
	for _, code := range []int{http.StatusNotFound, http.StatusNotImplemented} {
		cc := statusServer(t, code)
		_, err := cc.GetSyncDigest(ctx)
		require.Error(t, err)
		assert.True(t, IsNotSupported(err), "digest %d", code)
		_, err = cc.GetWorkloadGPUWindow(ctx, []string{"u1"}, "30m", "1m")
		require.Error(t, err)
		assert.True(t, IsNotSupported(err), "window %d", code)
		assert.Contains(t, err.Error(), fmt.Sprint(code))
	}
	for _, code := range []int{http.StatusUnauthorized, http.StatusInternalServerError} {
		cc := statusServer(t, code)
		_, err := cc.GetSyncDigest(ctx)
		require.Error(t, err)
		assert.False(t, IsNotSupported(err), "digest %d", code)
		_, err = cc.GetWorkloadGPUWindow(ctx, []string{"u1"}, "30m", "1m")
		require.Error(t, err)
		assert.False(t, IsNotSupported(err), "window %d", code)
	}
}

func TestGetSyncDigest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, syncDigestPath, r.URL.Path)
		_, _ = w.Write([]byte(`{"workloads":[{"uid":"u1","sync_version":"12","phase":"Running","deleted":false},` +
			`{"uid":"u2","sync_version":"7","phase":"Succeeded","deleted":true}]}`))
	}))
	defer srv.Close()
	c := NewClient(ClientConfig{})
	c.RegisterCluster("c1", srv.URL)

	digest, err := c.ForCluster("c1").GetSyncDigest(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []WorkloadSyncDigestEntry{
		{UID: "u1", SyncVersion: "12", Phase: "Running"},
		{UID: "u2", SyncVersion: "7", Phase: "Succeeded", Deleted: true},
	}, digest.Workloads)
}

// Large uid lists are split into requests of GPUWindowBatchSize and the
// answers are merged.
func TestGetWorkloadGPUWindowBatches(t *testing.T) {
	var sizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, gpuWindowPath, r.URL.Path)
		var req GPUWindowRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "30m", req.Window)
		assert.Equal(t, "1m", req.Step)
		sizes = append(sizes, len(req.WorkloadUIDs))
		// Only the first uid of each batch has data.
		resp := GPUWindowResponse{Workloads: []WorkloadGPUWindow{{
			UID:     req.WorkloadUIDs[0],
			Overall: GPUWindowOverall{Max: 3.2, Avg: 1.1, Samples: 30, ExpectedSamples: 30},
			Nodes:   []GPUWindowNode{{Node: "node-01", Max: 2, Samples: 30}},
		}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()
	c := NewClient(ClientConfig{})
	c.RegisterCluster("c1", srv.URL)

	uids := make([]string, 2*GPUWindowBatchSize+50)
	for i := range uids {
		uids[i] = fmt.Sprintf("u%d", i)
	}
	got, err := c.ForCluster("c1").GetWorkloadGPUWindow(context.Background(), uids, "30m", "1m")
	require.NoError(t, err)
	assert.Equal(t, []int{GPUWindowBatchSize, GPUWindowBatchSize, 50}, sizes)
	require.Len(t, got, 3)
	assert.Equal(t, "u0", got[0].UID)
	assert.Equal(t, fmt.Sprintf("u%d", GPUWindowBatchSize), got[1].UID)
	assert.Equal(t, 3.2, got[2].Overall.Max)
	assert.Equal(t, "node-01", got[2].Nodes[0].Node)

	// No uids: no request.
	sizes = nil
	got, err = c.ForCluster("c1").GetWorkloadGPUWindow(context.Background(), nil, "30m", "1m")
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Empty(t, sizes)
}
