// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package unified

import (
	"context"
	"encoding/json"

	mcpserver "github.com/AMD-AIG-AIMA/SAFE/common/pkg/mcp/server"
)

// ToMCPTool converts a unified Handler to an MCPTool.
func ToMCPTool[Req, Resp any](def *EndpointDef[Req, Resp]) *mcpserver.MCPTool {
	toolName := def.GetMCPToolName()
	return &mcpserver.MCPTool{
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
// If inputSchema is nil, an empty object schema is used.
func ToMCPToolFromRaw(name, description string, inputSchema map[string]any, handler RawMCPHandler) *mcpserver.MCPTool {
	if inputSchema == nil {
		inputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return &mcpserver.MCPTool{
		Name:        name,
		Description: description,
		InputSchema: inputSchema,
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
func (def *EndpointDef[Req, Resp]) GetMCPTool() *mcpserver.MCPTool {
	if def.HTTPOnly {
		return nil
	}
	if def.RawMCPHandler != nil {
		return ToMCPToolFromRaw(def.GetMCPToolName(), def.Description, def.MCPInputSchema, def.RawMCPHandler)
	}
	if def.Handler != nil {
		return ToMCPTool(def)
	}
	return nil
}
