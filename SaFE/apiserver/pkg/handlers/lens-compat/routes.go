/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package lenscompat

import (
	"net/http"
	"path"
	"strings"
)

// readOnlyRoutes are the Robust routes, relative to Prefix, that Web/apps
// read through the lens base URL. Requests carry the cluster's service token,
// so nothing outside this list is forwarded. "*" matches one path segment and
// a trailing "**" matches any remaining segments.
var readOnlyRoutes = compileRoutes([]string{
	"/alert-rule-advices",
	"/alert-rule-advices/summary",
	"/alert-rule-advices/*",
	"/alert-silences",
	"/alert-silences/silenced-alerts",
	"/alert-silences/*",
	"/alerts",
	"/alerts/by-cluster",
	"/alerts/summary",
	"/alerts/top-sources",
	"/alerts/trend",
	"/alerts/*",
	"/alerts/*/correlations",
	"/clusters/consumers",
	"/clusters/gpuHeatmap",
	"/clusters/overview",
	"/detection-status",
	"/detection-status/summary",
	"/detection-status/*",
	"/detection-status/*/coverage",
	"/detection-status/*/coverage/log-gap",
	"/detection-status/*/evidence",
	"/detection-status/*/tasks",
	"/github-runners/repositories",
	"/github-runners/repositories/*/*",
	"/github-runners/repositories/*/*/metrics/metadata",
	"/github-runners/repositories/*/*/run-summaries",
	"/github-runners/repositories/*/*/runner-sets",
	"/github-runners/run-summaries/*",
	"/github-runners/run-summaries/*/graph",
	"/github-runners/run-summaries/*/jobs",
	"/github-runners/runner-sets",
	"/github-runners/runner-sets/*/*",
	"/github-runners/runner-sets/by-id/*",
	"/github-runners/runner-sets/by-id/*/config",
	"/github-runners/runner-sets/by-id/*/runs",
	"/github-runners/runner-sets/by-id/*/stats",
	"/github-workflow-metrics/analysis-tasks",
	"/github-workflow-metrics/analysis-tasks/*",
	"/github-workflow-metrics/configs",
	"/github-workflow-metrics/configs/*",
	"/github-workflow-metrics/configs/*/backfill/status",
	"/github-workflow-metrics/configs/*/backfill/tasks",
	"/github-workflow-metrics/configs/*/dimensions",
	"/github-workflow-metrics/configs/*/fields",
	"/github-workflow-metrics/configs/*/runs",
	"/github-workflow-metrics/configs/*/schemas",
	"/github-workflow-metrics/configs/*/schemas/changes",
	"/github-workflow-metrics/configs/*/stats",
	"/github-workflow-metrics/configs/*/summary",
	"/github-workflow-metrics/runs",
	"/github-workflow-metrics/runs/*",
	"/github-workflow-metrics/runs/*/analysis-tasks",
	"/github-workflow-metrics/runs/*/jobs",
	"/github-workflow-metrics/runs/*/jobs/*/logs",
	"/github-workflow-metrics/runs/*/jobs/*/steps/*/logs",
	"/github-workflow-metrics/runs/*/metrics",
	"/github-workflow-metrics/runs/*/state",
	"/github-workflow-metrics/schemas/*",
	"/gpu-aggregation/clusters",
	"/gpu-aggregation/dimension-keys",
	"/gpu-aggregation/namespaces",
	"/gpu-aggregation/namespaces/hourly-stats",
	"/gpu-aggregation/snapshots",
	"/gpu-aggregation/snapshots/latest",
	"/gpu-aggregation/workloads/hourly-stats",
	"/job-execution-histories",
	"/job-execution-histories/recent-failures",
	"/job-execution-histories/statistics/*",
	"/job-execution-histories/*",
	"/log-alert-rule-templates",
	"/log-alert-rule-templates/*",
	"/log-alert-rules",
	"/log-alert-rules/multi-cluster",
	"/log-alert-rules/*",
	"/log-alert-rules/*/statistics",
	"/log-alert-rules/*/versions",
	"/metric-alert-rules",
	"/metric-alert-rules/*",
	"/metric-alert-rules/*/status",
	"/nodes",
	"/nodes/gpuUtilization",
	"/nodes/gpuUtilizationHistory",
	"/nodes/*",
	"/nodes/*/gpuDevices",
	"/nodes/*/gpuMetrics",
	"/nodes/*/workloads",
	"/nodes/*/workloadsHistory",
	"/notification-channels",
	"/notification-channels/types",
	"/notification-channels/*",
	"/perfetto/sessions/*",
	"/perfetto/sessions/*/ui/**",
	"/pyspy/file/*/*",
	"/pyspy/task/*",
	"/registry/config",
	"/registry/image-url",
	"/skills",
	"/skills/health",
	"/skills/*",
	"/skills/*/content",
	"/skillsets",
	"/skillsets/*",
	"/skillsets/*/skills",
	"/system-config",
	"/system-config/*",
	"/tracelens/resource-profiles",
	"/tracelens/sessions/*",
	"/tracelens/sessions/*/ui/**",
	"/tracelens/workloads/*/sessions",
	"/weekly-reports/gpu_utilization",
	"/weekly-reports/gpu_utilization/*",
	"/weekly-reports/gpu_utilization/*/export",
	"/workloadMetadata",
	"/workloads",
	"/workloads/gpuUtilizationHistory",
	"/workloads/hierarchy",
	"/workloads/statistics",
	"/workloads/*",
	"/workloads/*/events",
	"/workloads/*/gpuMetrics",
	"/workloads/*/hierarchy",
	"/workloads/*/profiler-files",
	"/workloads/*/stats",
	"/workloads/*/tracelens",
})

func compileRoutes(patterns []string) [][]string {
	routes := make([][]string, 0, len(patterns))
	for _, p := range patterns {
		routes = append(routes, strings.Split(strings.Trim(p, "/"), "/"))
	}
	return routes
}

// isReadOnlyRoute reports whether a request for rest, the path below Prefix,
// may be forwarded.
func isReadOnlyRoute(method, rest string) bool {
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	trimmed := strings.TrimSuffix(rest, "/")
	if !strings.HasPrefix(trimmed, "/") || path.Clean(trimmed) != trimmed {
		return false
	}
	segments := strings.Split(strings.TrimPrefix(trimmed, "/"), "/")
	for _, route := range readOnlyRoutes {
		if matchRoute(route, segments) {
			return true
		}
	}
	return false
}

func matchRoute(route, segments []string) bool {
	for i, part := range route {
		if part == "**" {
			return true
		}
		if i >= len(segments) || (part != "*" && part != segments[i]) {
			return false
		}
	}
	return len(route) == len(segments)
}
