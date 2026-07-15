/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestNewLogsClientDefaults(t *testing.T) {
	// Zero timeout falls back to the default and the trailing slash is trimmed.
	c := NewLogsClient(LogsClientConfig{BaseURL: "https://opensearch.local:9200/"})
	assert.Equal(t, "https://opensearch.local:9200", c.BaseURL())
	assert.Equal(t, defaultLogsTimeout, c.httpClient.Timeout)

	// Explicit timeout is honored.
	c2 := NewLogsClient(LogsClientConfig{BaseURL: "https://x", Timeout: 5 * time.Second})
	assert.Equal(t, 5*time.Second, c2.httpClient.Timeout)
}

func TestLogsClientRequest(t *testing.T) {
	// Empty base URL errors before issuing any request.
	empty := NewLogsClient(LogsClientConfig{})
	_, err := empty.Request(context.Background(), http.MethodGet, "/_search", nil)
	assert.ErrorContains(t, err, "base URL not configured")

	// 2xx returns the raw body verbatim; basic auth header set; path normalized.
	var gotUser, gotPass string
	var gotPath string
	var authOK bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, authOK = r.BasicAuth()
		gotPath = r.URL.Path
		w.Write([]byte(`{"hits":{"hits":[]}}`))
	}))
	defer srv.Close()

	c := NewLogsClient(LogsClientConfig{BaseURL: srv.URL, Username: "admin", Password: "secret"})
	// Path without a leading slash should be normalized to include one.
	out, err := c.Request(context.Background(), http.MethodGet, "node-2026.07.09/_search", nil)
	assert.NoError(t, err)
	assert.Contains(t, string(out), "hits")
	assert.True(t, authOK)
	assert.Equal(t, "admin", gotUser)
	assert.Equal(t, "secret", gotPass)
	assert.Equal(t, "/node-2026.07.09/_search", gotPath)

	// Non-2xx returns an error carrying the (truncated) response body.
	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"index_not_found"}`))
	}))
	defer errSrv.Close()

	errClient := NewLogsClient(LogsClientConfig{BaseURL: errSrv.URL})
	_, err = errClient.Request(context.Background(), http.MethodGet, "/missing/_search", nil)
	assert.ErrorContains(t, err, "HTTP 404")
	assert.ErrorContains(t, err, "index_not_found")
}

func TestLogsRegistry(t *testing.T) {
	reg := NewLogsRegistry(LogsClientConfig{})

	reg.RegisterCluster("core42", "https://os-core42:9200")
	assert.NotNil(t, reg.ForCluster("core42"))
	assert.Nil(t, reg.ForCluster("missing"))

	names := reg.ClusterNames()
	assert.Len(t, names, 1)
	assert.Equal(t, "core42", names[0])

	// Re-registering with a new endpoint swaps the client.
	old := reg.ForCluster("core42")
	reg.RegisterCluster("core42", "https://os-core42-v2:9200")
	assert.NotSame(t, old, reg.ForCluster("core42"))
	assert.Equal(t, "https://os-core42-v2:9200", reg.ForCluster("core42").BaseURL())

	reg.RemoveCluster("core42")
	assert.Nil(t, reg.ForCluster("core42"))
	assert.Empty(t, reg.ClusterNames())
}

func TestLogsDiscoveryResolveEndpoint(t *testing.T) {
	// Defaults are applied for empty interval / annotation key.
	d := NewLogsDiscovery(nil, nil, LogsDiscoveryConfig{DefaultEndpoint: "https://default:9200"})
	assert.Equal(t, 30*time.Second, d.interval)
	assert.Equal(t, defaultLogsEndpointAnnotation, d.annotationKey)

	// No annotation -> default endpoint.
	plain := &v1.Cluster{}
	assert.Equal(t, "https://default:9200", d.resolveEndpoint(plain))

	// Annotation present -> annotation wins over default.
	annotated := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{defaultLogsEndpointAnnotation: "https://override:9200"},
		},
	}
	assert.Equal(t, "https://override:9200", d.resolveEndpoint(annotated))

	// Empty annotation value -> falls back to default.
	emptyAnnot := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{defaultLogsEndpointAnnotation: ""},
		},
	}
	assert.Equal(t, "https://default:9200", d.resolveEndpoint(emptyAnnot))
}

func readyCluster(name, endpoint string) *v1.Cluster {
	c := &v1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	c.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	if endpoint != "" {
		c.Annotations = map[string]string{defaultLogsEndpointAnnotation: endpoint}
	}
	return c
}

func newFakeClusterClient(t *testing.T, clusters ...*v1.Cluster) *fake.ClientBuilder {
	t.Helper()
	scheme := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(scheme))
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, c := range clusters {
		builder = builder.WithObjects(c)
	}
	return builder
}

func TestLogsDiscoverySyncOnceListError(t *testing.T) {
	// A fake client built with an empty scheme cannot list ClusterList, so
	// syncOnce hits the list-error branch and registers nothing.
	badClient := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	reg := NewLogsRegistry(LogsClientConfig{})
	d := NewLogsDiscovery(badClient, reg, LogsDiscoveryConfig{})

	d.syncOnce(context.Background())
	assert.Empty(t, reg.ClusterNames())
}

func TestLogsDiscoveryStartStop(t *testing.T) {
	ready := readyCluster("ready-1", "https://os-ready-1:9200")
	k8sClient := newFakeClusterClient(t, ready).Build()
	reg := NewLogsRegistry(LogsClientConfig{})
	d := NewLogsDiscovery(k8sClient, reg, LogsDiscoveryConfig{Interval: 10 * time.Millisecond})

	d.Start(context.Background())

	// The initial syncOnce should register the ready cluster promptly.
	assert.Eventually(t, func() bool {
		return reg.ForCluster("ready-1") != nil
	}, time.Second, 10*time.Millisecond)

	// Stop is idempotent (sync.Once guards the close).
	d.Stop()
	d.Stop()
}

func TestLogsDiscoveryStartContextCancel(t *testing.T) {
	k8sClient := newFakeClusterClient(t).Build()
	reg := NewLogsRegistry(LogsClientConfig{})
	d := NewLogsDiscovery(k8sClient, reg, LogsDiscoveryConfig{Interval: 10 * time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	d.Start(ctx)
	// Cancelling the context stops the reconcile goroutine.
	cancel()
	assert.Empty(t, reg.ClusterNames())
}

func TestLogsDiscoverySyncOnce(t *testing.T) {
	ready := readyCluster("ready-1", "https://os-ready-1:9200")
	notReady := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "pending-1"}}

	k8sClient := newFakeClusterClient(t, ready, notReady).Build()
	reg := NewLogsRegistry(LogsClientConfig{})
	d := NewLogsDiscovery(k8sClient, reg, LogsDiscoveryConfig{})

	// Only the ready cluster with a resolvable endpoint is registered.
	d.syncOnce(context.Background())
	assert.NotNil(t, reg.ForCluster("ready-1"))
	assert.Nil(t, reg.ForCluster("pending-1"))
	assert.Equal(t, "https://os-ready-1:9200", reg.ForCluster("ready-1").BaseURL())

	// After the cluster disappears, the stale endpoint is removed on next sync.
	emptyClient := newFakeClusterClient(t).Build()
	d2 := NewLogsDiscovery(emptyClient, reg, LogsDiscoveryConfig{})
	d2.syncOnce(context.Background())
	assert.Nil(t, reg.ForCluster("ready-1"))
	assert.Empty(t, reg.ClusterNames())
}
