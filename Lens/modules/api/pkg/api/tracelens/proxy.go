// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tracelens

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	cpmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	tlconst "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/tracelens"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ProxyUI proxies requests to the TraceLens pod UI (Streamlit)
// Handles both HTTP and WebSocket connections
// Also handles /health endpoint for session status check
// Session data is now fetched from Control Plane database
func ProxyUI(c *gin.Context) {
	sessionID := c.Param("session_id")
	path := c.Param("path")

	// Handle health check endpoint
	if path == "/health" || path == "health" {
		proxyUIHealthCheckInternal(c, sessionID)
		return
	}

	// Get Control Plane facade
	cpFacade := cpdb.GetControlPlaneFacade()
	if cpFacade == nil {
		log.Error("Control plane not available")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "control plane not available"})
		return
	}
	facade := cpFacade.GetTraceLensSession()

	// Get session from Control Plane database
	session, err := facade.GetBySessionID(c, sessionID)
	if err != nil {
		log.Errorf("Failed to get session %s: %v", sessionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get session"})
		return
	}
	// Note: gorm may return empty struct with ID=0 instead of nil
	if session == nil || session.ID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Check session status
	if session.Status != cpmodel.SessionStatusReady {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "session not ready",
			"status":  session.Status,
			"message": session.StatusMessage,
		})
		return
	}

	// Validate pod IP and port
	if session.PodIP == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":  "session pod IP not available",
			"status": session.Status,
		})
		return
	}

	// Update last accessed timestamp
	if err := facade.UpdateLastAccessed(c, sessionID); err != nil {
		log.Warnf("Failed to update last accessed for session %s: %v", sessionID, err)
	}

	// Build target URL
	podPort := session.PodPort
	if podPort == 0 {
		podPort = int32(tlconst.DefaultPodPort)
	}
	targetHost := fmt.Sprintf("%s:%d", session.PodIP, podPort)

	// Check if this is a WebSocket upgrade request
	if isWebSocketUpgrade(c.Request) {
		proxyWebSocket(c, targetHost, path, sessionID)
		return
	}

	// Proxy HTTP request
	proxyHTTP(c, targetHost, path, sessionID)
}

// isWebSocketUpgrade checks if the request is a WebSocket upgrade
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Upgrade")) == "websocket" &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// proxyHTTP proxies HTTP requests to the TraceLens pod
func proxyHTTP(c *gin.Context, targetHost, path, sessionID string) {
	targetURL := &url.URL{
		Scheme: "http",
		Host:   targetHost,
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Custom director to modify the request
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// Build the full path including the base URL path that Streamlit expects
		// Note: This must match BASE_URL_PATH env var in pod (without /api prefix)
		basePath := fmt.Sprintf("/v1/tracelens/sessions/%s/ui", sessionID)
		req.URL.Path = basePath + path
		req.URL.RawPath = basePath + path

		// Preserve the original host header for proper routing
		req.Host = targetHost

		log.Debugf("Proxying HTTP request to %s%s", targetHost, req.URL.Path)
	}

	// Custom error handler
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Errorf("Proxy error for session %s: %v", sessionID, err)
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(fmt.Sprintf(`{"error": "proxy error", "message": "%s"}`, err.Error())))
	}

	// Custom transport with timeouts
	proxy.Transport = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	}

	// Modify response to handle any necessary header adjustments
	proxy.ModifyResponse = func(resp *http.Response) error {
		// Remove security headers that might conflict
		resp.Header.Del("X-Frame-Options")

		// Modify host-config response to add custom allowed origins
		if strings.HasSuffix(path, "/_stcore/host-config") {
			if err := modifyHostConfig(resp, c.Request); err != nil {
				log.Warnf("Failed to modify host-config response: %v", err)
			}
		}
		return nil
	}

	// Serve the request
	proxy.ServeHTTP(c.Writer, c.Request)
}

// proxyWebSocket proxies WebSocket connections to the TraceLens pod
func proxyWebSocket(c *gin.Context, targetHost, path, sessionID string) {
	// Get requested subprotocols from client
	requestedProtocols := websocket.Subprotocols(c.Request)

	// WebSocket upgrader for client connection
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for now
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		Subprotocols:    requestedProtocols, // Pass through client's requested subprotocols
	}

	// Build backend WebSocket URL first, before upgrading client
	// Note: This must match BASE_URL_PATH env var in pod (without /api prefix)
	basePath := fmt.Sprintf("/v1/tracelens/sessions/%s/ui", sessionID)
	backendURL := fmt.Sprintf("ws://%s%s%s", targetHost, basePath, path)

	// Copy headers for backend connection
	requestHeader := http.Header{}
	for key, values := range c.Request.Header {
		// Skip hop-by-hop headers (these will be handled by the dialer)
		if key == "Upgrade" || key == "Connection" || key == "Sec-Websocket-Key" ||
			key == "Sec-Websocket-Version" || key == "Sec-Websocket-Extensions" ||
			key == "Sec-Websocket-Protocol" {
			continue
		}
		for _, value := range values {
			requestHeader.Add(key, value)
		}
	}

	// Connect to backend first to get the negotiated subprotocol
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		Subprotocols:     requestedProtocols, // Request same subprotocols from backend
	}
	backendConn, backendResp, err := dialer.Dial(backendURL, requestHeader)
	if err != nil {
		log.Errorf("Failed to connect to backend WebSocket for session %s: %v", sessionID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to connect to backend"})
		return
	}
	defer backendConn.Close()

	// Get the subprotocol negotiated with backend
	negotiatedProtocol := ""
	if backendResp != nil {
		negotiatedProtocol = backendResp.Header.Get("Sec-Websocket-Protocol")
	}

	// Update upgrader with the negotiated subprotocol
	if negotiatedProtocol != "" {
		upgrader.Subprotocols = []string{negotiatedProtocol}
	}

	// Now upgrade client connection with the negotiated subprotocol
	clientConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Errorf("Failed to upgrade client WebSocket for session %s: %v", sessionID, err)
		return
	}
	defer clientConn.Close()

	log.Debugf("WebSocket proxy established for session %s: client <-> %s (subprotocol: %s)", sessionID, backendURL, negotiatedProtocol)

	// Error channel to track both connections
	errChan := make(chan error, 2)

	// Client -> Backend
	go copyWebSocket(backendConn, clientConn, errChan)

	// Backend -> Client
	go copyWebSocket(clientConn, backendConn, errChan)

	// Wait for either connection to close
	<-errChan
	log.Debugf("WebSocket proxy closed for session %s", sessionID)
}

// copyWebSocket copies messages from src to dst WebSocket connection
func copyWebSocket(dst, src *websocket.Conn, errChan chan<- error) {
	for {
		messageType, message, err := src.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				// Only log if it's not a normal close
				if err != io.EOF {
					log.Debugf("WebSocket read error: %v", err)
				}
			}
			errChan <- err
			return
		}

		if err := dst.WriteMessage(messageType, message); err != nil {
			if err != io.EOF {
				log.Debugf("WebSocket write error: %v", err)
			}
			errChan <- err
			return
		}
	}
}

// proxyUIHealthCheckInternal handles health check requests internally
func proxyUIHealthCheckInternal(c *gin.Context, sessionID string) {
	// Get Control Plane facade
	cpFacade := cpdb.GetControlPlaneFacade()
	if cpFacade == nil {
		log.Error("Control plane not available")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "control plane not available"})
		return
	}
	facade := cpFacade.GetTraceLensSession()

	// Get session from Control Plane database
	session, err := facade.GetBySessionID(c, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get session"})
		return
	}
	// Note: gorm may return empty struct with ID=0 instead of nil
	if session == nil || session.ID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"session_id":   sessionID,
		"cluster_name": session.ClusterName,
		"status":       session.Status,
		"pod_ip":       session.PodIP,
		"pod_port":     session.PodPort,
		"ready":        session.Status == cpmodel.SessionStatusReady,
	})
}

// modifyHostConfig modifies the Streamlit host-config response to add custom allowed origins
// This is necessary because Streamlit's default allowedOrigins doesn't include custom domains
func modifyHostConfig(resp *http.Response, req *http.Request) error {
	// Read the response body
	var bodyReader io.Reader = resp.Body
	var isGzipped bool

	// Check if response is gzipped
	if resp.Header.Get("Content-Encoding") == "gzip" {
		isGzipped = true
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		bodyReader = gzReader
	}

	body, err := io.ReadAll(bodyReader)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}
	resp.Body.Close()

	// Parse the JSON response
	var hostConfig map[string]interface{}
	if err := json.Unmarshal(body, &hostConfig); err != nil {
		// Not JSON, return original body
		resp.Body = io.NopCloser(bytes.NewReader(body))
		return nil
	}

	// Get the request origin to add to allowed origins
	origin := req.Header.Get("Origin")
	if origin == "" {
		// Try to construct from Host header
		scheme := "https"
		if req.TLS == nil {
			scheme = "http"
		}
		host := req.Host
		if host != "" {
			origin = fmt.Sprintf("%s://%s", scheme, host)
		}
	}

	// Get existing allowed origins
	allowedOrigins, ok := hostConfig["allowedOrigins"].([]interface{})
	if !ok {
		allowedOrigins = []interface{}{}
	}

	// Allow all origins since the outer layer has authentication
	// This prevents CORS issues when accessing from any domain
	allowedOrigins = append(allowedOrigins, "*")
	if origin != "" {
		allowedOrigins = append(allowedOrigins, origin)
	}
	hostConfig["allowedOrigins"] = allowedOrigins

	// Marshal back to JSON
	modifiedBody, err := json.Marshal(hostConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal modified host config: %w", err)
	}

	// Update response
	if isGzipped {
		// Re-compress the response
		var buf bytes.Buffer
		gzWriter := gzip.NewWriter(&buf)
		if _, err := gzWriter.Write(modifiedBody); err != nil {
			return fmt.Errorf("failed to gzip response: %w", err)
		}
		gzWriter.Close()
		modifiedBody = buf.Bytes()
	}

	resp.Body = io.NopCloser(bytes.NewReader(modifiedBody))
	resp.ContentLength = int64(len(modifiedBody))
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(modifiedBody)))

	return nil
}
