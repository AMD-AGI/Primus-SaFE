// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

// PlatformToolsConfig configuration for platform tools
type PlatformToolsConfig struct {
	// K8s configuration
	K8sEnabled    bool   `yaml:"k8s_enabled"`
	K8sAPIServer  string `yaml:"k8s_api_server"`
	K8sNamespace  string `yaml:"k8s_namespace"`
	K8sKubeconfig string `yaml:"k8s_kubeconfig"`

	// Metrics configuration
	MetricsEnabled bool   `yaml:"metrics_enabled"`
	MetricsURL     string `yaml:"metrics_url"` // VictoriaMetrics/Prometheus URL

	// Log configuration
	LogEnabled bool   `yaml:"log_enabled"`
	LogURL     string `yaml:"log_url"` // OpenSearch URL
}

// PlatformToolsProvider provides platform-specific MCP tools
type PlatformToolsProvider struct {
	config *PlatformToolsConfig
}

// NewPlatformToolsProvider creates a new platform tools provider
func NewPlatformToolsProvider(config *PlatformToolsConfig) *PlatformToolsProvider {
	if config == nil {
		config = &PlatformToolsConfig{
			K8sEnabled:     true,
			MetricsEnabled: true,
			LogEnabled:     true,
		}
	}
	return &PlatformToolsProvider{config: config}
}

// CreatePlatformTools creates all platform MCP tools
func (p *PlatformToolsProvider) CreatePlatformTools() []*unified.MCPTool {
	var tools []*unified.MCPTool

	if p.config.K8sEnabled {
		tools = append(tools, p.createK8sTools()...)
	}

	if p.config.MetricsEnabled {
		tools = append(tools, p.createMetricsTools()...)
	}

	if p.config.LogEnabled {
		tools = append(tools, p.createLogTools()...)
	}

	return tools
}

// createK8sTools creates Kubernetes diagnostic tools
func (p *PlatformToolsProvider) createK8sTools() []*unified.MCPTool {
	return []*unified.MCPTool{
		p.createK8sPodStatusTool(),
		p.createK8sNodeHealthTool(),
		p.createK8sResourceCheckTool(),
		p.createK8sClusterStatusTool(),
	}
}

func (p *PlatformToolsProvider) createK8sPodStatusTool() *unified.MCPTool {
	return &unified.MCPTool{
		Name:        "k8s_pod_status",
		Description: "Check Pod status in Kubernetes cluster. Returns pod phase, container status, restart counts, and events.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"namespace": map[string]interface{}{
					"type":        "string",
					"description": "Kubernetes namespace (use 'all' for all namespaces)",
				},
				"pod_name": map[string]interface{}{
					"type":        "string",
					"description": "Pod name or pattern (supports prefix matching)",
				},
				"label_selector": map[string]interface{}{
					"type":        "string",
					"description": "Label selector (e.g., 'app=nginx,env=prod')",
				},
				"include_events": map[string]interface{}{
					"type":        "boolean",
					"description": "Include recent events for pods",
					"default":     true,
				},
			},
			"required": []string{"namespace"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (interface{}, error) {
			var params struct {
				Namespace     string `json:"namespace"`
				PodName       string `json:"pod_name"`
				LabelSelector string `json:"label_selector"`
				IncludeEvents bool   `json:"include_events"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, err
			}

			// TODO: Implement actual K8s API call
			// This is a placeholder that returns sample data
			return p.mockK8sPodStatus(params.Namespace, params.PodName, params.LabelSelector)
		},
	}
}

func (p *PlatformToolsProvider) createK8sNodeHealthTool() *unified.MCPTool {
	return &unified.MCPTool{
		Name:        "k8s_node_health",
		Description: "Check node health and resource pressure in Kubernetes cluster.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"node_name": map[string]interface{}{
					"type":        "string",
					"description": "Node name (optional, returns all nodes if not specified)",
				},
				"include_conditions": map[string]interface{}{
					"type":        "boolean",
					"description": "Include detailed node conditions",
					"default":     true,
				},
			},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (interface{}, error) {
			var params struct {
				NodeName          string `json:"node_name"`
				IncludeConditions bool   `json:"include_conditions"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, err
			}

			// TODO: Implement actual K8s API call
			return p.mockK8sNodeHealth(params.NodeName)
		},
	}
}

func (p *PlatformToolsProvider) createK8sResourceCheckTool() *unified.MCPTool {
	return &unified.MCPTool{
		Name:        "k8s_resource_check",
		Description: "Check resource allocation and usage across the cluster or namespace.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"namespace": map[string]interface{}{
					"type":        "string",
					"description": "Namespace to check (optional, cluster-wide if not specified)",
				},
				"resource_type": map[string]interface{}{
					"type":        "string",
					"description": "Resource type to check: cpu, memory, gpu, all",
					"enum":        []string{"cpu", "memory", "gpu", "all"},
					"default":     "all",
				},
			},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (interface{}, error) {
			var params struct {
				Namespace    string `json:"namespace"`
				ResourceType string `json:"resource_type"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, err
			}

			if params.ResourceType == "" {
				params.ResourceType = "all"
			}

			// TODO: Implement actual K8s API call
			return p.mockK8sResourceCheck(params.Namespace, params.ResourceType)
		},
	}
}

func (p *PlatformToolsProvider) createK8sClusterStatusTool() *unified.MCPTool {
	return &unified.MCPTool{
		Name:        "k8s_cluster_status",
		Description: "Get overall cluster status summary including node count, pod count, and health.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"cluster": map[string]interface{}{
					"type":        "string",
					"description": "Cluster name (optional, uses default context)",
				},
			},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (interface{}, error) {
			var params struct {
				Cluster string `json:"cluster"`
			}
			json.Unmarshal(args, &params) // Ignore error, cluster is optional

			// TODO: Implement actual K8s API call
			return p.mockK8sClusterStatus(params.Cluster)
		},
	}
}

// createMetricsTools creates metrics query tools
func (p *PlatformToolsProvider) createMetricsTools() []*unified.MCPTool {
	return []*unified.MCPTool{
		p.createMetricsQueryTool(),
		p.createMetricsDiffTool(),
	}
}

func (p *PlatformToolsProvider) createMetricsQueryTool() *unified.MCPTool {
	return &unified.MCPTool{
		Name:        "metrics_query",
		Description: "Query metrics from VictoriaMetrics/Prometheus using PromQL.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "PromQL query string",
				},
				"start": map[string]interface{}{
					"type":        "string",
					"description": "Start time (RFC3339 or relative like 'now-1h')",
				},
				"end": map[string]interface{}{
					"type":        "string",
					"description": "End time (RFC3339 or relative, default: now)",
				},
				"step": map[string]interface{}{
					"type":        "string",
					"description": "Query step/resolution (e.g., '1m', '5m')",
					"default":    "1m",
				},
			},
			"required": []string{"query"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (interface{}, error) {
			var params struct {
				Query string `json:"query"`
				Start string `json:"start"`
				End   string `json:"end"`
				Step  string `json:"step"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, err
			}

			// TODO: Implement actual Prometheus/VictoriaMetrics query
			return p.mockMetricsQuery(params.Query, params.Start, params.End, params.Step)
		},
	}
}

func (p *PlatformToolsProvider) createMetricsDiffTool() *unified.MCPTool {
	return &unified.MCPTool{
		Name:        "metrics_diff",
		Description: "Compare metrics between two time periods to detect changes or regressions.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "PromQL query string",
				},
				"current_start": map[string]interface{}{
					"type":        "string",
					"description": "Current period start time",
				},
				"current_end": map[string]interface{}{
					"type":        "string",
					"description": "Current period end time",
				},
				"baseline_start": map[string]interface{}{
					"type":        "string",
					"description": "Baseline period start time",
				},
				"baseline_end": map[string]interface{}{
					"type":        "string",
					"description": "Baseline period end time",
				},
			},
			"required": []string{"query", "current_start", "current_end", "baseline_start", "baseline_end"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (interface{}, error) {
			var params struct {
				Query         string `json:"query"`
				CurrentStart  string `json:"current_start"`
				CurrentEnd    string `json:"current_end"`
				BaselineStart string `json:"baseline_start"`
				BaselineEnd   string `json:"baseline_end"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, err
			}

			// TODO: Implement actual metrics comparison
			return p.mockMetricsDiff(params.Query, params.CurrentStart, params.CurrentEnd, params.BaselineStart, params.BaselineEnd)
		},
	}
}

// createLogTools creates log search and analysis tools
func (p *PlatformToolsProvider) createLogTools() []*unified.MCPTool {
	return []*unified.MCPTool{
		p.createLogSearchTool(),
		p.createLogAggregateTool(),
	}
}

func (p *PlatformToolsProvider) createLogSearchTool() *unified.MCPTool {
	return &unified.MCPTool{
		Name:        "log_search",
		Description: "Search logs in OpenSearch with query and filters.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Search query (supports Lucene syntax)",
				},
				"time_range": map[string]interface{}{
					"type":        "string",
					"description": "Time range (e.g., '15m', '1h', '24h')",
					"default":    "1h",
				},
				"namespace": map[string]interface{}{
					"type":        "string",
					"description": "Filter by Kubernetes namespace",
				},
				"pod": map[string]interface{}{
					"type":        "string",
					"description": "Filter by pod name pattern",
				},
				"level": map[string]interface{}{
					"type":        "string",
					"description": "Filter by log level (ERROR, WARN, INFO, DEBUG)",
					"enum":        []string{"ERROR", "WARN", "INFO", "DEBUG"},
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of results",
					"default":     100,
				},
			},
			"required": []string{"query"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (interface{}, error) {
			var params struct {
				Query     string `json:"query"`
				TimeRange string `json:"time_range"`
				Namespace string `json:"namespace"`
				Pod       string `json:"pod"`
				Level     string `json:"level"`
				Limit     int    `json:"limit"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, err
			}

			if params.TimeRange == "" {
				params.TimeRange = "1h"
			}
			if params.Limit == 0 {
				params.Limit = 100
			}

			// TODO: Implement actual OpenSearch query
			return p.mockLogSearch(params.Query, params.TimeRange, params.Namespace, params.Pod, params.Level, params.Limit)
		},
	}
}

func (p *PlatformToolsProvider) createLogAggregateTool() *unified.MCPTool {
	return &unified.MCPTool{
		Name:        "log_aggregate",
		Description: "Aggregate logs for statistics (count errors by type, namespace, etc.).",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Base search query",
				},
				"time_range": map[string]interface{}{
					"type":        "string",
					"description": "Time range",
					"default":    "1h",
				},
				"agg_field": map[string]interface{}{
					"type":        "string",
					"description": "Field to aggregate on (e.g., 'kubernetes.namespace_name')",
				},
				"agg_type": map[string]interface{}{
					"type":        "string",
					"description": "Aggregation type",
					"enum":        []string{"terms", "date_histogram", "count"},
					"default":    "terms",
				},
			},
			"required": []string{"query", "agg_field"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (interface{}, error) {
			var params struct {
				Query     string `json:"query"`
				TimeRange string `json:"time_range"`
				AggField  string `json:"agg_field"`
				AggType   string `json:"agg_type"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return nil, err
			}

			// TODO: Implement actual OpenSearch aggregation
			return p.mockLogAggregate(params.Query, params.TimeRange, params.AggField, params.AggType)
		},
	}
}

// Mock implementations - replace with actual API calls

func (p *PlatformToolsProvider) mockK8sPodStatus(namespace, podName, labelSelector string) (interface{}, error) {
	return map[string]interface{}{
		"namespace": namespace,
		"pods": []map[string]interface{}{
			{
				"name":          "example-pod-abc12",
				"status":        "Running",
				"ready":         "2/2",
				"restarts":      0,
				"age":           "2d",
				"node":          "node-01",
				"ip":            "10.0.0.1",
				"container_status": []map[string]interface{}{
					{"name": "main", "state": "running", "ready": true},
					{"name": "sidecar", "state": "running", "ready": true},
				},
			},
		},
		"total":   1,
		"healthy": 1,
		"warning": 0,
		"error":   0,
	}, nil
}

func (p *PlatformToolsProvider) mockK8sNodeHealth(nodeName string) (interface{}, error) {
	nodes := []map[string]interface{}{
		{
			"name":   "node-01",
			"status": "Ready",
			"conditions": map[string]string{
				"Ready":          "True",
				"MemoryPressure": "False",
				"DiskPressure":   "False",
				"PIDPressure":    "False",
			},
			"capacity": map[string]string{
				"cpu":    "64",
				"memory": "256Gi",
				"gpu":    "8",
			},
			"allocatable": map[string]string{
				"cpu":    "62",
				"memory": "250Gi",
				"gpu":    "8",
			},
		},
	}

	if nodeName != "" {
		for _, n := range nodes {
			if n["name"] == nodeName {
				return n, nil
			}
		}
		return nil, fmt.Errorf("node not found: %s", nodeName)
	}

	return map[string]interface{}{
		"nodes":         nodes,
		"total":         len(nodes),
		"ready":         len(nodes),
		"not_ready":     0,
		"memory_pressure": 0,
		"disk_pressure": 0,
	}, nil
}

func (p *PlatformToolsProvider) mockK8sResourceCheck(namespace, resourceType string) (interface{}, error) {
	return map[string]interface{}{
		"namespace": namespace,
		"resources": map[string]interface{}{
			"cpu": map[string]interface{}{
				"capacity":    "256 cores",
				"allocatable": "248 cores",
				"requested":   "120 cores",
				"limits":      "200 cores",
				"usage":       "85 cores",
				"utilization": "68%",
			},
			"memory": map[string]interface{}{
				"capacity":    "1024 Gi",
				"allocatable": "1000 Gi",
				"requested":   "512 Gi",
				"limits":      "800 Gi",
				"usage":       "450 Gi",
				"utilization": "45%",
			},
			"gpu": map[string]interface{}{
				"capacity":    "32",
				"allocatable": "32",
				"requested":   "28",
				"usage":       "25",
				"utilization": "78%",
			},
		},
	}, nil
}

func (p *PlatformToolsProvider) mockK8sClusterStatus(cluster string) (interface{}, error) {
	return map[string]interface{}{
		"cluster":    cluster,
		"status":     "Healthy",
		"nodes": map[string]interface{}{
			"total":     15,
			"ready":     15,
			"not_ready": 0,
		},
		"pods": map[string]interface{}{
			"total":     342,
			"running":   320,
			"pending":   5,
			"failed":    2,
			"succeeded": 15,
		},
		"resources": map[string]interface{}{
			"cpu_utilization":    "65%",
			"memory_utilization": "72%",
			"gpu_utilization":    "78%",
		},
		"timestamp": time.Now().Format(time.RFC3339),
	}, nil
}

func (p *PlatformToolsProvider) mockMetricsQuery(query, start, end, step string) (interface{}, error) {
	return map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"resultType": "matrix",
			"result": []map[string]interface{}{
				{
					"metric": map[string]string{
						"__name__": "cpu_usage",
						"instance": "node-01",
					},
					"values": [][]interface{}{
						{1706745600, "0.45"},
						{1706745660, "0.48"},
						{1706745720, "0.52"},
					},
				},
			},
		},
		"query": query,
		"start": start,
		"end":   end,
		"step":  step,
	}, nil
}

func (p *PlatformToolsProvider) mockMetricsDiff(query, currentStart, currentEnd, baselineStart, baselineEnd string) (interface{}, error) {
	return map[string]interface{}{
		"query": query,
		"current_period": map[string]interface{}{
			"start": currentStart,
			"end":   currentEnd,
			"avg":   0.72,
			"max":   0.95,
			"min":   0.45,
		},
		"baseline_period": map[string]interface{}{
			"start": baselineStart,
			"end":   baselineEnd,
			"avg":   0.65,
			"max":   0.88,
			"min":   0.42,
		},
		"diff": map[string]interface{}{
			"avg_change":     "+10.8%",
			"max_change":     "+7.9%",
			"min_change":     "+7.1%",
			"is_significant": true,
		},
	}, nil
}

func (p *PlatformToolsProvider) mockLogSearch(query, timeRange, namespace, pod, level string, limit int) (interface{}, error) {
	logs := []map[string]interface{}{
		{
			"timestamp": "2026-02-01T10:30:15Z",
			"level":     "ERROR",
			"message":   "Connection refused to database",
			"namespace": "production",
			"pod":       "api-server-abc12",
			"container": "main",
		},
		{
			"timestamp": "2026-02-01T10:30:10Z",
			"level":     "WARN",
			"message":   "Retry attempt 3 of 5",
			"namespace": "production",
			"pod":       "api-server-abc12",
			"container": "main",
		},
	}

	return map[string]interface{}{
		"query":      query,
		"time_range": timeRange,
		"filters": map[string]string{
			"namespace": namespace,
			"pod":       pod,
			"level":     level,
		},
		"logs":  logs,
		"total": len(logs),
		"took":  "15ms",
	}, nil
}

func (p *PlatformToolsProvider) mockLogAggregate(query, timeRange, aggField, aggType string) (interface{}, error) {
	return map[string]interface{}{
		"query":      query,
		"time_range": timeRange,
		"aggregation": map[string]interface{}{
			"field": aggField,
			"type":  aggType,
			"buckets": []map[string]interface{}{
				{"key": "production", "doc_count": 1250},
				{"key": "staging", "doc_count": 450},
				{"key": "development", "doc_count": 120},
			},
		},
		"total_docs": 1820,
	}, nil
}

// Helper function to parse relative time strings
func parseRelativeTime(timeStr string) (time.Time, error) {
	if timeStr == "" || timeStr == "now" {
		return time.Now(), nil
	}

	if strings.HasPrefix(timeStr, "now-") {
		durationStr := strings.TrimPrefix(timeStr, "now-")
		duration, err := time.ParseDuration(durationStr)
		if err != nil {
			return time.Time{}, err
		}
		return time.Now().Add(-duration), nil
	}

	return time.Parse(time.RFC3339, timeStr)
}
