// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	mcpserver "github.com/AMD-AIG-AIMA/SAFE/common/pkg/mcp/server"
)

func secretTools() []*mcpserver.MCPTool {
	return []*mcpserver.MCPTool{
		secretList(),
	}
}

func secretList() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name: "secret_list",
		Description: "List the secrets the caller can see: id, name, type, workspaces and owner. Never returns a secret's value. " +
			"Use it to check that a workspace has the image-pull or SSH secret a workload needs.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type":         prop("string", "Secret type, e.g. ssh, image, general; comma separated for several"),
				"workspace_id": prop("string", "Only secrets of this workspace"),
			},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			q := url.Values{}
			setQuery(q, p, "type", "type")
			setQuery(q, p, "workspace_id", "workspaceId")
			suffix := ""
			if len(q) > 0 {
				suffix = "?" + q.Encode()
			}
			return APICall(ctx, http.MethodGet, "/secrets"+suffix, nil)
		},
	}
}
