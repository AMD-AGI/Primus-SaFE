// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestToolRequest struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type TestToolResponse struct {
	Message string `json:"message"`
	Total   int    `json:"total"`
}

func createTestTool() *MCPTool {
	return &MCPTool{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":  map[string]any{"type": "string"},
				"count": map[string]any{"type": "integer"},
			},
		},
		Handler: func(ctx context.Context, params json.RawMessage) (any, error) {
			var req TestToolRequest
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, err
			}
			return &TestToolResponse{
				Message: "Hello " + req.Name,
				Total:   req.Count * 2,
			}, nil
		},
	}
}

func mustMarshal(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

func TestServer_Initialize(t *testing.T) {
	server := New()
	server.RegisterTool(createTestTool())

	req := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`1`),
		Method:  MethodInitialize,
		Params: mustMarshal(InitializeParams{
			ProtocolVersion: MCPProtocolVersion,
			Capabilities:    ClientCapability{},
			ClientInfo:      Implementation{Name: "Test Client", Version: "1.0.0"},
		}),
	}

	resp := server.HandleRequest(context.Background(), &req)
	require.NotNil(t, resp)
	assert.Nil(t, resp.Error)

	var result InitializeResult
	resultBytes, _ := json.Marshal(resp.Result)
	err := json.Unmarshal(resultBytes, &result)
	require.NoError(t, err)

	assert.Equal(t, MCPProtocolVersion, result.ProtocolVersion)
	assert.NotNil(t, result.Capabilities.Tools)
	assert.Equal(t, ServerInfo.Name, result.ServerInfo.Name)
	assert.True(t, server.IsInitialized())
}

func TestServer_ToolsList(t *testing.T) {
	server := New()
	server.RegisterTool(createTestTool())

	req := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`2`),
		Method:  MethodToolsList,
	}

	resp := server.HandleRequest(context.Background(), &req)
	require.NotNil(t, resp)
	assert.Nil(t, resp.Error)

	var result ToolsListResult
	resultBytes, _ := json.Marshal(resp.Result)
	err := json.Unmarshal(resultBytes, &result)
	require.NoError(t, err)

	assert.Len(t, result.Tools, 1)
	assert.Equal(t, "test_tool", result.Tools[0].Name)
}

func TestServer_ToolsCall(t *testing.T) {
	server := New()
	server.RegisterTool(createTestTool())

	req := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`3`),
		Method:  MethodToolsCall,
		Params: mustMarshal(ToolsCallParams{
			Name:      "test_tool",
			Arguments: map[string]any{"name": "World", "count": 5},
		}),
	}

	resp := server.HandleRequest(context.Background(), &req)
	require.NotNil(t, resp)
	assert.Nil(t, resp.Error)

	var result ToolsCallResult
	resultBytes, _ := json.Marshal(resp.Result)
	err := json.Unmarshal(resultBytes, &result)
	require.NoError(t, err)

	assert.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	assert.Equal(t, "text", result.Content[0].Type)

	var toolResult TestToolResponse
	err = json.Unmarshal([]byte(result.Content[0].Text), &toolResult)
	require.NoError(t, err)
	assert.Equal(t, "Hello World", toolResult.Message)
	assert.Equal(t, 10, toolResult.Total)
}

func TestServer_ToolsCall_NotFound(t *testing.T) {
	server := New()

	req := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`4`),
		Method:  MethodToolsCall,
		Params:  mustMarshal(ToolsCallParams{Name: "nonexistent_tool"}),
	}

	resp := server.HandleRequest(context.Background(), &req)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Error)
	assert.Equal(t, ErrorCodeToolNotFound, resp.Error.Code)
}

func TestServer_MethodNotFound(t *testing.T) {
	server := New()
	req := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`5`),
		Method:  "unknown/method",
	}
	resp := server.HandleRequest(context.Background(), &req)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Error)
	assert.Equal(t, ErrorCodeMethodNotFound, resp.Error.Code)
}

func TestServer_Ping(t *testing.T) {
	server := New()
	req := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`6`),
		Method:  MethodPing,
	}
	resp := server.HandleRequest(context.Background(), &req)
	require.NotNil(t, resp)
	assert.Nil(t, resp.Error)
}

func TestServer_HandleMessage(t *testing.T) {
	server := New()
	server.RegisterTool(createTestTool())

	message := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	response, err := server.HandleMessage(context.Background(), []byte(message))
	require.NoError(t, err)
	require.NotNil(t, response)

	var resp JSONRPCResponse
	err = json.Unmarshal(response, &resp)
	require.NoError(t, err)
	assert.Nil(t, resp.Error)
}

func TestServer_HandleMessage_ParseError(t *testing.T) {
	server := New()
	response, err := server.HandleMessage(context.Background(), []byte(`{invalid json}`))
	require.NoError(t, err)
	require.NotNil(t, response)

	var resp JSONRPCResponse
	err = json.Unmarshal(response, &resp)
	require.NoError(t, err)
	require.NotNil(t, resp.Error)
	assert.Equal(t, ErrorCodeParseError, resp.Error.Code)
}

func TestStreamableHTTPTransport(t *testing.T) {
	server := New()
	server.RegisterTool(createTestTool())

	transport := NewStreamableHTTPTransport(server)
	ts := httptest.NewServer(transport.Handler())
	defer ts.Close()

	req := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`1`),
		Method:  MethodToolsList,
	}
	reqBody, _ := json.Marshal(req)

	resp, err := http.Post(ts.URL, "application/json", bytes.NewReader(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var rpcResp JSONRPCResponse
	err = json.NewDecoder(resp.Body).Decode(&rpcResp)
	require.NoError(t, err)
	assert.Nil(t, rpcResp.Error)
}

func TestStreamableHTTPClient(t *testing.T) {
	server := New()
	server.RegisterTool(createTestTool())

	transport := NewStreamableHTTPTransport(server)
	ts := httptest.NewServer(transport.Handler())
	defer ts.Close()

	client := NewStreamableHTTPClient(ts.URL)

	resp, err := client.Call(context.Background(), MethodInitialize, InitializeParams{
		ProtocolVersion: MCPProtocolVersion,
		ClientInfo:      Implementation{Name: "Test", Version: "1.0"},
	})
	require.NoError(t, err)
	assert.Nil(t, resp.Error)

	resp, err = client.Call(context.Background(), MethodToolsList, nil)
	require.NoError(t, err)
	assert.Nil(t, resp.Error)

	resp, err = client.Call(context.Background(), MethodToolsCall, ToolsCallParams{
		Name:      "test_tool",
		Arguments: map[string]any{"name": "Test", "count": 3},
	})
	require.NoError(t, err)
	assert.Nil(t, resp.Error)
}

func TestSTDIOTransport(t *testing.T) {
	server := New()
	server.RegisterTool(createTestTool())

	input := &bytes.Buffer{}
	output := &bytes.Buffer{}
	transport := NewSTDIOTransportWithIO(server, input, output)

	messages := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"Test","version":"1.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	}
	for _, msg := range messages {
		input.WriteString(msg + "\n")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_ = transport.Start(ctx)

	outputStr := output.String()
	lines := strings.Split(strings.TrimSpace(outputStr), "\n")
	require.GreaterOrEqual(t, len(lines), 1)

	var resp1 JSONRPCResponse
	err := json.Unmarshal([]byte(lines[0]), &resp1)
	require.NoError(t, err)
	assert.Nil(t, resp1.Error)
}

func TestSSETransport_Health(t *testing.T) {
	server := New()
	server.RegisterTool(createTestTool())

	transport := NewSSETransport(server)
	ts := httptest.NewServer(transport.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var health map[string]any
	err = json.NewDecoder(resp.Body).Decode(&health)
	require.NoError(t, err)
	assert.Equal(t, "ok", health["status"])
}

func TestServer_RegisterMultipleTools(t *testing.T) {
	server := New()
	tools := []*MCPTool{
		{
			Name: "tool1", Description: "First tool",
			InputSchema: map[string]any{"type": "object"},
			Handler:     func(ctx context.Context, params json.RawMessage) (any, error) { return "tool1 result", nil },
		},
		{
			Name: "tool2", Description: "Second tool",
			InputSchema: map[string]any{"type": "object"},
			Handler:     func(ctx context.Context, params json.RawMessage) (any, error) { return "tool2 result", nil },
		},
	}
	server.RegisterTools(tools)

	assert.Equal(t, 2, server.ToolCount())
	names := server.GetToolNames()
	assert.Contains(t, names, "tool1")
	assert.Contains(t, names, "tool2")
}

func TestServer_Instructions(t *testing.T) {
	server := New()
	server.SetInstructions("This server provides SaFE API tools for GPU cluster management.")

	req := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`1`),
		Method:  MethodInitialize,
		Params: mustMarshal(InitializeParams{
			ProtocolVersion: MCPProtocolVersion,
			ClientInfo:      Implementation{Name: "Test", Version: "1.0"},
		}),
	}

	resp := server.HandleRequest(context.Background(), &req)
	require.NotNil(t, resp)

	var result InitializeResult
	resultBytes, _ := json.Marshal(resp.Result)
	json.Unmarshal(resultBytes, &result)

	assert.Contains(t, result.Instructions, "SaFE API tools")
}

func TestNewJSONContent(t *testing.T) {
	data := map[string]any{"name": "test", "count": 42}
	content, err := NewJSONContent(data)
	require.NoError(t, err)
	assert.Equal(t, "text", content.Type)
	assert.Contains(t, content.Text, "test")
	assert.Contains(t, content.Text, "42")
}

func TestSSETransport_Message(t *testing.T) {
	server := New()
	server.RegisterTool(createTestTool())

	transport := NewSSETransport(server)
	ts := httptest.NewServer(transport.Handler())
	defer ts.Close()

	req := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`1`),
		Method:  MethodToolsList,
	}
	reqBody, _ := json.Marshal(req)

	// No session -> 400
	resp, err := http.Post(ts.URL+"/message", "application/json", bytes.NewReader(reqBody))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Invalid session -> 404
	resp2, err := http.Post(ts.URL+"/message?session_id=invalid", "application/json", bytes.NewReader(reqBody))
	require.NoError(t, err)
	defer func() { io.Copy(io.Discard, resp2.Body); resp2.Body.Close() }()
	assert.Equal(t, http.StatusNotFound, resp2.StatusCode)
}

func TestIntegration_FullFlow(t *testing.T) {
	server := New()

	type ClusterReq struct {
		Name string `json:"name"`
	}
	type ClusterResp struct {
		Status string `json:"status"`
		Nodes  int    `json:"nodes"`
	}

	tool := &MCPTool{
		Name:        "get_cluster",
		Description: "Get cluster status",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
		},
		Handler: func(ctx context.Context, params json.RawMessage) (any, error) {
			var req ClusterReq
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, err
			}
			return &ClusterResp{Status: "healthy", Nodes: 10}, nil
		},
	}
	server.RegisterTool(tool)

	transport := NewStreamableHTTPTransport(server)
	ts := httptest.NewServer(transport.Handler())
	defer ts.Close()

	client := NewStreamableHTTPClient(ts.URL)

	resp, err := client.Call(context.Background(), MethodInitialize, InitializeParams{
		ProtocolVersion: MCPProtocolVersion,
		ClientInfo:      Implementation{Name: "Integration Test", Version: "1.0"},
	})
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	resp, err = client.Call(context.Background(), MethodToolsList, nil)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	var toolsList ToolsListResult
	resultBytes, _ := json.Marshal(resp.Result)
	json.Unmarshal(resultBytes, &toolsList)
	require.Len(t, toolsList.Tools, 1)
	assert.Equal(t, "get_cluster", toolsList.Tools[0].Name)

	resp, err = client.Call(context.Background(), MethodToolsCall, ToolsCallParams{
		Name:      "get_cluster",
		Arguments: map[string]any{"name": "prod"},
	})
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	var callResult ToolsCallResult
	resultBytes, _ = json.Marshal(resp.Result)
	json.Unmarshal(resultBytes, &callResult)
	assert.False(t, callResult.IsError)
	require.Len(t, callResult.Content, 1)

	var clusterResp ClusterResp
	json.Unmarshal([]byte(callResult.Content[0].Text), &clusterResp)
	assert.Equal(t, "healthy", clusterResp.Status)
	assert.Equal(t, 10, clusterResp.Nodes)
}
