// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"strconv"

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
}

// ======================== Request Types ========================

type GithubRunnerSetsListRequest struct {
	Cluster   string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	Namespace string `json:"namespace" form:"namespace" mcp:"description=Filter by namespace"`
	WithStats string `json:"with_stats" form:"with_stats" mcp:"description=Include run statistics (true/false)"`
}

type GithubRunnerSetGetRequest struct {
	Cluster   string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	Namespace string `json:"namespace" form:"namespace" uri:"namespace" binding:"required" mcp:"description=Runner set namespace,required"`
	Name      string `json:"name" form:"name" uri:"name" binding:"required" mcp:"description=Runner set name,required"`
}

type GithubRunnerSetByIDRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Runner set ID,required"`
}

type GithubRunnerSetRunsRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Runner set ID,required"`
	Status  string `json:"status" form:"status" mcp:"description=Filter by status"`
	Offset  int    `json:"offset" form:"offset" mcp:"description=Pagination offset"`
	Limit   int    `json:"limit" form:"limit" mcp:"description=Pagination limit (max 100)"`
}

type GithubRunnerSetConfigRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Runner set ID,required"`
}

type GithubRunnerSetStatsRequest struct {
	Cluster string `json:"cluster" form:"cluster" mcp:"description=Cluster name"`
	ID      string `json:"id" form:"id" uri:"id" binding:"required" mcp:"description=Runner set ID,required"`
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

// Helper function to get cluster name
func getClusterNameForGithubWorkflowFromRequest(cluster string) (string, error) {
	if cluster == "" {
		return "default", nil
	}
	return cluster, nil
}
