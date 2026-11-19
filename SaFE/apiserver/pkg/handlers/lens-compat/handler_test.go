/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package lenscompat

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestNewHandler(t *testing.T) {
	h := NewHandler(robustclient.NewClient(robustclient.DefaultClientConfig()))
	if h == nil || h.proxies == nil {
		t.Fatal("expected initialized handler")
	}
}

func TestRegister(t *testing.T) {
	// Nil robust client: routes are skipped.
	engine := gin.New()
	NewHandler(nil).Register(engine)
	if len(engine.Routes()) != 0 {
		t.Errorf("expected no routes for nil client, got %d", len(engine.Routes()))
	}

	// Non-nil client: catch-all route registered.
	engine2 := gin.New()
	NewHandler(robustclient.NewClient(robustclient.DefaultClientConfig())).Register(engine2)
	if len(engine2.Routes()) == 0 {
		t.Error("expected routes to be registered for non-nil client")
	}
}

func TestProxy(t *testing.T) {
	rc := robustclient.NewClient(robustclient.DefaultClientConfig())
	h := NewHandler(rc)

	var backendHeaders http.Header
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workloads" {
			t.Errorf("backend got path %q, want /api/v1/workloads", r.URL.Path)
		}
		backendHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()
	rc.RegisterClusterWithToken("c1", backend.URL, "robust/c1-token", func() string { return "c1-token" })

	// Drive the handler through a real engine so the ResponseWriter is fully
	// initialized (ReverseProxy requires http.CloseNotifier support).
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(common.UserId, "u1")
		c.Set(common.UserName, "alice")
		c.Next()
	})
	engine.Any(Prefix+"/*path", h.proxy)
	srv := httptest.NewServer(engine)
	defer srv.Close()

	// Missing cluster query parameter -> 400.
	resp, err := http.Get(srv.URL + "/lens/v1/workloads")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing cluster: status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// Cluster not registered -> error (robust addon not installed).
	resp2, err := http.Get(srv.URL + "/lens/v1/workloads?cluster=unknown")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp2.StatusCode == http.StatusOK {
		t.Errorf("unknown cluster: expected error status, got %d", resp2.StatusCode)
	}
	resp2.Body.Close()

	// Registered cluster -> request is proxied with the cluster token instead of SaFE credentials.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/lens/v1/workloads?cluster=c1", nil)
	req.Header.Set("Cookie", "Token=safe-session")
	req.Header.Set("Authorization", "Bearer safe-key")
	req.Header.Set(robustclient.ServiceTokenHeader, "forged")
	resp3, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp3.StatusCode != http.StatusOK {
		t.Errorf("proxy: status = %d, want 200", resp3.StatusCode)
	}
	resp3.Body.Close()
	if got := backendHeaders.Get(robustclient.ServiceTokenHeader); got != "c1-token" {
		t.Errorf("backend service token = %q, want c1-token", got)
	}
	if backendHeaders.Get("Cookie") != "" || backendHeaders.Get("Authorization") != "" {
		t.Errorf("SaFE credentials reached the backend: cookie=%q authorization=%q",
			backendHeaders.Get("Cookie"), backendHeaders.Get("Authorization"))
	}
	if got := backendHeaders.Get(common.UserId); got != "u1" {
		t.Errorf("backend user id = %q, want u1", got)
	}

	// Writes and routes outside the allowlist -> 403, never forwarded.
	backendHeaders = nil
	resp4, err := http.Post(srv.URL+"/lens/v1/workloads?cluster=c1", "application/json", nil)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp4.StatusCode != http.StatusForbidden {
		t.Errorf("write: status = %d, want 403", resp4.StatusCode)
	}
	resp4.Body.Close()
	resp5, err := http.Get(srv.URL + "/lens/v1/management/clusters?cluster=c1")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp5.StatusCode != http.StatusForbidden {
		t.Errorf("unlisted route: status = %d, want 403", resp5.StatusCode)
	}
	resp5.Body.Close()
	if backendHeaders != nil {
		t.Error("a refused request reached the backend")
	}
}

func TestProxyWithoutClusterToken(t *testing.T) {
	rc := robustclient.NewClient(robustclient.ClientConfig{})
	h := NewHandler(rc)
	var token string
	seen := false
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, token = true, r.Header.Get(robustclient.ServiceTokenHeader)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()
	rc.RegisterCluster("c1", backend.URL)
	engine := gin.New()
	engine.Any(Prefix+"/*path", h.proxy)
	srv := httptest.NewServer(engine)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/lens/v1/nodes?cluster=c1", nil)
	req.Header.Set(robustclient.ServiceTokenHeader, "forged")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	resp.Body.Close()
	if !seen || token != "" {
		t.Errorf("backend seen=%v token=%q, want a forwarded request without a token", seen, token)
	}
}

func TestIsReadOnlyRoute(t *testing.T) {
	cases := []struct {
		method, rest string
		want         bool
	}{
		{http.MethodGet, "/workloads", true},
		{http.MethodHead, "/workloads", true},
		{http.MethodGet, "/workloads/", true},
		{http.MethodGet, "/workloads/uid-1/gpuMetrics", true},
		{http.MethodGet, "/gpu-aggregation/workloads/hourly-stats", true},
		{http.MethodGet, "/github-workflow-metrics/runs/1/jobs/2/steps/3/logs", true},
		{http.MethodGet, "/tracelens/sessions/s1/ui/", true},
		{http.MethodGet, "/tracelens/sessions/s1/ui/assets/app.js", true},
		{http.MethodPost, "/workloads", false},
		{http.MethodDelete, "/alerts/a1", false},
		{http.MethodGet, "/workloads/uid-1/unknown", false},
		{http.MethodGet, "/management/clusters", false},
		{http.MethodGet, "/agent/storage/conversations", false},
		{http.MethodGet, "/workloads/../management/clusters", false},
		{http.MethodGet, "//workloads", false},
		{http.MethodGet, "", false},
		{http.MethodGet, "/", false},
	}
	for _, tc := range cases {
		if got := isReadOnlyRoute(tc.method, tc.rest); got != tc.want {
			t.Errorf("isReadOnlyRoute(%s, %q) = %v, want %v", tc.method, tc.rest, got, tc.want)
		}
	}
}

func TestGetOrCreateProxy(t *testing.T) {
	h := NewHandler(robustclient.NewClient(robustclient.DefaultClientConfig()))

	p1, err := h.getOrCreateProxy("http://example.com", "c1")
	if err != nil || p1 == nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Second call returns the cached instance.
	p2, err := h.getOrCreateProxy("http://example.com", "c1")
	if err != nil || p2 != p1 {
		t.Errorf("expected cached proxy instance")
	}

	// Invalid URL -> parse error.
	if _, err := h.getOrCreateProxy("http://[::1]:namedport", "c1"); err == nil {
		t.Error("expected parse error for invalid URL")
	}
}

func TestSingleJoiningSlash(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"/api/v1/", "/gpu", "/api/v1/gpu"},
		{"/api/v1", "gpu", "/api/v1/gpu"},
		{"/api/v1", "/gpu", "/api/v1/gpu"},
		{"/api/v1/", "gpu", "/api/v1/gpu"},
	}
	for _, tc := range cases {
		if got := singleJoiningSlash(tc.a, tc.b); got != tc.want {
			t.Errorf("singleJoiningSlash(%q,%q) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}
}
