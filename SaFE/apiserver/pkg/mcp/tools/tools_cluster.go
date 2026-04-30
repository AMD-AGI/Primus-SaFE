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

func clusterTools() []*mcpserver.MCPTool {
	return []*mcpserver.MCPTool{
		clusterList(),
		clusterGet(),
		clusterCreate(),
		clusterUpdate(),
		clusterDelete(),
		clusterManageNodes(),
		clusterLogs(),
	}
}

func clusterList() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "cluster_list",
		Description: "List all available clusters. Must call this tool to get cluster_id before creating a workspace.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			return APICall(ctx, http.MethodGet, "/clusters", nil)
		},
	}
}

func clusterGet() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "cluster_get",
		Description: "Get detailed information of a specific cluster, including endpoint, node list, Kubernetes version, etc.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cluster_id": prop("string", "Cluster ID"),
			},
			"required": []string{"cluster_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			return APICall(ctx, http.MethodGet, fmt.Sprintf("/clusters/%s", getStr(p, "cluster_id")), nil)
		},
	}
}

func clusterCreate() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "cluster_create",
		Description: "Create a new Kubernetes cluster",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":                 prop("string", "Cluster name (unique identifier)"),
				"nodes":                propArray("Control plane node ID list", map[string]any{"type": "string"}),
				"ssh_secret_id":        prop("string", "SSH secret ID"),
				"kube_spray_image":     prop("string", "KubeSpray image address"),
				"kube_pods_subnet":     prop("string", "Pod subnet, e.g. '10.0.0.0/16'"),
				"kube_service_address": prop("string", "Service address range, e.g. '10.96.0.0/16'"),
				"kube_version":         prop("string", "Kubernetes version, e.g. '1.32.5'"),
				"description":          prop("string", "Cluster description (optional)"),
				"image_secret_id":      prop("string", "Image registry secret ID (optional)"),
				"kube_network_plugin":  propEnum("string", "Network plugin, default 'flannel'", []string{"flannel", "calico", "cilium"}),
				"kube_api_server_args": propObject("API Server extra arguments (optional)"),
				"labels":               propObject("Cluster labels (optional)"),
				"is_protected":         prop("boolean", "Whether to protect the cluster, default false"),
			},
			"required": []string{"name", "nodes", "ssh_secret_id", "kube_spray_image", "kube_pods_subnet", "kube_service_address", "kube_version"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			return APICall(ctx, http.MethodPost, "/clusters", p)
		},
	}
}

func clusterUpdate() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "cluster_update",
		Description: "Update cluster configuration (currently only supports modifying protection status)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cluster_id":   prop("string", "Cluster ID"),
				"is_protected": prop("boolean", "Whether to protect the cluster"),
			},
			"required": []string{"cluster_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "cluster_id")
			delete(p, "cluster_id")
			return APICall(ctx, http.MethodPatch, fmt.Sprintf("/clusters/%s", id), p)
		},
	}
}

func clusterDelete() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "cluster_delete",
		Description: "Delete a cluster. Note: cluster must not be protected and have no running workloads",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cluster_id": prop("string", "Cluster ID"),
			},
			"required": []string{"cluster_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			return APICall(ctx, http.MethodDelete, fmt.Sprintf("/clusters/%s", getStr(p, "cluster_id")), nil)
		},
	}
}

func clusterManageNodes() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "cluster_manage_nodes",
		Description: "Manage cluster nodes (add or remove nodes)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cluster_id": prop("string", "Cluster ID"),
				"node_ids":   propArray("Node ID list", map[string]any{"type": "string"}),
				"action":     propEnum("string", "Action type", []string{"add", "remove"}),
			},
			"required": []string{"cluster_id", "node_ids", "action"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "cluster_id")
			delete(p, "cluster_id")
			return APICall(ctx, http.MethodPost, fmt.Sprintf("/clusters/%s/nodes", id), p)
		},
	}
}

func clusterLogs() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "cluster_logs",
		Description: "Get cluster creation/operation logs",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cluster_id":    prop("string", "Cluster ID"),
				"tail_lines":    prop("integer", "Return last N lines of logs, default 1000"),
				"since_seconds": prop("integer", "Return logs from last N seconds (optional)"),
			},
			"required": []string{"cluster_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "cluster_id")
			q := url.Values{}
			setQuery(q, p, "tail_lines", "tailLines")
			setQuery(q, p, "since_seconds", "sinceSeconds")
			qs := ""
			if len(q) > 0 {
				qs = "?" + q.Encode()
			}
			return APICall(ctx, http.MethodGet, fmt.Sprintf("/clusters/%s/logs%s", id, qs), nil)
		},
	}
}
