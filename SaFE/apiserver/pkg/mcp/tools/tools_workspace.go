// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	mcpserver "github.com/AMD-AIG-AIMA/SAFE/common/pkg/mcp/server"
)

func workspaceTools() []*mcpserver.MCPTool {
	return []*mcpserver.MCPTool{
		workspaceList(),
		workspaceGet(),
		workspaceCreate(),
		workspaceUpdate(),
		workspaceScale(),
		workspaceManageNodes(),
		workspaceDelete(),
	}
}

func workspaceList() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "workspace_list",
		Description: "List all workspaces, supports filtering by cluster",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cluster_id": prop("string", "Filter by cluster ID (optional)"),
			},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, _ := parseParams(raw)
			q := url.Values{}
			setQuery(q, p, "cluster_id", "clusterId")
			qs := ""
			if len(q) > 0 {
				qs = "?" + q.Encode()
			}
			return APICall(ctx, http.MethodGet, "/workspaces"+qs, nil)
		},
	}
}

func workspaceGet() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "workspace_get",
		Description: "Get detailed information of a specific workspace",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": prop("string", "Workspace ID (format: clusterId-workspaceName)"),
			},
			"required": []string{"workspace_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			return APICall(ctx, http.MethodGet, fmt.Sprintf("/workspaces/%s", getStr(p, "workspace_id")), nil)
		},
	}
}

func workspaceCreate() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "workspace_create",
		Description: "Create a new workspace. Note: must call cluster_list and flavor_list first to get available clusters and node flavors.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":               prop("string", "Workspace name"),
				"cluster_id":         prop("string", "Cluster ID (required, get from cluster_list)"),
				"flavor_id":          prop("string", "Node flavor ID (required, get from flavor_list)"),
				"replica":            prop("integer", "Expected node count (optional)"),
				"description":        prop("string", "Description (optional)"),
				"queue_policy":       propEnum("string", "Queue policy", []string{"fifo", "balance"}),
				"scopes":             propArray("Supported service modules: Train, Infer, Authoring, CICD", map[string]any{"type": "string"}),
				"volumes":            propArray("Volume configuration list", map[string]any{"type": "object"}),
				"enable_preempt":     prop("boolean", "Whether to enable preemption, default false"),
				"is_default":         prop("boolean", "Whether to set as default workspace, default false"),
				"image_secret_ids":   propArray("Image pull secret ID list", map[string]any{"type": "string"}),
			},
			"required": []string{"name", "cluster_id", "flavor_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			return APICall(ctx, http.MethodPost, "/workspaces", p)
		},
	}
}

func workspaceUpdate() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "workspace_update",
		Description: "Update workspace configuration",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id":     prop("string", "Workspace ID (format: clusterId-workspaceName)"),
				"description":      prop("string", "New description"),
				"flavor_id":        prop("string", "New node flavor ID"),
				"replica":          prop("integer", "New replica count (node count)"),
				"queue_policy":     propEnum("string", "New queue policy", []string{"fifo", "balance"}),
				"scopes":           propArray("New service module list", map[string]any{"type": "string"}),
				"volumes":          propArray("New volume configuration list", map[string]any{"type": "object"}),
				"enable_preempt":   prop("boolean", "Whether to enable preemption"),
				"managers":         propArray("Manager user ID list", map[string]any{"type": "string"}),
				"is_default":       prop("boolean", "Whether to set as default workspace"),
				"image_secret_ids": propArray("Image pull secret ID list", map[string]any{"type": "string"}),
			},
			"required": []string{"workspace_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "workspace_id")
			delete(p, "workspace_id")
			return APICall(ctx, http.MethodPatch, fmt.Sprintf("/workspaces/%s", id), p)
		},
	}
}

func workspaceScale() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "workspace_scale",
		Description: "Scale workspace (shortcut, equivalent to workspace_update with only replica)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": prop("string", "Workspace ID (format: clusterId-workspaceName)"),
				"replica":      prop("integer", "New node count"),
			},
			"required": []string{"workspace_id", "replica"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "workspace_id")
			body := map[string]any{"replica": p["replica"]}
			return APICall(ctx, http.MethodPatch, fmt.Sprintf("/workspaces/%s", id), body)
		},
	}
}

func workspaceManageNodes() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "workspace_manage_nodes",
		Description: "Manage workspace nodes (add or remove nodes)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": prop("string", "Workspace ID (format: clusterId-workspaceName)"),
				"node_ids":     propArray("Node ID list", map[string]any{"type": "string"}),
				"action":       propEnum("string", "Action type", []string{"add", "remove"}),
			},
			"required": []string{"workspace_id", "node_ids", "action"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "workspace_id")
			delete(p, "workspace_id")
			return APICall(ctx, http.MethodPost, fmt.Sprintf("/workspaces/%s/nodes", id), p)
		},
	}
}

func workspaceDelete() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "workspace_delete",
		Description: "Delete a workspace. Note: workspace must have no running workloads",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": prop("string", "Workspace ID (format: clusterId-workspaceName)"),
			},
			"required": []string{"workspace_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			return APICall(ctx, http.MethodDelete, fmt.Sprintf("/workspaces/%s", getStr(p, "workspace_id")), nil)
		},
	}
}
