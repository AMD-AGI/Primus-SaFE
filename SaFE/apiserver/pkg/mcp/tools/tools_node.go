// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	mcpserver "github.com/AMD-AIG-AIMA/SAFE/common/pkg/mcp/server"
)

func nodeTools() []*mcpserver.MCPTool {
	return []*mcpserver.MCPTool{
		nodeList(), nodeGet(), nodeCreate(), nodeUpdate(), nodeDelete(),
		nodeStatus(), nodeCordon(), nodeUncordon(), nodeDrain(),
		nodeReboot(), nodeRebootLogs(), nodeExport(),
	}
}

func nodeList() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "node_list",
		Description: "List nodes with multiple filter options",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cluster_id":   prop("string", "Filter by cluster ID (empty string for unbound nodes)"),
				"workspace_id": prop("string", "Filter by workspace ID"),
				"flavor_id":    prop("string", "Filter by node flavor"),
				"node_id":      prop("string", "Filter by node ID"),
				"available":    prop("boolean", "Filter by availability"),
				"phase":        prop("string", "Filter by phase (Ready/SSHFailed/Managing/etc.)"),
				"brief":        prop("boolean", "Brief mode, return basic info only"),
				"limit":        prop("integer", "Items per page, default 100, -1 for all"),
			},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			q := url.Values{}
			setQuery(q, p, "cluster_id", "clusterId")
			setQuery(q, p, "workspace_id", "workspaceId")
			setQuery(q, p, "flavor_id", "flavorId")
			setQuery(q, p, "node_id", "nodeId")
			setQuery(q, p, "available", "available")
			setQuery(q, p, "phase", "phase")
			setQuery(q, p, "brief", "brief")
			setQuery(q, p, "limit", "limit")
			suffix := ""
			if len(q) > 0 {
				suffix = "?" + q.Encode()
			}
			return APICall(ctx, http.MethodGet, "/nodes"+suffix, nil)
		},
	}
}

func nodeGet() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "node_get",
		Description: "Get detailed information of a specific node",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"node_id": prop("string", "Node ID/name"),
			},
			"required": []string{"node_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "node_id")
			if id == "" {
				return nil, fmt.Errorf("node_id is required")
			}
			return APICall(ctx, http.MethodGet, "/nodes/"+url.PathEscape(id), nil)
		},
	}
}

func nodeCreate() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "node_create",
		Description: "Register a new node to the system",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"private_ip":    prop("string", "Node private IP (required)"),
				"flavor_id":     prop("string", "Node flavor ID (required, get from flavor_list)"),
				"template_id":   prop("string", "Node template ID (required)"),
				"ssh_secret_id": prop("string", "SSH secret ID (required)"),
				"hostname":      prop("string", "Node hostname (optional, defaults to private_ip)"),
				"public_ip":     prop("string", "Node public IP (optional)"),
				"port":          prop("integer", "SSH port, default 22"),
				"labels":        propObject(map[string]any{}, "Node labels (optional)"),
			},
			"required": []string{"private_ip", "flavor_id", "template_id", "ssh_secret_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			if getStr(p, "private_ip") == "" || getStr(p, "flavor_id") == "" || getStr(p, "template_id") == "" || getStr(p, "ssh_secret_id") == "" {
				return nil, fmt.Errorf("private_ip, flavor_id, template_id, and ssh_secret_id are required")
			}
			body := map[string]any{
				"privateIP":   getStr(p, "private_ip"),
				"flavorId":    getStr(p, "flavor_id"),
				"templateId":  getStr(p, "template_id"),
				"sshSecretId": getStr(p, "ssh_secret_id"),
			}
			if pub := getStr(p, "public_ip"); pub != "" {
				body["publicIP"] = pub
			}
			if h := getStr(p, "hostname"); h != "" {
				body["hostname"] = h
			}
			if port, ok := getInt(p, "port"); ok {
				body["port"] = int32(port)
			}
			if labels, ok := p["labels"]; ok && labels != nil {
				body["labels"] = labels
			}
			return APICall(ctx, http.MethodPost, "/nodes", body)
		},
	}
}

func nodeUpdate() *mcpserver.MCPTool {
	taintsItem := propObject(map[string]any{
		"key":    prop("string", ""),
		"value":  prop("string", ""),
		"effect": propEnum("string", "", []string{"NoSchedule", "PreferNoSchedule", "NoExecute"}),
	}, "")
	return &mcpserver.MCPTool{
		Name:        "node_update",
		Description: "Update node configuration (labels, taints, flavor, etc.)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"node_id":     prop("string", "Node ID"),
				"labels":      propObject(map[string]any{}, "Node labels dictionary"),
				"taints":      propArray(taintsItem, "Taints list"),
				"flavor_id":   prop("string", "New node flavor ID"),
				"template_id": prop("string", "New node template ID"),
				"private_ip":  prop("string", "New private IP"),
				"port":        prop("integer", "New SSH port"),
			},
			"required": []string{"node_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "node_id")
			if id == "" {
				return nil, fmt.Errorf("node_id is required")
			}
			body := map[string]any{}
			if _, ok := p["labels"]; ok {
				body["labels"] = p["labels"]
			}
			if _, ok := p["taints"]; ok {
				body["taints"] = p["taints"]
			}
			if v := getStr(p, "flavor_id"); v != "" {
				body["flavorId"] = v
			}
			if v := getStr(p, "template_id"); v != "" {
				body["templateId"] = v
			}
			if v := getStr(p, "private_ip"); v != "" {
				body["privateIP"] = v
			}
			if port, ok := getInt(p, "port"); ok {
				body["port"] = int32(port)
			}
			return APICall(ctx, http.MethodPatch, "/nodes/"+url.PathEscape(id), body)
		},
	}
}

func nodeDelete() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "node_delete",
		Description: "Delete a node. Set force=true to bypass safety checks. Drain/grace-period are not supported by the SaFE REST API and are intentionally not exposed here.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"node_id": prop("string", "Node ID/name"),
				"force":   prop("boolean", "Bypass safety checks (default false). Maps to ?force= on the REST call."),
			},
			"required": []string{"node_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "node_id")
			if id == "" {
				return nil, fmt.Errorf("node_id is required")
			}
			q := url.Values{}
			if force, ok := getBool(p, "force"); ok {
				q.Set("force", strconv.FormatBool(force))
			}
			suffix := ""
			if len(q) > 0 {
				suffix = "?" + q.Encode()
			}
			return APICall(ctx, http.MethodDelete, "/nodes/"+url.PathEscape(id)+suffix, nil)
		},
	}
}

func nodeStatus() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "node_status",
		Description: "Get node status information (ready status, capacity, conditions, etc.)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"node_id": prop("string", "Node ID/name"),
			},
			"required": []string{"node_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "node_id")
			if id == "" {
				return nil, fmt.Errorf("node_id is required")
			}
			return APICall(ctx, http.MethodGet, "/nodes/"+url.PathEscape(id), nil)
		},
	}
}

func nodeCordon() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "node_cordon",
		Description: "Mark node as unschedulable (cordon), prevent new Pods from being scheduled to this node",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"node_id": prop("string", "Node ID/name"),
			},
			"required": []string{"node_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "node_id")
			if id == "" {
				return nil, fmt.Errorf("node_id is required")
			}
			return APICall(ctx, http.MethodPatch, "/nodes/"+url.PathEscape(id), map[string]any{"unschedulable": true})
		},
	}
}

func nodeUncordon() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "node_uncordon",
		Description: "Remove unschedulable mark from node (uncordon), allow new Pods to be scheduled to this node",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"node_id": prop("string", "Node ID/name"),
			},
			"required": []string{"node_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "node_id")
			if id == "" {
				return nil, fmt.Errorf("node_id is required")
			}
			return APICall(ctx, http.MethodPatch, "/nodes/"+url.PathEscape(id), map[string]any{"unschedulable": false})
		},
	}
}

func nodeDrain() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "node_drain",
		Description: "Drain all workloads from a node",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"node_id":             prop("string", "Node ID/name"),
				"force":               prop("boolean", "Whether to force drain, default false"),
				"ignore_daemonsets":   prop("boolean", "Whether to ignore DaemonSets, default true"),
				"timeout":             prop("integer", "Timeout in seconds"),
			},
			"required": []string{"node_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "node_id")
			if id == "" {
				return nil, fmt.Errorf("node_id is required")
			}
			body := map[string]any{"drain": true}
			if b, ok := getBool(p, "force"); ok {
				body["force"] = b
			}
			if b, ok := getBool(p, "ignore_daemonsets"); ok {
				body["ignoreDaemonsets"] = b
			}
			if n, ok := getInt(p, "timeout"); ok {
				body["timeout"] = n
			}
			return APICall(ctx, http.MethodPatch, "/nodes/"+url.PathEscape(id), body)
		},
	}
}

func nodeReboot() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "node_reboot",
		Description: "Reboot a node (via OpsJob)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"node_id":  prop("string", "Node ID"),
				"job_name": prop("string", "Job name, default 'reboot-node'"),
			},
			"required": []string{"node_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "node_id")
			if id == "" {
				return nil, fmt.Errorf("node_id is required")
			}
			jobName := getStr(p, "job_name")
			if jobName == "" {
				jobName = "reboot-node"
			}
			body := map[string]any{
				"name": jobName,
				"type": "reboot",
				"inputs": []map[string]any{
					{"name": "node", "value": id},
				},
			}
			return APICall(ctx, http.MethodPost, "/opsjobs", body)
		},
	}
}

func nodeRebootLogs() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "node_reboot_logs",
		Description: "Get node reboot history logs",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"node_id":    prop("string", "Node ID"),
				"since_time": prop("string", "Start time filter (RFC3339 format)"),
				"until_time": prop("string", "End time filter (RFC3339 format)"),
				"limit":      prop("integer", "Items per page, default 100"),
			},
			"required": []string{"node_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "node_id")
			if id == "" {
				return nil, fmt.Errorf("node_id is required")
			}
			q := url.Values{}
			setQuery(q, p, "since_time", "sinceTime")
			setQuery(q, p, "until_time", "untilTime")
			setQuery(q, p, "limit", "limit")
			suffix := ""
			if len(q) > 0 {
				suffix = "?" + q.Encode()
			}
			return APICall(ctx, http.MethodGet, "/nodes/"+url.PathEscape(id)+"/reboot/logs"+suffix, nil)
		},
	}
}

func nodeExport() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "node_export",
		Description: "Export nodes list with multiple filtering options",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cluster_id":          prop("string", "Filter by cluster ID"),
				"workspace_id":        prop("string", "Filter by workspace ID"),
				"flavor_id":           prop("string", "Filter by node flavor ID"),
				"node_id":             prop("string", "Filter by node ID"),
				"available":           prop("boolean", "Filter by availability: true (available) / false (unavailable)"),
				"phase":               prop("string", "Filter by status (comma-separated, e.g. Ready,SSHFailed,Managing)"),
				"is_addons_installed": prop("boolean", "Filter by addon installation status"),
			},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			q := url.Values{}
			setQuery(q, p, "cluster_id", "clusterId")
			setQuery(q, p, "workspace_id", "workspaceId")
			setQuery(q, p, "flavor_id", "flavorId")
			setQuery(q, p, "node_id", "nodeId")
			setQuery(q, p, "available", "available")
			setQuery(q, p, "phase", "phase")
			setQuery(q, p, "is_addons_installed", "isAddonsInstalled")
			suffix := ""
			if len(q) > 0 {
				suffix = "?" + q.Encode()
			}
			return APICall(ctx, http.MethodGet, "/nodes/export"+suffix, nil)
		},
	}
}
