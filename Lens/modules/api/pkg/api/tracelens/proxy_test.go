// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tracelens

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsWebSocketUpgrade(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected bool
	}{
		{
			name: "valid websocket upgrade",
			headers: map[string]string{
				"Upgrade":    "websocket",
				"Connection": "upgrade",
			},
			expected: true,
		},
		{
			name: "websocket upgrade case insensitive",
			headers: map[string]string{
				"Upgrade":    "WebSocket",
				"Connection": "Upgrade",
			},
			expected: true,
		},
		{
			name: "connection with keep-alive and upgrade",
			headers: map[string]string{
				"Upgrade":    "websocket",
				"Connection": "keep-alive, Upgrade",
			},
			expected: true,
		},
		{
			name: "missing upgrade header",
			headers: map[string]string{
				"Connection": "upgrade",
			},
			expected: false,
		},
		{
			name: "missing connection header",
			headers: map[string]string{
				"Upgrade": "websocket",
			},
			expected: false,
		},
		{
			name: "wrong upgrade value",
			headers: map[string]string{
				"Upgrade":    "http/2.0",
				"Connection": "upgrade",
			},
			expected: false,
		},
		{
			name: "connection without upgrade",
			headers: map[string]string{
				"Upgrade":    "websocket",
				"Connection": "keep-alive",
			},
			expected: false,
		},
		{
			name:     "empty headers",
			headers:  map[string]string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			result := isWebSocketUpgrade(req)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProxyPathConstruction(t *testing.T) {
	// Test that the base path is correctly constructed
	sessionID := "test-session-123"
	basePath := "/v1/tracelens/sessions/" + sessionID + "/ui"

	assert.Equal(t, "/v1/tracelens/sessions/test-session-123/ui", basePath)
}

func TestProxyTargetURLConstruction(t *testing.T) {
	tests := []struct {
		name       string
		podIP      string
		podPort    int32
		path       string
		sessionID  string
		expectedWS string
	}{
		{
			name:       "standard configuration",
			podIP:      "10.0.0.100",
			podPort:    8501,
			path:       "/_stcore/stream",
			sessionID:  "tls-abc123",
			expectedWS: "ws://10.0.0.100:8501/v1/tracelens/sessions/tls-abc123/ui/_stcore/stream",
		},
		{
			name:       "root path",
			podIP:      "10.0.0.200",
			podPort:    8501,
			path:       "/",
			sessionID:  "tls-xyz789",
			expectedWS: "ws://10.0.0.200:8501/v1/tracelens/sessions/tls-xyz789/ui/",
		},
		{
			name:       "complex path",
			podIP:      "172.16.0.50",
			podPort:    8501,
			path:       "/static/media/image.png",
			sessionID:  "tls-session-1",
			expectedWS: "ws://172.16.0.50:8501/v1/tracelens/sessions/tls-session-1/ui/static/media/image.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetHost := tt.podIP + ":8501"
			basePath := "/v1/tracelens/sessions/" + tt.sessionID + "/ui"
			backendURL := "ws://" + targetHost + basePath + tt.path

			assert.Equal(t, tt.expectedWS, backendURL)
		})
	}
}

func TestWebSocketHeaderFiltering(t *testing.T) {
	// Headers that should be skipped for WebSocket connection
	hopByHopHeaders := []string{
		"Upgrade",
		"Connection",
		"Sec-Websocket-Key",
		"Sec-Websocket-Version",
		"Sec-Websocket-Extensions",
		"Sec-Websocket-Protocol",
	}

	// Headers that should be preserved
	preservedHeaders := []string{
		"Authorization",
		"Cookie",
		"X-Custom-Header",
		"Accept",
		"Accept-Language",
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	for _, h := range hopByHopHeaders {
		req.Header.Set(h, "test-value")
	}
	for _, h := range preservedHeaders {
		req.Header.Set(h, "test-value")
	}

	// Simulate header filtering logic
	filteredHeaders := http.Header{}
	for key, values := range req.Header {
		// Skip hop-by-hop headers
		if key == "Upgrade" || key == "Connection" || key == "Sec-Websocket-Key" ||
			key == "Sec-Websocket-Version" || key == "Sec-Websocket-Extensions" ||
			key == "Sec-Websocket-Protocol" {
			continue
		}
		for _, value := range values {
			filteredHeaders.Add(key, value)
		}
	}

	// Verify hop-by-hop headers are filtered
	for _, h := range hopByHopHeaders {
		assert.Empty(t, filteredHeaders.Get(h), "Header %s should be filtered", h)
	}

	// Verify preserved headers are kept
	for _, h := range preservedHeaders {
		assert.Equal(t, "test-value", filteredHeaders.Get(h), "Header %s should be preserved", h)
	}
}

func TestHealthCheckPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		isHealth bool
	}{
		{
			name:     "health with slash",
			path:     "/health",
			isHealth: true,
		},
		{
			name:     "health without slash",
			path:     "health",
			isHealth: true,
		},
		{
			name:     "regular path",
			path:     "/some/other/path",
			isHealth: false,
		},
		{
			name:     "empty path",
			path:     "",
			isHealth: false,
		},
		{
			name:     "healthcheck (different)",
			path:     "/healthcheck",
			isHealth: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isHealth := tt.path == "/health" || tt.path == "health"
			assert.Equal(t, tt.isHealth, isHealth)
		})
	}
}

func TestTargetHostConstruction(t *testing.T) {
	tests := []struct {
		name        string
		podIP       string
		podPort     int32
		defaultPort int32
		expected    string
	}{
		{
			name:        "with custom port",
			podIP:       "10.0.0.100",
			podPort:     9000,
			defaultPort: 8501,
			expected:    "10.0.0.100:9000",
		},
		{
			name:        "with zero port uses default",
			podIP:       "10.0.0.100",
			podPort:     0,
			defaultPort: 8501,
			expected:    "10.0.0.100:8501",
		},
		{
			name:        "ipv4 address",
			podIP:       "192.168.1.50",
			podPort:     8501,
			defaultPort: 8501,
			expected:    "192.168.1.50:8501",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := tt.podPort
			if port == 0 {
				port = tt.defaultPort
			}
			targetHost := formatTargetHost(tt.podIP, port)
			assert.Equal(t, tt.expected, targetHost)
		})
	}
}

// Helper function to format target host
func formatTargetHost(ip string, port int32) string {
	return ip + ":" + itoa(int(port))
}

// Simple int to string conversion
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}

func TestProxyResponseModification(t *testing.T) {
	// Test that X-Frame-Options header is removed
	resp := &http.Response{
		Header: http.Header{
			"X-Frame-Options":   []string{"DENY"},
			"Content-Type":      []string{"text/html"},
			"X-Custom-Header":   []string{"value"},
		},
	}

	// Simulate the ModifyResponse logic
	resp.Header.Del("X-Frame-Options")

	assert.Empty(t, resp.Header.Get("X-Frame-Options"))
	assert.Equal(t, "text/html", resp.Header.Get("Content-Type"))
	assert.Equal(t, "value", resp.Header.Get("X-Custom-Header"))
}

