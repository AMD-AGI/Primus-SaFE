// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"k8s.io/klog/v2"
)

type httpRequestKey struct{}

// ContextWithHTTPRequest attaches an HTTP request to a context so tool handlers can access auth headers, host, etc.
func ContextWithHTTPRequest(ctx context.Context, r *http.Request) context.Context {
	return context.WithValue(ctx, httpRequestKey{}, r)
}

// HTTPRequestFromContext retrieves the HTTP request stored in ctx, if any.
func HTTPRequestFromContext(ctx context.Context) (*http.Request, bool) {
	r, ok := ctx.Value(httpRequestKey{}).(*http.Request)
	return r, ok && r != nil
}

var ServerInfo = Implementation{
	Name:    "SaFE MCP Server",
	Version: "1.0.0",
}

// MCPTool represents an MCP tool definition that can be registered with the server.
type MCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Handler     MCPToolHandler `json:"-"`
}

// MCPToolHandler is the function signature for MCP tool handlers.
type MCPToolHandler func(ctx context.Context, params json.RawMessage) (any, error)

// Server represents an MCP server that handles JSON-RPC requests.
type Server struct {
	mu          sync.RWMutex
	tools       map[string]*MCPTool
	initialized bool
	clientInfo  *Implementation
	Instructions string
}

func New() *Server {
	return &Server{
		tools: make(map[string]*MCPTool),
	}
}

func (s *Server) RegisterTools(tools []*MCPTool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, tool := range tools {
		s.tools[tool.Name] = tool
		klog.Infof("MCP Server: Registered tool %s", tool.Name)
	}
}

func (s *Server) RegisterTool(tool *MCPTool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = tool
	klog.Infof("MCP Server: Registered tool %s", tool.Name)
}

func (s *Server) SetInstructions(instructions string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Instructions = instructions
}

func (s *Server) HandleRequest(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	klog.V(4).Infof("MCP Server: Handling request method=%s id=%s", req.Method, req.GetIDString())

	switch req.Method {
	case MethodInitialize:
		return s.handleInitialize(ctx, req)
	case MethodInitialized:
		return nil
	case MethodToolsList:
		return s.handleToolsList(ctx, req)
	case MethodToolsCall:
		return s.handleToolsCall(ctx, req)
	case MethodPing:
		return s.handlePing(ctx, req)
	case MethodResourcesList:
		return s.handleResourcesList(ctx, req)
	case MethodPromptsList:
		return s.handlePromptsList(ctx, req)
	default:
		return NewErrorResponse(req.ID, ErrorCodeMethodNotFound,
			fmt.Sprintf("Method not found: %s", req.Method), nil)
	}
}

func (s *Server) handleInitialize(_ context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	var params InitializeParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return NewErrorResponse(req.ID, ErrorCodeInvalidParams, "Invalid initialize params", err.Error())
	}

	s.mu.Lock()
	s.initialized = true
	s.clientInfo = &params.ClientInfo
	s.mu.Unlock()

	klog.Infof("MCP Server: Initialized by client %s v%s (protocol: %s)",
		params.ClientInfo.Name, params.ClientInfo.Version, params.ProtocolVersion)

	result := InitializeResult{
		ProtocolVersion: MCPProtocolVersion,
		Capabilities: ServerCapability{
			Tools: &ToolsCapability{ListChanged: false},
		},
		ServerInfo:   ServerInfo,
		Instructions: s.Instructions,
	}
	return NewResponse(req.ID, result)
}

func (s *Server) handleToolsList(_ context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tools := make([]ToolDefinition, 0, len(s.tools))
	for _, tool := range s.tools {
		tools = append(tools, ToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}
	klog.V(4).Infof("MCP Server: Listed %d tools", len(tools))
	return NewResponse(req.ID, ToolsListResult{Tools: tools})
}

func (s *Server) handleToolsCall(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	var params ToolsCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return NewErrorResponse(req.ID, ErrorCodeInvalidParams, "Invalid tools/call params", err.Error())
	}

	klog.Infof("MCP Server: Calling tool %s", params.Name)

	s.mu.RLock()
	tool, exists := s.tools[params.Name]
	s.mu.RUnlock()

	if !exists {
		return NewErrorResponse(req.ID, ErrorCodeToolNotFound,
			fmt.Sprintf("Tool not found: %s", params.Name), nil)
	}

	argsJSON, err := json.Marshal(params.Arguments)
	if err != nil {
		return NewErrorResponse(req.ID, ErrorCodeInvalidParams, "Failed to marshal arguments", err.Error())
	}

	result, err := tool.Handler(ctx, argsJSON)
	if err != nil {
		klog.Errorf("MCP Server: Tool %s execution failed: %v", params.Name, err)
		return NewResponse(req.ID, ToolsCallResult{
			Content: []ContentBlock{NewTextContent(err.Error())},
			IsError: true,
		})
	}

	content, err := NewJSONContent(result)
	if err != nil {
		return NewErrorResponse(req.ID, ErrorCodeInternalError, "Failed to marshal result", err.Error())
	}
	klog.V(4).Infof("MCP Server: Tool %s executed successfully", params.Name)
	return NewResponse(req.ID, ToolsCallResult{Content: []ContentBlock{content}})
}

func (s *Server) handlePing(_ context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	return NewResponse(req.ID, PingResult{})
}

func (s *Server) handleResourcesList(_ context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	return NewResponse(req.ID, ResourcesListResult{Resources: []ResourceDefinition{}})
}

func (s *Server) handlePromptsList(_ context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	return NewResponse(req.ID, PromptsListResult{Prompts: []PromptDefinition{}})
}

// HandleMessage processes a raw JSON message and returns the response bytes.
func (s *Server) HandleMessage(ctx context.Context, data []byte) ([]byte, error) {
	req, err := ParseRequest(data)
	if err != nil {
		resp := NewErrorResponse(nil, ErrorCodeParseError, "Parse error", err.Error())
		return json.Marshal(resp)
	}
	if req.JSONRPC != JSONRPCVersion {
		resp := NewErrorResponse(req.ID, ErrorCodeInvalidRequest, "Invalid JSON-RPC version", nil)
		return json.Marshal(resp)
	}
	resp := s.HandleRequest(ctx, req)
	if resp == nil {
		return nil, nil
	}
	return json.Marshal(resp)
}

func (s *Server) IsInitialized() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.initialized
}

func (s *Server) GetClientInfo() *Implementation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clientInfo
}

func (s *Server) ToolCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tools)
}

func (s *Server) GetToolNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.tools))
	for name := range s.tools {
		names = append(names, name)
	}
	return names
}
