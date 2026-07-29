// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package router

import (
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
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
	MountRoutes(engine, srv, basePath)
	return engine
}

// doRPC issues a JSON-RPC POST against the MCP Streamable HTTP endpoint
// (the base path itself per the 2025-03-26 spec) and decodes the response.
func doRPC(t *testing.T, ts *httptest.Server, basePath string, payload any, headers map[string]string) (int, map[string]any) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+strings.TrimRight(basePath, "/"), bytes.NewReader(body))
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

// TestMCPRoutes_DefaultBasePath verifies the standard MCP endpoints are
// reachable through Gin when mounted under the default /mcp prefix:
//   - SSE transport: GET /mcp/sse and GET /mcp both open SSE streams,
//     POST /mcp/messages receives client-to-server JSON-RPC.
//   - Streamable HTTP transport: POST /mcp.
//   - Auxiliary: GET /mcp/health, GET /mcp/info.
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
		{"info", http.MethodGet, "/mcp/info", nil, http.StatusOK},
		{"health", http.MethodGet, "/mcp/health", nil, http.StatusOK},
		{"streamable HTTP accepts POST at base", http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`), http.StatusOK},
		{"GET on base opens SSE stream (Cursor compat)", http.MethodGet, "/mcp", nil, http.StatusOK},
		{"POST on /sse handled as Streamable HTTP (Cursor compat)", http.MethodPost, "/mcp/sse", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"ping"}`), http.StatusOK},
		{"messages rejects missing session", http.MethodPost, "/mcp/messages", strings.NewReader("{}"), http.StatusBadRequest},
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

	want := basePath + "/messages?session_id="
	if !strings.Contains(string(buf[:n]), want) {
		t.Fatalf("sse endpoint event did not contain %q; got: %q", want, string(buf[:n]))
	}
}

// TestMCPRoutes_GETBaseOpensSSE is a regression test for Cursor's MCP client
// (and any other client that treats the configured base URL as an SSE
// endpoint). Such clients GET the base path and expect an SSE stream whose
// first "endpoint" event tells them where to POST messages. Returning 405 on
// GET base would break those clients with "Session not found" because they'd
// never get a session_id.
func TestMCPRoutes_GETBaseOpensSSE(t *testing.T) {
	srv := mcpserver.New()
	const basePath = "/mcp"
	ts := httptest.NewServer(newTestEngineWithMCP(t, srv, basePath))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+basePath, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET base: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET base status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}

	buf := make([]byte, 1024)
	n, _ := resp.Body.Read(buf)
	cancel()

	want := basePath + "/messages?session_id="
	if !strings.Contains(string(buf[:n]), want) {
		t.Fatalf("endpoint event missing %q; got: %q", want, string(buf[:n]))
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

// --- merged from handlers_wireup_test.go ---

// TestHandlersGo_WiresMCPRouter locks down the MCP wire-up inside the
// apiserver's handlers.go so the regression introduced by PR #511 (merge
// commit 3175930d, which silently dropped the mcprouter import + InitRoutes
// call when merging an out-of-date branch back into main) cannot reoccur.
//
// The follow-up fix 73834fa4 "restore MCP config..." only restored the
// commonconfig helpers (IsMCPEnable, GetMCPBasePath, ...) but missed the
// call site in handlers.go, leaving every freshly built apiserver image
// silently shipping without any MCP routes under mcp.base_path despite
// mcp.enabled=true in the Helm chart.
//
// The test lives here (not in pkg/handlers) so it can run without pulling in
// the apiserver's containers/storage dependency tree (libbtrfs-dev). It uses
// go/ast static analysis of handlers.go to assert:
//  1. the mcprouter import is present,
//  2. InitHttpHandlers checks commonconfig.IsMCPEnable(),
//  3. InitHttpHandlers calls mcprouter.InitRoutes(engine) inside that guard.
func TestHandlersGo_WiresMCPRouter(t *testing.T) {
	const (
		mcpRouterImport = "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/mcp/router"
		funcName        = "InitHttpHandlers"
		gateFn          = "IsMCPEnable"
		initFn          = "InitRoutes"
	)

	handlersPath := locateHandlersGo(t)

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, handlersPath, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %s: %v", handlersPath, err)
	}

	var (
		hasMCPImport bool
		mcpAlias     string
	)
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if path == mcpRouterImport {
			hasMCPImport = true
			if imp.Name != nil {
				mcpAlias = imp.Name.Name
			} else {
				mcpAlias = "router"
			}
			break
		}
	}
	if !hasMCPImport {
		t.Fatalf("handlers.go must import %q (regression: PR #511 dropped it; restore the mcprouter import)", mcpRouterImport)
	}

	var initFunc *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Name.Name == funcName {
			initFunc = fn
			break
		}
	}
	if initFunc == nil {
		t.Fatalf("function %s not found in handlers.go", funcName)
	}

	var (
		sawGate    bool
		initInGate bool
	)
	ast.Inspect(initFunc, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		if !isCallSelector(ifStmt.Cond, "commonconfig", gateFn) {
			return true
		}
		sawGate = true
		ast.Inspect(ifStmt.Body, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}
			if isCallSelector(call, mcpAlias, initFn) {
				initInGate = true
				return false
			}
			return true
		})
		return true
	})

	if !sawGate {
		t.Errorf("InitHttpHandlers must guard MCP wire-up with commonconfig.%s()", gateFn)
	}
	if !initInGate {
		t.Errorf("InitHttpHandlers must call %s.%s(engine) inside the commonconfig.%s() gate (regression: PR #511 dropped it)", mcpAlias, initFn, gateFn)
	}
}

// locateHandlersGo resolves the absolute path to apiserver/pkg/handlers/handlers.go
// from this test file, independent of the caller's cwd, so both `go test ./...`
// in apiserver/ and `go test ./pkg/mcp/router/...` work the same.
func locateHandlersGo(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	// thisFile = .../apiserver/pkg/mcp/router/handlers_wireup_test.go
	// target   = .../apiserver/pkg/handlers/handlers.go
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "handlers", "handlers.go")
}

// isCallSelector reports whether expr is a call like pkg.fn(...).
func isCallSelector(expr ast.Expr, pkg, fn string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == pkg && sel.Sel.Name == fn
}
