// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	mcpserver "github.com/AMD-AIG-AIMA/SAFE/common/pkg/mcp/server"
)

func flavorTools() []*mcpserver.MCPTool {
	return []*mcpserver.MCPTool{
		flavorList(),
		flavorGet(),
	}
}

func flavorList() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "flavor_list",
		Description: "List all available node flavors. Must call this tool to get flavor_id before creating a workspace.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			return APICall(ctx, http.MethodGet, "/nodeflavors", nil)
		},
	}
}

func flavorGet() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "flavor_get",
		Description: "Get detailed information of a specific node flavor",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"flavor_id": prop("string", "Node flavor ID"),
			},
			"required": []string{"flavor_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			return APICall(ctx, http.MethodGet, fmt.Sprintf("/nodeflavors/%s", getStr(p, "flavor_id")), nil)
		},
	}
}
