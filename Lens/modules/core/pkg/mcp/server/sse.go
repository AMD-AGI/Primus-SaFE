// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/google/uuid"
)

// SSETransport implements the MCP SSE transport protocol.
// It provides HTTP endpoints for SSE streaming and message handling.
type SSETransport struct {
	server       *Server
	sessions     sync.Map // map[string]*sseSession
	sessionCount int64

	// Configuration
	HeartbeatInterval time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
}

// sseSession represents an active SSE connection.
type sseSession struct {
	id         string
	writer     http.ResponseWriter
	flusher    http.Flusher
	ctx        context.Context
	cancel     context.CancelFunc
	messages   chan []byte
	created    time.Time
	lastActive time.Time
	mu         sync.Mutex
}

// NewSSETransport creates a new SSE transport for the given MCP server.
func NewSSETransport(server *Server) *SSETransport {
	return &SSETransport{
		server:            server,
		HeartbeatInterval: 30 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
}

// Handler returns an http.Handler that handles MCP SSE requests.
// It should be mounted at the MCP endpoint path (e.g., "/mcp").
func (t *SSETransport) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", t.handleSSE)
	mux.HandleFunc("/message", t.handleMessage)
	mux.HandleFunc("/health", t.handleHealth)
	return mux
}

// handleSSE handles the SSE connection endpoint.
// Clients connect here to receive server-sent events.
func (t *SSETransport) handleSSE(w http.ResponseWriter, r *http.Request) {
	// Only accept GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if the client supports SSE
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Create session
	sessionID := uuid.New().String()
	ctx, cancel := context.WithCancel(r.Context())

	session := &sseSession{
		id:         sessionID,
		writer:     w,
		flusher:    flusher,
		ctx:        ctx,
		cancel:     cancel,
		messages:   make(chan []byte, 100),
		created:    time.Now(),
		lastActive: time.Now(),
	}

	t.sessions.Store(sessionID, session)
	atomic.AddInt64(&t.sessionCount, 1)

	log.Infof("MCP SSE: New session %s from %s", sessionID, r.RemoteAddr)

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Session-ID", sessionID)

	// Send the endpoint event to tell client where to POST messages
	messageEndpoint := fmt.Sprintf("/mcp/message?session_id=%s", sessionID)
	t.sendEvent(session, "endpoint", messageEndpoint)

	// Start heartbeat goroutine
	go t.heartbeat(session)

	// Main event loop
	for {
		select {
		case <-ctx.Done():
			log.Infof("MCP SSE: Session %s closed (context done)", sessionID)
			goto cleanup
		case msg := <-session.messages:
			if err := t.sendEvent(session, "message", string(msg)); err != nil {
				log.Errorf("MCP SSE: Failed to send message to session %s: %v", sessionID, err)
				goto cleanup
			}
		}
	}

cleanup:
	t.sessions.Delete(sessionID)
	atomic.AddInt64(&t.sessionCount, -1)
	cancel()
	log.Infof("MCP SSE: Session %s cleaned up", sessionID)
}

// handleMessage handles incoming messages from clients.
// Clients POST JSON-RPC messages here.
func (t *SSETransport) handleMessage(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get session ID from query parameter
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "Missing session_id", http.StatusBadRequest)
		return
	}

	// Find session
	sessionVal, ok := t.sessions.Load(sessionID)
	if !ok {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	session := sessionVal.(*sseSession)

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	log.Debugf("MCP SSE: Received message for session %s: %s", sessionID, string(body))

	// Update last active time
	session.mu.Lock()
	session.lastActive = time.Now()
	session.mu.Unlock()

	// Handle the message
	response, err := t.server.HandleMessage(session.ctx, body)
	if err != nil {
		log.Errorf("MCP SSE: Failed to handle message: %v", err)
		http.Error(w, "Failed to handle message", http.StatusInternalServerError)
		return
	}

	// If there's a response, send it through the SSE channel
	if response != nil {
		select {
		case session.messages <- response:
			// Message queued successfully
		default:
			log.Warnf("MCP SSE: Message queue full for session %s", sessionID)
		}
	}

	// Acknowledge the message
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"accepted"}`))
}

// handleHealth handles health check requests.
func (t *SSETransport) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	resp := map[string]any{
		"status":        "ok",
		"sessions":      atomic.LoadInt64(&t.sessionCount),
		"tools":         t.server.ToolCount(),
		"initialized":   t.server.IsInitialized(),
	}

	json.NewEncoder(w).Encode(resp)
}

// sendEvent sends an SSE event to a session.
func (t *SSETransport) sendEvent(session *sseSession, event, data string) error {
	session.mu.Lock()
	defer session.mu.Unlock()

	// Format: event: <event>\ndata: <data>\n\n
	var sb strings.Builder
	if event != "" {
		sb.WriteString("event: ")
		sb.WriteString(event)
		sb.WriteString("\n")
	}

	// Handle multiline data
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		sb.WriteString("data: ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	_, err := session.writer.Write([]byte(sb.String()))
	if err != nil {
		return err
	}

	session.flusher.Flush()
	return nil
}

// heartbeat sends periodic heartbeat events to keep the connection alive.
func (t *SSETransport) heartbeat(session *sseSession) {
	ticker := time.NewTicker(t.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-session.ctx.Done():
			return
		case <-ticker.C:
			if err := t.sendEvent(session, "ping", ""); err != nil {
				log.Debugf("MCP SSE: Heartbeat failed for session %s: %v", session.id, err)
				session.cancel()
				return
			}
		}
	}
}

// SessionCount returns the number of active sessions.
func (t *SSETransport) SessionCount() int64 {
	return atomic.LoadInt64(&t.sessionCount)
}

// CloseSession closes a specific session.
func (t *SSETransport) CloseSession(sessionID string) {
	if sessionVal, ok := t.sessions.Load(sessionID); ok {
		session := sessionVal.(*sseSession)
		session.cancel()
	}
}

// CloseAllSessions closes all active sessions.
func (t *SSETransport) CloseAllSessions() {
	t.sessions.Range(func(key, value any) bool {
		session := value.(*sseSession)
		session.cancel()
		return true
	})
}

// ServeHTTP implements http.Handler for easy integration with existing servers.
func (t *SSETransport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.Handler().ServeHTTP(w, r)
}

// StartStandalone starts the SSE transport as a standalone HTTP server.
func (t *SSETransport) StartStandalone(ctx context.Context, addr string) error {
	server := &http.Server{
		Addr:    addr,
		Handler: t.Handler(),
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		log.Info("MCP SSE: Shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	log.Infof("MCP SSE: Starting server on %s", addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// StreamableHTTPTransport provides a simpler HTTP-based transport using
// request/response pattern (not SSE). This is easier to test and debug.
type StreamableHTTPTransport struct {
	server *Server
}

// NewStreamableHTTPTransport creates a new streamable HTTP transport.
func NewStreamableHTTPTransport(server *Server) *StreamableHTTPTransport {
	return &StreamableHTTPTransport{server: server}
}

// Handler returns an http.Handler for the streamable HTTP transport.
func (t *StreamableHTTPTransport) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Read request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// Handle the message
		response, err := t.server.HandleMessage(r.Context(), body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Send response
		w.Header().Set("Content-Type", "application/json")
		if response != nil {
			w.Write(response)
		}
	})
}

// StreamableHTTPClient is a simple client for the streamable HTTP transport.
// This is useful for testing.
type StreamableHTTPClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewStreamableHTTPClient creates a new client.
func NewStreamableHTTPClient(baseURL string) *StreamableHTTPClient {
	return &StreamableHTTPClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Call sends a JSON-RPC request and returns the response.
func (c *StreamableHTTPClient) Call(ctx context.Context, method string, params any) (*JSONRPCResponse, error) {
	// Create request
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	req := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`1`),
		Method:  method,
		Params:  paramsJSON,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// Send request
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	// Parse response
	var resp JSONRPCResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// SSEClient is a simple client for testing SSE transport.
type SSEClient struct {
	BaseURL    string
	HTTPClient *http.Client
	sessionID  string
}

// NewSSEClient creates a new SSE client.
func NewSSEClient(baseURL string) *SSEClient {
	return &SSEClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 0, // No timeout for SSE
		},
	}
}

// Connect establishes an SSE connection and returns a channel for receiving events.
func (c *SSEClient) Connect(ctx context.Context) (<-chan string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/sse", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	c.sessionID = resp.Header.Get("X-Session-ID")

	events := make(chan string, 100)

	go func() {
		defer resp.Body.Close()
		defer close(events)

		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}

			// Parse SSE event
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				data = strings.TrimSuffix(data, "\n")
				select {
				case events <- data:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return events, nil
}

// SendMessage sends a message to the server.
func (c *SSEClient) SendMessage(ctx context.Context, method string, params any) error {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return err
	}

	req := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`1`),
		Method:  method,
		Params:  paramsJSON,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/message?session_id=%s", c.BaseURL, c.sessionID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(reqBody)))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status: %d", httpResp.StatusCode)
	}

	return nil
}

// SessionID returns the current session ID.
func (c *SSEClient) SessionID() string {
	return c.sessionID
}
