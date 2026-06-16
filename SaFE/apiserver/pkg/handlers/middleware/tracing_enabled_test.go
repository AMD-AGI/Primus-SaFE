/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// exercise drives an engine with success, error, and proxy routes.
func exerciseTracing(t *testing.T, mw gin.HandlerFunc) {
	t.Helper()
	engine := gin.New()
	engine.Use(mw)
	engine.GET("/ok", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	engine.GET("/err", func(c *gin.Context) {
		_ = c.Error(errors.New("boom"))
		c.String(http.StatusInternalServerError, strings.Repeat("e", maxResponseBodySize+50))
	})
	engine.GET("/api/v1/llm-proxy/chat", func(c *gin.Context) { c.String(http.StatusOK, "p") })

	for _, p := range []string{"/ok", "/err", "/api/v1/llm-proxy/chat"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, nil)
		engine.ServeHTTP(w, req)
	}
}

func TestHandleTracingAllEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	commonconfig.SetValue("tracing.enable", "true")
	defer commonconfig.SetValue("tracing.enable", "false")
	exerciseTracing(t, HandleTracingAll())
}

func TestHandleTracingErrorOnlyEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	commonconfig.SetValue("tracing.enable", "true")
	defer commonconfig.SetValue("tracing.enable", "false")
	exerciseTracing(t, HandleTracingErrorOnly())
}

func TestHandleTracingDispatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	commonconfig.SetValue("tracing.enable", "true")
	defer commonconfig.SetValue("tracing.enable", "false")

	commonconfig.SetValue("tracing.mode", "all")
	exerciseTracing(t, HandleTracing())

	commonconfig.SetValue("tracing.mode", "error_only")
	exerciseTracing(t, HandleTracing())
	commonconfig.SetValue("tracing.mode", "")
}
