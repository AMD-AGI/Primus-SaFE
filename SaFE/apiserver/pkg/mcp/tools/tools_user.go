// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tools

import (
	"context"
	"encoding/json"
	"net/http"

	mcpserver "github.com/AMD-AIG-AIMA/SAFE/common/pkg/mcp/server"
)

func userTools() []*mcpserver.MCPTool {
	return []*mcpserver.MCPTool{
		userSelf(),
	}
}

func userSelf() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name: "user_self",
		Description: "Get the record of the user making the call: name, email, roles, the workspaces the user can use and the ones the user manages. " +
			"Use it to answer who the caller is and what the caller may do. apikey_current describes the API key, not the user.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			if _, err := parseParams(raw); err != nil {
				return nil, err
			}
			return APICall(ctx, http.MethodGet, "/users/self", nil)
		},
	}
}
