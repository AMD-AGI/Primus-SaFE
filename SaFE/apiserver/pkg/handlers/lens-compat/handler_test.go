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

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/gpu" {
			t.Errorf("backend got path %q, want /api/v1/gpu", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()
	rc.RegisterCluster("c1", backend.URL)

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
	resp, err := http.Get(srv.URL + "/lens/v1/gpu")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing cluster: status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// Cluster not registered -> error (robust addon not installed).
	resp2, err := http.Get(srv.URL + "/lens/v1/gpu?cluster=unknown")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp2.StatusCode == http.StatusOK {
		t.Errorf("unknown cluster: expected error status, got %d", resp2.StatusCode)
	}
	resp2.Body.Close()

	// Registered cluster -> request is proxied to the backend.
	resp3, err := http.Get(srv.URL + "/lens/v1/gpu?cluster=c1")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	if resp3.StatusCode != http.StatusOK {
		t.Errorf("proxy: status = %d, want 200", resp3.StatusCode)
	}
	resp3.Body.Close()
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
