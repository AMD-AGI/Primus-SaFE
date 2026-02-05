// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

func init() {
	// GitHub Runners endpoints (GET only)
	unified.Register(&unified.EndpointDef[GithubRunnerSetsListRequest, GithubRunnerSetsListResponse]{
		Name:        "github_runner_sets_list",
		Description: "List all discovered AutoScalingRunnerSets in the cluster",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-runners/runner-sets",
		MCPToolName: "lens_github_runner_sets_list",
		Handler:     handleGithubRunnerSetsList,
	})

	unified.Register(&unified.EndpointDef[GithubRunnerSetGetRequest, *dbmodel.GithubRunnerSets]{
		Name:        "github_runner_set_get",
		Description: "Get a specific AutoScalingRunnerSet by namespace and name",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-runners/runner-sets/:namespace/:name",
		MCPToolName: "lens_github_runner_set_get",
		Handler:     handleGithubRunnerSetGet,
	})

	unified.Register(&unified.EndpointDef[GithubRunnerSetByIDRequest, *dbmodel.GithubRunnerSets]{
		Name:        "github_runner_set_by_id",
		Description: "Get a runner set by ID",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-runners/runner-sets/by-id/:id",
		MCPToolName: "lens_github_runner_set_by_id",
		Handler:     handleGithubRunnerSetByID,
	})

	unified.Register(&unified.EndpointDef[GithubRunnerSetRunsRequest, GithubRunnerSetRunsResponse]{
		Name:        "github_runner_set_runs",
		Description: "List workflow runs for a runner set",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-runners/runner-sets/by-id/:id/runs",
		MCPToolName: "lens_github_runner_set_runs",
		Handler:     handleGithubRunnerSetRuns,
	})

	unified.Register(&unified.EndpointDef[GithubRunnerSetConfigRequest, *dbmodel.GithubWorkflowConfigs]{
		Name:        "github_runner_set_config",
		Description: "Get the config associated with a runner set (may return null)",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-runners/runner-sets/by-id/:id/config",
		MCPToolName: "lens_github_runner_set_config",
		Handler:     handleGithubRunnerSetConfig,
	})

	unified.Register(&unified.EndpointDef[GithubRunnerSetStatsRequest, GithubRunnerSetStatsResponse]{
		Name:        "github_runner_set_stats",
		Description: "Get statistics for a runner set",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-runners/runner-sets/by-id/:id/stats",
		MCPToolName: "lens_github_runner_set_stats",
		Handler:     handleGithubRunnerSetStats,
	})

	// ========== Repository Endpoints ==========
	unified.Register(&unified.EndpointDef[GithubRepositoriesListRequest, GithubRepositoriesListResponse]{
		Name:        "github_repositories_list",
		Description: "List all repositories with aggregated runner set statistics",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-runners/repositories",
		MCPToolName: "lens_github_repositories_list",
		Handler:     handleGithubRepositoriesList,
	})

	unified.Register(&unified.EndpointDef[GithubRepositoryGetRequest, *database.RepositorySummary]{
		Name:        "github_repository_get",
		Description: "Get repository details with aggregated statistics",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-runners/repositories/:owner/:repo",
		MCPToolName: "lens_github_repository_get",
		Handler:     handleGithubRepositoryGet,
	})

	unified.Register(&unified.EndpointDef[GithubRepositoryRunnerSetsRequest, GithubRepositoryRunnerSetsResponse]{
		Name:        "github_repository_runner_sets",
		Description: "List runner sets for a specific repository",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-runners/repositories/:owner/:repo/runner-sets",
		MCPToolName: "lens_github_repository_runner_sets",
		Handler:     handleGithubRepositoryRunnerSets,
	})

	unified.Register(&unified.EndpointDef[GithubRepositoryMetricsMetadataRequest, GithubRepositoryMetricsMetadataResponse]{
		Name:        "github_repository_metrics_metadata",
		Description: "Get metrics metadata for all configs in a repository",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-runners/repositories/:owner/:repo/metrics/metadata",
		MCPToolName: "lens_github_repository_metrics_metadata",
		Handler:     handleGithubRepositoryMetricsMetadata,
	})

	unified.Register(&unified.EndpointDef[GithubRepositoryMetricsTrendsRequest, *database.MetricsTrendsResult]{
		Name:        "github_repository_metrics_trends",
		Description: "Query metrics trends across all configs in a repository",
		HTTPMethod:  "POST",
		HTTPPath:    "/github-runners/repositories/:owner/:repo/metrics/trends",
		MCPToolName: "lens_github_repository_metrics_trends",
		Handler:     handleGithubRepositoryMetricsTrends,
	})

	// ========== Run Summary Endpoints ==========
	unified.Register(&unified.EndpointDef[GithubRunSummariesListRequest, GithubRunSummariesListResponse]{
		Name:        "github_run_summaries_list",
		Description: "List workflow run summaries for a repository",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-runners/repositories/:owner/:repo/run-summaries",
		MCPToolName: "lens_github_run_summaries_list",
		Handler:     handleGithubRunSummariesList,
	})

	unified.Register(&unified.EndpointDef[GithubRunSummaryGetRequest, *dbmodel.GithubWorkflowRunSummaries]{
		Name:        "github_run_summary_get",
		Description: "Get a specific workflow run summary by ID",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-runners/run-summaries/:id",
		MCPToolName: "lens_github_run_summary_get",
		Handler:     handleGithubRunSummaryGet,
	})

	unified.Register(&unified.EndpointDef[GithubRunSummaryJobsRequest, GithubRunSummaryJobsResponse]{
		Name:        "github_run_summary_jobs",
		Description: "List jobs for a workflow run summary",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-runners/run-summaries/:id/jobs",
		MCPToolName: "lens_github_run_summary_jobs",
		Handler:     handleGithubRunSummaryJobs,
	})

	// GitHub run summary graph endpoint
	unified.Register(&unified.EndpointDef[GithubRunSummaryGraphRequest, GithubRunSummaryGraphResponse]{
		Name:        "github_run_summary_graph",
		Description: "Get workflow DAG graph with GitHub job info for visualization",
		HTTPMethod:  "GET",
		HTTPPath:    "/github-runners/run-summaries/:id/graph",
		MCPToolName: "lens_github_run_summary_graph",
		Handler:     handleGithubRunSummaryGraph,
	})
}

// ======================== Request Types ========================

type GithubRunnerSetsListRequest struct {
	Cluster   string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	Namespace string `json:"namespace" form:"namespace" mcp:"description=Filter by namespace"`
	WithStats string `json:"with_stats" form:"with_stats" mcp:"description=Include run statistics (true/false)"`
}

type GithubRunnerSetGetRequest struct {
	Cluster   string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	Namespace string `json:"namespace" form:"namespace" param:"namespace" binding:"required" mcp:"description=Runner set namespace,required"`
	Name      string `json:"name" form:"name" param:"name" binding:"required" mcp:"description=Runner set name,required"`
}

type GithubRunnerSetByIDRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" param:"id" binding:"required" mcp:"description=Runner set ID,required"`
}

type GithubRunnerSetRunsRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" param:"id" binding:"required" mcp:"description=Runner set ID,required"`
	Status  string `json:"status" form:"status" mcp:"description=Filter by status"`
	Offset  int    `json:"offset" form:"offset" mcp:"description=Pagination offset"`
	Limit   int    `json:"limit" form:"limit" mcp:"description=Pagination limit (max 100)"`
}

type GithubRunnerSetConfigRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" param:"id" binding:"required" mcp:"description=Runner set ID,required"`
}

type GithubRunnerSetStatsRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" param:"id" binding:"required" mcp:"description=Runner set ID,required"`
}

// Repository Request Types
type GithubRepositoriesListRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
}

type GithubRepositoryGetRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	Owner   string `json:"owner" form:"owner" param:"owner" binding:"required" mcp:"description=GitHub owner,required"`
	Repo    string `json:"repo" form:"repo" param:"repo" binding:"required" mcp:"description=GitHub repository name,required"`
}

type GithubRepositoryRunnerSetsRequest struct {
	Cluster   string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	Owner     string `json:"owner" form:"owner" param:"owner" binding:"required" mcp:"description=GitHub owner,required"`
	Repo      string `json:"repo" form:"repo" param:"repo" binding:"required" mcp:"description=GitHub repository name,required"`
	WithStats string `json:"with_stats" form:"with_stats" mcp:"description=Include run statistics (true/false)"`
}

type GithubRepositoryMetricsMetadataRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	Owner   string `json:"owner" form:"owner" param:"owner" binding:"required" mcp:"description=GitHub owner,required"`
	Repo    string `json:"repo" form:"repo" param:"repo" binding:"required" mcp:"description=GitHub repository name,required"`
}

type GithubRepositoryMetricsTrendsRequest struct {
	Cluster                string                 `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	Owner                  string                 `json:"owner" form:"owner" param:"owner" mcp:"description=GitHub owner,required"`
	Repo                   string                 `json:"repo" form:"repo" param:"repo" mcp:"description=GitHub repository name,required"`
	Start                  string                 `json:"start" mcp:"description=Start time (RFC3339)"`
	End                    string                 `json:"end" mcp:"description=End time (RFC3339)"`
	ConfigIDs              []int64                `json:"config_ids" mcp:"description=Filter by specific config IDs"`
	Dimensions             map[string]interface{} `json:"dimensions" mcp:"description=Dimension filters"`
	MetricFields           []string               `json:"metric_fields" mcp:"description=Metric fields to query,required"`
	Interval               string                 `json:"interval" mcp:"description=Aggregation interval (1h, 6h, 1d, 1w)"`
	GroupBy                []string               `json:"group_by" mcp:"description=Dimension fields to group by"`
	AggregateAcrossConfigs bool                   `json:"aggregate_across_configs" mcp:"description=Merge all configs or separate series per config"`
}

// Run Summary Request Types
type GithubRunSummariesListRequest struct {
	Cluster          string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	Owner            string `json:"owner" query:"owner" param:"owner" binding:"required" mcp:"description=GitHub owner,required"`
	Repo             string `json:"repo" query:"repo" param:"repo" binding:"required" mcp:"description=GitHub repository name,required"`
	Status           string `json:"status" query:"status" mcp:"description=Filter by status (queued, in_progress, completed)"`
	Conclusion       string `json:"conclusion" query:"conclusion" mcp:"description=Filter by conclusion (success, failure, cancelled)"`
	CollectionStatus string `json:"collection_status" query:"collection_status" mcp:"description=Filter by collection status"`
	WorkflowPath     string `json:"workflow_path" query:"workflow_path" mcp:"description=Filter by workflow path"`
	HeadBranch       string `json:"head_branch" query:"head_branch" mcp:"description=Filter by branch"`
	EventName        string `json:"event_name" query:"event_name" mcp:"description=Filter by event name"`
	RunnerSetID      string `json:"runner_set_id" query:"runner_set_id" mcp:"description=Filter by runner set ID"`
	Offset           int    `json:"offset" query:"offset" mcp:"description=Pagination offset"`
	Limit            int    `json:"limit" query:"limit" mcp:"description=Pagination limit (max 100)"`
}

type GithubRunSummaryGetRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" param:"id" binding:"required" mcp:"description=Run summary ID,required"`
}

type GithubRunSummaryJobsRequest struct {
	Cluster string `json:"cluster" query:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" param:"id" binding:"required" mcp:"description=Run summary ID,required"`
}

// ======================== Response Types ========================

type GithubRunnerSetsListResponse struct {
	RunnerSets interface{} `json:"runner_sets"`
}

type GithubRunnerSetRunsResponse struct {
	Runs   []*dbmodel.GithubWorkflowRuns `json:"runs"`
	Total  int64                         `json:"total"`
	Offset int                           `json:"offset"`
	Limit  int                           `json:"limit"`
}

type GithubRunnerSetStatsResponse struct {
	Total      int    `json:"total"`
	Pending    int    `json:"pending"`
	Completed  int    `json:"completed"`
	Failed     int    `json:"failed"`
	Skipped    int    `json:"skipped"`
	HasConfig  bool   `json:"has_config,omitempty"`
	ConfigID   int64  `json:"config_id,omitempty"`
	ConfigName string `json:"config_name,omitempty"`
}

// Repository Response Types
type GithubRepositoriesListResponse struct {
	Repositories []*database.RepositorySummary `json:"repositories"`
}

type GithubRepositoryRunnerSetsResponse struct {
	RunnerSets interface{} `json:"runner_sets"`
}

// ConfigMetricsInfo contains metrics metadata for a single config
type ConfigMetricsInfo struct {
	ConfigID        int64    `json:"config_id"`
	ConfigName      string   `json:"config_name"`
	RunnerSetID     int64    `json:"runner_set_id"`
	RunnerSetName   string   `json:"runner_set_name"`
	SchemaID        int64    `json:"schema_id,omitempty"`
	SchemaVersion   int32    `json:"schema_version,omitempty"`
	DimensionFields []string `json:"dimension_fields"`
	MetricFields    []string `json:"metric_fields"`
	RecordCount     int64    `json:"record_count"`
}

type GithubRepositoryMetricsMetadataResponse struct {
	Owner            string              `json:"owner"`
	Repo             string              `json:"repo"`
	Configs          []ConfigMetricsInfo `json:"configs"`
	CommonDimensions []string            `json:"common_dimensions"`
	CommonMetrics    []string            `json:"common_metrics"`
	AllDimensions    []string            `json:"all_dimensions"`
	AllMetrics       []string            `json:"all_metrics"`
}

// Run Summary Response Types
type GithubRunSummariesListResponse struct {
	RunSummaries []*dbmodel.GithubWorkflowRunSummaries `json:"run_summaries"`
	Total        int64                                 `json:"total"`
}

type GithubRunSummaryJobsResponse struct {
	Jobs  []*dbmodel.GithubWorkflowRuns `json:"jobs"`
	Total int64                         `json:"total"`
}

// GithubRunSummaryGraphRequest is the request for getting workflow DAG graph
type GithubRunSummaryGraphRequest struct {
	ID      string `json:"id" uri:"id"`
	Cluster string `json:"cluster" form:"cluster"`
}

// GithubJobNode represents a job node in the workflow DAG
type GithubJobNode struct {
	ID              int64    `json:"id"`
	GithubJobID     int64    `json:"github_job_id"`
	Name            string   `json:"name"`
	Status          string   `json:"status"`
	Conclusion      string   `json:"conclusion,omitempty"`
	Needs           []string `json:"needs,omitempty"`
	StartedAt       string   `json:"started_at,omitempty"`
	CompletedAt     string   `json:"completed_at,omitempty"`
	DurationSeconds int      `json:"duration_seconds"`
	StepsCount      int      `json:"steps_count"`
	StepsCompleted  int      `json:"steps_completed"`
	StepsFailed     int      `json:"steps_failed"`
	HTMLURL         string   `json:"html_url,omitempty"`
}

// GithubRunSummaryGraphResponse contains the workflow DAG graph
type GithubRunSummaryGraphResponse struct {
	Jobs  []*GithubJobNode `json:"jobs"`
	Total int64            `json:"total"`
}

// ======================== Handler Implementations ========================

func handleGithubRunnerSetsList(ctx context.Context, req *GithubRunnerSetsListRequest) (*GithubRunnerSetsListResponse, error) {
	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubRunnerSet()

	var runnerSets interface{}
	var listErr error

	if req.WithStats == "true" {
		runnerSets, listErr = facade.ListWithRunStats(ctx)
	} else if req.Namespace != "" {
		runnerSets, listErr = facade.ListByNamespace(ctx, req.Namespace)
	} else {
		runnerSets, listErr = facade.List(ctx)
	}

	if listErr != nil {
		return nil, errors.WrapError(listErr, "failed to list runner sets", errors.CodeDatabaseError)
	}

	return &GithubRunnerSetsListResponse{
		RunnerSets: runnerSets,
	}, nil
}

func handleGithubRunnerSetGet(ctx context.Context, req *GithubRunnerSetGetRequest) (**dbmodel.GithubRunnerSets, error) {
	if req.Namespace == "" || req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("namespace and name are required")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubRunnerSet()
	runnerSet, err := facade.GetByNamespaceName(ctx, req.Namespace, req.Name)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get runner set", errors.CodeDatabaseError)
	}
	if runnerSet == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("runner set not found")
	}

	return &runnerSet, nil
}

func handleGithubRunnerSetByID(ctx context.Context, req *GithubRunnerSetByIDRequest) (**dbmodel.GithubRunnerSets, error) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid runner set id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubRunnerSet()
	runnerSet, err := facade.GetByID(ctx, id)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get runner set", errors.CodeDatabaseError)
	}
	if runnerSet == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("runner set not found")
	}

	return &runnerSet, nil
}

func handleGithubRunnerSetRuns(ctx context.Context, req *GithubRunnerSetRunsRequest) (*GithubRunnerSetRunsResponse, error) {
	runnerSetID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid runner set id")
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

	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	runs, total, err := runFacade.List(ctx, &database.GithubWorkflowRunFilter{
		RunnerSetID: runnerSetID,
		Status:      req.Status,
		Offset:      offset,
		Limit:       limit,
	})
	if err != nil {
		return nil, errors.WrapError(err, "failed to list runs", errors.CodeDatabaseError)
	}

	return &GithubRunnerSetRunsResponse{
		Runs:   runs,
		Total:  total,
		Offset: offset,
		Limit:  limit,
	}, nil
}

func handleGithubRunnerSetConfig(ctx context.Context, req *GithubRunnerSetConfigRequest) (**dbmodel.GithubWorkflowConfigs, error) {
	runnerSetID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid runner set id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Get runner set first
	runnerSetFacade := database.GetFacadeForCluster(clusterName).GetGithubRunnerSet()
	runnerSet, err := runnerSetFacade.GetByID(ctx, runnerSetID)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get runner set", errors.CodeDatabaseError)
	}
	if runnerSet == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("runner set not found")
	}

	// Find config by runner set namespace/name
	configFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
	configs, err := configFacade.ListByRunnerSet(ctx, runnerSet.Namespace, runnerSet.Name)
	if err != nil {
		return nil, errors.WrapError(err, "failed to list configs", errors.CodeDatabaseError)
	}

	// Return first enabled config, or null if none
	for _, config := range configs {
		if config.Enabled {
			return &config, nil
		}
	}

	// No enabled config found - return nil (not an error)
	var nilConfig *dbmodel.GithubWorkflowConfigs
	return &nilConfig, nil
}

func handleGithubRunnerSetStats(ctx context.Context, req *GithubRunnerSetStatsRequest) (*GithubRunnerSetStatsResponse, error) {
	runnerSetID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid runner set id")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()

	// Count runs by status
	allRuns, _, _ := runFacade.List(ctx, &database.GithubWorkflowRunFilter{
		RunnerSetID: runnerSetID,
	})

	stats := &GithubRunnerSetStatsResponse{
		Total:     len(allRuns),
		Pending:   0,
		Completed: 0,
		Failed:    0,
		Skipped:   0,
	}

	for _, run := range allRuns {
		switch run.Status {
		case database.WorkflowRunStatusPending:
			stats.Pending++
		case database.WorkflowRunStatusCompleted:
			stats.Completed++
		case database.WorkflowRunStatusFailed:
			stats.Failed++
		case database.WorkflowRunStatusSkipped:
			stats.Skipped++
		}
	}

	// Get config info
	runnerSetFacade := database.GetFacadeForCluster(clusterName).GetGithubRunnerSet()
	runnerSet, _ := runnerSetFacade.GetByID(ctx, runnerSetID)
	if runnerSet != nil {
		configFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
		configs, _ := configFacade.ListByRunnerSet(ctx, runnerSet.Namespace, runnerSet.Name)
		for _, config := range configs {
			if config.Enabled {
				stats.HasConfig = true
				stats.ConfigID = config.ID
				stats.ConfigName = config.Name
				break
			}
		}
	}

	return stats, nil
}

// ======================== Repository Handler Implementations ========================

func handleGithubRepositoriesList(ctx context.Context, req *GithubRepositoriesListRequest) (*GithubRepositoriesListResponse, error) {
	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubRunnerSet()
	repositories, err := facade.ListRepositories(ctx)
	if err != nil {
		return nil, errors.WrapError(err, "failed to list repositories", errors.CodeDatabaseError)
	}

	return &GithubRepositoriesListResponse{
		Repositories: repositories,
	}, nil
}

func handleGithubRepositoryGet(ctx context.Context, req *GithubRepositoryGetRequest) (**database.RepositorySummary, error) {
	if req.Owner == "" || req.Repo == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("owner and repo are required")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubRunnerSet()
	summary, err := facade.GetRepositorySummary(ctx, req.Owner, req.Repo)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get repository summary", errors.CodeDatabaseError)
	}
	if summary == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("repository not found")
	}

	return &summary, nil
}

func handleGithubRepositoryRunnerSets(ctx context.Context, req *GithubRepositoryRunnerSetsRequest) (*GithubRepositoryRunnerSetsResponse, error) {
	if req.Owner == "" || req.Repo == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("owner and repo are required")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubRunnerSet()

	var runnerSets interface{}
	var listErr error

	if req.WithStats == "true" {
		runnerSets, listErr = facade.ListByRepositoryWithStats(ctx, req.Owner, req.Repo)
	} else {
		runnerSets, listErr = facade.ListByRepository(ctx, req.Owner, req.Repo)
	}

	if listErr != nil {
		return nil, errors.WrapError(listErr, "failed to list runner sets", errors.CodeDatabaseError)
	}

	return &GithubRepositoryRunnerSetsResponse{
		RunnerSets: runnerSets,
	}, nil
}

func handleGithubRepositoryMetricsMetadata(ctx context.Context, req *GithubRepositoryMetricsMetadataRequest) (*GithubRepositoryMetricsMetadataResponse, error) {
	if req.Owner == "" || req.Repo == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("owner and repo are required")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Get all runner sets for this repository
	runnerSetFacade := database.GetFacadeForCluster(clusterName).GetGithubRunnerSet()
	runnerSets, err := runnerSetFacade.ListByRepository(ctx, req.Owner, req.Repo)
	if err != nil {
		return nil, errors.WrapError(err, "failed to list runner sets", errors.CodeDatabaseError)
	}

	configFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
	schemaFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowSchema()
	metricsFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowMetrics()

	response := &GithubRepositoryMetricsMetadataResponse{
		Owner:   req.Owner,
		Repo:    req.Repo,
		Configs: make([]ConfigMetricsInfo, 0),
	}

	allDimensions := make(map[string]bool)
	allMetrics := make(map[string]bool)
	dimensionSets := make([]map[string]bool, 0)
	metricSets := make([]map[string]bool, 0)

	for _, rs := range runnerSets {
		// Find config for this runner set
		configs, err := configFacade.ListByRunnerSet(ctx, rs.Namespace, rs.Name)
		if err != nil {
			continue
		}

		for _, config := range configs {
			if !config.Enabled {
				continue
			}

			info := ConfigMetricsInfo{
				ConfigID:      config.ID,
				ConfigName:    config.Name,
				RunnerSetID:   rs.ID,
				RunnerSetName: rs.Name,
			}

			// Get active schema for this config
			schema, err := schemaFacade.GetActiveByConfig(ctx, config.ID)
			if err == nil && schema != nil {
				info.SchemaID = schema.ID
				info.SchemaVersion = schema.Version
				info.RecordCount = schema.RecordCount

				// Parse dimension and metric fields from schema
				var dimFields, metricFields []string
				if len(schema.DimensionFields) > 0 {
					json.Unmarshal(schema.DimensionFields, &dimFields)
				}
				if len(schema.MetricFields) > 0 {
					json.Unmarshal(schema.MetricFields, &metricFields)
				}
				info.DimensionFields = dimFields
				info.MetricFields = metricFields

				// Track for intersection/union calculation
				dimSet := make(map[string]bool)
				for _, d := range dimFields {
					dimSet[d] = true
					allDimensions[d] = true
				}
				dimensionSets = append(dimensionSets, dimSet)

				metricSet := make(map[string]bool)
				for _, m := range metricFields {
					metricSet[m] = true
					allMetrics[m] = true
				}
				metricSets = append(metricSets, metricSet)
			} else {
				// No schema, try to get fields from metrics directly
				dimFields, _ := metricsFacade.GetAvailableDimensions(ctx, config.ID)
				metricFields, _ := metricsFacade.GetAvailableMetricFields(ctx, config.ID)
				info.DimensionFields = dimFields
				info.MetricFields = metricFields

				dimSet := make(map[string]bool)
				for _, d := range dimFields {
					dimSet[d] = true
					allDimensions[d] = true
				}
				dimensionSets = append(dimensionSets, dimSet)

				metricSet := make(map[string]bool)
				for _, m := range metricFields {
					metricSet[m] = true
					allMetrics[m] = true
				}
				metricSets = append(metricSets, metricSet)
			}

			response.Configs = append(response.Configs, info)
		}
	}

	// Calculate common (intersection) and all (union) fields
	for d := range allDimensions {
		response.AllDimensions = append(response.AllDimensions, d)
		// Check if in all sets
		inAll := true
		for _, set := range dimensionSets {
			if !set[d] {
				inAll = false
				break
			}
		}
		if inAll && len(dimensionSets) > 0 {
			response.CommonDimensions = append(response.CommonDimensions, d)
		}
	}

	for m := range allMetrics {
		response.AllMetrics = append(response.AllMetrics, m)
		// Check if in all sets
		inAll := true
		for _, set := range metricSets {
			if !set[m] {
				inAll = false
				break
			}
		}
		if inAll && len(metricSets) > 0 {
			response.CommonMetrics = append(response.CommonMetrics, m)
		}
	}

	return response, nil
}

func handleGithubRepositoryMetricsTrends(ctx context.Context, req *GithubRepositoryMetricsTrendsRequest) (**database.MetricsTrendsResult, error) {
	if req.Owner == "" || req.Repo == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("owner and repo are required")
	}
	if len(req.MetricFields) == 0 {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("metric_fields is required")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	// Parse time range
	var start, end *time.Time
	if req.Start != "" {
		if t, parseErr := time.Parse(time.RFC3339, req.Start); parseErr == nil {
			start = &t
		}
	}
	if req.End != "" {
		if t, parseErr := time.Parse(time.RFC3339, req.End); parseErr == nil {
			end = &t
		}
	}

	interval := req.Interval
	if interval == "" {
		interval = "1d"
	}

	// Get configs to query
	var configIDs []int64
	if len(req.ConfigIDs) > 0 {
		configIDs = req.ConfigIDs
	} else {
		// Get all configs for this repository
		runnerSetFacade := database.GetFacadeForCluster(clusterName).GetGithubRunnerSet()
		runnerSets, err := runnerSetFacade.ListByRepository(ctx, req.Owner, req.Repo)
		if err != nil {
			return nil, errors.WrapError(err, "failed to list runner sets", errors.CodeDatabaseError)
		}

		configFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
		for _, rs := range runnerSets {
			configs, _ := configFacade.ListByRunnerSet(ctx, rs.Namespace, rs.Name)
			for _, config := range configs {
				if config.Enabled {
					configIDs = append(configIDs, config.ID)
				}
			}
		}
	}

	if len(configIDs) == 0 {
		emptyResult := &database.MetricsTrendsResult{
			Interval: interval,
			Series:   make([]database.MetricSeriesData, 0),
		}
		return &emptyResult, nil
	}

	// Query metrics from all configs
	metricsFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowMetrics()
	configFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()

	result := &database.MetricsTrendsResult{
		Interval:   interval,
		Series:     make([]database.MetricSeriesData, 0),
		Timestamps: make([]time.Time, 0),
	}

	timestampSet := make(map[time.Time]bool)

	for _, configID := range configIDs {
		query := &database.MetricsTrendsQuery{
			ConfigID:     configID,
			Start:        start,
			End:          end,
			Dimensions:   req.Dimensions,
			MetricFields: req.MetricFields,
			Interval:     interval,
			GroupBy:      req.GroupBy,
		}

		configResult, err := metricsFacade.GetMetricsTrends(ctx, query)
		if err != nil {
			continue
		}

		// Collect timestamps
		for _, ts := range configResult.Timestamps {
			timestampSet[ts] = true
		}

		// Add series with config info
		if req.AggregateAcrossConfigs {
			// Merge into existing series
			for _, series := range configResult.Series {
				result.Series = append(result.Series, series)
			}
		} else {
			// Add config info to series name
			config, _ := configFacade.GetByID(ctx, configID)
			configName := ""
			if config != nil {
				configName = config.Name
			}

			for _, series := range configResult.Series {
				series.Name = configName + " - " + series.Field
				result.Series = append(result.Series, series)
			}
		}
	}

	// Convert timestamp set to sorted slice
	for ts := range timestampSet {
		result.Timestamps = append(result.Timestamps, ts)
	}

	return &result, nil
}

// Helper function to get cluster name
func getClusterNameForGithubWorkflowFromRequest(cluster string) (string, error) {
	if cluster == "" {
		cm := clientsets.GetClusterManager()
		// Use default dataplane cluster, not current cluster (which is control-plane)
		defaultCluster := cm.GetDefaultClusterName()
		if defaultCluster != "" {
			return defaultCluster, nil
		}
		// Fallback: try to find a dataplane cluster from available clusters
		currentCluster := cm.GetCurrentClusterName()
		clusterNames := cm.GetClusterNames()
		for _, name := range clusterNames {
			// Skip control-plane cluster
			if name != currentCluster && name != "control-plane" {
				return name, nil
			}
		}
		// Last resort: use current cluster
		return currentCluster, nil
	}
	return cluster, nil
}

// ======================== Run Summary Handler Implementations ========================

func handleGithubRunSummariesList(ctx context.Context, req *GithubRunSummariesListRequest) (*GithubRunSummariesListResponse, error) {
	if req.Owner == "" || req.Repo == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("owner and repo are required")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRunSummary()

	// Build filter
	filter := &database.RunSummaryFilter{
		Owner:            req.Owner,
		Repo:             req.Repo,
		Status:           req.Status,
		Conclusion:       req.Conclusion,
		CollectionStatus: req.CollectionStatus,
		WorkflowPath:     req.WorkflowPath,
		HeadBranch:       req.HeadBranch,
		EventName:        req.EventName,
	}

	if req.RunnerSetID != "" {
		if id, parseErr := strconv.ParseInt(req.RunnerSetID, 10, 64); parseErr == nil {
			filter.RunnerSetID = id
		}
	}

	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	filter.Offset = offset
	filter.Limit = limit
	summaries, total, err := facade.ListByRepo(ctx, req.Owner, req.Repo, filter)
	if err != nil {
		return nil, errors.WrapError(err, "failed to list run summaries", errors.CodeDatabaseError)
	}

	return &GithubRunSummariesListResponse{
		RunSummaries: summaries,
		Total:        total,
	}, nil
}

func handleGithubRunSummaryGet(ctx context.Context, req *GithubRunSummaryGetRequest) (**dbmodel.GithubWorkflowRunSummaries, error) {
	if req.ID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("id is required")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	id, parseErr := strconv.ParseInt(req.ID, 10, 64)
	if parseErr != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid id format")
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRunSummary()
	summary, err := facade.GetByID(ctx, id)
	if err != nil {
		return nil, errors.WrapError(err, "failed to get run summary", errors.CodeDatabaseError)
	}
	if summary == nil {
		return nil, errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("run summary not found")
	}

	return &summary, nil
}

func handleGithubRunSummaryJobs(ctx context.Context, req *GithubRunSummaryJobsRequest) (*GithubRunSummaryJobsResponse, error) {
	if req.ID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("id is required")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	id, parseErr := strconv.ParseInt(req.ID, 10, 64)
	if parseErr != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid id format")
	}

	// Get jobs associated with this run summary
	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	jobs, err := runFacade.ListByRunSummaryID(ctx, id)
	if err != nil {
		return nil, errors.WrapError(err, "failed to list jobs for run summary", errors.CodeDatabaseError)
	}

	return &GithubRunSummaryJobsResponse{
		Jobs:  jobs,
		Total: int64(len(jobs)),
	}, nil
}

func handleGithubRunSummaryGraph(ctx context.Context, req *GithubRunSummaryGraphRequest) (*GithubRunSummaryGraphResponse, error) {
	if req.ID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("id is required")
	}

	clusterName, err := getClusterNameForGithubWorkflowFromRequest(req.Cluster)
	if err != nil {
		return nil, err
	}

	id, parseErr := strconv.ParseInt(req.ID, 10, 64)
	if parseErr != nil {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("invalid id format")
	}

	// Get GitHub jobs for this run summary
	jobFacade := database.NewGithubWorkflowJobFacade()
	_ = clusterName // TODO: add cluster support to job facade
	githubJobs, err := jobFacade.ListByRunSummaryID(ctx, id)
	if err != nil {
		return nil, errors.WrapError(err, "failed to list github jobs for run summary", errors.CodeDatabaseError)
	}

	// Convert to response nodes with needs parsing
	nodes := make([]*GithubJobNode, len(githubJobs))
	for i, job := range githubJobs {
		node := &GithubJobNode{
			ID:              job.ID,
			GithubJobID:     job.GithubJobID,
			Name:            job.Name,
			Status:          job.Status,
			Conclusion:      job.Conclusion,
			DurationSeconds: job.DurationSeconds,
			StepsCount:      job.StepsCount,
			StepsCompleted:  job.StepsCompleted,
			StepsFailed:     job.StepsFailed,
			HTMLURL:         job.HTMLURL,
		}

		if job.StartedAt != nil {
			node.StartedAt = job.StartedAt.Format("2006-01-02T15:04:05Z")
		}
		if job.CompletedAt != nil {
			node.CompletedAt = job.CompletedAt.Format("2006-01-02T15:04:05Z")
		}

		// Parse needs JSON array
		if job.Needs != "" {
			var needs []string
			if err := json.Unmarshal([]byte(job.Needs), &needs); err == nil {
				node.Needs = needs
			}
		}

		nodes[i] = node
	}

	return &GithubRunSummaryGraphResponse{
		Jobs:  nodes,
		Total: int64(len(nodes)),
	}, nil
}
