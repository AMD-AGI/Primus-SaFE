// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

// ServerInfo contains information about this MCP server.
var ServerInfo = Implementation{
	Name:    "Lens MCP Server",
	Version: "1.0.0",
}

// Server represents an MCP server that handles JSON-RPC requests.
type Server struct {
	mu          sync.RWMutex
	tools       map[string]*unified.MCPTool
	initialized bool
	clientInfo  *Implementation

	// Optional server instructions for the client
	Instructions string
}

// New creates a new MCP server instance.
func New() *Server {
	return &Server{
		tools: make(map[string]*unified.MCPTool),
	}
}

// RegisterTools registers MCP tools with the server.
func (s *Server) RegisterTools(tools []*unified.MCPTool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, tool := range tools {
		s.tools[tool.Name] = tool
		log.Infof("MCP Server: Registered tool %s", tool.Name)
	}
}

// RegisterTool registers a single MCP tool with the server.
func (s *Server) RegisterTool(tool *unified.MCPTool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tools[tool.Name] = tool
	log.Infof("MCP Server: Registered tool %s", tool.Name)
}

// SetInstructions sets the server instructions that will be sent to clients.
func (s *Server) SetInstructions(instructions string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Instructions = instructions
}

// HandleRequest processes a JSON-RPC request and returns a response.
func (s *Server) HandleRequest(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	log.Debugf("MCP Server: Handling request method=%s id=%s", req.Method, req.GetIDString())

	switch req.Method {
	case MethodInitialize:
		return s.handleInitialize(ctx, req)
	case MethodInitialized:
		// This is a notification, no response needed
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

// handleInitialize handles the initialize request.
func (s *Server) handleInitialize(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	var params InitializeParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return NewErrorResponse(req.ID, ErrorCodeInvalidParams,
			"Invalid initialize params", err.Error())
	}

	s.mu.Lock()
	s.initialized = true
	s.clientInfo = &params.ClientInfo
	s.mu.Unlock()

	log.Infof("MCP Server: Initialized by client %s v%s (protocol: %s)",
		params.ClientInfo.Name, params.ClientInfo.Version, params.ProtocolVersion)

	result := InitializeResult{
		ProtocolVersion: MCPProtocolVersion,
		Capabilities: ServerCapability{
			Tools: &ToolsCapability{
				ListChanged: false,
			},
			// Resources and Prompts can be added later
		},
		ServerInfo:   ServerInfo,
		Instructions: s.Instructions,
	}

	return NewResponse(req.ID, result)
}

// handleToolsList handles the tools/list request.
func (s *Server) handleToolsList(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
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

	log.Debugf("MCP Server: Listed %d tools", len(tools))

	return NewResponse(req.ID, ToolsListResult{Tools: tools})
}

// handleToolsCall handles the tools/call request.
func (s *Server) handleToolsCall(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	var params ToolsCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return NewErrorResponse(req.ID, ErrorCodeInvalidParams,
			"Invalid tools/call params", err.Error())
	}

	log.Infof("MCP Server: Calling tool %s", params.Name)

	s.mu.RLock()
	tool, exists := s.tools[params.Name]
	s.mu.RUnlock()

	if !exists {
		return NewErrorResponse(req.ID, ErrorCodeToolNotFound,
			fmt.Sprintf("Tool not found: %s", params.Name), nil)
	}

	// Convert arguments to JSON for the handler
	argsJSON, err := json.Marshal(params.Arguments)
	if err != nil {
		return NewErrorResponse(req.ID, ErrorCodeInvalidParams,
			"Failed to marshal arguments", err.Error())
	}

	// Execute the tool handler
	result, err := tool.Handler(ctx, argsJSON)
	if err != nil {
		log.Errorf("MCP Server: Tool %s execution failed: %v", params.Name, err)

		// Return error as content with isError flag
		return NewResponse(req.ID, ToolsCallResult{
			Content: []ContentBlock{NewTextContent(err.Error())},
			IsError: true,
		})
	}

	// Convert result to content
	content, err := NewJSONContent(result)
	if err != nil {
		return NewErrorResponse(req.ID, ErrorCodeInternalError,
			"Failed to marshal result", err.Error())
	}

	log.Debugf("MCP Server: Tool %s executed successfully", params.Name)

	return NewResponse(req.ID, ToolsCallResult{
		Content: []ContentBlock{content},
	})
}

// handlePing handles the ping request.
func (s *Server) handlePing(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	return NewResponse(req.ID, PingResult{})
}

// handleResourcesList handles the resources/list request.
func (s *Server) handleResourcesList(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	// Return empty list for now - resources can be added later
	return NewResponse(req.ID, ResourcesListResult{Resources: []ResourceDefinition{}})
}

// handlePromptsList handles the prompts/list request.
func (s *Server) handlePromptsList(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse {
	// Return empty list for now - prompts can be added later
	return NewResponse(req.ID, PromptsListResult{Prompts: []PromptDefinition{}})
}

// HandleMessage processes a raw JSON message and returns the response as bytes.
// This is the main entry point for processing incoming messages.
func (s *Server) HandleMessage(ctx context.Context, data []byte) ([]byte, error) {
	req, err := ParseRequest(data)
	if err != nil {
		resp := NewErrorResponse(nil, ErrorCodeParseError, "Parse error", err.Error())
		return json.Marshal(resp)
	}

	// Validate JSON-RPC version
	if req.JSONRPC != JSONRPCVersion {
		resp := NewErrorResponse(req.ID, ErrorCodeInvalidRequest,
			"Invalid JSON-RPC version", nil)
		return json.Marshal(resp)
	}

	// Handle the request
	resp := s.HandleRequest(ctx, req)

	// Notifications don't get responses
	if resp == nil {
		return nil, nil
	}

	return json.Marshal(resp)
}

// IsInitialized returns true if the server has been initialized by a client.
func (s *Server) IsInitialized() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.initialized
}

// GetClientInfo returns information about the connected client.
func (s *Server) GetClientInfo() *Implementation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clientInfo
}

// ToolCount returns the number of registered tools.
func (s *Server) ToolCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tools)
}

// GetToolNames returns the names of all registered tools.
func (s *Server) GetToolNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.tools))
	for name := range s.tools {
		names = append(names, name)
	}
	return names
}
