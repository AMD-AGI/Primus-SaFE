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
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeSyncNotSupported(t *testing.T) {
	ctx := context.Background()
	for _, code := range []int{http.StatusNotFound, http.StatusNotImplemented} {
		cc := statusServer(t, code)
		_, err := cc.GetNodeSyncDigest(ctx)
		assert.True(t, IsNotSupported(err), "digest %d", code)
		assert.True(t, IsNotSupported(cc.SyncNode(ctx, NodeSyncPayload{Name: "n1"})), "sync %d", code)
		_, err = cc.SyncNodes(ctx, []NodeSyncPayload{{Name: "n1"}})
		assert.True(t, IsNotSupported(err), "batch %d", code)
		assert.True(t, IsNotSupported(cc.DeleteNode(ctx, "n1")), "delete %d", code)
	}
	cc := statusServer(t, http.StatusInternalServerError)
	_, err := cc.GetNodeSyncDigest(ctx)
	require.Error(t, err)
	assert.False(t, IsNotSupported(err))
	err = cc.SyncNode(ctx, NodeSyncPayload{Name: "n1"})
	require.Error(t, err)
	assert.False(t, IsNotSupported(err))
}

func TestGetNodeSyncDigest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, nodeSyncDigestPath, r.URL.Path)
		_, _ = w.Write([]byte(`{"nodes":[{"name":"n1","sync_version":"123456","deleted":false},` +
			`{"name":"n2","sync_version":"7","deleted":true}]}`))
	}))
	defer srv.Close()
	c := NewClient(ClientConfig{})
	c.RegisterCluster("c1", srv.URL)

	digest, err := c.ForCluster("c1").GetNodeSyncDigest(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []NodeSyncDigestEntry{
		{Name: "n1", SyncVersion: "123456"},
		{Name: "n2", SyncVersion: "7", Deleted: true},
	}, digest.Nodes)
}

func TestSyncNodeAndDelete(t *testing.T) {
	var mu sync.Mutex
	var got []string
	var body map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, r.Method+" "+r.URL.EscapedPath())
		if r.Method == http.MethodPost {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := NewClient(ClientConfig{})
	c.RegisterCluster("c1", srv.URL)
	cc := c.ForCluster("c1")

	require.NoError(t, cc.SyncNode(context.Background(), NodeSyncPayload{
		Name: "vk-1", Workspace: "ws", Phase: NodeSyncPhaseReady, Lifecycle: NodeSyncLifecycleExternal,
		External: &NodeSyncExternal{Provider: "spur", AllocationID: "a1", Generation: 2},
	}))
	require.NoError(t, cc.DeleteNode(context.Background(), "a/b"))

	assert.Equal(t, []string{"POST " + nodeSyncPath, "DELETE " + nodeSyncPath + "/a%2Fb"}, got)
	assert.Equal(t, "vk-1", body["name"])
	assert.Equal(t, "external", body["lifecycle"])
	external := body["external"].(map[string]interface{})
	assert.Equal(t, "a1", external["allocation_id"])
	_, hasHostKey := external["host_key"]
	assert.False(t, hasHostKey, "an empty host key is omitted")
	_, hasAddress := body["address"]
	assert.False(t, hasAddress, "an unknown address is omitted")
	assert.Equal(t, float64(0), body["gpu_count"])
}

// Batches are sent in order; a failed batch does not stop the others.
func TestSyncNodesBatches(t *testing.T) {
	var mu sync.Mutex
	var sizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, nodeSyncBatchPath, r.URL.Path)
		var batch NodeSyncBatchPayload
		require.NoError(t, json.NewDecoder(r.Body).Decode(&batch))
		sizes = append(sizes, len(batch.Nodes))
		if len(sizes) == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := NewClient(ClientConfig{})
	c.RegisterCluster("c1", srv.URL)

	nodes := make([]NodeSyncPayload, 2*NodeSyncBatchSize+5)
	for i := range nodes {
		nodes[i].Name = fmt.Sprintf("n%d", i)
	}
	synced, err := c.ForCluster("c1").SyncNodes(context.Background(), nodes)
	require.Error(t, err)
	assert.Equal(t, NodeSyncBatchSize+5, synced)
	assert.Equal(t, []int{NodeSyncBatchSize, NodeSyncBatchSize, 5}, sizes)

	sizes = nil
	synced, err = c.ForCluster("c1").SyncNodes(context.Background(), nil)
	require.NoError(t, err)
	assert.Zero(t, synced)
	assert.Empty(t, sizes)
}
