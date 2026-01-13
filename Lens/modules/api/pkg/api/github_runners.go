// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// ========== Runner Sets API ==========

// ListGithubRunnerSets handles GET /v1/github-runners/runner-sets
// Lists all discovered AutoScalingRunnerSets in the cluster
func ListGithubRunnerSets(ctx *gin.Context) {
	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	namespace := ctx.Query("namespace")
	withStats := ctx.Query("with_stats") == "true"

	facade := database.GetFacadeForCluster(clusterName).GetGithubRunnerSet()

	var runnerSets interface{}
	var listErr error

	if withStats {
		// Return runner sets with run statistics and config info
		runnerSets, listErr = facade.ListWithRunStats(ctx.Request.Context())
	} else if namespace != "" {
		runnerSets, listErr = facade.ListByNamespace(ctx.Request.Context(), namespace)
	} else {
		runnerSets, listErr = facade.List(ctx.Request.Context())
	}

	if listErr != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list runner sets: %v", listErr)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list runner sets", listErr))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"runner_sets": runnerSets,
	}))
}

// GetGithubRunnerSet handles GET /v1/github-runners/runner-sets/:namespace/:name
// Gets a specific AutoScalingRunnerSet
func GetGithubRunnerSet(ctx *gin.Context) {
	namespace := ctx.Param("namespace")
	name := ctx.Param("name")

	if namespace == "" || name == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "namespace and name are required", nil))
		return
	}

	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubRunnerSet()
	runnerSet, err := facade.GetByNamespaceName(ctx.Request.Context(), namespace, name)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get runner set: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get runner set", err))
		return
	}
	if runnerSet == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "runner set not found", nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), runnerSet))
}

// GetGithubRunnerSetByID handles GET /v1/github-runners/runner-sets/:id
// Gets a runner set by ID
func GetGithubRunnerSetByID(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid runner set id", nil))
		return
	}

	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubRunnerSet()
	runnerSet, err := facade.GetByID(ctx.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get runner set: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get runner set", err))
		return
	}
	if runnerSet == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "runner set not found", nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), runnerSet))
}

// ListRunsByRunnerSet handles GET /v1/github-runners/runner-sets/:id/runs
// Lists workflow runs for a runner set
func ListRunsByRunnerSet(ctx *gin.Context) {
	idStr := ctx.Param("id")
	runnerSetID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid runner set id", nil))
		return
	}

	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Parse pagination
	offset := 0
	limit := 20
	if offsetStr := ctx.Query("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}
	}
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if limit > 100 {
		limit = 100
	}

	status := ctx.Query("status")

	// Get runs
	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	runs, total, err := runFacade.List(ctx.Request.Context(), &database.GithubWorkflowRunFilter{
		RunnerSetID: runnerSetID,
		Status:      status,
		Offset:      offset,
		Limit:       limit,
	})
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list runs: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list runs", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"runs":   runs,
		"total":  total,
		"offset": offset,
		"limit":  limit,
	}))
}

// GetConfigByRunnerSet handles GET /v1/github-runners/runner-sets/:id/config
// Gets the config associated with a runner set (may return null)
func GetConfigByRunnerSet(ctx *gin.Context) {
	idStr := ctx.Param("id")
	runnerSetID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid runner set id", nil))
		return
	}

	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Get runner set first
	runnerSetFacade := database.GetFacadeForCluster(clusterName).GetGithubRunnerSet()
	runnerSet, err := runnerSetFacade.GetByID(ctx.Request.Context(), runnerSetID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get runner set: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get runner set", err))
		return
	}
	if runnerSet == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "runner set not found", nil))
		return
	}

	// Find config by runner set namespace/name
	configFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
	configs, err := configFacade.ListByRunnerSet(ctx.Request.Context(), runnerSet.Namespace, runnerSet.Name)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list configs: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list configs", err))
		return
	}

	// Return first enabled config, or null if none
	for _, config := range configs {
		if config.Enabled {
			ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), config))
			return
		}
	}

	// No enabled config found
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), nil))
}

// GetStatsByRunnerSet handles GET /v1/github-runners/runner-sets/:id/stats
// Gets statistics for a runner set
func GetStatsByRunnerSet(ctx *gin.Context) {
	idStr := ctx.Param("id")
	runnerSetID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid runner set id", nil))
		return
	}

	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()

	// Count runs by status
	allRuns, _, _ := runFacade.List(ctx.Request.Context(), &database.GithubWorkflowRunFilter{
		RunnerSetID: runnerSetID,
	})

	stats := map[string]interface{}{
		"total":     len(allRuns),
		"pending":   0,
		"completed": 0,
		"failed":    0,
		"skipped":   0,
	}

	for _, run := range allRuns {
		switch run.Status {
		case database.WorkflowRunStatusPending:
			stats["pending"] = stats["pending"].(int) + 1
		case database.WorkflowRunStatusCompleted:
			stats["completed"] = stats["completed"].(int) + 1
		case database.WorkflowRunStatusFailed:
			stats["failed"] = stats["failed"].(int) + 1
		case database.WorkflowRunStatusSkipped:
			stats["skipped"] = stats["skipped"].(int) + 1
		}
	}

	// Get config info
	runnerSetFacade := database.GetFacadeForCluster(clusterName).GetGithubRunnerSet()
	runnerSet, _ := runnerSetFacade.GetByID(ctx.Request.Context(), runnerSetID)
	if runnerSet != nil {
		configFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
		configs, _ := configFacade.ListByRunnerSet(ctx.Request.Context(), runnerSet.Namespace, runnerSet.Name)
		for _, config := range configs {
			if config.Enabled {
				stats["has_config"] = true
				stats["config_id"] = config.ID
				stats["config_name"] = config.Name
				break
			}
		}
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), stats))
}

// CreateConfigForRunnerSet handles POST /v1/github-runners/runner-sets/:id/config
// Creates a config for a runner set
func CreateConfigForRunnerSet(ctx *gin.Context) {
	idStr := ctx.Param("id")
	runnerSetID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid runner set id", nil))
		return
	}

	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Get runner set
	runnerSetFacade := database.GetFacadeForCluster(clusterName).GetGithubRunnerSet()
	runnerSet, err := runnerSetFacade.GetByID(ctx.Request.Context(), runnerSetID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get runner set: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get runner set", err))
		return
	}
	if runnerSet == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "runner set not found", nil))
		return
	}

	// Parse request body
	var req struct {
		Name          string   `json:"name" binding:"required"`
		Description   string   `json:"description"`
		FilePatterns  []string `json:"file_patterns" binding:"required"`
		WorkflowFilter string  `json:"workflow_filter"`
		BranchFilter   string  `json:"branch_filter"`
		Enabled       *bool    `json:"enabled"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid request body", err))
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// Create config
	config := &model.GithubWorkflowConfigs{
		Name:               req.Name,
		Description:        req.Description,
		RunnerSetNamespace: runnerSet.Namespace,
		RunnerSetName:      runnerSet.Name,
		RunnerSetUID:       runnerSet.UID,
		GithubOwner:        runnerSet.GithubOwner,
		GithubRepo:         runnerSet.GithubRepo,
		FilePatterns:       req.FilePatterns,
		WorkflowFilter:     req.WorkflowFilter,
		BranchFilter:       req.BranchFilter,
		Enabled:            enabled,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	configFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
	if err := configFacade.Create(ctx.Request.Context(), config); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to create config: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to create config", err))
		return
	}

	ctx.JSON(http.StatusCreated, rest.SuccessResp(ctx.Request.Context(), config))
}

// ========== Workflow Run Details API ==========

// GetGithubWorkflowRunCommit handles GET /v1/github-workflow-metrics/runs/:id/commit
// Gets commit details for a workflow run
func GetGithubWorkflowRunCommit(ctx *gin.Context) {
	idStr := ctx.Param("id")
	runID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid run id", nil))
		return
	}

	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowCommit()
	commit, err := facade.GetByRunID(ctx.Request.Context(), runID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get commit: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get commit", err))
		return
	}
	if commit == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "commit not found for this run", nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), commit))
}

// GetGithubWorkflowRunDetailsAPI handles GET /v1/github-workflow-metrics/runs/:id/details
// Gets workflow run details from GitHub
func GetGithubWorkflowRunDetailsAPI(ctx *gin.Context) {
	idStr := ctx.Param("id")
	runID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid run id", nil))
		return
	}

	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	facade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRunDetails()
	details, err := facade.GetByRunID(ctx.Request.Context(), runID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get workflow run details: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get details", err))
		return
	}
	if details == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "workflow run details not found", nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), details))
}

// ========== Analytics API ==========

// GetGithubWorkflowAnalytics handles GET /v1/github-workflow-metrics/configs/:id/analytics
// Gets analytics for a config's workflow runs
func GetGithubWorkflowAnalytics(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Parse time range
	var since, until *time.Time
	if sinceStr := ctx.Query("since"); sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = &t
		}
	}
	if untilStr := ctx.Query("until"); untilStr != "" {
		if t, err := time.Parse(time.RFC3339, untilStr); err == nil {
			until = &t
		}
	}

	workflowName := ctx.Query("workflow_name")
	event := ctx.Query("event")
	branch := ctx.Query("branch")

	// Get run analytics
	detailsFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRunDetails()
	analytics, err := detailsFacade.GetAnalytics(ctx.Request.Context(), &database.WorkflowAnalyticsFilter{
		Since:        since,
		Until:        until,
		WorkflowName: workflowName,
		Event:        event,
		Branch:       branch,
	})
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get workflow analytics: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get analytics", err))
		return
	}

	// Get commit stats
	commitFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowCommit()
	commitStats, _ := commitFacade.GetStats(ctx.Request.Context(), since, until)

	// Get run stats from runs table
	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	runs, total, _ := runFacade.List(ctx.Request.Context(), &database.GithubWorkflowRunFilter{
		ConfigID: configID,
		Since:    since,
		Until:    until,
	})

	// Calculate average execution time from runs
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

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"config_id":              configID,
		"total_runs":             total,
		"workflow_analytics":     analytics,
		"commit_stats":           commitStats,
		"avg_execution_seconds":  avgExecutionTime,
	}))
}

// GetGithubWorkflowRunHistory handles GET /v1/github-workflow-metrics/configs/:id/history
// Gets detailed execution history for a config
func GetGithubWorkflowRunHistory(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Parse pagination
	offset := 0
	limit := 20
	if offsetStr := ctx.Query("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}
	}
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if limit > 100 {
		limit = 100
	}

	// Get runs
	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	runs, total, err := runFacade.List(ctx.Request.Context(), &database.GithubWorkflowRunFilter{
		ConfigID: configID,
		Offset:   offset,
		Limit:    limit,
	})
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list runs: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list runs", err))
		return
	}

	// Enrich runs with commit and details
	commitFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowCommit()
	detailsFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRunDetails()

	type EnrichedRun struct {
		*model.GithubWorkflowRuns
		Commit      interface{} `json:"commit,omitempty"`
		RunDetails  interface{} `json:"run_details,omitempty"`
		Duration    float64     `json:"duration_seconds,omitempty"`
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
		if commit, _ := commitFacade.GetByRunID(ctx.Request.Context(), run.ID); commit != nil {
			enriched.Commit = commit
		}

		// Get run details if available
		if details, _ := detailsFacade.GetByRunID(ctx.Request.Context(), run.ID); details != nil {
			enriched.RunDetails = details
		}

		enrichedRuns = append(enrichedRuns, enriched)
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"runs":   enrichedRuns,
		"total":  total,
		"offset": offset,
		"limit":  limit,
	}))
}

