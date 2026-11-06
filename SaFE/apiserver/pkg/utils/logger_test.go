/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"gotest.tools/assert"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
)

func TestLog(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(Logger(), gin.Recovery())
	engine.GET("/")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		assert.Error(t, err, "failed to new request")
	}
	engine.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusOK, recorder.Code)
}

// TestLoggerDoesNotFlushOnRequestPath: the middleware must not flush klog while serving a request.
// A flush syncs every log file to disk before net/http sends the response, so a slow disk delayed
// every response by seconds.
func TestLoggerDoesNotFlushOnRequestPath(t *testing.T) {
	var flushes atomic.Int32
	klog.SetLoggerWithOptions(
		textlogger.NewLogger(textlogger.NewConfig(textlogger.Output(io.Discard))),
		klog.FlushLogger(func() { flushes.Add(1) }),
	)
	defer klog.ClearLogger()

	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(Logger(), gin.Recovery())
	engine.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(http.MethodGet, "/", nil)
	assert.NilError(t, err)
	engine.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, int32(0), flushes.Load(), "the request path flushed klog")
}
