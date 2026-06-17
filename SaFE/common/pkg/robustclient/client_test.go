/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package robustclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func testServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"meta":{"code":2000},"data":{"value":"hi"}}`))
	})
	mux.HandleFunc("/nulldata", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"meta":{"code":2000},"data":null}`))
	})
	mux.HandleFunc("/bizerr", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"meta":{"code":4001,"message":"bad"},"data":null}`))
	})
	mux.HandleFunc("/plain", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"value":"plain"}`))
	})
	mux.HandleFunc("/raw", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`raw-body`))
	})
	mux.HandleFunc("/err500", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("boom"))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/unhealthy", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	return httptest.NewServer(mux)
}

func TestClientClusterManagement(t *testing.T) {
	c := NewClient(DefaultClientConfig())
	assert.Nil(t, c.ForCluster("none"))
	c.RegisterCluster("c1", "http://x")
	assert.NotNil(t, c.ForCluster("c1"))
	assert.Equal(t, []string{"c1"}, c.ClusterNames())
	assert.Equal(t, "c1", c.ForCluster("c1").ClusterName())
	assert.Equal(t, "http://x", c.ForCluster("c1").BaseURL())
	c.RemoveCluster("c1")
	assert.Nil(t, c.ForCluster("c1"))
}

func TestClusterClientRequests(t *testing.T) {
	srv := testServer()
	defer srv.Close()
	c := NewClient(DefaultClientConfig())
	c.RegisterCluster("c1", srv.URL)
	cc := c.ForCluster("c1")
	ctx := context.Background()

	// GET with envelope data
	var out struct {
		Value string `json:"value"`
	}
	assert.NoError(t, cc.Get(ctx, "/ok", url.Values{"q": {"1"}}, &out))
	assert.Equal(t, "hi", out.Value)

	// GET out==nil just checks status
	assert.NoError(t, cc.Get(ctx, "/ok", nil, nil))

	// null data envelope -> no decode, no error
	assert.NoError(t, cc.Get(ctx, "/nulldata", nil, &out))

	// business error code -> error
	assert.Error(t, cc.Get(ctx, "/bizerr", nil, &out))

	// plain json (no envelope) decodes directly
	out.Value = ""
	assert.NoError(t, cc.Get(ctx, "/plain", nil, &out))
	assert.Equal(t, "plain", out.Value)

	// POST + Delete
	assert.NoError(t, cc.Post(ctx, "/ok", map[string]string{"a": "b"}, &out))
	assert.NoError(t, cc.Delete(ctx, "/nulldata", nil))

	// HTTP 500 -> error
	assert.Error(t, cc.Get(ctx, "/err500", nil, &out))

	// raw get/post
	b, err := cc.RawGet(ctx, "/raw", url.Values{"x": {"y"}})
	assert.NoError(t, err)
	assert.Equal(t, "raw-body", string(b))
	_, err = cc.RawPost(ctx, "/raw", map[string]string{"a": "b"})
	assert.NoError(t, err)
	// raw non-2xx -> error
	_, err = cc.RawGet(ctx, "/err500", nil)
	assert.Error(t, err)

	// health checks
	assert.NoError(t, cc.HealthCheck(ctx))
}

func TestHealthCheckUnhealthy(t *testing.T) {
	srv := testServer()
	defer srv.Close()
	c := NewClient(DefaultClientConfig())
	c.RegisterCluster("c1", srv.URL)
	// point base to a path that returns non-200 for /healthz by using a base
	// whose /healthz is unhealthy: re-register with a server returning 503.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer bad.Close()
	c.RegisterCluster("c1", bad.URL)
	assert.Error(t, c.ForCluster("c1").HealthCheck(context.Background()))
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "abc", truncate("abc", 5))
	assert.Equal(t, "ab...", truncate("abcdef", 2))
	assert.True(t, strings.HasSuffix(truncate(strings.Repeat("x", 10), 3), "..."))
}

func TestResolveRobustEndpoint(t *testing.T) {
	c := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	assert.Equal(t, defaultRobustEndpoint, resolveRobustEndpoint(c))
	c.Annotations = map[string]string{annotationRobustEndpoint: "http://custom"}
	assert.Equal(t, "http://custom", resolveRobustEndpoint(c))
}
