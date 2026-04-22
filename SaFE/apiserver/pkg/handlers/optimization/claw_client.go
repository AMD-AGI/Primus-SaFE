/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

// Reasonable timeouts for Claw's control-plane REST endpoints. Streaming reads
// use a separate client without a per-request timeout because Hyperloom runs
// can stretch to 60+ minutes.
const (
	clawControlTimeout = 30 * time.Second
	clawStreamTimeout  = 0 // 0 = no timeout; context cancels the stream

	clawPathSessions        = "/sessions"
	clawPathSessionMessage  = "/sessions/%s/messages"
	clawPathStream          = "/chat/sessions/%s/messages"
	clawPathInterrupt       = "/chat/sessions/%s/interrupt"
	clawPathSessionFiles    = "/sessions/%s/files"
	clawPathSessionDownload = "/sessions/%s/files/%s/download"
	clawPathSessionStream   = "/sessions/%s/files/%s/stream"

	clawKeepalivePrefix = ":"
	clawDataPrefix      = "data:"
	clawEventPrefix     = "event:"
	clawIDPrefix        = "id:"
)

// ClawClient is a thin HTTP + SSE client for the PrimusClaw backend. It is
// intentionally lean — just enough surface for the optimization handler to
// create sessions, send the Hyperloom prompt, and stream events back.
type ClawClient struct {
	baseURL       string
	apiKey        string
	controlClient *http.Client
	streamClient  *http.Client
}

// NewClawClient wires a client for the given base URL. The base URL should be
// the Claw v1 root, e.g. "https://.../claw-api/v1". apiKey is an optional
// default Bearer; per-request values override via WithClawBearer on context.
func NewClawClient(baseURL, apiKey string) *ClawClient {
	transport := &http.Transport{
		// Claw is often fronted by self-signed / internal certs; reuse the
		// same relaxed policy as the LiteLLM client.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // nolint:gosec
	}
	return &ClawClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		controlClient: &http.Client{
			Timeout:   clawControlTimeout,
			Transport: transport,
		},
		streamClient: &http.Client{
			Timeout:   clawStreamTimeout,
			Transport: transport,
		},
	}
}

// SessionRequest mirrors Claw's POST /sessions body.
type SessionRequest struct {
	Name         string                 `json:"name,omitempty"`
	AgentID      string                 `json:"agent_id,omitempty"`
	SystemPrompt string                 `json:"system_prompt,omitempty"`
	Config       map[string]interface{} `json:"config,omitempty"`
}

// SessionResponse wraps the common response envelope for session-creation.
// Claw returns { code, data: { session_id, ... }, request_id }. We parse
// defensively: fall back to top-level session_id if the server changes shape.
type SessionResponse struct {
	SessionID string `json:"session_id"`
}

// MessageRequest maps to POST /sessions/{id}/messages.
type MessageRequest struct {
	Content     string                   `json:"content,omitempty"`
	Contents    []MessageContent         `json:"contents,omitempty"`
	MessageType string                   `json:"messageType,omitempty"`
	TaskMode    string                   `json:"taskMode,omitempty"`
	Attachments []map[string]interface{} `json:"attachments,omitempty"`
	Tools       []int                    `json:"tools,omitempty"`
	ExtData     map[string]interface{}   `json:"extData,omitempty"`
	WorkspaceID string                   `json:"workspaceId,omitempty"`
}

// MessageContent is a single segment in a multi-part message payload.
type MessageContent struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// ClawSSEEvent is the minimally-parsed wire representation of a server-sent
// event from Claw. Subscribers interpret Data according to Event.
type ClawSSEEvent struct {
	ID    string
	Event string
	Data  string
}

// ClawArtifact mirrors the session file metadata returned by PrimusClaw.
type ClawArtifact struct {
	Path         string `json:"path"`
	Run          *int   `json:"run,omitempty"`
	Size         int64  `json:"size,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
}

// CreateSession creates a new Claw session. AgentID defaults to the value
// configured for the Model Optimization feature if empty.
func (c *ClawClient) CreateSession(ctx context.Context, req *SessionRequest) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal session request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+clawPathSessions, bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}
	c.applyHeaders(httpReq)

	resp, err := c.controlClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("claw POST /sessions: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		klog.ErrorS(nil, "claw: create session failed",
			"status", resp.StatusCode, "body", truncate(string(raw), 512))
		return "", fmt.Errorf("claw returned HTTP %d: %s", resp.StatusCode, truncate(string(raw), 512))
	}

	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
		// Older Claw versions return fields at the top level.
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", fmt.Errorf("claw create session: parse response: %w", err)
	}
	if envelope.SessionID != "" {
		return envelope.SessionID, nil
	}
	var data SessionResponse
	if len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			return "", fmt.Errorf("claw create session: parse data: %w", err)
		}
	}
	if data.SessionID == "" {
		return "", fmt.Errorf("claw create session: empty session id in response: %s", truncate(string(raw), 256))
	}
	return data.SessionID, nil
}

// SendMessage fires a message into an existing Claw session. This is a
// fire-and-forget request — actual model output comes back via Stream.
func (c *ClawClient) SendMessage(ctx context.Context, sessionID string, req *MessageRequest) error {
	if sessionID == "" {
		return fmt.Errorf("claw send message: empty session id")
	}
	if req.MessageType == "" {
		req.MessageType = "text"
	}
	if req.TaskMode == "" {
		req.TaskMode = "agent"
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal message request: %w", err)
	}

	url := c.baseURL + fmt.Sprintf(clawPathSessionMessage, sessionID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.applyHeaders(httpReq)

	resp, err := c.controlClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("claw POST /messages: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		klog.ErrorS(nil, "claw: send message failed",
			"status", resp.StatusCode, "session_id", sessionID, "body", truncate(string(raw), 512))
		return fmt.Errorf("claw returned HTTP %d: %s", resp.StatusCode, truncate(string(raw), 512))
	}
	// Drain so the connection can be reused.
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// Stream subscribes to a Claw session's event stream. onEvent is invoked once
// per parsed event until ctx is cancelled or the upstream closes. When
// afterEventID is non-empty the server will replay history starting right
// after that id, enabling resume-from-breakpoint.
func (c *ClawClient) Stream(
	ctx context.Context,
	sessionID string,
	afterEventID string,
	onEvent func(ClawSSEEvent) error,
) error {
	if sessionID == "" {
		return fmt.Errorf("claw stream: empty session id")
	}

	url := c.baseURL + fmt.Sprintf(clawPathStream, sessionID)
	if afterEventID != "" {
		url += "?after_event_id=" + afterEventID
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	c.applyHeaders(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.streamClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("claw GET stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("claw stream HTTP %d: %s", resp.StatusCode, truncate(string(raw), 512))
	}

	return parseSSEStream(ctx, resp.Body, onEvent)
}

// DeleteSession best-effort removes a Claw session. Failures are logged but
// not fatal — a dangling session will time out upstream eventually.
func (c *ClawClient) DeleteSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	url := c.baseURL + clawPathSessions + "/" + sessionID
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	c.applyHeaders(httpReq)

	resp, err := c.controlClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		raw, _ := io.ReadAll(resp.Body)
		klog.Warningf("claw: delete session %s returned HTTP %d: %s",
			sessionID, resp.StatusCode, truncate(string(raw), 256))
	}
	return nil
}

// InterruptSession asks Claw to stop the currently running task inside the
// session's executor sandbox. The session itself remains alive.
func (c *ClawClient) InterruptSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("claw interrupt: empty session id")
	}
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+fmt.Sprintf(clawPathInterrupt, sessionID), bytes.NewReader([]byte(`{}`)),
	)
	if err != nil {
		return err
	}
	c.applyHeaders(req)
	resp, err := c.controlClient.Do(req)
	if err != nil {
		return fmt.Errorf("claw interrupt request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("claw interrupt HTTP %d: %s", resp.StatusCode, truncate(string(raw), 256))
	}
	return nil
}

// ListSessionFiles returns the flattened session artifact list Claw stores in S3.
func (c *ClawClient) ListSessionFiles(ctx context.Context, sessionID string) ([]ClawArtifact, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("claw list files: empty session id")
	}
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, c.baseURL+fmt.Sprintf(clawPathSessionFiles, sessionID), nil,
	)
	if err != nil {
		return nil, err
	}
	c.applyHeaders(req)
	resp, err := c.controlClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claw list files request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("claw list files HTTP %d: %s", resp.StatusCode, truncate(string(raw), 256))
	}

	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("claw list files parse envelope: %w", err)
	}
	var items []ClawArtifact
	if len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, &items); err != nil {
			return nil, fmt.Errorf("claw list files parse items: %w", err)
		}
	}
	return items, nil
}

// ReadSessionFile downloads the content of a single session artifact by its
// session-relative path (e.g. "claw-1/optimization_report.md").
func (c *ClawClient) ReadSessionFile(ctx context.Context, sessionID, filePath string) ([]byte, error) {
	if sessionID == "" || filePath == "" {
		return nil, fmt.Errorf("claw read file: session id and file path are required")
	}
	escaped := escapeClawFilePath(filePath)
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, c.baseURL+fmt.Sprintf(clawPathSessionStream, sessionID, escaped), nil,
	)
	if err != nil {
		return nil, err
	}
	c.applyHeaders(req)
	resp, err := c.streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claw read file request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("claw read file HTTP %d: %s", resp.StatusCode, truncate(string(raw), 256))
	}
	return raw, nil
}

// DownloadProxyPath returns the Claw backend's own download proxy path for a
// given session artifact, which can be useful for clients that prefer to hit
// Claw directly. SaFE's /artifacts endpoint instead proxies the bytes itself.
func (c *ClawClient) DownloadProxyPath(ctx context.Context, sessionID, filePath string) (string, error) {
	if sessionID == "" || filePath == "" {
		return "", fmt.Errorf("claw download path: session id and file path are required")
	}
	escaped := escapeClawFilePath(filePath)
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, c.baseURL+fmt.Sprintf(clawPathSessionDownload, sessionID, escaped), nil,
	)
	if err != nil {
		return "", err
	}
	c.applyHeaders(req)
	resp, err := c.controlClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("claw download path request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("claw download path HTTP %d: %s", resp.StatusCode, truncate(string(raw), 256))
	}
	var env struct {
		Data struct {
			APIPath string `json:"api_path"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", fmt.Errorf("claw download path parse: %w", err)
	}
	return env.Data.APIPath, nil
}

// applyHeaders sets the authorization + content-type headers used by all
// endpoints on the Claw control plane. Per-request bearer (via WithClawBearer
// on req.Context()) overrides the static apiKey from configuration.
func (c *ClawClient) applyHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	bearer := ""
	if req != nil {
		bearer = clawBearerFromContext(req.Context())
	}
	if bearer == "" {
		bearer = c.apiKey
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
}

// parseSSEStream reads a `text/event-stream` body, assembling multi-line
// data fields into ClawSSEEvent values. Comment lines (beginning with ':')
// act as keepalives and are silently dropped. The parser terminates on ctx
// cancellation, upstream EOF, or a non-nil error returned by onEvent.
func parseSSEStream(
	ctx context.Context,
	body io.Reader,
	onEvent func(ClawSSEEvent) error,
) error {
	scanner := bufio.NewScanner(body)
	// SSE messages occasionally carry very large tool_use payloads. Bump the
	// per-line buffer so we don't truncate and cause parse errors downstream.
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	var current ClawSSEEvent
	var dataBuf strings.Builder

	flush := func() error {
		if dataBuf.Len() == 0 && current.ID == "" && current.Event == "" {
			return nil
		}
		current.Data = dataBuf.String()
		err := onEvent(current)
		current = ClawSSEEvent{}
		dataBuf.Reset()
		return err
	}

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := scanner.Text()

		switch {
		case line == "":
			if err := flush(); err != nil {
				return err
			}
		case strings.HasPrefix(line, clawKeepalivePrefix):
			// Keepalive comment — ignore.
		case strings.HasPrefix(line, clawIDPrefix):
			current.ID = strings.TrimSpace(strings.TrimPrefix(line, clawIDPrefix))
		case strings.HasPrefix(line, clawEventPrefix):
			current.Event = strings.TrimSpace(strings.TrimPrefix(line, clawEventPrefix))
		case strings.HasPrefix(line, clawDataPrefix):
			chunk := strings.TrimPrefix(line, clawDataPrefix)
			chunk = strings.TrimLeft(chunk, " ")
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(chunk)
		default:
			// Unknown prefix — skip rather than fail.
		}
	}
	// Flush any trailing event that did not end with a blank line.
	if err := flush(); err != nil {
		return err
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return err
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func escapeClawFilePath(filePath string) string {
	parts := strings.Split(strings.ReplaceAll(filePath, "\\", "/"), "/")
	escaped := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		escaped = append(escaped, url.PathEscape(p))
	}
	return strings.Join(escaped, "/")
}
