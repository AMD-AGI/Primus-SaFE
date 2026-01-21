// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/backfill"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

func init() {
	// ========== Config Endpoints ==========
	unified.Register(&unified.EndpointDef[GithubWorkflowConfigsListRequest, GithubWorkflowConfigsListResponse]{
		Name:        "github_workflow_configs_list",
		Description: "List all GitHub workflow metric collection configurations",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/configs",
		MCPToolName: "lens_github_workflow_configs_list",
		Handler:     handleGithubWorkflowConfigsList,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowConfigGetRequest, *dbmodel.GithubWorkflowConfigs]{
		Name:        "github_workflow_config_get",
		Description: "Get a specific GitHub workflow configuration by ID",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/configs/:id",
		MCPToolName: "lens_github_workflow_config_get",
		Handler:     handleGithubWorkflowConfigGet,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowRunsListRequest, GithubWorkflowRunsListResponse]{
		Name:        "github_workflow_config_runs",
		Description: "List workflow runs for a specific configuration",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/configs/:id/runs",
		MCPToolName: "lens_github_workflow_config_runs",
		Handler:     handleGithubWorkflowConfigRuns,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowSchemasListRequest, GithubWorkflowSchemasListResponse]{
		Name:        "github_workflow_schemas_list",
		Description: "List schemas for a configuration",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/configs/:id/schemas",
		MCPToolName: "lens_github_workflow_schemas_list",
		Handler:     handleGithubWorkflowSchemasList,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowActiveSchemaRequest, *dbmodel.GithubWorkflowMetricSchemas]{
		Name:        "github_workflow_active_schema",
		Description: "Get the active schema for a configuration",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/configs/:id/schemas/active",
		MCPToolName: "lens_github_workflow_active_schema",
		Handler:     handleGithubWorkflowActiveSchema,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowMetricsListRequest, GithubWorkflowMetricsListResponse]{
		Name:        "github_workflow_metrics_list",
		Description: "List metrics for a configuration",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/configs/:id/metrics",
		MCPToolName: "lens_github_workflow_metrics_list",
		Handler:     handleGithubWorkflowMetricsList,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowStatsRequest, GithubWorkflowStatsResponse]{
		Name:        "github_workflow_stats",
		Description: "Get statistics for a configuration's metrics",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/configs/:id/stats",
		MCPToolName: "lens_github_workflow_stats",
		Handler:     handleGithubWorkflowStats,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowSummaryRequest, GithubWorkflowSummaryResponse]{
		Name:        "github_workflow_summary",
		Description: "Get summary statistics for a configuration",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/configs/:id/summary",
		MCPToolName: "lens_github_workflow_summary",
		Handler:     handleGithubWorkflowSummary,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowDimensionsRequest, GithubWorkflowDimensionsResponse]{
		Name:        "github_workflow_dimensions",
		Description: "Get available dimensions with their values for a configuration",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/configs/:id/dimensions",
		MCPToolName: "lens_github_workflow_dimensions",
		Handler:     handleGithubWorkflowDimensions,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowDimensionValuesRequest, GithubWorkflowDimensionValuesResponse]{
		Name:        "github_workflow_dimension_values",
		Description: "Get values for a single dimension",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/configs/:id/dimensions/:dimension/values",
		MCPToolName: "lens_github_workflow_dimension_values",
		Handler:     handleGithubWorkflowDimensionValues,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowFieldsRequest, GithubWorkflowFieldsResponse]{
		Name:        "github_workflow_fields",
		Description: "Get available fields (dimension and metric fields) for a configuration",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/configs/:id/fields",
		MCPToolName: "lens_github_workflow_fields",
		Handler:     handleGithubWorkflowFields,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowBackfillStatusRequest, GithubWorkflowBackfillStatusResponse]{
		Name:        "github_workflow_backfill_status",
		Description: "Get backfill status for a configuration",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/configs/:id/backfill/status",
		MCPToolName: "lens_github_workflow_backfill_status",
		Handler:     handleGithubWorkflowBackfillStatus,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowBackfillTasksRequest, GithubWorkflowBackfillTasksResponse]{
		Name:        "github_workflow_backfill_tasks",
		Description: "List backfill tasks for a configuration",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/configs/:id/backfill/tasks",
		MCPToolName: "lens_github_workflow_backfill_tasks",
		Handler:     handleGithubWorkflowBackfillTasks,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowEphemeralRunnersRequest, GithubWorkflowEphemeralRunnersResponse]{
		Name:        "github_workflow_ephemeral_runners",
		Description: "List completed ephemeral runners for a configuration",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/configs/:id/runners",
		MCPToolName: "lens_github_workflow_ephemeral_runners",
		Handler:     handleGithubWorkflowEphemeralRunners,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowDashboardRequest, GithubWorkflowDashboardResponse]{
		Name:        "github_workflow_dashboard",
		Description: "Get dashboard summary for a configuration",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/configs/:id/dashboard",
		MCPToolName: "lens_github_workflow_dashboard",
		Handler:     handleGithubWorkflowDashboard,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowDashboardBuildsRequest, GithubWorkflowDashboardBuildsResponse]{
		Name:        "github_workflow_dashboard_builds",
		Description: "Get recent builds for dashboard",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/configs/:id/dashboard/builds",
		MCPToolName: "lens_github_workflow_dashboard_builds",
		Handler:     handleGithubWorkflowDashboardBuilds,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowCommitStatsRequest, GithubWorkflowCommitStatsResponse]{
		Name:        "github_workflow_commit_stats",
		Description: "Get commit statistics for a configuration",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/configs/:id/commits/stats",
		MCPToolName: "lens_github_workflow_commit_stats",
		Handler:     handleGithubWorkflowCommitStats,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowAnalyticsRequest, GithubWorkflowAnalyticsResponse]{
		Name:        "github_workflow_analytics",
		Description: "Get analytics for a configuration's workflow runs",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/configs/:id/analytics",
		MCPToolName: "lens_github_workflow_analytics",
		Handler:     handleGithubWorkflowAnalytics,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowRunHistoryRequest, GithubWorkflowRunHistoryResponse]{
		Name:        "github_workflow_run_history",
		Description: "Get detailed execution history for a configuration",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/configs/:id/history",
		MCPToolName: "lens_github_workflow_run_history",
		Handler:     handleGithubWorkflowRunHistory,
	})

	// ========== Run Endpoints ==========
	unified.Register(&unified.EndpointDef[GithubWorkflowAllRunsRequest, GithubWorkflowRunsListResponse]{
		Name:        "github_workflow_runs_list",
		Description: "List all workflow runs globally",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/runs",
		MCPToolName: "lens_github_workflow_runs_list",
		Handler:     handleGithubWorkflowAllRuns,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowRunGetRequest, *dbmodel.GithubWorkflowRuns]{
		Name:        "github_workflow_run_get",
		Description: "Get a specific workflow run by ID",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/runs/:id",
		MCPToolName: "lens_github_workflow_run_get",
		Handler:     handleGithubWorkflowRunGet,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowRunMetricsRequest, GithubWorkflowRunMetricsResponse]{
		Name:        "github_workflow_run_metrics",
		Description: "Get metrics extracted from a specific workflow run",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/runs/:id/metrics",
		MCPToolName: "lens_github_workflow_run_metrics",
		Handler:     handleGithubWorkflowRunMetrics,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowRunDetailRequest, GithubWorkflowRunDetailResponse]{
		Name:        "github_workflow_run_detail",
		Description: "Get run detail with commits and performance comparison",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/runs/:id/detail",
		MCPToolName: "lens_github_workflow_run_detail",
		Handler:     handleGithubWorkflowRunDetail,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowRunCommitRequest, *dbmodel.GithubWorkflowCommits]{
		Name:        "github_workflow_run_commit",
		Description: "Get commit details for a workflow run",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/runs/:id/commit",
		MCPToolName: "lens_github_workflow_run_commit",
		Handler:     handleGithubWorkflowRunCommit,
	})

	unified.Register(&unified.EndpointDef[GithubWorkflowRunDetailsAPIRequest, *dbmodel.GithubWorkflowRunDetails]{
		Name:        "github_workflow_run_details_api",
		Description: "Get workflow run details from GitHub",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/runs/:id/details",
		MCPToolName: "lens_github_workflow_run_details_api",
		Handler:     handleGithubWorkflowRunDetailsAPI,
	})

	// ========== Schema Endpoints ==========
	unified.Register(&unified.EndpointDef[GithubWorkflowSchemaGetRequest, *dbmodel.GithubWorkflowMetricSchemas]{
		Name:        "github_workflow_schema_get",
		Description: "Get a specific schema by ID",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-workflow-metrics/schemas/:id",
		MCPToolName: "lens_github_workflow_schema_get",
		Handler:     handleGithubWorkflowSchemaGet,
	})
}

// ======================== Request Types ========================

type GithubWorkflowConfigsListRequest struct {
	Cluster     string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	GithubOwner string `json:"github_owner" form:"github_owner" mcp:"description=Filter by GitHub owner"`
	GithubRepo  string `json:"github_repo" form:"github_repo" mcp:"description=Filter by GitHub repo"`
	Enabled     string `json:"enabled" form:"enabled" mcp:"description=Filter by enabled status (true/false)"`
	Offset      int    `json:"offset" form:"offset" mcp:"description=Pagination offset"`
	Limit       int    `json:"limit" form:"limit" mcp:"description=Pagination limit"`
}

type GithubWorkflowConfigGetRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Config ID,required"`
}

type GithubWorkflowRunsListRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Config ID,required"`
	Status  string `json:"status" form:"status" mcp:"description=Filter by status"`
	Offset  int    `json:"offset" form:"offset" mcp:"description=Pagination offset"`
	Limit   int    `json:"limit" form:"limit" mcp:"description=Pagination limit"`
}

type GithubWorkflowSchemasListRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Config ID,required"`
}

type GithubWorkflowActiveSchemaRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Config ID,required"`
}

type GithubWorkflowMetricsListRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Config ID,required"`
	Offset  int    `json:"offset" form:"offset" mcp:"description=Pagination offset"`
	Limit   int    `json:"limit" form:"limit" mcp:"description=Pagination limit"`
	Since   string `json:"since" form:"since" mcp:"description=Start time (RFC3339)"`
	Until   string `json:"until" form:"until" mcp:"description=End time (RFC3339)"`
}

type GithubWorkflowStatsRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Config ID,required"`
}

type GithubWorkflowSummaryRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Config ID,required"`
}

type GithubWorkflowDimensionsRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Config ID,required"`
}

type GithubWorkflowDimensionValuesRequest struct {
	Cluster   string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID        string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Config ID,required"`
	Dimension string `json:"dimension" form:"dimension" uri:"dimension" binding:"required" mcp:"description=Dimension name,required"`
}

type GithubWorkflowFieldsRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Config ID,required"`
}

type GithubWorkflowBackfillStatusRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Config ID,required"`
}

type GithubWorkflowBackfillTasksRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Config ID,required"`
	Status  string `json:"status" form:"status" mcp:"description=Filter by status"`
	Offset  int    `json:"offset" form:"offset" mcp:"description=Pagination offset"`
	Limit   int    `json:"limit" form:"limit" mcp:"description=Pagination limit"`
}

type GithubWorkflowEphemeralRunnersRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Config ID,required"`
	Status  string `json:"status" form:"status" mcp:"description=Filter by status"`
	Offset  int    `json:"offset" form:"offset" mcp:"description=Pagination offset"`
	Limit   int    `json:"limit" form:"limit" mcp:"description=Pagination limit"`
}

type GithubWorkflowDashboardRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Config ID,required"`
}

type GithubWorkflowDashboardBuildsRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Config ID,required"`
	Limit   int    `json:"limit" form:"limit" mcp:"description=Number of builds to return"`
}

type GithubWorkflowCommitStatsRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Config ID,required"`
	Since   string `json:"since" form:"since" mcp:"description=Start time (RFC3339)"`
	Until   string `json:"until" form:"until" mcp:"description=End time (RFC3339)"`
}

type GithubWorkflowAnalyticsRequest struct {
	Cluster      string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID           string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Config ID,required"`
	Since        string `json:"since" form:"since" mcp:"description=Start time (RFC3339)"`
	Until        string `json:"until" form:"until" mcp:"description=End time (RFC3339)"`
	WorkflowName string `json:"workflow_name" form:"workflow_name" mcp:"description=Filter by workflow name"`
	Event        string `json:"event" form:"event" mcp:"description=Filter by event type"`
	Branch       string `json:"branch" form:"branch" mcp:"description=Filter by branch"`
}

type GithubWorkflowRunHistoryRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Config ID,required"`
	Offset  int    `json:"offset" form:"offset" mcp:"description=Pagination offset"`
	Limit   int    `json:"limit" form:"limit" mcp:"description=Pagination limit"`
}

type GithubWorkflowAllRunsRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	Status  string `json:"status" form:"status" mcp:"description=Filter by status"`
	Offset  int    `json:"offset" form:"offset" mcp:"description=Pagination offset"`
	Limit   int    `json:"limit" form:"limit" mcp:"description=Pagination limit"`
}

type GithubWorkflowRunGetRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Run ID,required"`
}

type GithubWorkflowRunMetricsRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Run ID,required"`
}

type GithubWorkflowRunDetailRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Run ID,required"`
}

type GithubWorkflowRunCommitRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Run ID,required"`
}

type GithubWorkflowRunDetailsAPIRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Run ID,required"`
}

type GithubWorkflowSchemaGetRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Schema ID,required"`
}

// ======================== Response Types ========================

type GithubWorkflowConfigsListResponse struct {
	Configs     []*dbmodel.GithubWorkflowConfigs `json:"configs"`
	Total       int64                            `json:"total"`
	Offset      int                              `json:"offset"`
	Limit       int                              `json:"limit"`
	ClusterName string                           `json:"cluster_name"`
}

type GithubWorkflowRunsListResponse struct {
	Runs   []*dbmodel.GithubWorkflowRuns `json:"runs"`
	Total  int64                         `json:"total"`
	Offset int                           `json:"offset"`
	Limit  int                           `json:"limit"`
}

type GithubWorkflowSchemasListResponse struct {
	Schemas []*dbmodel.GithubWorkflowMetricSchemas `json:"schemas"`
}

type GithubWorkflowMetricsListResponse struct {
	Metrics []*dbmodel.GithubWorkflowMetrics `json:"metrics"`
	Total   int64                            `json:"total"`
	Offset  int                              `json:"offset"`
	Limit   int                              `json:"limit"`
}

type GithubWorkflowStatsResponse struct {
	TotalRecords int64     `json:"total_records"`
	LatestEntry  time.Time `json:"latest_entry,omitempty"`
	OldestEntry  time.Time `json:"oldest_entry,omitempty"`
}

type GithubWorkflowSummaryResponse struct {
	TotalRuns    int64   `json:"total_runs"`
	TotalMetrics int64   `json:"total_metrics"`
	SuccessRate  float64 `json:"success_rate,omitempty"`
}

type GithubWorkflowDimensionsResponse struct {
	Dimensions map[string][]string `json:"dimensions"`
}

type GithubWorkflowDimensionValuesResponse struct {
	Values []string `json:"values"`
}

type GithubWorkflowFieldsResponse struct {
	DimensionFields []string `json:"dimension_fields"`
	MetricFields    []string `json:"metric_fields"`
}

type GithubWorkflowBackfillTasksResponse struct {
	Tasks  interface{} `json:"tasks"`
	Total  int64       `json:"total"`
	Offset int         `json:"offset"`
	Limit  int         `json:"limit"`
}

type GithubWorkflowBackfillStatusResponse struct {
	Status interface{} `json:"status"`
}

type GithubWorkflowEphemeralRunnersResponse struct {
	Runners interface{} `json:"runners"`
	Total   int64       `json:"total"`
	Offset  int         `json:"offset"`
	Limit   int         `json:"limit"`
}

type GithubWorkflowDashboardResponse struct {
	Summary interface{} `json:"summary"`
}

type GithubWorkflowDashboardBuildsResponse struct {
	Builds interface{} `json:"builds"`
}

type GithubWorkflowCommitStatsResponse struct {
	Stats interface{} `json:"stats"`
}

type GithubWorkflowAnalyticsResponse struct {
	ConfigID            int64       `json:"config_id"`
	TotalRuns           int64       `json:"total_runs"`
	WorkflowAnalytics   interface{} `json:"workflow_analytics"`
	CommitStats         interface{} `json:"commit_stats"`
	AvgExecutionSeconds float64     `json:"avg_execution_seconds"`
}

type GithubWorkflowRunHistoryResponse struct {
	Runs   interface{} `json:"runs"`
	Total  int64       `json:"total"`
	Offset int         `json:"offset"`
	Limit  int         `json:"limit"`
}

type GithubWorkflowRunMetricsResponse struct {
	Metrics []*dbmodel.GithubWorkflowMetrics `json:"metrics"`
}

type GithubWorkflowRunDetailResponse struct {
	Run              *dbmodel.GithubWorkflowRuns        `json:"run"`
	Commit           *dbmodel.GithubWorkflowCommits     `json:"commit,omitempty"`
	Details          *dbmodel.GithubWorkflowRunDetails  `json:"details,omitempty"`
	Metrics          []*dbmodel.GithubWorkflowMetrics   `json:"metrics,omitempty"`
	DurationSeconds  float64                            `json:"duration_seconds,omitempty"`
}

// ======================== Handler Implementations ========================

func handleGithubWorkflowConfigsList(ctx context.Context, req *GithubWorkflowConfigsListRequest) (*GithubWorkflowConfigsListResponse, error) {
	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	filter := &database.GithubWorkflowConfigFilter{
		GithubOwner: req.GithubOwner,
		GithubRepo:  req.GithubRepo,
		Offset:      req.Offset,
		Limit:       req.Limit,
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if req.Enabled != "" {
		enabled := req.Enabled == "true"
		filter.Enabled = &enabled
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
	configs, total, err := facade.List(ctx, filter)
	if err != nil {
		return nil, errors.WrapError(err, "failed to list configs", errors.CodeDatabaseError)
	}

	return &GithubWorkflowConfigsListResponse{
		Configs:     configs,
		Total:       total,
		Offset:      filter.Offset,
		Limit:       filter.Limit,
		ClusterName: clusterName,
	}, nil
}

func handleGithubWorkflowConfigGet(ctx context.Context, req *GithubWorkflowConfigGetRequest) (**dbmodel.GithubWorkflowConfigs, error) {
	configID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid config id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
	config, err := facade.GetByID(ctx, configID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get config", errors.CodeDatabaseError)
	}
	if config == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("config not found")
	}

	return &config, nil
}

func handleGithubWorkflowConfigRuns(ctx context.Context, req *GithubWorkflowRunsListRequest) (*GithubWorkflowRunsListResponse, error) {
	configID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid config id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	offset := req.Offset
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	runs, total, err := facade.List(ctx, &database.GithubWorkflowRunFilter{
		ConfigID: configID,
		Status:   req.Status,
		Offset:   offset,
		Limit:    limit,
	})
	if err != nil {
		return nil, errors.WrapError(err, "failed to list runs", errors.CodeDatabaseError)
	}

	return &GithubWorkflowRunsListResponse{
		Runs:   runs,
		Total:  total,
		Offset: offset,
		Limit:  limit,
	}, nil
}

func handleGithubWorkflowSchemasList(ctx context.Context, req *GithubWorkflowSchemasListRequest) (*GithubWorkflowSchemasListResponse, error) {
	configID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid config id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowSchema()
	schemas, err := facade.ListByConfig(ctx, configID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to list schemas", errors.CodeDatabaseError)
	}

	return &GithubWorkflowSchemasListResponse{
		Schemas: schemas,
	}, nil
}

func handleGithubWorkflowActiveSchema(ctx context.Context, req *GithubWorkflowActiveSchemaRequest) (**dbmodel.GithubWorkflowMetricSchemas, error) {
	configID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid config id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowSchema()
	schema, err := facade.GetActiveByConfig(ctx, configID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get active schema", errors.CodeDatabaseError)
	}
	if schema == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("no active schema found")
	}

	return &schema, nil
}

func handleGithubWorkflowMetricsList(ctx context.Context, req *GithubWorkflowMetricsListRequest) (*GithubWorkflowMetricsListResponse, error) {
	configID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid config id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	offset := req.Offset
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	var since, until *time.Time
	if req.Since != "" {
		if t, parseErr := time.Parse(time.RFC3339, req.Since); parseErr == nil {
			since = &t
		}
	}
	if req.Until != "" {
		if t, parseErr := time.Parse(time.RFC3339, req.Until); parseErr == nil {
			until = &t
		}
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowMetrics()
	metrics, total, err := facade.List(ctx, &database.GithubWorkflowMetricsFilter{
		ConfigID: configID,
		Start:    since,
		End:      until,
		Offset:   offset,
		Limit:    limit,
	})
	if err != nil {
		return nil, errors.WrapError(err, "failed to list metrics", errors.CodeDatabaseError)
	}

	return &GithubWorkflowMetricsListResponse{
		Metrics: metrics,
		Total:   total,
		Offset:  offset,
		Limit:   limit,
	}, nil
}

func handleGithubWorkflowStats(ctx context.Context, req *GithubWorkflowStatsRequest) (*GithubWorkflowStatsResponse, error) {
	configID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid config id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowMetrics()
	
	// Get count and summary instead of GetStats which doesn't exist
	totalRecords, err := facade.CountByConfig(ctx, configID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get stats", errors.CodeDatabaseError)
	}

	return &GithubWorkflowStatsResponse{
		TotalRecords: totalRecords,
	}, nil
}

func handleGithubWorkflowSummary(ctx context.Context, req *GithubWorkflowSummaryRequest) (*GithubWorkflowSummaryResponse, error) {
	configID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid config id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	_, totalRuns, _ := runFacade.List(ctx, &database.GithubWorkflowRunFilter{
		ConfigID: configID,
	})

	metricFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowMetrics()
	_, totalMetrics, _ := metricFacade.List(ctx, &database.GithubWorkflowMetricsFilter{
		ConfigID: configID,
	})

	return &GithubWorkflowSummaryResponse{
		TotalRuns:    totalRuns,
		TotalMetrics: totalMetrics,
	}, nil
}

func handleGithubWorkflowDimensions(ctx context.Context, req *GithubWorkflowDimensionsRequest) (*GithubWorkflowDimensionsResponse, error) {
	configID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid config id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowMetrics()
	
	// Get available dimension keys
	dimensionKeys, err := facade.GetAvailableDimensions(ctx, configID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get dimensions", errors.CodeDatabaseError)
	}

	// Build dimensions map with values
	dimensions := make(map[string][]string)
	for _, key := range dimensionKeys {
		values, _ := facade.GetDistinctDimensionValues(ctx, configID, key, nil, nil)
		dimensions[key] = values
	}

	return &GithubWorkflowDimensionsResponse{
		Dimensions: dimensions,
	}, nil
}

func handleGithubWorkflowDimensionValues(ctx context.Context, req *GithubWorkflowDimensionValuesRequest) (*GithubWorkflowDimensionValuesResponse, error) {
	configID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid config id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowMetrics()
	values, err := facade.GetDistinctDimensionValues(ctx, configID, req.Dimension, nil, nil)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get dimension values", errors.CodeDatabaseError)
	}

	return &GithubWorkflowDimensionValuesResponse{
		Values: values,
	}, nil
}

func handleGithubWorkflowFields(ctx context.Context, req *GithubWorkflowFieldsRequest) (*GithubWorkflowFieldsResponse, error) {
	configID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid config id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Get active schema for field info
	schemaFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowSchema()
	schema, err := schemaFacade.GetActiveByConfig(ctx, configID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get active schema", errors.CodeDatabaseError)
	}

	resp := &GithubWorkflowFieldsResponse{
		DimensionFields: []string{},
		MetricFields:    []string{},
	}

	if schema != nil {
		// DimensionFields and MetricFields are ExtJSON (json.RawMessage)
		if len(schema.DimensionFields) > 0 {
			var dimFields []string
			if err := json.Unmarshal(schema.DimensionFields, &dimFields); err == nil {
				resp.DimensionFields = dimFields
			}
		}
		if len(schema.MetricFields) > 0 {
			var metricFields []string
			if err := json.Unmarshal(schema.MetricFields, &metricFields); err == nil {
				resp.MetricFields = metricFields
			}
		}
	}

	return resp, nil
}

func handleGithubWorkflowBackfillStatus(ctx context.Context, req *GithubWorkflowBackfillStatusRequest) (*GithubWorkflowBackfillStatusResponse, error) {
	configID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid config id")
	}

	_, err = getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	taskManager := backfill.GetTaskManager()
	tasks := taskManager.GetTasksByConfig(configID)

	// Calculate status from tasks
	var pending, running, completed, failed int
	for _, task := range tasks {
		switch task.Status {
		case "pending":
			pending++
		case "running":
			running++
		case "completed":
			completed++
		case "failed":
			failed++
		}
	}

	return &GithubWorkflowBackfillStatusResponse{
		Status: map[string]interface{}{
			"pending":   pending,
			"running":   running,
			"completed": completed,
			"failed":    failed,
			"total":     len(tasks),
		},
	}, nil
}

func handleGithubWorkflowBackfillTasks(ctx context.Context, req *GithubWorkflowBackfillTasksRequest) (*GithubWorkflowBackfillTasksResponse, error) {
	configID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid config id")
	}

	_, err = getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	offset := req.Offset
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	taskManager := backfill.GetTaskManager()
	allTasks := taskManager.GetTasksByConfig(configID)

	// Filter by status if provided
	var filteredTasks []*backfill.BackfillTask
	for _, task := range allTasks {
		if req.Status == "" || string(task.Status) == req.Status {
			filteredTasks = append(filteredTasks, task)
		}
	}

	// Apply pagination
	total := int64(len(filteredTasks))
	start := offset
	if start > len(filteredTasks) {
		start = len(filteredTasks)
	}
	end := start + limit
	if end > len(filteredTasks) {
		end = len(filteredTasks)
	}

	return &GithubWorkflowBackfillTasksResponse{
		Tasks:  filteredTasks[start:end],
		Total:  total,
		Offset: offset,
		Limit:  limit,
	}, nil
}

func handleGithubWorkflowEphemeralRunners(ctx context.Context, req *GithubWorkflowEphemeralRunnersRequest) (*GithubWorkflowEphemeralRunnersResponse, error) {
	configID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid config id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	offset := req.Offset
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	// Query runs with runner set info
	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	runs, total, err := runFacade.List(ctx, &database.GithubWorkflowRunFilter{
		ConfigID: configID,
		Status:   req.Status,
		Offset:   offset,
		Limit:    limit,
	})
	if err != nil {
		return nil, errors.WrapError(err, "failed to list runners", errors.CodeDatabaseError)
	}

	// Extract runner info from runs
	var runners []interface{}
	for _, run := range runs {
		if run.RunnerSetName != "" {
			runners = append(runners, map[string]interface{}{
				"run_id":               run.ID,
				"runner_set_name":      run.RunnerSetName,
				"runner_set_namespace": run.RunnerSetNamespace,
				"workload_uid":         run.WorkloadUID,
				"workload_name":        run.WorkloadName,
				"status":               run.Status,
				"started_at":           run.WorkloadStartedAt,
				"completed_at":         run.WorkloadCompletedAt,
			})
		}
	}

	return &GithubWorkflowEphemeralRunnersResponse{
		Runners: runners,
		Total:   total,
		Offset:  offset,
		Limit:   limit,
	}, nil
}

func handleGithubWorkflowDashboard(ctx context.Context, req *GithubWorkflowDashboardRequest) (*GithubWorkflowDashboardResponse, error) {
	configID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid config id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Get dashboard summary using service
	summary, err := getDashboardSummaryForConfig(ctx, clusterName, configID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get dashboard summary", errors.CodeDatabaseError)
	}

	return &GithubWorkflowDashboardResponse{
		Summary: summary,
	}, nil
}

func handleGithubWorkflowDashboardBuilds(ctx context.Context, req *GithubWorkflowDashboardBuildsRequest) (*GithubWorkflowDashboardBuildsResponse, error) {
	configID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid config id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	// Get recent builds using service
	builds, err := getDashboardRecentBuildsForConfig(ctx, clusterName, configID, limit)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get dashboard builds", errors.CodeDatabaseError)
	}

	return &GithubWorkflowDashboardBuildsResponse{
		Builds: builds,
	}, nil
}

func handleGithubWorkflowCommitStats(ctx context.Context, req *GithubWorkflowCommitStatsRequest) (*GithubWorkflowCommitStatsResponse, error) {
	_, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid config id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	var since, until *time.Time
	if req.Since != "" {
		if t, parseErr := time.Parse(time.RFC3339, req.Since); parseErr == nil {
			since = &t
		}
	}
	if req.Until != "" {
		if t, parseErr := time.Parse(time.RFC3339, req.Until); parseErr == nil {
			until = &t
		}
	}

	commitFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowCommit()
	stats, err := commitFacade.GetStats(ctx, since, until)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get commit stats", errors.CodeDatabaseError)
	}

	return &GithubWorkflowCommitStatsResponse{
		Stats: stats,
	}, nil
}

func handleGithubWorkflowAnalytics(ctx context.Context, req *GithubWorkflowAnalyticsRequest) (*GithubWorkflowAnalyticsResponse, error) {
	configID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid config id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	var since, until *time.Time
	if req.Since != "" {
		if t, parseErr := time.Parse(time.RFC3339, req.Since); parseErr == nil {
			since = &t
		}
	}
	if req.Until != "" {
		if t, parseErr := time.Parse(time.RFC3339, req.Until); parseErr == nil {
			until = &t
		}
	}

	// Get run analytics
	detailsFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRunDetails()
	analytics, err := detailsFacade.GetAnalytics(ctx, &database.WorkflowAnalyticsFilter{
		Since:        since,
		Until:        until,
		WorkflowName: req.WorkflowName,
		Event:        req.Event,
		Branch:       req.Branch,
	})
	if err != nil {
		return nil, errors.WrapError(err, "failed to get analytics", errors.CodeDatabaseError)
	}

	// Get commit stats
	commitFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowCommit()
	commitStats, _ := commitFacade.GetStats(ctx, since, until)

	// Get run stats
	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	runs, total, _ := runFacade.List(ctx, &database.GithubWorkflowRunFilter{
		ConfigID: configID,
		Since:    since,
		Until:    until,
	})

	// Calculate average execution time
	var totalDuration float64
	var completedRuns int
	for _, run := range runs {
		if !run.WorkloadCompletedAt.IsZero() && !run.WorkloadStartedAt.IsZero() {
			totalDuration += run.WorkloadCompletedAt.Sub(run.WorkloadStartedAt).Seconds()
			completedRuns++
		}
	}

	avgExecutionTime := float64(0)
	if completedRuns > 0 {
		avgExecutionTime = totalDuration / float64(completedRuns)
	}

	return &GithubWorkflowAnalyticsResponse{
		ConfigID:            configID,
		TotalRuns:           total,
		WorkflowAnalytics:   analytics,
		CommitStats:         commitStats,
		AvgExecutionSeconds: avgExecutionTime,
	}, nil
}

func handleGithubWorkflowRunHistory(ctx context.Context, req *GithubWorkflowRunHistoryRequest) (*GithubWorkflowRunHistoryResponse, error) {
	configID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid config id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	offset := req.Offset
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Get runs
	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	runs, total, err := runFacade.List(ctx, &database.GithubWorkflowRunFilter{
		ConfigID: configID,
		Offset:   offset,
		Limit:    limit,
	})
	if err != nil {
		return nil, errors.WrapError(err, "failed to list runs", errors.CodeDatabaseError)
	}

	// Enrich runs with commit and details
	commitFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowCommit()
	detailsFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRunDetails()

	type EnrichedRun struct {
		*dbmodel.GithubWorkflowRuns
		Commit     interface{} `json:"commit,omitempty"`
		RunDetails interface{} `json:"run_details,omitempty"`
		Duration   float64     `json:"duration_seconds,omitempty"`
	}

	enrichedRuns := make([]EnrichedRun, 0, len(runs))
	for _, run := range runs {
		enriched := EnrichedRun{
			GithubWorkflowRuns: run,
		}

		// Calculate duration
		if !run.WorkloadCompletedAt.IsZero() && !run.WorkloadStartedAt.IsZero() {
			enriched.Duration = run.WorkloadCompletedAt.Sub(run.WorkloadStartedAt).Seconds()
		}

		// Get commit if available
		if commit, _ := commitFacade.GetByRunID(ctx, run.ID); commit != nil {
			enriched.Commit = commit
		}

		// Get run details if available
		if details, _ := detailsFacade.GetByRunID(ctx, run.ID); details != nil {
			enriched.RunDetails = details
		}

		enrichedRuns = append(enrichedRuns, enriched)
	}

	return &GithubWorkflowRunHistoryResponse{
		Runs:   enrichedRuns,
		Total:  total,
		Offset: offset,
		Limit:  limit,
	}, nil
}

func handleGithubWorkflowAllRuns(ctx context.Context, req *GithubWorkflowAllRunsRequest) (*GithubWorkflowRunsListResponse, error) {
	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	offset := req.Offset
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	runs, total, err := facade.List(ctx, &database.GithubWorkflowRunFilter{
		Status: req.Status,
		Offset: offset,
		Limit:  limit,
	})
	if err != nil {
		return nil, errors.WrapError(err, "failed to list runs", errors.CodeDatabaseError)
	}

	return &GithubWorkflowRunsListResponse{
		Runs:   runs,
		Total:  total,
		Offset: offset,
		Limit:  limit,
	}, nil
}

func handleGithubWorkflowRunGet(ctx context.Context, req *GithubWorkflowRunGetRequest) (**dbmodel.GithubWorkflowRuns, error) {
	runID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid run id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	run, err := facade.GetByID(ctx, runID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get run", errors.CodeDatabaseError)
	}
	if run == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("run not found")
	}

	return &run, nil
}

func handleGithubWorkflowRunMetrics(ctx context.Context, req *GithubWorkflowRunMetricsRequest) (*GithubWorkflowRunMetricsResponse, error) {
	runID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid run id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowMetrics()
	metrics, _, err := facade.List(ctx, &database.GithubWorkflowMetricsFilter{
		RunID: runID,
	})
	if err != nil {
		return nil, errors.WrapError(err, "failed to list metrics", errors.CodeDatabaseError)
	}

	return &GithubWorkflowRunMetricsResponse{
		Metrics: metrics,
	}, nil
}

func handleGithubWorkflowRunDetail(ctx context.Context, req *GithubWorkflowRunDetailRequest) (*GithubWorkflowRunDetailResponse, error) {
	runID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid run id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Get run
	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	run, err := runFacade.GetByID(ctx, runID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get run", errors.CodeDatabaseError)
	}
	if run == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("run not found")
	}

	resp := &GithubWorkflowRunDetailResponse{
		Run: run,
	}

	// Calculate duration
	if !run.WorkloadCompletedAt.IsZero() && !run.WorkloadStartedAt.IsZero() {
		resp.DurationSeconds = run.WorkloadCompletedAt.Sub(run.WorkloadStartedAt).Seconds()
	}

	// Get commit
	commitFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowCommit()
	if commit, _ := commitFacade.GetByRunID(ctx, runID); commit != nil {
		resp.Commit = commit
	}

	// Get details
	detailsFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRunDetails()
	if details, _ := detailsFacade.GetByRunID(ctx, runID); details != nil {
		resp.Details = details
	}

	// Get metrics
	metricsFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowMetrics()
	if metrics, _, _ := metricsFacade.List(ctx, &database.GithubWorkflowMetricsFilter{RunID: runID}); metrics != nil {
		resp.Metrics = metrics
	}

	return resp, nil
}

func handleGithubWorkflowRunCommit(ctx context.Context, req *GithubWorkflowRunCommitRequest) (**dbmodel.GithubWorkflowCommits, error) {
	runID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid run id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowCommit()
	commit, err := facade.GetByRunID(ctx, runID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get commit", errors.CodeDatabaseError)
	}
	if commit == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("commit not found for this run")
	}

	return &commit, nil
}

func handleGithubWorkflowRunDetailsAPI(ctx context.Context, req *GithubWorkflowRunDetailsAPIRequest) (**dbmodel.GithubWorkflowRunDetails, error) {
	runID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid run id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRunDetails()
	details, err := facade.GetByRunID(ctx, runID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get workflow run details", errors.CodeDatabaseError)
	}
	if details == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("workflow run details not found")
	}

	return &details, nil
}

func handleGithubWorkflowSchemaGet(ctx context.Context, req *GithubWorkflowSchemaGetRequest) (**dbmodel.GithubWorkflowMetricSchemas, error) {
	schemaID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid schema id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowSchema()
	schema, err := facade.GetByID(ctx, schemaID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get schema", errors.CodeDatabaseError)
	}
	if schema == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("schema not found")
	}

	return &schema, nil
}

// Helper functions

func splitStringTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func getDashboardSummaryForConfig(ctx context.Context, clusterName string, configID int64) (interface{}, error) {
	// Get config first
	configFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
	config, err := configFacade.GetByID(ctx, configID)
	if err != nil || config == nil {
		return nil, err
	}

	// Generate dashboard summary
	service := NewDashboardService(clusterName)
	return service.GenerateDashboardSummary(ctx, config, time.Now())
}

func getDashboardRecentBuildsForConfig(ctx context.Context, clusterName string, configID int64, limit int) (interface{}, error) {
	// Return recent runs
	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	runs, _, err := runFacade.List(ctx, &database.GithubWorkflowRunFilter{ConfigID: configID, Limit: limit})
	return runs, err
}
