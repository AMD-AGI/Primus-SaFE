// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	mcpserver "github.com/AMD-AIG-AIMA/SAFE/common/pkg/mcp/server"
)

// newTestEngineWithMCP returns a fresh Gin engine that has the given MCP server
// mounted under basePath plus a minimal fake REST backend at /api/v1/echo that
// echoes back the headers it received. Tests use the same engine as both the
// MCP entrypoint and the REST backend so APICall loops back over loopback.
func newTestEngineWithMCP(t *testing.T, srv *mcpserver.Server, basePath string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/api/v1/echo", func(c *gin.Context) {
		hdrs := map[string]string{}
		for k, v := range c.Request.Header {
			if len(v) > 0 {
				hdrs[k] = v[0]
			}
		}
		c.JSON(http.StatusOK, gin.H{"headers": hdrs, "ok": true})
	})
	mountMCPRoutes(engine, srv, basePath)
	return engine
}

// doRPC issues a JSON-RPC POST against the MCP /rpc endpoint and decodes the response.
func doRPC(t *testing.T, ts *httptest.Server, basePath string, payload any, headers map[string]string) (int, map[string]any) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+strings.TrimRight(basePath, "/")+"/rpc", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) == 0 {
		return resp.StatusCode, nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode response %q: %v", string(raw), err)
	}
	return resp.StatusCode, out
}

// TestMCPRoutes_DefaultBasePath verifies the four routes are reachable through Gin
// when mounted under the default /mcp prefix.
func TestMCPRoutes_DefaultBasePath(t *testing.T) {
	srv := mcpserver.New()
	ts := httptest.NewServer(newTestEngineWithMCP(t, srv, "/mcp"))
	defer ts.Close()

	cases := []struct {
		name   string
		method string
		path   string
		body   io.Reader
		want   int
	}{
		{"index", http.MethodGet, "/mcp/", nil, http.StatusOK},
		{"health", http.MethodGet, "/mcp/health", nil, http.StatusOK},
		{"rpc accepts POST", http.MethodPost, "/mcp/rpc", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`), http.StatusOK},
		{"message rejects missing session", http.MethodPost, "/mcp/message", strings.NewReader("{}"), http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, err := http.NewRequest(c.method, ts.URL+c.path, c.body)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			if c.body != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("do request: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != c.want {
				t.Fatalf("%s: status = %d, want %d", c.path, resp.StatusCode, c.want)
			}
		})
	}
}

// TestMCPRoutes_CustomBasePath verifies a custom mount path works end-to-end and
// that the SSE "endpoint" event reflects that path (regression for the
// previously hardcoded /mcp/message string).
func TestMCPRoutes_CustomBasePath(t *testing.T) {
	srv := mcpserver.New()
	const basePath = "/api/v2/mcp"
	ts := httptest.NewServer(newTestEngineWithMCP(t, srv, basePath))
	defer ts.Close()

	resp, err := http.Get(ts.URL + basePath + "/health")
	if err != nil {
		t.Fatalf("GET health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("custom base health: status = %d", resp.StatusCode)
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+basePath+"/sse", nil)
	if err != nil {
		t.Fatalf("new sse request: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)

	sseResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET sse: %v", err)
	}
	defer sseResp.Body.Close()
	if sseResp.StatusCode != http.StatusOK {
		t.Fatalf("sse status = %d", sseResp.StatusCode)
	}

	buf := make([]byte, 1024)
	n, _ := sseResp.Body.Read(buf)
	cancel()

	want := basePath + "/message?session_id="
	if !strings.Contains(string(buf[:n]), want) {
		t.Fatalf("sse endpoint event did not contain %q; got: %q", want, string(buf[:n]))
	}
}

// TestMCPRoutes_RPCPing exercises the full RPC path through Gin and confirms
// the JSON-RPC envelope is returned.
func TestMCPRoutes_RPCPing(t *testing.T) {
	srv := mcpserver.New()
	ts := httptest.NewServer(newTestEngineWithMCP(t, srv, "/mcp"))
	defer ts.Close()

	status, out := doRPC(t, ts, "/mcp", map[string]any{
		"jsonrpc": "2.0",
		"id":      "abc",
		"method":  "ping",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if got := out["jsonrpc"]; got != "2.0" {
		t.Fatalf("jsonrpc = %v, want 2.0", got)
	}
	if got := out["id"]; got != "abc" {
		t.Fatalf("id = %v, want abc", got)
	}
}

// TestMCPRoutes_AuthHeaderForwarded registers a synthetic MCP tool that calls
// our /api/v1/echo backend and asserts that an Authorization header sent on the
// MCP RPC request is propagated all the way through APICall.
func TestMCPRoutes_AuthHeaderForwarded(t *testing.T) {
	srv := mcpserver.New()
	srv.RegisterTool(&mcpserver.MCPTool{
		Name:        "echo_headers",
		Description: "test tool that hits the loopback echo backend",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		Handler: func(ctx context.Context, _ json.RawMessage) (any, error) {
			inReq, _ := mcpserver.HTTPRequestFromContext(ctx)
			if inReq == nil {
				return nil, nil
			}
			url := "http://" + inReq.Host + "/api/v1/echo"
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return nil, err
			}
			if v := inReq.Header.Get("Authorization"); v != "" {
				req.Header.Set("Authorization", v)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			var parsed map[string]any
			_ = json.Unmarshal(b, &parsed)
			return parsed, nil
		},
	})

	ts := httptest.NewServer(newTestEngineWithMCP(t, srv, "/mcp"))
	defer ts.Close()

	const token = "Bearer test-token-xyz"
	status, out := doRPC(t, ts, "/mcp", map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]any{"name": "echo_headers", "arguments": map[string]any{}},
	}, map[string]string{"Authorization": token})

	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %v", status, out)
	}

	result, _ := out["result"].(map[string]any)
	if result == nil {
		t.Fatalf("missing result: %v", out)
	}
	contentSlice, _ := result["content"].([]any)
	if len(contentSlice) == 0 {
		t.Fatalf("empty content: %v", result)
	}
	first, _ := contentSlice[0].(map[string]any)
	text, _ := first["text"].(string)
	if !strings.Contains(text, token) {
		t.Fatalf("expected Authorization %q to be forwarded; tool result text = %q", token, text)
	}
}
