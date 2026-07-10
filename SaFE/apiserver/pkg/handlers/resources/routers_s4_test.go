/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// TestClusterReadRoutesRequireAuth verifies S4: cluster list and detail, which
// expose control-plane endpoints, node IPs, subnets and secret IDs, are no
// longer served from the unauthenticated group. Unauthenticated GET requests
// must be rejected by the auth middleware before reaching the handler.
func TestClusterReadRoutesRequireAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	// A zero handler is fine: the auth middleware must abort before any handler
	// runs, so the handler body is never invoked for unauthenticated requests.
	InitCustomRouters(engine, &Handler{})

	root := "/" + common.PrimusRouterCustomRootPath
	for _, path := range []string{root + "/clusters", root + "/clusters/foo"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("GET %s = %d, want %d (must require authentication)", path, w.Code, http.StatusUnauthorized)
		}
	}
}

// TestEnvsRouteStaysPublic verifies /envs remains reachable without auth, since
// the login page needs it (SSO URL, feature flags) before authentication.
func TestEnvsRouteStaysPublic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	InitCustomRouters(engine, &Handler{})

	root := "/" + common.PrimusRouterCustomRootPath
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, root+"/envs", nil)
	engine.ServeHTTP(w, req)
	if w.Code == http.StatusUnauthorized {
		t.Fatalf("GET /envs = 401, but it must stay public for the login page")
	}
}