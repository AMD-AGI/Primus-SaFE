// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package api provides unified API endpoints for alert operations.
// These endpoints work for both HTTP REST and MCP protocols.
package api

import (
	"context"
	"strconv"
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

// handleAlertList handles alert list requests.
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

	filter := &database.AlertEventsFilter{
		Offset: (pageNum - 1) * pageSize,
		Limit:  pageSize,
	}

	if clusterName != "" && clusterName != "all" {
		filter.ClusterName = &clusterName
	}
	if req.Source != "" {
		filter.Source = &req.Source
	}
	if req.AlertName != "" {
		filter.AlertName = &req.AlertName
	}
	if req.Severity != "" {
		filter.Severity = &req.Severity
	}
	if req.Status != "" {
		filter.Status = &req.Status
	}
	if req.WorkloadID != "" {
		filter.WorkloadID = &req.WorkloadID
	}
	if req.PodName != "" {
		filter.PodName = &req.PodName
	}
	if req.NodeName != "" {
		filter.NodeName = &req.NodeName
	}

	facade := database.GetFacadeForCluster(clusterName).GetAlert()
	alerts, total, err := facade.ListAlertEventss(ctx, filter)
	if err != nil {
		return nil, err
	}

	return &AlertListResponse{
		Data:     alerts,
		Total:    total,
		PageNum:  pageNum,
		PageSize: pageSize,
	}, nil
}

// handleAlertDetail handles alert detail requests.
func handleAlertDetail(ctx context.Context, req *AlertDetailRequest) (*AlertDetailResponse, error) {
	clusterName := req.Cluster
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	facade := database.GetFacadeForCluster(clusterName).GetAlert()
	alert, err := facade.GetAlertEventsByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if alert == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("alert not found")
	}

	return alert, nil
}

// handleAlertSummary handles alert summary requests.
func handleAlertSummary(ctx context.Context, req *AlertSummaryRequest) (*AlertSummaryResponse, error) {
	clusterName := req.Cluster
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	facade := database.GetFacadeForCluster(clusterName).GetAlert()
	now := time.Now()
	oneHourAgo := now.Add(-time.Hour)

	// Get current firing alerts
	currentFilter := &database.AlertEventsFilter{
		Status: strPtr(AlertStatusFiring),
		Limit:  10000,
	}
	if clusterName != "" && clusterName != "all" {
		currentFilter.ClusterName = &clusterName
	}

	alerts, _, err := facade.ListAlertEventss(ctx, currentFilter)
	if err != nil {
		return nil, err
	}

	currentCounts := map[string]int{
		SeverityCritical: 0,
		SeverityHigh:     0,
		SeverityWarning:  0,
		SeverityInfo:     0,
	}
	for _, alert := range alerts {
		currentCounts[alert.Severity]++
	}

	// Get historical counts
	historicalFilter := &database.AlertEventsFilter{
		Status:       strPtr(AlertStatusFiring),
		StartsBefore: &oneHourAgo,
		Limit:        10000,
	}
	if clusterName != "" && clusterName != "all" {
		historicalFilter.ClusterName = &clusterName
	}

	historicalAlerts, _, err := facade.ListAlertEventss(ctx, historicalFilter)
	if err != nil {
		log.Warnf("Failed to get historical alerts: %v", err)
	}

	historicalCounts := map[string]int{
		SeverityCritical: 0,
		SeverityHigh:     0,
		SeverityWarning:  0,
		SeverityInfo:     0,
	}
	for _, alert := range historicalAlerts {
		historicalCounts[alert.Severity]++
	}

	return &AlertSummaryResponse{
		Critical: SeverityCount{
			Count:  currentCounts[SeverityCritical],
			Change: currentCounts[SeverityCritical] - historicalCounts[SeverityCritical],
		},
		High: SeverityCount{
			Count:  currentCounts[SeverityHigh],
			Change: currentCounts[SeverityHigh] - historicalCounts[SeverityHigh],
		},
		Warning: SeverityCount{
			Count:  currentCounts[SeverityWarning],
			Change: currentCounts[SeverityWarning] - historicalCounts[SeverityWarning],
		},
		Info: SeverityCount{
			Count:  currentCounts[SeverityInfo],
			Change: currentCounts[SeverityInfo] - historicalCounts[SeverityInfo],
		},
	}, nil
}

// handleAlertTrend handles alert trend requests.
func handleAlertTrend(ctx context.Context, req *AlertTrendRequest) (*AlertTrendResponse, error) {
	clusterName := req.Cluster
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	hours := req.Hours
	if hours <= 0 {
		hours = 24
	}

	facade := database.GetFacadeForCluster(clusterName).GetAlert()
	now := time.Now()
	startTime := now.Add(-time.Duration(hours) * time.Hour)

	filter := &database.AlertEventsFilter{
		StartsAfter: &startTime,
		Limit:       10000,
	}
	if clusterName != "" && clusterName != "all" {
		filter.ClusterName = &clusterName
	}

	alerts, _, err := facade.ListAlertEventss(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Determine interval
	var interval time.Duration
	if req.GroupBy == "day" {
		interval = 24 * time.Hour
	} else {
		interval = time.Hour
	}

	// Create time buckets
	buckets := make(map[int64]*AlertTrendPoint)
	current := startTime.Truncate(interval)
	for current.Before(now) {
		buckets[current.Unix()] = &AlertTrendPoint{
			Timestamp: current,
			Critical:  0,
			High:      0,
			Warning:   0,
			Info:      0,
		}
		current = current.Add(interval)
	}

	// Fill buckets
	for _, alert := range alerts {
		bucketTime := alert.StartsAt.Truncate(interval).Unix()
		if bucket, ok := buckets[bucketTime]; ok {
			switch alert.Severity {
			case SeverityCritical:
				bucket.Critical++
			case SeverityHigh:
				bucket.High++
			case SeverityWarning:
				bucket.Warning++
			case SeverityInfo:
				bucket.Info++
			}
		}
	}

	// Convert to sorted slice
	result := make(AlertTrendResponse, 0, len(buckets))
	current = startTime.Truncate(interval)
	for current.Before(now) {
		if bucket, ok := buckets[current.Unix()]; ok {
			result = append(result, bucket)
		}
		current = current.Add(interval)
	}

	return &result, nil
}

// handleAlertTopSources handles top alert sources requests.
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

	facade := database.GetFacadeForCluster(clusterName).GetAlert()
	startTime := time.Now().Add(-time.Duration(hours) * time.Hour)

	filter := &database.AlertEventsFilter{
		StartsAfter: &startTime,
		Limit:       10000,
	}
	if clusterName != "" && clusterName != "all" {
		filter.ClusterName = &clusterName
	}

	alerts, _, err := facade.ListAlertEventss(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Count by alert name
	counts := make(map[string]int)
	for _, alert := range alerts {
		counts[alert.AlertName]++
	}

	// Sort and limit
	sources := make(AlertTopSourcesResponse, 0, len(counts))
	for name, count := range counts {
		sources = append(sources, AlertSourceCount{
			AlertName: name,
			Count:     count,
		})
	}

	// Sort by count descending
	for i := 0; i < len(sources); i++ {
		for j := i + 1; j < len(sources); j++ {
			if sources[j].Count > sources[i].Count {
				sources[i], sources[j] = sources[j], sources[i]
			}
		}
	}

	if len(sources) > limit {
		sources = sources[:limit]
	}

	return &sources, nil
}

// handleAlertsByCluster handles alerts by cluster requests.
func handleAlertsByCluster(ctx context.Context, req *AlertsByClusterRequest) (*AlertsByClusterResponse, error) {
	hours := req.Hours
	if hours <= 0 {
		hours = 24
	}

	clusterManager := clientsets.GetClusterManager()
	clusterNames := clusterManager.GetClusterNames()
	startTime := time.Now().Add(-time.Duration(hours) * time.Hour)

	result := make(AlertsByClusterResponse, 0, len(clusterNames))

	for _, clusterName := range clusterNames {
		facade := database.GetFacadeForCluster(clusterName).GetAlert()
		filter := &database.AlertEventsFilter{
			StartsAfter: &startTime,
			Status:      strPtr(AlertStatusFiring),
			ClusterName: &clusterName,
			Limit:       10000,
		}

		alerts, _, err := facade.ListAlertEventss(ctx, filter)
		if err != nil {
			log.Warnf("Failed to get alerts for cluster %s: %v", clusterName, err)
			continue
		}

		result = append(result, ClusterAlertCount{
			ClusterName: clusterName,
			Count:       len(alerts),
		})
	}

	// Sort by count descending
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Count > result[i].Count {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return &result, nil
}

// handleAlertCorrelations handles alert correlations requests.
func handleAlertCorrelations(ctx context.Context, req *AlertCorrelationsRequest) (*AlertCorrelationsResponse, error) {
	clusterName := req.Cluster
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	facade := database.GetFacadeForCluster(clusterName).GetAlert()
	alert, err := facade.GetAlertEventsByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if alert == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("alert not found")
	}

	// Find related alerts within time window
	timeWindow := 30 * time.Minute
	startTime := alert.StartsAt.Add(-timeWindow)
	endTime := alert.StartsAt.Add(timeWindow)

	filter := &database.AlertEventsFilter{
		StartsAfter:  &startTime,
		StartsBefore: &endTime,
		Limit:        100,
	}

	if alert.WorkloadID != "" {
		filter.WorkloadID = &alert.WorkloadID
	} else if alert.PodName != "" {
		filter.PodName = &alert.PodName
	} else if alert.NodeName != "" {
		filter.NodeName = &alert.NodeName
	}

	relatedAlerts, _, err := facade.ListAlertEventss(ctx, filter)
	if err != nil {
		return nil, err
	}

	correlations := make([]AlertCorrelation, 0)
	for _, related := range relatedAlerts {
		if related.ID == req.ID {
			continue
		}

		correlationType := "time"
		score := 0.5

		if related.WorkloadID == alert.WorkloadID && alert.WorkloadID != "" {
			correlationType = "workload"
			score = 0.9
		} else if related.PodName == alert.PodName && alert.PodName != "" {
			correlationType = "pod"
			score = 0.85
		} else if related.NodeName == alert.NodeName && alert.NodeName != "" {
			correlationType = "node"
			score = 0.7
		}

		correlations = append(correlations, AlertCorrelation{
			AlertID:         related.ID,
			AlertName:       related.AlertName,
			Severity:        related.Severity,
			CorrelationType: correlationType,
			Score:           score,
		})
	}

	return &AlertCorrelationsResponse{
		Correlations: correlations,
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
