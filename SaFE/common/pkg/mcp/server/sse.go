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

	"github.com/google/uuid"
	"k8s.io/klog/v2"
)

// SSETransport implements the MCP SSE transport protocol.
type SSETransport struct {
	server       *Server
	sessions     sync.Map
	sessionCount int64

	HeartbeatInterval time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration

	// MessageEndpointPath is the absolute URL path returned to clients via the SSE
	// "endpoint" event so they know where to POST JSON-RPC messages. Defaults to
	// "/mcp/message" but should be overridden when mounted under a custom prefix.
	MessageEndpointPath string

	// SendQueueTimeout caps how long HandleMessage blocks trying to enqueue a
	// JSON-RPC response to a slow consumer. On expiry the request fails with
	// 503 instead of silently dropping the response.
	SendQueueTimeout time.Duration

	// AllowedOrigins is the CORS allowlist for browser-based MCP clients.
	// Empty (default) means same-origin only: no Access-Control-Allow-Origin
	// header is emitted, so cross-origin fetch from arbitrary sites is
	// blocked. Add explicit origins (e.g. "https://app.example.com") to
	// opt in. The wildcard "*" is supported but discouraged because
	// requests bearing cookies will then be rejected by browsers anyway,
	// and any future flip to Allow-Credentials would open a CSRF hole.
	AllowedOrigins []string
}

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

func NewSSETransport(server *Server) *SSETransport {
	return &SSETransport{
		server:              server,
		HeartbeatInterval:   30 * time.Second,
		ReadTimeout:         60 * time.Second,
		WriteTimeout:        10 * time.Second,
		MessageEndpointPath: "/mcp/message",
		SendQueueTimeout:    5 * time.Second,
	}
}

// Handler returns an http.Handler that serves the SSE transport on absolute paths
// /sse, /message, /health. Kept for standalone usage; framework integrations
// should call HandleSSE / HandleMessage / HandleHealth directly so the mount
// path is controlled by the framework router.
func (t *SSETransport) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", t.HandleSSE)
	mux.HandleFunc("/message", t.HandleMessage)
	mux.HandleFunc("/health", t.HandleHealth)
	return mux
}

// HandleSSE handles the GET /sse stream open request.
func (t *SSETransport) HandleSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

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

	klog.Infof("MCP SSE: New session %s from %s", sessionID, r.RemoteAddr)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	t.applyCORS(w, r)
	w.Header().Set("X-Session-ID", sessionID)

	endpointPath := t.MessageEndpointPath
	if endpointPath == "" {
		endpointPath = "/mcp/message"
	}
	messageEndpoint := fmt.Sprintf("%s?session_id=%s", endpointPath, sessionID)
	t.sendEvent(session, "endpoint", messageEndpoint)

	go t.heartbeat(session)

	for {
		select {
		case <-ctx.Done():
			klog.Infof("MCP SSE: Session %s closed (context done)", sessionID)
			goto cleanup
		case msg := <-session.messages:
			if err := t.sendEvent(session, "message", string(msg)); err != nil {
				klog.Errorf("MCP SSE: Failed to send message to session %s: %v", sessionID, err)
				goto cleanup
			}
		}
	}

cleanup:
	t.sessions.Delete(sessionID)
	atomic.AddInt64(&t.sessionCount, -1)
	cancel()
	klog.Infof("MCP SSE: Session %s cleaned up", sessionID)
}

// HandleMessage handles POST /message?session_id=... JSON-RPC messages.
func (t *SSETransport) HandleMessage(w http.ResponseWriter, r *http.Request) {
	t.applyCORS(w, r)
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "Missing session_id", http.StatusBadRequest)
		return
	}
	sessionVal, ok := t.sessions.Load(sessionID)
	if !ok {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	session := sessionVal.(*sseSession)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	klog.V(4).Infof("MCP SSE: Received message for session %s: %s", sessionID, string(body))

	session.mu.Lock()
	session.lastActive = time.Now()
	session.mu.Unlock()

	ctx := ContextWithHTTPRequest(session.ctx, r)
	response, err := t.server.HandleMessage(ctx, body)
	if err != nil {
		klog.Errorf("MCP SSE: Failed to handle message: %v", err)
		http.Error(w, "Failed to handle message", http.StatusInternalServerError)
		return
	}

	if response != nil {
		// Bounded wait so a slow / stalled SSE consumer can't silently drop
		// JSON-RPC responses (the previous non-blocking send would return 202
		// with the response thrown away). On overload, fail loudly with 503
		// so the client can retry instead of hanging on a never-arriving id.
		timeout := t.SendQueueTimeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case session.messages <- response:
		case <-r.Context().Done():
			return
		case <-session.ctx.Done():
			http.Error(w, "Session closed", http.StatusGone)
			return
		case <-timer.C:
			klog.Errorf("MCP SSE: Dropping response for session %s after %s: queue full", sessionID, timeout)
			http.Error(w, "Session message queue is full", http.StatusServiceUnavailable)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"accepted"}`))
}

// HandleHealth returns transport-level health info.
func (t *SSETransport) HandleHealth(w http.ResponseWriter, r *http.Request) {
	t.applyCORS(w, r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	resp := map[string]any{
		"status":      "ok",
		"sessions":    atomic.LoadInt64(&t.sessionCount),
		"tools":       t.server.ToolCount(),
		"initialized": t.server.IsInitialized(),
	}
	json.NewEncoder(w).Encode(resp)
}

// writeCORS sets Access-Control-Allow-Origin only when the inbound Origin
// matches the allowlist (or "*" is explicitly configured). Empty allowlist =
// same-origin only, so no header is emitted and browsers fall back to
// same-origin enforcement. Vary: Origin is set whenever the allowlist is
// non-empty to avoid cache poisoning.
func writeCORS(w http.ResponseWriter, r *http.Request, allowed []string) {
	if len(allowed) == 0 {
		return
	}
	w.Header().Add("Vary", "Origin")
	origin := r.Header.Get("Origin")
	for _, a := range allowed {
		if a == "*" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			return
		}
		if origin != "" && origin == a {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			return
		}
	}
}

func (t *SSETransport) applyCORS(w http.ResponseWriter, r *http.Request) {
	writeCORS(w, r, t.AllowedOrigins)
}

func (t *StreamableHTTPTransport) applyCORS(w http.ResponseWriter, r *http.Request) {
	writeCORS(w, r, t.AllowedOrigins)
}

func (t *SSETransport) sendEvent(session *sseSession, event, data string) error {
	session.mu.Lock()
	defer session.mu.Unlock()

	var sb strings.Builder
	if event != "" {
		sb.WriteString("event: ")
		sb.WriteString(event)
		sb.WriteString("\n")
	}
	for _, line := range strings.Split(data, "\n") {
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

func (t *SSETransport) heartbeat(session *sseSession) {
	ticker := time.NewTicker(t.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-session.ctx.Done():
			return
		case <-ticker.C:
			if err := t.sendEvent(session, "ping", ""); err != nil {
				klog.V(4).Infof("MCP SSE: Heartbeat failed for session %s: %v", session.id, err)
				session.cancel()
				return
			}
		}
	}
}

func (t *SSETransport) SessionCount() int64 {
	return atomic.LoadInt64(&t.sessionCount)
}

func (t *SSETransport) CloseSession(sessionID string) {
	if sessionVal, ok := t.sessions.Load(sessionID); ok {
		session := sessionVal.(*sseSession)
		session.cancel()
	}
}

func (t *SSETransport) CloseAllSessions() {
	t.sessions.Range(func(key, value any) bool {
		session := value.(*sseSession)
		session.cancel()
		return true
	})
}

func (t *SSETransport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.Handler().ServeHTTP(w, r)
}

func (t *SSETransport) StartStandalone(ctx context.Context, addr string) error {
	srv := &http.Server{Addr: addr, Handler: t.Handler()}
	go func() {
		<-ctx.Done()
		klog.Info("MCP SSE: Shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()
	klog.Infof("MCP SSE: Starting server on %s", addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// StreamableHTTPTransport provides a simpler HTTP request/response transport.
type StreamableHTTPTransport struct {
	server *Server

	// AllowedOrigins behaves the same way as SSETransport.AllowedOrigins.
	// Empty = same-origin only. See SSETransport.applyCORS for details.
	AllowedOrigins []string
}

func NewStreamableHTTPTransport(server *Server) *StreamableHTTPTransport {
	return &StreamableHTTPTransport{server: server}
}

func (t *StreamableHTTPTransport) Handler() http.Handler {
	return http.HandlerFunc(t.HandleRPC)
}

// HandleRPC processes a single JSON-RPC POST request and writes the JSON response.
func (t *StreamableHTTPTransport) HandleRPC(w http.ResponseWriter, r *http.Request) {
	t.applyCORS(w, r)
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	ctx := ContextWithHTTPRequest(r.Context(), r)
	response, err := t.server.HandleMessage(ctx, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if response != nil {
		w.Write(response)
	}
}

// StreamableHTTPClient is a simple client for the streamable HTTP transport.
// Headers is optional and applied to every outgoing request, useful for tests
// or scripts that need to attach Authorization etc.
type StreamableHTTPClient struct {
	BaseURL    string
	HTTPClient *http.Client
	Headers    map[string]string
}

func NewStreamableHTTPClient(baseURL string) *StreamableHTTPClient {
	return &StreamableHTTPClient{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *StreamableHTTPClient) Call(ctx context.Context, method string, params any) (*JSONRPCResponse, error) {
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range c.Headers {
		httpReq.Header.Set(k, v)
	}

	httpResp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

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

func NewSSEClient(baseURL string) *SSEClient {
	return &SSEClient{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 0},
	}
}

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

func (c *SSEClient) SessionID() string {
	return c.sessionID
}
