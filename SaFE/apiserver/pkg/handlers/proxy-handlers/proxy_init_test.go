/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package proxyhandlers

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// TestNewProxyHandler_FromConfig verifies the constructor builds a handler from config.
func TestNewProxyHandler_FromConfig(t *testing.T) {
	handler, err := NewProxyHandler()
	require.NoError(t, err)
	require.NotNil(t, handler)
	assert.NotNil(t, handler.proxies)
}

// TestInitProxyRoutes verifies routes are registered for each configured proxy prefix.
func TestInitProxyRoutes(t *testing.T) {
	handler := &ProxyHandler{proxies: make(map[string]*proxyConfig)}
	require.NoError(t, handler.addProxy(commonconfig.ProxyService{
		Name:    "svc1",
		Prefix:  "/api/svc1",
		Target:  "http://localhost:8080",
		Enabled: true,
	}))

	engine := gin.New()
	InitProxyRoutes(engine, handler)

	var found bool
	for _, r := range engine.Routes() {
		if r.Path == "/api/svc1/*proxyPath" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected proxy route to be registered")
}

// TestInitProxyRoutesEmpty verifies no routes are registered when there are no proxies.
func TestInitProxyRoutesEmpty(t *testing.T) {
	handler := &ProxyHandler{proxies: make(map[string]*proxyConfig)}
	engine := gin.New()
	InitProxyRoutes(engine, handler)
	assert.Empty(t, engine.Routes())
}
