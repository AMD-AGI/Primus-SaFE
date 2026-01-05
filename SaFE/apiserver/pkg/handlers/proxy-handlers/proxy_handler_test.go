/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package proxyhandlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// TestNewProxyHandler tests the creation of ProxyHandler
func TestNewProxyHandler(t *testing.T) {
	tests := []struct {
		name          string
		services      []commonconfig.ProxyService
		expectedCount int
	}{
		{
			name:          "no services",
			services:      []commonconfig.ProxyService{},
			expectedCount: 0,
		},
		{
			name: "single enabled service",
			services: []commonconfig.ProxyService{
				{
					Name:    "test-service",
					Prefix:  "/api/test",
					Target:  "http://localhost:8080",
					Enabled: true,
				},
			},
			expectedCount: 1,
		},
		{
			name: "disabled service",
			services: []commonconfig.ProxyService{
				{
					Name:    "disabled-service",
					Prefix:  "/api/disabled",
					Target:  "http://localhost:8080",
					Enabled: false,
				},
			},
			expectedCount: 0,
		},
		{
			name: "multiple services",
			services: []commonconfig.ProxyService{
				{
					Name:    "service1",
					Prefix:  "/api/service1",
					Target:  "http://localhost:8081",
					Enabled: true,
				},
				{
					Name:    "service2",
					Prefix:  "/api/service2",
					Target:  "http://localhost:8082",
					Enabled: true,
				},
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &ProxyHandler{
				proxies: make(map[string]*proxyConfig),
			}

			for _, service := range tt.services {
				if service.Enabled {
					err := handler.addProxy(service)
					assert.NoError(t, err)
				}
			}

			assert.Equal(t, tt.expectedCount, len(handler.proxies))
		})
	}
}

// TestAddProxy tests adding a proxy service
func TestAddProxy(t *testing.T) {
	tests := []struct {
		name        string
		service     commonconfig.ProxyService
		expectError bool
	}{
		{
			name: "valid service",
			service: commonconfig.ProxyService{
				Name:    "test-service",
				Prefix:  "/api/test",
				Target:  "http://localhost:8080",
				Enabled: true,
			},
			expectError: false,
		},
		{
			name: "invalid target URL",
			service: commonconfig.ProxyService{
				Name:    "invalid-service",
				Prefix:  "/api/invalid",
				Target:  "://invalid-url",
				Enabled: true,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &ProxyHandler{
				proxies: make(map[string]*proxyConfig),
			}

			err := handler.addProxy(tt.service)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, handler.proxies[tt.service.Prefix])
			}
		})
	}
}

// TestCreateProxyHandler tests the proxy handler creation
func TestCreateProxyHandler(t *testing.T) {
	// Track if backend received the request
	backendCalled := false
	receivedPath := ""
	receivedUserId := ""
	receivedUserName := ""

	// Create a test backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendCalled = true
		receivedPath = r.URL.Path
		receivedUserId = r.Header.Get(common.UserId)
		receivedUserName = r.Header.Get(common.UserName)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"success"}`))
	}))
	defer backend.Close()

	// Create proxy handler
	handler := &ProxyHandler{
		proxies: make(map[string]*proxyConfig),
	}

	service := commonconfig.ProxyService{
		Name:    "test-service",
		Prefix:  "/api/test",
		Target:  backend.URL,
		Enabled: true,
	}
	err := handler.addProxy(service)
	assert.NoError(t, err)

	// Create test request using real HTTP client
	router := gin.New()
	router.GET("/api/test/*proxyPath", func(c *gin.Context) {
		// Simulate authentication middleware setting userId and userName
		c.Set(common.UserId, "test-user-123")
		c.Set(common.UserName, "Test User")
		handler.createProxyHandler(handler.proxies["/api/test"])(c)
	})

	// Start test server
	testServer := httptest.NewServer(router)
	defer testServer.Close()

	// Make real HTTP request
	resp, err := http.Get(testServer.URL + "/api/test/endpoint")
	assert.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, backendCalled)
	assert.Equal(t, "/endpoint", receivedPath)
	assert.Equal(t, "test-user-123", receivedUserId)
	assert.Equal(t, "Test User", receivedUserName)
}

// TestPrefixStripping tests that URL prefixes are correctly stripped
func TestPrefixStripping(t *testing.T) {
	tests := []struct {
		name         string
		prefix       string
		requestPath  string
		expectedPath string
	}{
		{
			name:         "simple prefix",
			prefix:       "/api/test",
			requestPath:  "/api/test/endpoint",
			expectedPath: "/endpoint",
		},
		{
			name:         "nested path",
			prefix:       "/agent/qa",
			requestPath:  "/agent/qa/v1/chat",
			expectedPath: "/v1/chat",
		},
		{
			name:         "root after prefix",
			prefix:       "/api",
			requestPath:  "/api/health",
			expectedPath: "/health",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test backend to verify the path
			receivedPath := ""
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedPath = r.URL.Path
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			// Create proxy handler
			handler := &ProxyHandler{
				proxies: make(map[string]*proxyConfig),
			}

			service := commonconfig.ProxyService{
				Name:    "test-service",
				Prefix:  tt.prefix,
				Target:  backend.URL,
				Enabled: true,
			}
			err := handler.addProxy(service)
			assert.NoError(t, err)

			// Create test router and server
			router := gin.New()
			router.GET(tt.prefix+"/*proxyPath", func(c *gin.Context) {
				c.Set(common.UserId, "test-user")
				handler.createProxyHandler(handler.proxies[tt.prefix])(c)
			})

			testServer := httptest.NewServer(router)
			defer testServer.Close()

			// Make real HTTP request
			resp, err := http.Get(testServer.URL + tt.requestPath)
			assert.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, tt.expectedPath, receivedPath)
		})
	}
}

// TestUserIdHeaderInjection tests that userId and userName are properly injected into headers
func TestUserIdHeaderInjection(t *testing.T) {
	tests := []struct {
		name             string
		setUserId        bool
		userId           string
		setUserName      bool
		userName         string
		expectedUserId   string
		expectedUserName string
	}{
		{
			name:             "userId and userName present",
			setUserId:        true,
			userId:           "user-123",
			setUserName:      true,
			userName:         "John Doe",
			expectedUserId:   "user-123",
			expectedUserName: "John Doe",
		},
		{
			name:             "only userId present",
			setUserId:        true,
			userId:           "user-456",
			setUserName:      false,
			userName:         "",
			expectedUserId:   "user-456",
			expectedUserName: "",
		},
		{
			name:             "userId empty string",
			setUserId:        true,
			userId:           "",
			setUserName:      true,
			userName:         "Jane Doe",
			expectedUserId:   "",
			expectedUserName: "Jane Doe",
		},
		{
			name:             "neither set",
			setUserId:        false,
			userId:           "",
			setUserName:      false,
			userName:         "",
			expectedUserId:   "",
			expectedUserName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receivedUserId := ""
			receivedUserName := ""
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedUserId = r.Header.Get(common.UserId)
				receivedUserName = r.Header.Get(common.UserName)
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			handler := &ProxyHandler{
				proxies: make(map[string]*proxyConfig),
			}

			service := commonconfig.ProxyService{
				Name:    "test-service",
				Prefix:  "/api/test",
				Target:  backend.URL,
				Enabled: true,
			}
			err := handler.addProxy(service)
			assert.NoError(t, err)

			router := gin.New()
			router.GET("/api/test/*proxyPath", func(c *gin.Context) {
				if tt.setUserId {
					c.Set(common.UserId, tt.userId)
				}
				if tt.setUserName {
					c.Set(common.UserName, tt.userName)
				}
				handler.createProxyHandler(handler.proxies["/api/test"])(c)
			})

			testServer := httptest.NewServer(router)
			defer testServer.Close()

			resp, err := http.Get(testServer.URL + "/api/test/endpoint")
			assert.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, tt.expectedUserId, receivedUserId)
			assert.Equal(t, tt.expectedUserName, receivedUserName)
		})
	}
}

// TestProxyErrorHandling tests error handling when backend is unavailable
func TestProxyErrorHandling(t *testing.T) {
	handler := &ProxyHandler{
		proxies: make(map[string]*proxyConfig),
	}

	// Use an invalid target that will fail
	service := commonconfig.ProxyService{
		Name:    "test-service",
		Prefix:  "/api/test",
		Target:  "http://localhost:99999", // Invalid port
		Enabled: true,
	}
	err := handler.addProxy(service)
	assert.NoError(t, err)

	router := gin.New()
	router.GET("/api/test/*proxyPath", func(c *gin.Context) {
		c.Set(common.UserId, "test-user")
		handler.createProxyHandler(handler.proxies["/api/test"])(c)
	})

	testServer := httptest.NewServer(router)
	defer testServer.Close()

	resp, err := http.Get(testServer.URL + "/api/test/endpoint")
	assert.NoError(t, err)
	defer resp.Body.Close()

	// Should return Bad Gateway
	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
}

// TestGetProxyInfo tests the proxy info endpoint
func TestGetProxyInfo(t *testing.T) {
	handler := &ProxyHandler{
		proxies: make(map[string]*proxyConfig),
	}

	services := []commonconfig.ProxyService{
		{
			Name:    "service1",
			Prefix:  "/api/service1",
			Target:  "http://localhost:8081",
			Enabled: true,
		},
		{
			Name:    "service2",
			Prefix:  "/api/service2",
			Target:  "http://localhost:8082",
			Enabled: true,
		},
	}

	for _, service := range services {
		err := handler.addProxy(service)
		assert.NoError(t, err)
	}

	router := gin.New()
	router.GET("/proxy/info", handler.GetProxyInfo)

	req := httptest.NewRequest("GET", "/proxy/info", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body, _ := io.ReadAll(w.Body)

	// Verify response contains both services
	assert.Contains(t, string(body), "service1")
	assert.Contains(t, string(body), "service2")
}

// TestHealthCheck tests the health check endpoint
func TestHealthCheck(t *testing.T) {
	handler := &ProxyHandler{
		proxies: make(map[string]*proxyConfig),
	}

	service := commonconfig.ProxyService{
		Name:    "test-service",
		Prefix:  "/api/test",
		Target:  "http://localhost:8080",
		Enabled: true,
	}
	err := handler.addProxy(service)
	assert.NoError(t, err)

	tests := []struct {
		name         string
		serviceName  string
		expectedCode int
	}{
		{
			name:         "existing service",
			serviceName:  "test-service",
			expectedCode: http.StatusOK,
		},
		{
			name:         "non-existing service",
			serviceName:  "non-existing",
			expectedCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/health/:service", handler.HealthCheck)

			path := "/health/" + tt.serviceName
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}

// TestHTTPMethods tests that all HTTP methods are properly proxied
func TestHTTPMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			receivedMethod := ""
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedMethod = r.Method
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			handler := &ProxyHandler{
				proxies: make(map[string]*proxyConfig),
			}

			service := commonconfig.ProxyService{
				Name:    "test-service",
				Prefix:  "/api/test",
				Target:  backend.URL,
				Enabled: true,
			}
			err := handler.addProxy(service)
			assert.NoError(t, err)

			router := gin.New()
			router.Any("/api/test/*proxyPath", func(c *gin.Context) {
				c.Set(common.UserId, "test-user")
				handler.createProxyHandler(handler.proxies["/api/test"])(c)
			})

			testServer := httptest.NewServer(router)
			defer testServer.Close()

			req, _ := http.NewRequest(method, testServer.URL+"/api/test/endpoint", nil)
			client := &http.Client{}
			resp, err := client.Do(req)
			assert.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, method, receivedMethod)
		})
	}
}
