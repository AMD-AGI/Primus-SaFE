/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package proxyhandlers

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// ProxyHandler manages reverse proxy handlers for configured services
type ProxyHandler struct {
	proxies map[string]*proxyConfig
}

// proxyConfig holds configuration for a single proxy service
type proxyConfig struct {
	service commonconfig.ProxyService
	proxy   *httputil.ReverseProxy
}

// NewProxyHandler creates a new ProxyHandler with configured services
func NewProxyHandler() (*ProxyHandler, error) {
	handler := &ProxyHandler{
		proxies: make(map[string]*proxyConfig),
	}

	services := commonconfig.GetProxyServices()
	for _, service := range services {
		if !service.Enabled {
			klog.Infof("Proxy service %s is disabled, skipping", service.Name)
			continue
		}

		if err := handler.addProxy(service); err != nil {
			klog.ErrorS(err, "failed to add proxy", "service", service.Name)
			continue
		}
		klog.Infof("Added proxy service: %s, prefix: %s, target: %s", service.Name, service.Prefix, service.Target)
	}

	return handler, nil
}

// addProxy adds a reverse proxy for the given service
func (h *ProxyHandler) addProxy(service commonconfig.ProxyService) error {
	targetURL, err := url.Parse(service.Target)
	if err != nil {
		return err
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Customize the director to strip the prefix
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// Strip the prefix from the path
		req.URL.Path = strings.TrimPrefix(req.URL.Path, service.Prefix)
		if !strings.HasPrefix(req.URL.Path, "/") {
			req.URL.Path = "/" + req.URL.Path
		}

		// User ID is already set in the request header by createProxyHandler
		klog.V(4).Infof("Proxy request: %s %s -> %s", req.Method, service.Prefix, req.URL.String())
	}

	// Custom error handler
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		klog.ErrorS(err, "proxy error", "service", service.Name, "url", r.URL.String())
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("Bad Gateway"))
	}

	h.proxies[service.Prefix] = &proxyConfig{
		service: service,
		proxy:   proxy,
	}

	return nil
}

// InitProxyRoutes initializes proxy routes for all configured services
func InitProxyRoutes(engine *gin.Engine, handler *ProxyHandler) {
	for prefix, config := range handler.proxies {
		// Create a route group for this proxy
		group := engine.Group(prefix)

		// Apply authentication middleware
		group.Use(func(c *gin.Context) {
			err := authority.ParseToken(c)
			if err != nil {
				apiutils.AbortWithApiError(c, err)
				return
			}
			c.Next()
		})

		// Add the proxy handler - capture all paths under this prefix
		group.Any("/*proxyPath", handler.createProxyHandler(config))

		klog.Infof("Registered proxy route: %s/* -> %s", prefix, config.service.Target)
	}
}

// createProxyHandler creates a gin handler for the reverse proxy
func (h *ProxyHandler) createProxyHandler(config *proxyConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user ID from gin context and add to request header
		if userId, exists := c.Get(common.UserId); exists {
			if userIdStr, ok := userId.(string); ok {
				c.Request.Header.Set(common.UserId, userIdStr)
			}
		}

		// Get username from gin context and add to request header
		if userName, exists := c.Get(common.UserName); exists {
			if userNameStr, ok := userName.(string); ok {
				c.Request.Header.Set(common.UserName, userNameStr)
			}
		}

		// Serve the reverse proxy
		config.proxy.ServeHTTP(c.Writer, c.Request)
	}
}

// GetProxyInfo returns information about all configured proxies
func (h *ProxyHandler) GetProxyInfo(c *gin.Context) {
	type proxyInfo struct {
		Name    string `json:"name"`
		Prefix  string `json:"prefix"`
		Target  string `json:"target"`
		Enabled bool   `json:"enabled"`
	}

	var infos []proxyInfo
	for _, config := range h.proxies {
		infos = append(infos, proxyInfo{
			Name:    config.service.Name,
			Prefix:  config.service.Prefix,
			Target:  config.service.Target,
			Enabled: config.service.Enabled,
		})
	}

	c.JSON(200, infos)
}

// HealthCheck provides a health check endpoint for proxy services
func (h *ProxyHandler) HealthCheck(c *gin.Context) {
	serviceName := c.Param("service")

	if serviceName == "" {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("service name is required"))
		return
	}

	// Find the proxy by service name
	var found *proxyConfig
	for _, config := range h.proxies {
		if config.service.Name == serviceName {
			found = config
			break
		}
	}

	if found == nil {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage("proxy service not found"))
		return
	}

	// Simple health check - just return the service info
	c.JSON(200, gin.H{
		"service": found.service.Name,
		"status":  "active",
		"target":  found.service.Target,
	})
}
