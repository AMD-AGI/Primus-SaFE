package perfetto

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	pftconst "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/perfetto"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ProxyUI proxies requests to the Perfetto pod UI (nginx serving static files)
// Handles both HTTP and WebSocket connections
func ProxyUI(c *gin.Context) {
	sessionID := c.Param("session_id")
	path := c.Param("path")

	// Handle health check endpoint
	if path == "/health" || path == "health" {
		proxyUIHealthCheckInternal(c, sessionID)
		return
	}

	// Get cluster
	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	// Get session from database
	facade := database.GetFacadeForCluster(clients.ClusterName).GetTraceLensSession()
	session, err := facade.GetBySessionID(c, sessionID)
	if err != nil {
		log.Errorf("Failed to get session %s: %v", sessionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get session"})
		return
	}
	if session == nil || session.ID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Check session status
	if session.Status != pftconst.StatusReady {
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
		podPort = int32(pftconst.DefaultPodPort)
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

// proxyHTTP proxies HTTP requests to the Perfetto pod
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

		// For Perfetto, we proxy directly to the path without base path manipulation
		// The nginx in the pod serves static files at root
		req.URL.Path = path
		if req.URL.Path == "" || req.URL.Path == "/" {
			req.URL.Path = "/"
		}
		req.URL.RawPath = req.URL.Path

		// Preserve the original host header for proper routing
		req.Host = targetHost

		log.Debugf("Proxying Perfetto HTTP request to %s%s", targetHost, req.URL.Path)
	}

	// Custom error handler
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Errorf("Perfetto proxy error for session %s: %v", sessionID, err)
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
		// Remove security headers that might conflict with iframe embedding
		resp.Header.Del("X-Frame-Options")
		return nil
	}

	// Serve the request
	proxy.ServeHTTP(c.Writer, c.Request)
}

// proxyWebSocket proxies WebSocket connections to the Perfetto pod
func proxyWebSocket(c *gin.Context, targetHost, path, sessionID string) {
	// Get requested subprotocols from client
	requestedProtocols := websocket.Subprotocols(c.Request)

	// WebSocket upgrader for client connection
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		Subprotocols:    requestedProtocols,
	}

	// Build backend WebSocket URL
	backendURL := fmt.Sprintf("ws://%s%s", targetHost, path)

	// Copy headers for backend connection
	requestHeader := http.Header{}
	for key, values := range c.Request.Header {
		if key == "Upgrade" || key == "Connection" || key == "Sec-Websocket-Key" ||
			key == "Sec-Websocket-Version" || key == "Sec-Websocket-Extensions" ||
			key == "Sec-Websocket-Protocol" {
			continue
		}
		for _, value := range values {
			requestHeader.Add(key, value)
		}
	}

	// Connect to backend
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		Subprotocols:     requestedProtocols,
	}
	backendConn, backendResp, err := dialer.Dial(backendURL, requestHeader)
	if err != nil {
		log.Errorf("Failed to connect to backend WebSocket for Perfetto session %s: %v", sessionID, err)
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

	// Now upgrade client connection
	clientConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Errorf("Failed to upgrade client WebSocket for Perfetto session %s: %v", sessionID, err)
		return
	}
	defer clientConn.Close()

	log.Debugf("Perfetto WebSocket proxy established for session %s: client <-> %s", sessionID, backendURL)

	// Error channel to track both connections
	errChan := make(chan error, 2)

	// Client -> Backend
	go copyWebSocket(backendConn, clientConn, errChan)

	// Backend -> Client
	go copyWebSocket(clientConn, backendConn, errChan)

	// Wait for either connection to close
	<-errChan
	log.Debugf("Perfetto WebSocket proxy closed for session %s", sessionID)
}

// copyWebSocket copies messages from src to dst WebSocket connection
func copyWebSocket(dst, src *websocket.Conn, errChan chan<- error) {
	for {
		messageType, message, err := src.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				if err != io.EOF {
					log.Debugf("Perfetto WebSocket read error: %v", err)
				}
			}
			errChan <- err
			return
		}

		if err := dst.WriteMessage(messageType, message); err != nil {
			if err != io.EOF {
				log.Debugf("Perfetto WebSocket write error: %v", err)
			}
			errChan <- err
			return
		}
	}
}

// proxyUIHealthCheckInternal handles health check requests internally
func proxyUIHealthCheckInternal(c *gin.Context, sessionID string) {
	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	facade := database.GetFacadeForCluster(clients.ClusterName).GetTraceLensSession()
	session, err := facade.GetBySessionID(c, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get session"})
		return
	}
	if session == nil || session.ID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"session_id": sessionID,
		"status":     session.Status,
		"pod_ip":     session.PodIP,
		"pod_port":   session.PodPort,
		"ready":      session.Status == pftconst.StatusReady,
	})
}

