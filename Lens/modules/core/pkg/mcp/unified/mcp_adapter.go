// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package unified

import (
	"context"
	"encoding/json"
)

// ToMCPTool converts a unified Handler to an MCPTool.
// It generates the JSON Schema from the request type and wraps the handler.
func ToMCPTool[Req, Resp any](def *EndpointDef[Req, Resp]) *MCPTool {
	toolName := def.GetMCPToolName()

	return &MCPTool{
		Name:        toolName,
		Description: def.Description,
		InputSchema: GenerateJSONSchema[Req](),
		Handler: func(ctx context.Context, params json.RawMessage) (any, error) {
			var req Req
			if err := BindMCPRequest(params, &req); err != nil {
				return nil, err
			}
			return def.Handler(ctx, &req)
		},
	}
}

// ToMCPToolFromRaw creates an MCPTool from a RawMCPHandler.
func ToMCPToolFromRaw(name, description string, handler RawMCPHandler) *MCPTool {
	return &MCPTool{
		Name:        name,
		Description: description,
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(ctx context.Context, params json.RawMessage) (any, error) {
			var paramsMap map[string]any
			if len(params) > 0 {
				if err := json.Unmarshal(params, &paramsMap); err != nil {
					return nil, err
				}
			}
			return handler(ctx, paramsMap)
		},
	}
}

// GetMCPTool returns an MCPTool for MCP registration, or nil if HTTP-only.
func (def *EndpointDef[Req, Resp]) GetMCPTool() *MCPTool {
	// HTTPOnly endpoints should not have MCP tools
	if def.HTTPOnly {
		return nil
	}

	// Priority: RawMCPHandler > Handler
	if def.RawMCPHandler != nil {
		return ToMCPToolFromRaw(def.GetMCPToolName(), def.Description, def.RawMCPHandler)
	}

	if def.Handler != nil {
		return ToMCPTool(def)
	}

	// No handler available for MCP (e.g., HTTP-only with RawHTTPHandler)
	return nil
}
