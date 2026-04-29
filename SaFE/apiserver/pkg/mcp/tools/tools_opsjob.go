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

func opsjobTools() []*mcpserver.MCPTool {
	return []*mcpserver.MCPTool{
		opsjobList(), opsjobGet(), opsjobCreate(), opsjobStop(), opsjobDelete(),
	}
}

func opsjobInputItemSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":  prop("string", "Selector name: node, addon.template, workload, workspace, cluster, node.template, node.host, image, script, secret, endpoint, dest.path, label"),
			"value": prop("string", "Selector value (e.g. nodeId, workloadId, workspaceId)"),
		},
		"required": []string{"name", "value"},
	}
}

func opsjobResourceSchema() map[string]any {
	return propObject(map[string]any{
		"cpu":               prop("string", "CPU cores, e.g. '8'"),
		"memory":            prop("string", "Memory size, e.g. '32Gi'"),
		"gpu":               prop("string", "Number of GPUs"),
		"ephemeralStorage":  prop("string", "Ephemeral storage size"),
		"sharedMemory":      prop("string", "Shared memory size"),
	}, "Container resources (preflight only)")
}

func opsjobCreateSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":    prop("string", "Job name, used to generate ops job ID"),
			"type":    propEnum("string", "Ops job type", []string{"addon", "preflight", "dumplog", "reboot", "exportimage", "prewarm", "download"}),
			"inputs":  propArray(opsjobInputItemSchema(), "Input selectors specifying targets and parameters"),
			"timeout_second": prop("integer", "Timeout in seconds, 0 or negative means no timeout"),
			"ttl_seconds_after_finished": prop("integer", "Job TTL after completion in seconds"),
			"excluded_nodes":             propArray(prop("string", ""), "Node IDs to exclude from execution"),
			"is_tolerate_all":            prop("boolean", "Whether to tolerate all node taints, default true"),
			"image":                      prop("string", "Container image (preflight only)"),
			"entry_point":                prop("string", "Startup command, Base64 encoded (preflight only)"),
			"resource":                   opsjobResourceSchema(),
			"env":                        propObject(map[string]any{}, "Environment variables as key-value pairs (preflight only)"),
			"hostpath":                   propArray(prop("string", ""), "Host paths to mount (preflight only)"),
			"workspace_id":               prop("string", "Workspace ID (preflight only, required for non-admin users)"),
			"batch_count":                prop("integer", "Parallel nodes per batch (addon only, default 1)"),
			"available_ratio":            prop("number", "Success ratio threshold (addon only, default 1.0)"),
			"security_upgrade":           prop("boolean", "Wait until node idle before upgrade (addon only)"),
		},
		"required": []string{"name", "type", "inputs"},
	}
}

func buildOpsjobCreateBody(p map[string]any) map[string]any {
	body := map[string]any{
		"name":   getStr(p, "name"),
		"type":   getStr(p, "type"),
		"inputs": p["inputs"],
	}
	if _, ok := p["timeout_second"]; ok {
		if n, ok := getInt(p, "timeout_second"); ok {
			body["timeoutSecond"] = n
		}
	}
	if _, ok := p["ttl_seconds_after_finished"]; ok {
		if n, ok := getInt(p, "ttl_seconds_after_finished"); ok {
			body["ttlSecondsAfterFinished"] = n
		}
	}
	if en, ok := p["excluded_nodes"]; ok && en != nil {
		body["excludedNodes"] = en
	}
	if b, ok := getBool(p, "is_tolerate_all"); ok {
		body["isTolerateAll"] = b
	}
	if v := getStr(p, "image"); v != "" {
		body["image"] = v
	}
	if v := getStr(p, "entry_point"); v != "" {
		body["entryPoint"] = v
	}
	if r, ok := p["resource"]; ok && r != nil {
		body["resource"] = r
	}
	if env, ok := p["env"]; ok && env != nil {
		body["env"] = env
	}
	if hp, ok := p["hostpath"]; ok && hp != nil {
		body["hostpath"] = hp
	}
	if v := getStr(p, "workspace_id"); v != "" {
		body["workspaceId"] = v
	}
	if _, ok := p["batch_count"]; ok {
		if n, ok := getInt(p, "batch_count"); ok {
			body["batchCount"] = n
		}
	}
	if _, ok := p["available_ratio"]; ok {
		if f, ok := getFloat(p, "available_ratio"); ok {
			body["availableRatio"] = f
		}
	}
	if b, ok := getBool(p, "security_upgrade"); ok {
		body["securityOperation"] = b
	}
	return body
}

func opsjobList() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "opsjob_list",
		Description: "List ops jobs with optional filters by cluster, workspace, status, type, user, time range, etc.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"offset":   prop("integer", "Pagination offset, default 0"),
				"limit":    prop("integer", "Records per page, default 100"),
				"sort_by":  prop("string", "Sort field, default 'creationTime'"),
				"order":    propEnum("string", "Sort order, default 'desc'", []string{"desc", "asc"}),
				"cluster_id":   prop("string", "Filter by cluster ID"),
				"workspace_id": prop("string", "Filter by workspace ID"),
				"user_name":    prop("string", "Filter by submitter username (fuzzy match)"),
				"phase": propEnum("string", "Filter by job status", []string{
					"Succeeded", "Failed", "Running", "Pending",
				}),
				"type": propEnum("string", "Filter by job type", []string{
					"addon", "preflight", "dumplog", "reboot", "exportimage", "prewarm", "download",
				}),
				"job_name": prop("string", "Filter by job name (fuzzy match)"),
				"since":    prop("string", "Start time filter (RFC3339), default until-720h"),
				"until":    prop("string", "End time filter (RFC3339), default now"),
			},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			q := url.Values{}
			setQuery(q, p, "offset", "offset")
			setQuery(q, p, "limit", "limit")
			setQuery(q, p, "sort_by", "sortBy")
			setQuery(q, p, "order", "order")
			setQuery(q, p, "cluster_id", "clusterId")
			setQuery(q, p, "workspace_id", "workspaceId")
			setQuery(q, p, "user_name", "userName")
			setQuery(q, p, "phase", "phase")
			setQuery(q, p, "type", "type")
			setQuery(q, p, "job_name", "jobName")
			setQuery(q, p, "since", "since")
			setQuery(q, p, "until", "until")
			suffix := ""
			if len(q) > 0 {
				suffix = "?" + q.Encode()
			}
			return APICall(ctx, http.MethodGet, "/opsjobs"+suffix, nil)
		},
	}
}

func opsjobGet() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "opsjob_get",
		Description: "Get detailed information of a specific ops job, including conditions, inputs, outputs, and resource config",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id": prop("string", "Ops job ID"),
			},
			"required": []string{"job_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "job_id")
			if id == "" {
				return nil, fmt.Errorf("job_id is required")
			}
			return APICall(ctx, http.MethodGet, "/opsjobs/"+url.PathEscape(id), nil)
		},
	}
}

func opsjobCreate() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "opsjob_create",
		Description: "Create an OpsJob (operations job) to perform administrative tasks. Supported types: addon (install/upgrade addons on nodes), preflight (run preflight checks on nodes/cluster), dumplog (dump workload logs to S3), reboot (reboot a node), exportimage (export workload image to registry), prewarm (pre-pull image to all nodes in workspace), download (download files from S3 to nodes). At least one scope selector must be provided via inputs: node/workspace/cluster/workload.",
		InputSchema: opsjobCreateSchema(),
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			if getStr(p, "name") == "" || getStr(p, "type") == "" {
				return nil, fmt.Errorf("name and type are required")
			}
			if inputs, ok := p["inputs"].([]any); !ok || len(inputs) == 0 {
				return nil, fmt.Errorf("inputs is required")
			}
			body := buildOpsjobCreateBody(p)
			return APICall(ctx, http.MethodPost, "/opsjobs", body)
		},
	}
}

func opsjobStop() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "opsjob_stop",
		Description: "Stop a running ops job",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id": prop("string", "Ops job ID to stop"),
			},
			"required": []string{"job_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "job_id")
			if id == "" {
				return nil, fmt.Errorf("job_id is required")
			}
			return APICall(ctx, http.MethodPost, "/opsjobs/"+url.PathEscape(id)+"/stop", nil)
		},
	}
}

func opsjobDelete() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "opsjob_delete",
		Description: "Delete an ops job",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id": prop("string", "Ops job ID to delete"),
			},
			"required": []string{"job_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "job_id")
			if id == "" {
				return nil, fmt.Errorf("job_id is required")
			}
			return APICall(ctx, http.MethodDelete, "/opsjobs/"+url.PathEscape(id), nil)
		},
	}
}
