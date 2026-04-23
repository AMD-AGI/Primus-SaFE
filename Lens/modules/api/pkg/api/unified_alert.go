// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package api provides unified API endpoints for alert operations.
// These endpoints work for both HTTP REST and MCP protocols.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

// ===== Alert List =====

// AlertListRequest represents the request for listing alerts.
type AlertListRequest struct {
	Cluster    string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Source     string `json:"source" query:"source" mcp:"source,description=Alert source filter"`
	AlertName  string `json:"alert_name" query:"alert_name" mcp:"alert_name,description=Alert name filter"`
	Severity   string `json:"severity" query:"severity" mcp:"severity,description=Severity filter (critical/high/warning/info)"`
	Status     string `json:"status" query:"status" mcp:"status,description=Status filter (firing/resolved/silenced)"`
	WorkloadID string `json:"workload_id" query:"workload_id" mcp:"workload_id,description=Workload ID filter"`
	PodName    string `json:"pod_name" query:"pod_name" mcp:"pod_name,description=Pod name filter"`
	NodeName   string `json:"node_name" query:"node_name" mcp:"node_name,description=Node name filter"`
	PageNum    int    `json:"pageNum" query:"pageNum" mcp:"pageNum,description=Page number (default 1)"`
	PageSize   int    `json:"pageSize" query:"pageSize" mcp:"pageSize,description=Items per page (default 20)"`
}

// AlertListResponse represents the alert list response.
type AlertListResponse struct {
	Data     []*dbmodel.AlertEvents `json:"data"`
	Total    int64                  `json:"total"`
	PageNum  int                    `json:"pageNum"`
	PageSize int                    `json:"pageSize"`
}

// ===== Alert Detail =====

// AlertDetailRequest represents the request for alert detail.
type AlertDetailRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	ID      string `json:"id" param:"id" mcp:"id,description=Alert ID,required"`
}

// AlertDetailResponse is the alert detail.
type AlertDetailResponse = dbmodel.AlertEvents

// ===== Alert Summary =====

// AlertSummaryRequest represents the request for alert summary.
type AlertSummaryRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
}

// AlertSummaryResponse represents the alert summary by severity.
type AlertSummaryResponse struct {
	Critical SeverityCount `json:"critical"`
	High     SeverityCount `json:"high"`
	Warning  SeverityCount `json:"warning"`
	Info     SeverityCount `json:"info"`
}

// SeverityCount represents count and change for a severity level.
type SeverityCount struct {
	Count  int `json:"count"`
	Change int `json:"change"`
}

// ===== Alert Trend =====

// AlertTrendRequest represents the request for alert trend.
type AlertTrendRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	GroupBy string `json:"group_by" query:"group_by" mcp:"group_by,description=Group by interval (hour/day)"`
	Hours   int    `json:"hours" query:"hours" mcp:"hours,description=Number of hours to look back (default 24)"`
}

// AlertTrendResponse represents the alert trend data.
type AlertTrendResponse []*AlertTrendPoint

// ===== Alert Top Sources =====

// AlertTopSourcesRequest represents the request for top alert sources.
type AlertTopSourcesRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Limit   int    `json:"limit" query:"limit" mcp:"limit,description=Number of top sources to return (default 10)"`
	Hours   int    `json:"hours" query:"hours" mcp:"hours,description=Number of hours to look back (default 24)"`
}

// AlertTopSourcesResponse represents the top alert sources.
type AlertTopSourcesResponse []AlertSourceCount

// AlertSourceCount represents an alert source and its count.
type AlertSourceCount struct {
	AlertName string `json:"alert_name"`
	Count     int    `json:"count"`
}

// ===== Alerts By Cluster =====

// AlertsByClusterRequest represents the request for alerts by cluster.
type AlertsByClusterRequest struct {
	Hours int `json:"hours" query:"hours" mcp:"hours,description=Number of hours to look back (default 24)"`
}

// AlertsByClusterResponse represents alerts grouped by cluster.
type AlertsByClusterResponse []ClusterAlertCount

// ClusterAlertCount represents a cluster and its alert count.
type ClusterAlertCount struct {
	ClusterName string `json:"cluster_name"`
	Count       int    `json:"count"`
}

// ===== Alert Correlations =====

// AlertCorrelationsRequest represents the request for alert correlations.
type AlertCorrelationsRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	ID      string `json:"id" param:"id" mcp:"id,description=Alert ID,required"`
}

// AlertCorrelationsResponse represents correlated alerts.
type AlertCorrelationsResponse struct {
	Correlations []AlertCorrelation `json:"correlations"`
}

// AlertCorrelation represents a correlated alert.
type AlertCorrelation struct {
	AlertID         string  `json:"alert_id"`
	AlertName       string  `json:"alert_name"`
	Severity        string  `json:"severity"`
	CorrelationType string  `json:"correlation_type"`
	Score           float64 `json:"score"`
}

// ===== Metric Alert Rules List =====

// MetricRulesListRequest represents the request for listing metric alert rules.
type MetricRulesListRequest struct {
	Name        string `json:"name" query:"name" mcp:"name,description=Rule name filter"`
	ClusterName string `json:"cluster_name" query:"cluster_name" mcp:"cluster_name,description=Cluster name filter"`
	Enabled     string `json:"enabled" query:"enabled" mcp:"enabled,description=Enabled filter (true/false)"`
	SyncStatus  string `json:"sync_status" query:"sync_status" mcp:"sync_status,description=Sync status filter"`
	PageNum     int    `json:"pageNum" query:"pageNum" mcp:"pageNum,description=Page number (default 1)"`
	PageSize    int    `json:"pageSize" query:"pageSize" mcp:"pageSize,description=Items per page (default 20)"`
}

// MetricRulesListResponse represents the metric rules list response.
type MetricRulesListResponse struct {
	Data     []*dbmodel.MetricAlertRules `json:"data"`
	Total    int64                       `json:"total"`
	PageNum  int                         `json:"pageNum"`
	PageSize int                         `json:"pageSize"`
}

// ===== Metric Alert Rule Detail =====

// MetricRuleDetailRequest represents the request for metric rule detail.
type MetricRuleDetailRequest struct {
	ID int64 `json:"id" param:"id" mcp:"id,description=Rule ID,required"`
}

// MetricRuleDetailResponse is the metric rule detail.
type MetricRuleDetailResponse = dbmodel.MetricAlertRules

// ===== Metric Alert Rule Status =====

// MetricRuleStatusRequest represents the request for metric rule status.
type MetricRuleStatusRequest struct {
	ID int64 `json:"id" param:"id" mcp:"id,description=Rule ID,required"`
}

// MetricRuleStatusResponse represents the VMRule status.
type MetricRuleStatusResponse struct {
	Status       string `json:"status"`
	LastSyncTime string `json:"last_sync_time,omitempty"`
	Error        string `json:"error,omitempty"`
}

// ===== Log Alert Rules List =====

// LogRulesListRequest represents the request for listing log alert rules.
type LogRulesListRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	Name     string `json:"name" query:"name" mcp:"name,description=Rule name filter"`
	Enabled  string `json:"enabled" query:"enabled" mcp:"enabled,description=Enabled filter (true/false)"`
	Severity string `json:"severity" query:"severity" mcp:"severity,description=Severity filter"`
	PageNum  int    `json:"pageNum" query:"pageNum" mcp:"pageNum,description=Page number (default 1)"`
	PageSize int    `json:"pageSize" query:"pageSize" mcp:"pageSize,description=Items per page (default 20)"`
}

// LogRulesListResponse represents the log rules list response.
type LogRulesListResponse struct {
	Rules       []*dbmodel.LogAlertRules `json:"rules"`
	Total       int64                    `json:"total"`
	Offset      int                      `json:"offset"`
	Limit       int                      `json:"limit"`
	ClusterName string                   `json:"cluster_name"`
}

// ===== Log Alert Rules Multi-Cluster =====

// LogRulesMultiClusterRequest represents the request for multi-cluster log rules.
type LogRulesMultiClusterRequest struct {
	Name     string `json:"name" query:"name" mcp:"name,description=Rule name filter"`
	Enabled  string `json:"enabled" query:"enabled" mcp:"enabled,description=Enabled filter (true/false)"`
	Severity string `json:"severity" query:"severity" mcp:"severity,description=Severity filter"`
	PageNum  int    `json:"pageNum" query:"pageNum" mcp:"pageNum,description=Page number (default 1)"`
	PageSize int    `json:"pageSize" query:"pageSize" mcp:"pageSize,description=Items per page (default 20)"`
}

// LogRulesMultiClusterResponse represents multi-cluster log rules.
type LogRulesMultiClusterResponse struct {
	Clusters []ClusterLogRules `json:"clusters"`
}

// ClusterLogRules represents log rules for a cluster.
type ClusterLogRules struct {
	ClusterName string                   `json:"cluster_name"`
	Rules       []*dbmodel.LogAlertRules `json:"rules"`
	Total       int64                    `json:"total"`
	Error       string                   `json:"error,omitempty"`
}

// ===== Log Alert Rule Detail =====

// LogRuleDetailRequest represents the request for log rule detail.
type LogRuleDetailRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	ID      int64  `json:"id" param:"id" mcp:"id,description=Rule ID,required"`
}

// LogRuleDetailResponse is the log rule detail.
type LogRuleDetailResponse = dbmodel.LogAlertRules

// ===== Log Alert Rule Statistics =====

// LogRuleStatisticsRequest represents the request for log rule statistics.
type LogRuleStatisticsRequest struct {
	Cluster  string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	ID       int64  `json:"id" param:"id" mcp:"id,description=Rule ID,required"`
	DateFrom string `json:"date_from" query:"date_from" mcp:"date_from,description=Start date (YYYY-MM-DD)"`
	DateTo   string `json:"date_to" query:"date_to" mcp:"date_to,description=End date (YYYY-MM-DD)"`
}

// LogRuleStatisticsResponse represents log rule statistics.
type LogRuleStatisticsResponse struct {
	Summary    interface{} `json:"summary"`
	Statistics interface{} `json:"statistics"`
}

// ===== Log Alert Rule Versions =====

// LogRuleVersionsRequest represents the request for log rule versions.
type LogRuleVersionsRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"cluster,description=Target cluster name (optional)"`
	ID      int64  `json:"id" param:"id" mcp:"id,description=Rule ID,required"`
}

// LogRuleVersionsResponse represents log rule versions.
type LogRuleVersionsResponse struct {
	Versions interface{} `json:"versions"`
	Total    int         `json:"total"`
}

// ===== Register Alert Endpoints =====

func init() {
	// Phase 7: Alert List - replaces ListAlertEvents
	unified.Register(&unified.EndpointDef[AlertListRequest, AlertListResponse]{
		Name:        "alert_list",
		Description: "List alerts with filtering by cluster, source, severity, status, workload, pod, or node.",
		HTTPMethod:  "GET",
		HTTPPath:    "/alerts",
		MCPToolName: "lens_alert_list",
		Handler:     handleAlertList,
	})

	// Phase 7: Alert Summary - replaces GetAlertSummary
	unified.Register(&unified.EndpointDef[AlertSummaryRequest, AlertSummaryResponse]{
		Name:        "alert_summary",
		Description: "Get alert summary by severity level with counts and changes from the last hour.",
		HTTPMethod:  "GET",
		HTTPPath:    "/alerts/summary",
		MCPToolName: "lens_alert_summary",
		Handler:     handleAlertSummary,
	})

	// Phase 7: Alert Trend - replaces GetAlertTrend
	unified.Register(&unified.EndpointDef[AlertTrendRequest, AlertTrendResponse]{
		Name:        "alert_trend",
		Description: "Get alert trend data over time grouped by hour or day.",
		HTTPMethod:  "GET",
		HTTPPath:    "/alerts/trend",
		MCPToolName: "lens_alert_trend",
		Handler:     handleAlertTrend,
	})

	// Phase 7: Alert Top Sources - replaces GetTopAlertSources
	unified.Register(&unified.EndpointDef[AlertTopSourcesRequest, AlertTopSourcesResponse]{
		Name:        "alert_top_sources",
		Description: "Get the top alert sources (alert names) by occurrence count.",
		HTTPMethod:  "GET",
		HTTPPath:    "/alerts/top-sources",
		MCPToolName: "lens_alert_top_sources",
		Handler:     handleAlertTopSources,
	})

	// Phase 7: Alerts By Cluster - replaces GetAlertsByCluster
	unified.Register(&unified.EndpointDef[AlertsByClusterRequest, AlertsByClusterResponse]{
		Name:        "alerts_by_cluster",
		Description: "Get alert counts grouped by cluster.",
		HTTPMethod:  "GET",
		HTTPPath:    "/alerts/by-cluster",
		MCPToolName: "lens_alerts_by_cluster",
		Handler:     handleAlertsByCluster,
	})

	// Phase 7: Alert Detail - replaces GetAlertEvent
	unified.Register(&unified.EndpointDef[AlertDetailRequest, AlertDetailResponse]{
		Name:        "alert_detail",
		Description: "Get detailed information about a specific alert by ID.",
		HTTPMethod:  "GET",
		HTTPPath:    "/alerts/:id",
		MCPToolName: "lens_alert_detail",
		Handler:     handleAlertDetail,
	})

	// Phase 7: Alert Correlations - replaces GetAlertCorrelations
	unified.Register(&unified.EndpointDef[AlertCorrelationsRequest, AlertCorrelationsResponse]{
		Name:        "alert_correlations",
		Description: "Get alerts correlated with a specific alert by workload, pod, node, or time.",
		HTTPMethod:  "GET",
		HTTPPath:    "/alerts/:id/correlations",
		MCPToolName: "lens_alert_correlations",
		Handler:     handleAlertCorrelations,
	})

	// Phase 7: Metric Alert Rules List - replaces ListMetricAlertRules
	unified.Register(&unified.EndpointDef[MetricRulesListRequest, MetricRulesListResponse]{
		Name:        "metric_rules_list",
		Description: "List metric alert rules with filtering by name, cluster, enabled status, and sync status.",
		HTTPMethod:  "GET",
		HTTPPath:    "/metric-alert-rules",
		MCPToolName: "lens_metric_rules_list",
		Handler:     handleMetricRulesList,
	})

	// Phase 7: Metric Alert Rule Detail - replaces GetMetricAlertRule
	unified.Register(&unified.EndpointDef[MetricRuleDetailRequest, MetricRuleDetailResponse]{
		Name:        "metric_rule_detail",
		Description: "Get detailed information about a specific metric alert rule by ID.",
		HTTPMethod:  "GET",
		HTTPPath:    "/metric-alert-rules/:id",
		MCPToolName: "lens_metric_rule_detail",
		Handler:     handleMetricRuleDetail,
	})

	// Phase 7: Metric Alert Rule Status - replaces GetVMRuleStatus
	unified.Register(&unified.EndpointDef[MetricRuleStatusRequest, MetricRuleStatusResponse]{
		Name:        "metric_rule_status",
		Description: "Get the sync status of a metric alert rule (VMRule) in the cluster.",
		HTTPMethod:  "GET",
		HTTPPath:    "/metric-alert-rules/:id/status",
		MCPToolName: "lens_metric_rule_status",
		Handler:     handleMetricRuleStatus,
	})

	// Phase 7: Log Alert Rules List - replaces ListLogAlertRules
	unified.Register(&unified.EndpointDef[LogRulesListRequest, LogRulesListResponse]{
		Name:        "log_rules_list",
		Description: "List log alert rules with filtering by cluster, name, enabled status, and severity.",
		HTTPMethod:  "GET",
		HTTPPath:    "/log-alert-rules",
		MCPToolName: "lens_log_rules_list",
		Handler:     handleLogRulesList,
	})

	// Phase 7: Log Alert Rules Multi-Cluster - replaces ListLogAlertRulesMultiCluster
	unified.Register(&unified.EndpointDef[LogRulesMultiClusterRequest, LogRulesMultiClusterResponse]{
		Name:        "log_rules_multi_cluster",
		Description: "List log alert rules from all clusters.",
		HTTPMethod:  "GET",
		HTTPPath:    "/log-alert-rules/multi-cluster",
		MCPToolName: "lens_log_rules_multi_cluster",
		Handler:     handleLogRulesMultiCluster,
	})

	// Phase 7: Log Alert Rule Detail - replaces GetLogAlertRule
	unified.Register(&unified.EndpointDef[LogRuleDetailRequest, LogRuleDetailResponse]{
		Name:        "log_rule_detail",
		Description: "Get detailed information about a specific log alert rule by ID.",
		HTTPMethod:  "GET",
		HTTPPath:    "/log-alert-rules/:id",
		MCPToolName: "lens_log_rule_detail",
		Handler:     handleLogRuleDetail,
	})

	// Phase 7: Log Alert Rule Statistics - replaces GetLogAlertRuleStatistics
	unified.Register(&unified.EndpointDef[LogRuleStatisticsRequest, LogRuleStatisticsResponse]{
		Name:        "log_rule_statistics",
		Description: "Get statistics for a log alert rule including trigger counts over time.",
		HTTPMethod:  "GET",
		HTTPPath:    "/log-alert-rules/:id/statistics",
		MCPToolName: "lens_log_rule_statistics",
		Handler:     handleLogRuleStatistics,
	})

	// Phase 7: Log Alert Rule Versions - replaces GetLogAlertRuleVersions
	unified.Register(&unified.EndpointDef[LogRuleVersionsRequest, LogRuleVersionsResponse]{
		Name:        "log_rule_versions",
		Description: "Get version history for a log alert rule.",
		HTTPMethod:  "GET",
		HTTPPath:    "/log-alert-rules/:id/versions",
		MCPToolName: "lens_log_rule_versions",
		Handler:     handleLogRuleVersions,
	})
}

// ===== Handler Implementations =====

func handleAlertList(ctx context.Context, req *AlertListRequest) (*AlertListResponse, error) {
	clusterName := req.Cluster
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	pageNum := req.PageNum
	pageSize := req.PageSize
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	if clusterName == "all" {
		return alertListFromAllRobustClusters(ctx, req, pageNum, pageSize)
	}

	rc, err := getRobustClient(clusterName)
	if err != nil {
		return nil, err
	}

	fetchLimit := pageNum * pageSize
	if fetchLimit < pageSize {
		fetchLimit = pageSize
	}
	if fetchLimit > 10000 {
		fetchLimit = 10000
	}

	p := robustAlertListParams(req, fetchLimit)
	raw, err := rc.GetRaw(ctx, "/alerts", p)
	if err != nil {
		return nil, fmt.Errorf("robust alerts list: %w", err)
	}

	alerts, _, err := decodeRobustAlertListPayload(raw)
	if err != nil {
		return nil, err
	}
	for _, a := range alerts {
		if a.ClusterName == "" {
			a.ClusterName = clusterName
		}
	}
	alerts = filterAlertListClientSide(alerts, req)

	total := int64(len(alerts))
	start := (pageNum - 1) * pageSize
	if start > len(alerts) {
		start = len(alerts)
	}
	end := start + pageSize
	if end > len(alerts) {
		end = len(alerts)
	}
	page := alerts[start:end]

	return &AlertListResponse{
		Data:     page,
		Total:    total,
		PageNum:  pageNum,
		PageSize: pageSize,
	}, nil
}

func handleAlertDetail(ctx context.Context, req *AlertDetailRequest) (*AlertDetailResponse, error) {
	clusterName := req.Cluster
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	rc, err := getRobustClient(clusterName)
	if err != nil {
		return nil, err
	}

	path := "/alerts/" + url.PathEscape(req.ID)
	raw, err := rc.GetRaw(ctx, path, nil)
	if err != nil {
		return nil, fmt.Errorf("robust alert detail: %w", err)
	}

	alert, err := decodeRobustAlertObject(raw)
	if err != nil {
		return nil, err
	}
	if alert == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("alert not found")
	}
	if alert.ClusterName == "" {
		alert.ClusterName = clusterName
	}

	return alert, nil
}

func handleAlertSummary(ctx context.Context, req *AlertSummaryRequest) (*AlertSummaryResponse, error) {
	clusterName := req.Cluster
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	if clusterName == "all" {
		return alertSummaryFromAllRobustClusters(ctx)
	}

	rc, err := getRobustClient(clusterName)
	if err != nil {
		return nil, err
	}

	raw, err := rc.GetRaw(ctx, "/alerts/summary", nil)
	if err != nil {
		return nil, fmt.Errorf("robust alerts summary: %w", err)
	}

	var payload struct {
		FiringTotal int            `json:"firing_total"`
		BySeverity  map[string]int `json:"by_severity"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode alert summary: %w", err)
	}

	by := payload.BySeverity
	if by == nil {
		by = map[string]int{}
	}

	count := func(want string) int {
		for k, v := range by {
			if strings.EqualFold(k, want) {
				return v
			}
		}
		return 0
	}

	return &AlertSummaryResponse{
		Critical: SeverityCount{Count: count(SeverityCritical), Change: 0},
		High:     SeverityCount{Count: count(SeverityHigh), Change: 0},
		Warning:  SeverityCount{Count: count(SeverityWarning), Change: 0},
		Info:     SeverityCount{Count: count(SeverityInfo), Change: 0},
	}, nil
}

func handleAlertTrend(ctx context.Context, req *AlertTrendRequest) (*AlertTrendResponse, error) {
	clusterName := req.Cluster
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	hours := req.Hours
	if hours <= 0 {
		hours = 24
	}

	if clusterName == "all" {
		return nil, fmt.Errorf("alert trend with cluster=all is not supported for Robust data plane")
	}

	rc, err := getRobustClient(clusterName)
	if err != nil {
		return nil, err
	}

	p := url.Values{}
	p.Set("hours", strconv.Itoa(hours))
	if req.GroupBy != "" {
		p.Set("group_by", req.GroupBy)
	}

	raw, err := rc.GetRaw(ctx, "/alerts/trend", p)
	if err != nil {
		return nil, fmt.Errorf("robust alerts trend: %w", err)
	}

	var payload struct {
		Trend []struct {
			Hour     string `json:"hour"`
			Severity string `json:"severity"`
			Count    int    `json:"count"`
		} `json:"trend"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode alert trend: %w", err)
	}

	interval := time.Hour
	if req.GroupBy == "day" {
		interval = 24 * time.Hour
	}

	now := time.Now()
	startTime := now.Add(-time.Duration(hours) * time.Hour)
	buckets := make(map[int64]*AlertTrendPoint)

	for _, row := range payload.Trend {
		t, err := time.Parse(time.RFC3339, row.Hour)
		if err != nil {
			continue
		}
		bucketKey := t.Truncate(interval).Unix()
		pt, ok := buckets[bucketKey]
		if !ok {
			pt = &AlertTrendPoint{Timestamp: t.Truncate(interval)}
			buckets[bucketKey] = pt
		}
		switch row.Severity {
		case SeverityCritical:
			pt.Critical += row.Count
		case SeverityHigh:
			pt.High += row.Count
		case SeverityWarning:
			pt.Warning += row.Count
		case SeverityInfo:
			pt.Info += row.Count
		}
	}

	if len(buckets) == 0 {
		cur := startTime.Truncate(interval)
		for cur.Before(now) {
			buckets[cur.Unix()] = &AlertTrendPoint{
				Timestamp: cur,
			}
			cur = cur.Add(interval)
		}
	}

	keys := make([]int64, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	out := make(AlertTrendResponse, 0, len(keys))
	for _, k := range keys {
		out = append(out, buckets[k])
	}
	return &out, nil
}

func handleAlertTopSources(ctx context.Context, req *AlertTopSourcesRequest) (*AlertTopSourcesResponse, error) {
	clusterName := req.Cluster
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	hours := req.Hours
	if hours <= 0 {
		hours = 24
	}

	if clusterName == "all" {
		return nil, fmt.Errorf("alert top-sources with cluster=all is not supported for Robust data plane")
	}

	rc, err := getRobustClient(clusterName)
	if err != nil {
		return nil, err
	}

	p := url.Values{}
	p.Set("limit", strconv.Itoa(limit))
	p.Set("hours", strconv.Itoa(hours))

	raw, err := rc.GetRaw(ctx, "/alerts/top-sources", p)
	if err != nil {
		return nil, fmt.Errorf("robust alerts top-sources: %w", err)
	}

	var payload struct {
		TopSources []struct {
			AlertName string `json:"alert_name"`
			Count     int    `json:"count"`
		} `json:"top_sources"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode top sources: %w", err)
	}

	out := make(AlertTopSourcesResponse, 0, len(payload.TopSources))
	for _, s := range payload.TopSources {
		out = append(out, AlertSourceCount{AlertName: s.AlertName, Count: s.Count})
	}
	return &out, nil
}

func handleAlertsByCluster(ctx context.Context, req *AlertsByClusterRequest) (*AlertsByClusterResponse, error) {
	hours := req.Hours
	if hours <= 0 {
		hours = 24
	}

	clusterManager := clientsets.GetClusterManager()
	clusterNames := clusterManager.GetClusterNames()
	result := make(AlertsByClusterResponse, 0, len(clusterNames))

	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	for _, clusterName := range clusterNames {
		rc, err := getRobustClient(clusterName)
		if err != nil {
			log.Warnf("Robust client for cluster %s: %v", clusterName, err)
			continue
		}

		p := url.Values{}
		p.Set("status", AlertStatusFiring)
		p.Set("limit", "10000")

		raw, err := rc.GetRaw(ctx, "/alerts", p)
		if err != nil {
			log.Warnf("Failed to list alerts for cluster %s: %v", clusterName, err)
			continue
		}

		alerts, _, err := decodeRobustAlertListPayload(raw)
		if err != nil {
			log.Warnf("Decode alerts for cluster %s: %v", clusterName, err)
			continue
		}

		n := 0
		for _, a := range alerts {
			if a.StartsAt.After(cutoff) || a.StartsAt.Equal(cutoff) {
				n++
			}
		}

		result = append(result, ClusterAlertCount{
			ClusterName: clusterName,
			Count:       n,
		})
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Count > result[j].Count })
	return &result, nil
}

func handleAlertCorrelations(ctx context.Context, req *AlertCorrelationsRequest) (*AlertCorrelationsResponse, error) {
	clusterName := req.Cluster
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	rc, err := getRobustClient(clusterName)
	if err != nil {
		return nil, err
	}

	basePath := "/alerts/" + url.PathEscape(req.ID)
	rawBase, err := rc.GetRaw(ctx, basePath, nil)
	if err != nil {
		return nil, fmt.Errorf("robust alert detail: %w", err)
	}
	baseAlert, err := decodeRobustAlertObject(rawBase)
	if err != nil {
		return nil, err
	}
	if baseAlert == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("alert not found")
	}

	raw, err := rc.GetRaw(ctx, basePath+"/correlations", nil)
	if err != nil {
		return nil, fmt.Errorf("robust alert correlations: %w", err)
	}

	var payload struct {
		Correlations []struct {
			ID         int64  `json:"id"`
			AlertName  string `json:"alert_name"`
			Severity   string `json:"severity"`
			NodeName   string `json:"node_name"`
			WorkloadID string `json:"workload_id"`
		} `json:"correlations"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode correlations: %w", err)
	}

	correlations := make([]AlertCorrelation, 0, len(payload.Correlations))
	for _, rel := range payload.Correlations {
		rid := strconv.FormatInt(rel.ID, 10)
		if rid == req.ID {
			continue
		}
		correlationType := "time"
		score := 0.5
		if rel.WorkloadID != "" && rel.WorkloadID == baseAlert.WorkloadID {
			correlationType = "workload"
			score = 0.9
		} else if rel.NodeName != "" && rel.NodeName == baseAlert.NodeName {
			correlationType = "node"
			score = 0.7
		}
		correlations = append(correlations, AlertCorrelation{
			AlertID:         rid,
			AlertName:       rel.AlertName,
			Severity:        rel.Severity,
			CorrelationType: correlationType,
			Score:           score,
		})
	}

	return &AlertCorrelationsResponse{Correlations: correlations}, nil
}

func robustAlertListParams(req *AlertListRequest, limit int) url.Values {
	p := url.Values{}
	if req.Status != "" {
		p.Set("status", req.Status)
	}
	if req.Severity != "" {
		p.Set("severity", req.Severity)
	}
	if req.NodeName != "" {
		p.Set("node_name", req.NodeName)
	}
	if req.WorkloadID != "" {
		p.Set("workload_id", req.WorkloadID)
	}
	if req.Source != "" {
		p.Set("source", req.Source)
	}
	if req.AlertName != "" {
		p.Set("alert_name", req.AlertName)
	}
	if req.PodName != "" {
		p.Set("pod_name", req.PodName)
	}
	p.Set("limit", strconv.Itoa(limit))
	return p
}

func decodeRobustAlertListPayload(raw json.RawMessage) ([]*dbmodel.AlertEvents, int, error) {
	var top struct {
		Alerts []json.RawMessage `json:"alerts"`
		Count  int               `json:"count"`
	}
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, 0, fmt.Errorf("decode alert list envelope: %w", err)
	}
	out := make([]*dbmodel.AlertEvents, 0, len(top.Alerts))
	for _, item := range top.Alerts {
		ev, err := decodeRobustAlertObject(item)
		if err != nil || ev == nil {
			continue
		}
		out = append(out, ev)
	}
	return out, top.Count, nil
}

func decodeRobustAlertObject(raw json.RawMessage) (*dbmodel.AlertEvents, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("decode alert object: %w", err)
	}
	if len(m) == 0 {
		return nil, nil
	}

	ev := &dbmodel.AlertEvents{
		ID:          stringFromAny(m["id"]),
		AlertName:   stringField(m, "alert_name"),
		Severity:    stringField(m, "severity"),
		Status:      stringField(m, "status"),
		Source:      stringField(m, "source"),
		WorkloadID:  stringField(m, "workload_id"),
		PodName:     stringField(m, "pod_name"),
		NodeName:    stringField(m, "node_name"),
		ClusterName: stringField(m, "cluster_name"),
	}

	startStr := stringField(m, "starts_at")
	if startStr == "" {
		startStr = stringField(m, "fired_at")
	}
	if startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			ev.StartsAt = t
		}
	}
	endStr := stringField(m, "ends_at")
	if endStr == "" {
		endStr = stringField(m, "resolved_at")
	}
	if endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			ev.EndsAt = t
		}
	}

	if labels, ok := m["labels"]; ok && labels != nil {
		b, _ := json.Marshal(labels)
		_ = json.Unmarshal(b, &ev.Labels)
	}
	if ann, ok := m["annotations"]; ok && ann != nil {
		b, _ := json.Marshal(ann)
		_ = json.Unmarshal(b, &ev.Annotations)
	}
	if msg := stringField(m, "message"); msg != "" {
		ev.RawData = dbmodel.ExtType{"message": msg}
	}
	return ev, nil
}

func stringField(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func stringFromAny(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case json.Number:
		return t.String()
	default:
		return fmt.Sprint(t)
	}
}

func filterAlertListClientSide(alerts []*dbmodel.AlertEvents, req *AlertListRequest) []*dbmodel.AlertEvents {
	if req.Source == "" && req.AlertName == "" && req.PodName == "" {
		return alerts
	}
	out := make([]*dbmodel.AlertEvents, 0, len(alerts))
	for _, a := range alerts {
		if req.Source != "" && a.Source != req.Source {
			continue
		}
		if req.AlertName != "" && a.AlertName != req.AlertName {
			continue
		}
		if req.PodName != "" && a.PodName != req.PodName {
			continue
		}
		out = append(out, a)
	}
	return out
}

func alertListFromAllRobustClusters(ctx context.Context, req *AlertListRequest, pageNum, pageSize int) (*AlertListResponse, error) {
	cm := clientsets.GetClusterManager()
	names := cm.GetClusterNames()
	merged := make([]*dbmodel.AlertEvents, 0)

	fetchLimit := 2000
	for _, name := range names {
		rc, err := getRobustClient(name)
		if err != nil {
			log.Warnf("Robust client for cluster %s: %v", name, err)
			continue
		}
		p := robustAlertListParams(req, fetchLimit)
		raw, err := rc.GetRaw(ctx, "/alerts", p)
		if err != nil {
			log.Warnf("Robust alerts list for cluster %s: %v", name, err)
			continue
		}
		alerts, _, err := decodeRobustAlertListPayload(raw)
		if err != nil {
			log.Warnf("Decode alerts for cluster %s: %v", name, err)
			continue
		}
		for _, a := range alerts {
			if a.ClusterName == "" {
				a.ClusterName = name
			}
			merged = append(merged, a)
		}
	}

	merged = filterAlertListClientSide(merged, req)
	sort.Slice(merged, func(i, j int) bool { return merged[i].StartsAt.After(merged[j].StartsAt) })

	total := int64(len(merged))
	start := (pageNum - 1) * pageSize
	if start > len(merged) {
		start = len(merged)
	}
	end := start + pageSize
	if end > len(merged) {
		end = len(merged)
	}
	page := merged[start:end]

	return &AlertListResponse{
		Data:     page,
		Total:    total,
		PageNum:  pageNum,
		PageSize: pageSize,
	}, nil
}

func alertSummaryFromAllRobustClusters(ctx context.Context) (*AlertSummaryResponse, error) {
	cm := clientsets.GetClusterManager()
	names := cm.GetClusterNames()
	totals := map[string]int{
		SeverityCritical: 0,
		SeverityHigh:     0,
		SeverityWarning:  0,
		SeverityInfo:     0,
	}

	for _, name := range names {
		rc, err := getRobustClient(name)
		if err != nil {
			log.Warnf("Robust client for cluster %s: %v", name, err)
			continue
		}
		raw, err := rc.GetRaw(ctx, "/alerts/summary", nil)
		if err != nil {
			log.Warnf("Robust alerts summary for cluster %s: %v", name, err)
			continue
		}
		var payload struct {
			BySeverity map[string]int `json:"by_severity"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			log.Warnf("Decode alert summary for cluster %s: %v", name, err)
			continue
		}
		for k, v := range payload.BySeverity {
			for _, sev := range []string{SeverityCritical, SeverityHigh, SeverityWarning, SeverityInfo} {
				if strings.EqualFold(k, sev) {
					totals[sev] += v
					break
				}
			}
		}
	}

	return &AlertSummaryResponse{
		Critical: SeverityCount{Count: totals[SeverityCritical], Change: 0},
		High:     SeverityCount{Count: totals[SeverityHigh], Change: 0},
		Warning:  SeverityCount{Count: totals[SeverityWarning], Change: 0},
		Info:     SeverityCount{Count: totals[SeverityInfo], Change: 0},
	}, nil
}

// handleMetricRulesList handles metric rules list requests.
func handleMetricRulesList(ctx context.Context, req *MetricRulesListRequest) (*MetricRulesListResponse, error) {
	pageNum := req.PageNum
	pageSize := req.PageSize
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	filter := &database.MetricAlertRuleFilter{
		Offset: (pageNum - 1) * pageSize,
		Limit:  pageSize,
	}

	if req.Name != "" {
		filter.Name = &req.Name
	}
	if req.ClusterName != "" {
		filter.ClusterName = &req.ClusterName
	}
	if req.Enabled != "" {
		enabled, err := strconv.ParseBool(req.Enabled)
		if err == nil {
			filter.Enabled = &enabled
		}
	}
	if req.SyncStatus != "" {
		filter.SyncStatus = &req.SyncStatus
	}

	facade := database.GetFacade().GetMetricAlertRule()
	rules, total, err := facade.ListMetricAlertRules(ctx, filter)
	if err != nil {
		return nil, err
	}

	return &MetricRulesListResponse{
		Data:     rules,
		Total:    total,
		PageNum:  pageNum,
		PageSize: pageSize,
	}, nil
}

// handleMetricRuleDetail handles metric rule detail requests.
func handleMetricRuleDetail(ctx context.Context, req *MetricRuleDetailRequest) (*MetricRuleDetailResponse, error) {
	facade := database.GetFacade().GetMetricAlertRule()
	rule, err := facade.GetMetricAlertRuleByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if rule == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("rule not found")
	}

	return rule, nil
}

// handleMetricRuleStatus handles metric rule status requests.
func handleMetricRuleStatus(ctx context.Context, req *MetricRuleStatusRequest) (*MetricRuleStatusResponse, error) {
	facade := database.GetFacade().GetMetricAlertRule()
	rule, err := facade.GetMetricAlertRuleByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if rule == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("rule not found")
	}

	return &MetricRuleStatusResponse{
		Status:       rule.SyncStatus,
		LastSyncTime: rule.UpdatedAt.Format(time.RFC3339),
		Error:        rule.SyncMessage,
	}, nil
}

// handleLogRulesList handles log rules list requests.
func handleLogRulesList(ctx context.Context, req *LogRulesListRequest) (*LogRulesListResponse, error) {
	clusterName := req.Cluster
	if clusterName == "" {
		defaultCluster := clientsets.GetClusterManager().GetDefaultClusterName()
		if defaultCluster != "" {
			clusterName = defaultCluster
		} else {
			clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
		}
	}

	pageNum := req.PageNum
	pageSize := req.PageSize
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	filter := &database.LogAlertRuleFilter{
		ClusterName: clusterName,
		Offset:      (pageNum - 1) * pageSize,
		Limit:       pageSize,
	}

	if req.Name != "" {
		filter.Keyword = req.Name
	}
	if req.Enabled != "" {
		enabled, err := strconv.ParseBool(req.Enabled)
		if err == nil {
			filter.Enabled = &enabled
		}
	}
	if req.Severity != "" {
		filter.Severity = req.Severity
	}

	facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()
	rules, total, err := facade.ListLogAlertRules(ctx, filter)
	if err != nil {
		return nil, err
	}

	return &LogRulesListResponse{
		Rules:       rules,
		Total:       total,
		Offset:      filter.Offset,
		Limit:       filter.Limit,
		ClusterName: clusterName,
	}, nil
}

// handleLogRulesMultiCluster handles multi-cluster log rules requests.
func handleLogRulesMultiCluster(ctx context.Context, req *LogRulesMultiClusterRequest) (*LogRulesMultiClusterResponse, error) {
	clusterManager := clientsets.GetClusterManager()
	clusterNames := clusterManager.GetClusterNames()

	pageNum := req.PageNum
	pageSize := req.PageSize
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	results := make([]ClusterLogRules, 0, len(clusterNames))

	for _, clusterName := range clusterNames {
		filter := &database.LogAlertRuleFilter{
			ClusterName: clusterName,
			Offset:      (pageNum - 1) * pageSize,
			Limit:       pageSize,
		}

		if req.Name != "" {
			filter.Keyword = req.Name
		}
		if req.Enabled != "" {
			enabled, err := strconv.ParseBool(req.Enabled)
			if err == nil {
				filter.Enabled = &enabled
			}
		}
		if req.Severity != "" {
			filter.Severity = req.Severity
		}

		facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()
		rules, total, err := facade.ListLogAlertRules(ctx, filter)

		result := ClusterLogRules{
			ClusterName: clusterName,
		}

		if err != nil {
			log.Warnf("Failed to list rules from cluster %s: %v", clusterName, err)
			result.Error = err.Error()
		} else {
			result.Rules = rules
			result.Total = total
		}

		results = append(results, result)
	}

	return &LogRulesMultiClusterResponse{
		Clusters: results,
	}, nil
}

// handleLogRuleDetail handles log rule detail requests.
func handleLogRuleDetail(ctx context.Context, req *LogRuleDetailRequest) (*LogRuleDetailResponse, error) {
	clusterName := req.Cluster
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()
	rule, err := facade.GetLogAlertRuleByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if rule == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("rule not found")
	}

	return rule, nil
}

// handleLogRuleStatistics handles log rule statistics requests.
func handleLogRuleStatistics(ctx context.Context, req *LogRuleStatisticsRequest) (*LogRuleStatisticsResponse, error) {
	clusterName := req.Cluster
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Parse time range (reuse existing parseTimeRange logic)
	var dateFrom, dateTo time.Time
	if req.DateFrom != "" {
		parsed, err := time.Parse("2006-01-02", req.DateFrom)
		if err == nil {
			dateFrom = parsed
		}
	}
	if req.DateTo != "" {
		parsed, err := time.Parse("2006-01-02", req.DateTo)
		if err == nil {
			dateTo = parsed
		}
	}
	if dateFrom.IsZero() {
		dateFrom = time.Now().AddDate(0, 0, -7)
	}
	if dateTo.IsZero() {
		dateTo = time.Now()
	}

	facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()

	summary, err := facade.GetRuleStatisticsSummary(ctx, req.ID, dateFrom, dateTo)
	if err != nil {
		return nil, err
	}

	filter := &database.LogAlertRuleStatisticFilter{
		RuleID:      req.ID,
		ClusterName: clusterName,
		DateFrom:    dateFrom,
		DateTo:      dateTo,
		Limit:       100,
	}

	stats, err := facade.ListRuleStatistics(ctx, filter)
	if err != nil {
		return nil, err
	}

	return &LogRuleStatisticsResponse{
		Summary:    summary,
		Statistics: stats,
	}, nil
}

// handleLogRuleVersions handles log rule versions requests.
func handleLogRuleVersions(ctx context.Context, req *LogRuleVersionsRequest) (*LogRuleVersionsResponse, error) {
	clusterName := req.Cluster
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	facade := database.GetFacadeForCluster(clusterName).GetLogAlertRule()
	versions, err := facade.ListRuleVersions(ctx, req.ID)
	if err != nil {
		return nil, err
	}

	return &LogRuleVersionsResponse{
		Versions: versions,
		Total:    len(versions),
	}, nil
}
