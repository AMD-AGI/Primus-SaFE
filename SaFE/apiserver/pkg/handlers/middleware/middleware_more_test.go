/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestToNullString(t *testing.T) {
	assert.False(t, toNullString(nil).Valid)
	assert.False(t, toNullString("").Valid)
	assert.False(t, toNullString(123).Valid)
	v := toNullString("hello")
	assert.True(t, v.Valid)
	assert.Equal(t, "hello", v.String)
}

func TestToStringValue(t *testing.T) {
	assert.Equal(t, "", toStringValue(nil))
	assert.Equal(t, "", toStringValue(42))
	assert.Equal(t, "x", toStringValue("x"))
}

func TestIsProxyRoute(t *testing.T) {
	assert.True(t, isProxyRoute("/api/v1/llm-proxy/chat"))
	assert.False(t, isProxyRoute("/api/v1/workloads"))
}

func TestPreprocess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: common.Name, Value: "  myname  "}}

	Preprocess()(c)
	assert.Equal(t, "myname", c.GetString(common.Name))
}

func TestAuthorizeMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	nextCalled := false
	engine.GET("/p", Authorize(), func(c *gin.Context) {
		nextCalled = true
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/p", nil)
	engine.ServeHTTP(w, req)
	// No credentials -> ParseToken fails -> request aborted.
	assert.NotEqual(t, http.StatusOK, w.Code)
	assert.False(t, nextCalled)
}

func TestResponseBodyWriter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	rw := &responseBodyWriter{
		ResponseWriter: c.Writer,
		body:           &bytes.Buffer{},
		traceId:        "trace-123",
	}
	rw.WriteHeader(http.StatusOK)
	assert.Equal(t, "trace-123", w.Header().Get("X-Trace-Id"))

	n, err := rw.Write([]byte("hello"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", rw.body.String())
}

func TestHttpHeaderCarrier(t *testing.T) {
	h := http.Header{}
	carrier := &httpHeaderCarrier{header: h}
	carrier.Set("X-Foo", "bar")
	assert.Equal(t, "bar", carrier.Get("X-Foo"))
	assert.Contains(t, carrier.Keys(), "X-Foo")
}

func TestHandleTracingMiddlewares(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mw := range []gin.HandlerFunc{HandleTracing(), HandleTracingAll(), HandleTracingErrorOnly()} {
		engine := gin.New()
		engine.Use(mw)
		engine.GET("/t", func(c *gin.Context) { c.Status(http.StatusOK) })
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}
}
