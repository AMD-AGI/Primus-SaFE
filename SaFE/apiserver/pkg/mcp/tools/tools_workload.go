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

func workloadTools() []*mcpserver.MCPTool {
	return []*mcpserver.MCPTool{
		workloadList(), workloadGet(), workloadCreate(), workloadUpdate(), workloadStop(),
		workloadBatchStop(), workloadDelete(), workloadBatchDelete(), workloadPodLogs(), workloadLogsDownload(),
	}
}

func workloadResourceItemSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"cpu":               prop("string", "CPU cores, e.g. '4'"),
			"memory":            prop("string", "Memory size, e.g. '256Gi'"),
			"gpu":               prop("string", "Number of GPUs"),
			"replica":           prop("integer", "Number of replicas"),
			"ephemeralStorage":  prop("string", "Ephemeral storage size, e.g. '50Gi'"),
			"sharedMemory":      prop("string", "Shared memory size, e.g. '64Gi'"),
		},
		"required": []string{"cpu", "memory", "replica"},
	}
}

func workloadCreateSchema() map[string]any {
	secretsItem := propObject(map[string]any{
		"id":   prop("string", ""),
		"type": propEnum("string", "", []string{"image", "general"}),
	}, "")
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"display_name": prop("string", "Workload display name"),
			"workspace_id": prop("string", "Workspace ID"),
			"kind": propEnum("string", "Workload type", []string{
				"PyTorchJob", "Deployment", "StatefulSet", "Authoring", "TorchFT", "RayJob", "AutoscalingRunnerSet",
			}),
			"images":    propArray(prop("string", ""), "Container image URLs array. Length should match resources array."),
			"resources": propArray(workloadResourceItemSchema(), "Resources array. For PyTorchJob: [master, worker]. For TorchFT: [lighthouse, worker]. For RayJob: [head, worker_group_1, worker_group_2(optional)], head replica must be 1."),
			"entry_points": propArray(prop("string", ""), "Startup commands (Base64 encoded) array. Length should match resources array. Not required for Authoring type. For RayJob: these are node initialization scripts (e.g. init-head.sh, init-worker.sh), NOT the training entrypoint (use env.RAY_JOB_ENTRYPOINT for that)."),
			"description":  prop("string", "Workload description"),
			"priority":     prop("integer", "Priority level (0-2), default 0"),
			"timeout":      prop("integer", "Timeout in seconds, default 0 (no timeout)"),
			"max_retry":    prop("integer", "Maximum retry count, default 0"),
			"env":          propObject(map[string]any{}, "Environment variables as key-value pairs. For RayJob: RAY_JOB_ENTRYPOINT is required (the main training command submitted to Ray cluster, e.g. 'bash my-train-job.sh')."),
			"specified_nodes": propArray(prop("string", ""), "List of node IDs to run on"),
			"excluded_nodes":  propArray(prop("string", ""), "List of node IDs to exclude"),
			"is_supervised":   prop("boolean", "Enable monitoring, default false"),
			"ttl_seconds_after_finished": prop("integer", "TTL after completion in seconds, default 60"),
			"customer_labels":            propObject(map[string]any{}, "Custom labels as key-value pairs"),
			"secrets":                    propArray(secretsItem, "List of secrets"),
			"is_tolerate_all":            prop("boolean", "Tolerate all node taints, default false"),
			"preheat":                    prop("boolean", "Pre-pull image, default false"),
			"sticky_nodes":               prop("boolean", "Use same nodes during retries/failovers, default false"),
			"liveness":                   propObject(map[string]any{}, "Liveness probe configuration (Deployment/StatefulSet only)"),
			"readiness":                  propObject(map[string]any{}, "Readiness probe configuration (Deployment/StatefulSet only)"),
			"service":                    propObject(map[string]any{}, "Service configuration (Deployment/StatefulSet only)"),
		},
		"required": []string{"display_name", "workspace_id", "kind", "images", "resources"},
	}
}

func buildWorkloadCreateBody(p map[string]any) (map[string]any, error) {
	if getStr(p, "display_name") == "" || getStr(p, "workspace_id") == "" || getStr(p, "kind") == "" {
		return nil, fmt.Errorf("display_name, workspace_id, and kind are required")
	}
	imgs, ok := p["images"].([]any)
	if !ok || len(imgs) == 0 {
		return nil, fmt.Errorf("images is required")
	}
	res, ok := p["resources"].([]any)
	if !ok || len(res) == 0 {
		return nil, fmt.Errorf("resources is required")
	}
	kind := getStr(p, "kind")
	body := map[string]any{
		"displayName":   getStr(p, "display_name"),
		"workspaceId":   getStr(p, "workspace_id"),
		"groupVersionKind": map[string]any{
			"group":   "",
			"version": "v1",
			"kind":    kind,
		},
		"images":    p["images"],
		"resources": p["resources"],
	}
	if v := getStr(p, "description"); v != "" {
		body["description"] = v
	}
	if _, ok := p["priority"]; ok {
		if n, ok := getInt(p, "priority"); ok {
			body["priority"] = n
		}
	}
	if _, ok := p["timeout"]; ok {
		if n, ok := getInt(p, "timeout"); ok {
			body["timeout"] = n
		}
	}
	if _, ok := p["max_retry"]; ok {
		if n, ok := getInt(p, "max_retry"); ok {
			body["maxRetry"] = n
		}
	}
	if env, ok := p["env"]; ok && env != nil {
		body["env"] = env
	}
	if sn, ok := p["specified_nodes"]; ok && sn != nil {
		body["specifiedNodes"] = sn
	}
	if en, ok := p["excluded_nodes"]; ok && en != nil {
		body["excludedNodes"] = en
	}
	if b, ok := getBool(p, "is_supervised"); ok {
		body["isSupervised"] = b
	}
	if _, ok := p["ttl_seconds_after_finished"]; ok {
		if n, ok := getInt(p, "ttl_seconds_after_finished"); ok {
			body["ttlSecondsAfterFinished"] = n
		}
	}
	if cl, ok := p["customer_labels"]; ok && cl != nil {
		body["customerLabels"] = cl
	}
	if sec, ok := p["secrets"]; ok && sec != nil {
		body["secrets"] = sec
	}
	if b, ok := getBool(p, "is_tolerate_all"); ok {
		body["isTolerateAll"] = b
	}
	if b, ok := getBool(p, "preheat"); ok {
		body["preheat"] = b
	}
	if b, ok := getBool(p, "sticky_nodes"); ok && b {
		body["annotations"] = map[string]any{"primus-safe.retry.on.original.nodes": "true"}
	}
	if ep, ok := p["entry_points"]; ok && ep != nil {
		body["entryPoints"] = ep
	}
	if lv, ok := p["liveness"]; ok && lv != nil {
		body["liveness"] = lv
	}
	if rd, ok := p["readiness"]; ok && rd != nil {
		body["readiness"] = rd
	}
	if svc, ok := p["service"]; ok && svc != nil {
		body["service"] = svc
	}
	return body, nil
}

func workloadList() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "workload_list",
		Description: "List workloads with optional filters. Use this to query workloads by workspace, cluster, user, status, or type.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": prop("string", "Filter by workspace ID"),
				"cluster_id":   prop("string", "Filter by cluster ID"),
				"user_id":      prop("string", "Filter by user ID"),
				"phase": propEnum("string", "Filter by status phase", []string{
					"Succeeded", "Failed", "Pending", "Running", "Stopped", "Updating", "NotReady",
				}),
				"kind": propEnum("string", "Filter by workload type", []string{
					"PyTorchJob", "Deployment", "StatefulSet", "Authoring", "AutoscalingRunnerSet", "TorchFT", "RayJob",
				}),
				"workload_id": prop("string", "Fuzzy match by workload ID"),
				"offset":      prop("integer", "Pagination offset, default 0"),
				"limit":       prop("integer", "Number of items per page, default 100"),
				"sort_by":     prop("string", "Sort field, default 'creationTime'"),
				"order":       propEnum("string", "Sort order", []string{"desc", "asc"}),
			},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			q := url.Values{}
			setQuery(q, p, "workspace_id", "workspaceId")
			setQuery(q, p, "cluster_id", "clusterId")
			setQuery(q, p, "user_id", "userId")
			setQuery(q, p, "phase", "phase")
			setQuery(q, p, "kind", "kind")
			setQuery(q, p, "workload_id", "workloadId")
			setQuery(q, p, "offset", "offset")
			setQuery(q, p, "limit", "limit")
			setQuery(q, p, "sort_by", "sortBy")
			setQuery(q, p, "order", "order")
			suffix := ""
			if len(q) > 0 {
				suffix = "?" + q.Encode()
			}
			return APICall(ctx, http.MethodGet, "/workloads"+suffix, nil)
		},
	}
}

func workloadGet() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "workload_get",
		Description: "Get detailed information of a specific workload by its ID",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workload_id": prop("string", "Workload ID"),
			},
			"required": []string{"workload_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "workload_id")
			if id == "" {
				return nil, fmt.Errorf("workload_id is required")
			}
			return APICall(ctx, http.MethodGet, "/workloads/"+url.PathEscape(id), nil)
		},
	}
}

func workloadCreate() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "workload_create",
		Description: "Create a new workload. Supports PyTorchJob (distributed training), Deployment (inference), StatefulSet (stateful service), Authoring (development), TorchFT (fault-tolerant training), RayJob (Ray distributed training), and AutoscalingRunnerSet (CI/CD).",
		InputSchema: workloadCreateSchema(),
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			body, err := buildWorkloadCreateBody(p)
			if err != nil {
				return nil, err
			}
			return APICall(ctx, http.MethodPost, "/workloads", body)
		},
	}
}

func workloadUpdate() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "workload_update",
		Description: "Update an existing workload. Only specified fields will be updated. Note: array/object fields require complete values.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workload_id":   prop("string", "Workload ID to update"),
				"priority":      prop("integer", "New priority level (0-2)"),
				"images":        propArray(prop("string", ""), "New container image URLs array (must provide complete array)"),
				"entry_points":  propArray(prop("string", ""), "New startup commands (Base64 encoded) array (must provide complete array)"),
				"description":   prop("string", "New description"),
				"timeout":       prop("integer", "New timeout in seconds"),
				"max_retry":     prop("integer", "New maximum retry count"),
				"env":           propObject(map[string]any{}, "New environment variables (must provide complete object)"),
				"service":       propObject(map[string]any{}, "New service configuration"),
				"resources": propArray(map[string]any{
					"type": "object",
				}, "New resources array (must provide complete array)"),
			},
			"required": []string{"workload_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "workload_id")
			if id == "" {
				return nil, fmt.Errorf("workload_id is required")
			}
			body := map[string]any{}
			if _, ok := p["priority"]; ok {
				if n, ok := getInt(p, "priority"); ok {
					body["priority"] = n
				}
			}
			if _, ok := p["images"]; ok {
				body["images"] = p["images"]
			}
			if _, ok := p["entry_points"]; ok {
				body["entryPoints"] = p["entry_points"]
			}
			if v := getStr(p, "description"); v != "" {
				body["description"] = v
			}
			if _, ok := p["timeout"]; ok {
				if n, ok := getInt(p, "timeout"); ok {
					body["timeout"] = n
				}
			}
			if _, ok := p["max_retry"]; ok {
				if n, ok := getInt(p, "max_retry"); ok {
					body["maxRetry"] = n
				}
			}
			if _, ok := p["env"]; ok {
				body["env"] = p["env"]
			}
			if _, ok := p["service"]; ok {
				body["service"] = p["service"]
			}
			if _, ok := p["resources"]; ok {
				body["resources"] = p["resources"]
			}
			return APICall(ctx, http.MethodPatch, "/workloads/"+url.PathEscape(id), body)
		},
	}
}

func workloadStop() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "workload_stop",
		Description: "Stop a running workload",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workload_id": prop("string", "Workload ID to stop"),
			},
			"required": []string{"workload_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "workload_id")
			if id == "" {
				return nil, fmt.Errorf("workload_id is required")
			}
			return APICall(ctx, http.MethodPost, "/workloads/"+url.PathEscape(id)+"/stop", nil)
		},
	}
}

func workloadBatchStop() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "workload_batch_stop",
		Description: "Stop multiple running workloads at once",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workload_ids": propArray(prop("string", ""), "List of workload IDs to stop"),
			},
			"required": []string{"workload_ids"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			ids, ok := p["workload_ids"].([]any)
			if !ok || len(ids) == 0 {
				return nil, fmt.Errorf("workload_ids is required")
			}
			body := map[string]any{"workloadIds": p["workload_ids"]}
			return APICall(ctx, http.MethodPost, "/workloads/stop", body)
		},
	}
}

func workloadDelete() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "workload_delete",
		Description: "Delete a workload by its ID",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workload_id": prop("string", "Workload ID to delete"),
			},
			"required": []string{"workload_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "workload_id")
			if id == "" {
				return nil, fmt.Errorf("workload_id is required")
			}
			return APICall(ctx, http.MethodDelete, "/workloads/"+url.PathEscape(id), nil)
		},
	}
}

func workloadBatchDelete() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "workload_batch_delete",
		Description: "Delete multiple workloads at once",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workload_ids": propArray(prop("string", ""), "List of workload IDs to delete"),
			},
			"required": []string{"workload_ids"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			if _, ok := p["workload_ids"].([]any); !ok {
				return nil, fmt.Errorf("workload_ids is required")
			}
			body := map[string]any{"workloadIds": p["workload_ids"]}
			return APICall(ctx, http.MethodPost, "/workloads/delete", body)
		},
	}
}

func workloadPodLogs() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "workload_pod_logs",
		Description: "Get logs from a specific pod of a workload",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workload_id":    prop("string", "Workload ID"),
				"pod_id":         prop("string", "Pod ID"),
				"tail_lines":     prop("integer", "Number of lines to return from the end, default 1000"),
				"container":      prop("string", "Container name (optional, for multi-container pods)"),
				"since_seconds":  prop("integer", "Return logs from last N seconds (optional)"),
			},
			"required": []string{"workload_id", "pod_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			wid := getStr(p, "workload_id")
			pid := getStr(p, "pod_id")
			if wid == "" || pid == "" {
				return nil, fmt.Errorf("workload_id and pod_id are required")
			}
			q := url.Values{}
			setQuery(q, p, "tail_lines", "tailLines")
			setQuery(q, p, "container", "container")
			setQuery(q, p, "since_seconds", "sinceSeconds")
			suffix := ""
			if len(q) > 0 {
				suffix = "?" + q.Encode()
			}
			path := "/workloads/" + url.PathEscape(wid) + "/pods/" + url.PathEscape(pid) + "/logs" + suffix
			return APICall(ctx, http.MethodGet, path, nil)
		},
	}
}

func workloadLogsDownload() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name: "workload_logs_download",
		Description: "Download workload logs to a local file.\n\nHOW TO USE:\n1. Run this curl command directly (recommended):\n   curl -L -o <output_file> \"<api_base_url>/api/v1/workloads/<workload_id>/logs/download\" \\\n     -H \"Content-Type: application/json\" -H \"Authorization: Bearer <token>\" -d '{}'\n   \n   The -L flag follows the HTTP 303 redirect to download the log file directly.\n\n2. Or call this tool to get the presigned S3 URL, then download separately:\n   curl -o <output_file> \"<presigned_url_from_this_tool>\"\n\nNOTES:\n- The download may take several minutes (DumpLog job runs internally)\n- Requires OpenSearch and S3 enabled on the cluster\n- api_base_url is the SaFE API server address (e.g., https://example.primus-safe.amd.com)\n",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workload_id":     prop("string", "Workload ID to download logs for"),
				"timeout_second":  prop("integer", "Timeout in seconds for waiting DumpLog job completion (default: 900 = 15 minutes). Set to 0 or omit to use server default."),
			},
			"required": []string{"workload_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id := getStr(p, "workload_id")
			if id == "" {
				return nil, fmt.Errorf("workload_id is required")
			}
			body := map[string]any{}
			if _, ok := p["timeout_second"]; ok {
				if n, ok := getInt(p, "timeout_second"); ok {
					body["timeoutSecond"] = n
				}
			}
			return APICall(ctx, http.MethodPost, "/workloads/"+url.PathEscape(id)+"/logs/download", body)
		},
	}
}
