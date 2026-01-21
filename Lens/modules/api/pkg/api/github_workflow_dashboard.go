// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// ========== Dashboard API Response Types ==========

// DashboardSummaryResponse represents the dashboard summary response
type DashboardSummaryResponse struct {
	Config       *ConfigInfo          `json:"config"`
	SummaryDate  string               `json:"summary_date"`
	Build        *BuildInfo           `json:"build"`
	CodeChanges  *CodeChangesInfo     `json:"code_changes"`
	Performance  *PerformanceInfo     `json:"performance"`
	Contributors []ContributorInfo    `json:"contributors"`
	Anomalies    *AnomaliesInfo       `json:"anomalies"`
	GeneratedAt  time.Time            `json:"generated_at"`
}

// ConfigInfo represents basic config information
type ConfigInfo struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	GithubOwner string `json:"github_owner"`
	GithubRepo  string `json:"github_repo"`
}

// BuildInfo represents build status information
type BuildInfo struct {
	CurrentRunID      *int64     `json:"current_run_id"`
	Status            string     `json:"status"`
	DurationSeconds   *int       `json:"duration_seconds"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	GithubRunNumber   int32      `json:"github_run_number,omitempty"`
	HeadSHA           string     `json:"head_sha,omitempty"`
	HeadBranch        string     `json:"head_branch,omitempty"`
}

// CodeChangesInfo represents code changes summary
type CodeChangesInfo struct {
	CommitCount      int    `json:"commit_count"`
	PRCount          int    `json:"pr_count"`
	ContributorCount int    `json:"contributor_count"`
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	CommitRange      *CommitRange `json:"commit_range,omitempty"`
}

// CommitRange represents the commit range
type CommitRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// PerformanceInfo represents performance summary
type PerformanceInfo struct {
	OverallChangePercent *float64              `json:"overall_change_percent"`
	RegressionCount      int                   `json:"regression_count"`
	ImprovementCount     int                   `json:"improvement_count"`
	NewMetricCount       int                   `json:"new_metric_count"`
	TopImprovements      []PerformanceChangeResponse `json:"top_improvements"`
	TopRegressions       []PerformanceChangeResponse `json:"top_regressions"`
}

// PerformanceChangeResponse represents a performance change entry
type PerformanceChangeResponse struct {
	Metric        string                 `json:"metric"`
	Dimensions    map[string]interface{} `json:"dimensions,omitempty"`
	CurrentValue  float64                `json:"current_value"`
	PreviousValue float64                `json:"previous_value"`
	ChangePercent float64                `json:"change_percent"`
	Unit          string                 `json:"unit,omitempty"`
	LikelyCommit  *CommitInfoResponse    `json:"likely_commit,omitempty"`
}

// CommitInfoResponse represents commit information
type CommitInfoResponse struct {
	SHA     string `json:"sha"`
	Author  string `json:"author"`
	Message string `json:"message"`
}

// ContributorInfo represents contributor information
type ContributorInfo struct {
	Author    string `json:"author"`
	Email     string `json:"email,omitempty"`
	Commits   int    `json:"commits"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	PRs       int    `json:"prs,omitempty"`
}

// AnomaliesInfo represents anomaly alerts
type AnomaliesInfo struct {
	RegressionAlerts int `json:"regression_alerts"`
	NewMetrics       int `json:"new_metrics"`
	FlakyTests       int `json:"flaky_tests"`
}

// RecentBuildInfo represents a build in the recent builds list
type RecentBuildInfo struct {
	RunID            int64      `json:"run_id"`
	GithubRunNumber  int32      `json:"github_run_number"`
	Status           string     `json:"status"`
	PerfChangePercent *float64  `json:"perf_change_percent"`
	HeadSHA          string     `json:"head_sha"`
	LastCommitter    string     `json:"last_committer,omitempty"`
	CompletedAt      *time.Time `json:"completed_at"`
	DurationSeconds  *int       `json:"duration_seconds"`
	MetricsCount     int32      `json:"metrics_count"`
}

// ========== Dashboard API Handlers ==========

// GetDashboardSummary handles GET /v1/github-workflow-metrics/configs/:id/dashboard
// Returns the dashboard summary for a config
func GetDashboardSummary(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	// Parse date parameter (default: today)
	dateStr := ctx.Query("date")
	var summaryDate time.Time
	if dateStr != "" {
		summaryDate, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid date format, expected YYYY-MM-DD", nil))
			return
		}
	} else {
		summaryDate = time.Now()
	}

	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Get config
	configFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowConfig()
	config, err := configFacade.GetByID(ctx.Request.Context(), configID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get github workflow config: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get config", err))
		return
	}
	if config == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "config not found", nil))
		return
	}

	// Try to get cached summary
	summaryFacade := database.GetFacadeForCluster(clusterName).GetDashboardSummary()
	cachedSummary, err := summaryFacade.GetByConfigAndDate(ctx.Request.Context(), configID, summaryDate)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Warningf("Failed to get cached summary: %v", err)
	}

	if cachedSummary != nil && !cachedSummary.IsStale {
		// Return cached summary
		response := convertCachedSummaryToResponse(config, cachedSummary)
		ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), response))
		return
	}

	// Generate summary using DashboardService (Phase 2 implementation)
	service := NewDashboardService(clusterName)
	response, err := service.GenerateDashboardSummary(ctx.Request.Context(), config, summaryDate)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to generate dashboard summary: %v", err)
		// Fallback to basic summary
		response = generateBasicDashboardSummary(ctx, clusterName, config, summaryDate)
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), response))
}

// GetDashboardRecentBuilds handles GET /v1/github-workflow-metrics/configs/:id/dashboard/builds
// Returns the recent builds list for a config
func GetDashboardRecentBuilds(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	limit := 10
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 50 {
			limit = l
		}
	}

	offset := 0
	if offsetStr := ctx.Query("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Get runs
	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	runs, total, err := runFacade.List(ctx.Request.Context(), &database.GithubWorkflowRunFilter{
		ConfigID: configID,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list runs: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to list runs", err))
		return
	}

	// Convert to response format
	builds := make([]RecentBuildInfo, 0, len(runs))
	for _, run := range runs {
		build := RecentBuildInfo{
			RunID:           run.ID,
			GithubRunNumber: run.GithubRunNumber,
			Status:          run.Status,
			HeadSHA:         run.HeadSha,
			MetricsCount:    run.MetricsCount,
		}
		if !run.WorkloadCompletedAt.IsZero() {
			build.CompletedAt = &run.WorkloadCompletedAt
		}
		if !run.WorkloadStartedAt.IsZero() && !run.WorkloadCompletedAt.IsZero() {
			duration := int(run.WorkloadCompletedAt.Sub(run.WorkloadStartedAt).Seconds())
			build.DurationSeconds = &duration
		}
		builds = append(builds, build)
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"builds": builds,
		"total":  total,
		"offset": offset,
		"limit":  limit,
	}))
}

// RefreshDashboardSummary handles POST /v1/github-workflow-metrics/configs/:id/dashboard/refresh
// Triggers a refresh of the dashboard summary cache
func RefreshDashboardSummary(ctx *gin.Context) {
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

	// Mark existing summaries as stale
	summaryFacade := database.GetFacadeForCluster(clusterName).GetDashboardSummary()
	if err := summaryFacade.MarkAllStaleForConfig(ctx.Request.Context(), configID); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to mark summaries as stale: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to refresh dashboard", err))
		return
	}

	log.GlobalLogger().WithContext(ctx).Infof("Dashboard summary cache invalidated for config %d", configID)
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message":   "Dashboard refresh triggered",
		"config_id": configID,
	}))
}

// GetRunDetail handles GET /v1/github-workflow-metrics/runs/:id/detail
// Returns detailed information for a specific run including commits and performance comparison
func GetRunDetail(ctx *gin.Context) {
	runIDStr := ctx.Param("id")
	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid run_id", nil))
		return
	}

	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Get run
	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	run, err := runFacade.GetByID(ctx.Request.Context(), runID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get run: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get run", err))
		return
	}
	if run == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "run not found", nil))
		return
	}

	// Get previous run
	var previousRun *dbmodel.GithubWorkflowRuns
	previousRuns, _, _ := runFacade.List(ctx.Request.Context(), &database.GithubWorkflowRunFilter{
		ConfigID: run.ConfigID,
		Status:   database.WorkflowRunStatusCompleted,
		Limit:    1,
	})
	for _, pr := range previousRuns {
		if pr.ID < run.ID {
			previousRun = pr
			break
		}
	}

	// Get commit details
	commitFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowCommit()
	commit, _ := commitFacade.GetByRunID(ctx.Request.Context(), runID)

	// Build response
	response := gin.H{
		"run": gin.H{
			"id":                run.ID,
			"github_run_number": run.GithubRunNumber,
			"status":            run.Status,
			"head_sha":          run.HeadSha,
			"head_branch":       run.HeadBranch,
			"completed_at":      run.WorkloadCompletedAt,
		},
	}

	if previousRun != nil {
		duration := int(run.WorkloadCompletedAt.Sub(run.WorkloadStartedAt).Seconds())
		response["run"].(gin.H)["duration_seconds"] = duration
		response["previous_run"] = gin.H{
			"id":           previousRun.ID,
			"head_sha":     previousRun.HeadSha,
			"completed_at": previousRun.WorkloadCompletedAt,
		}
	}

	if commit != nil {
		commitInfo := gin.H{
			"sha":          commit.SHA,
			"message":      commit.Message,
			"author":       commit.AuthorName,
			"committed_at": commit.CommitterDate,
			"additions":    commit.Additions,
			"deletions":    commit.Deletions,
		}
		var files []string
		if err := commit.Files.UnmarshalTo(&files); err == nil {
			commitInfo["files"] = files
		}
		response["commits"] = []gin.H{commitInfo}
	}

	// Phase 2: Performance comparison using DashboardService
	service := NewDashboardService(clusterName)
	perfComparison := gin.H{
		"total_compared": 0,
		"improved":       0,
		"regressed":      0,
		"unchanged":      0,
		"new":            0,
		"details":        []interface{}{},
	}
	regressionAnalysis := []interface{}{}

	if previousRun != nil {
		perfSummary, err := service.calculatePerformanceChanges(ctx.Request.Context(), run, previousRun)
		if err == nil && perfSummary != nil {
			totalCompared := perfSummary.ImprovementCount + perfSummary.RegressionCount
			unchanged := 0
			if totalCompared > 0 {
				// Estimate unchanged based on total metrics
				metricsFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowMetrics()
				currentCount, _ := metricsFacade.CountByRun(ctx.Request.Context(), run.ID)
				unchanged = int(currentCount) - totalCompared - perfSummary.NewMetricCount
				if unchanged < 0 {
					unchanged = 0
				}
			}

			perfComparison = gin.H{
				"total_compared": totalCompared + unchanged,
				"improved":       perfSummary.ImprovementCount,
				"regressed":      perfSummary.RegressionCount,
				"unchanged":      unchanged,
				"new":            perfSummary.NewMetricCount,
				"details":        append(perfSummary.TopImprovements, perfSummary.TopRegressions...),
			}

			// Analyze regressions if any
			if len(perfSummary.TopRegressions) > 0 && commit != nil {
				commits := []*dbmodel.GithubWorkflowCommits{commit}
				service.analyzeRegressions(ctx.Request.Context(), perfSummary, commits)

				for _, reg := range perfSummary.TopRegressions {
					analysis := gin.H{
						"metric":         reg.Metric,
						"change_percent": reg.ChangePercent,
					}
					if reg.LikelyCommit != nil {
						analysis["likely_commit"] = reg.LikelyCommit
						analysis["reason"] = "File path correlation"
					}
					regressionAnalysis = append(regressionAnalysis, analysis)
				}
			}
		}
	}

	response["performance_comparison"] = perfComparison
	response["regression_analysis"] = regressionAnalysis

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), response))
}

// GetCommitStats handles GET /v1/github-workflow-metrics/configs/:id/commits/stats
// Returns commit statistics for analytics
func GetCommitStats(ctx *gin.Context) {
	configIDStr := ctx.Param("id")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid config_id", nil))
		return
	}

	// Parse date range
	startDateStr := ctx.Query("start_date")
	endDateStr := ctx.Query("end_date")

	if startDateStr == "" || endDateStr == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "start_date and end_date are required", nil))
		return
	}

	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid start_date format", nil))
		return
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid end_date format", nil))
		return
	}

	clusterName, err := getClusterNameForGithubWorkflow(ctx)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Get commit statistics
	commitFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowCommit()
	stats, err := commitFacade.GetStats(ctx.Request.Context(), &startDate, &endDate)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get commit stats: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, "failed to get commit stats", err))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"period": gin.H{
			"start": startDateStr,
			"end":   endDateStr,
		},
		"summary": gin.H{
			"total_commits":      stats.TotalCommits,
			"total_contributors": stats.UniqueAuthors,
			"total_additions":    stats.TotalAdditions,
			"total_deletions":    stats.TotalDeletions,
		},
		"config_id": configID,
	}))
}

// ========== Helper Functions ==========

// convertCachedSummaryToResponse converts a cached summary to API response format
func convertCachedSummaryToResponse(config *dbmodel.GithubWorkflowConfigs, summary *dbmodel.DashboardSummaries) *DashboardSummaryResponse {
	response := &DashboardSummaryResponse{
		Config: &ConfigInfo{
			ID:          config.ID,
			Name:        config.Name,
			GithubOwner: config.GithubOwner,
			GithubRepo:  config.GithubRepo,
		},
		SummaryDate: summary.SummaryDate.Format("2006-01-02"),
		Build: &BuildInfo{
			CurrentRunID: summary.CurrentRunID,
			Status:       summary.BuildStatus,
			DurationSeconds: summary.BuildDurationSeconds,
		},
		CodeChanges: &CodeChangesInfo{
			CommitCount:      summary.CommitCount,
			PRCount:          summary.PRCount,
			ContributorCount: summary.ContributorCount,
			Additions:        summary.TotalAdditions,
			Deletions:        summary.TotalDeletions,
		},
		Performance: &PerformanceInfo{
			OverallChangePercent: summary.OverallPerfChangePercent,
			RegressionCount:      summary.RegressionCount,
			ImprovementCount:     summary.ImprovementCount,
			NewMetricCount:       summary.NewMetricCount,
		},
		GeneratedAt: summary.GeneratedAt,
	}

	// Parse top improvements
	var improvements []dbmodel.PerformanceChange
	if err := summary.TopImprovements.UnmarshalTo(&improvements); err == nil {
		response.Performance.TopImprovements = convertPerformanceChanges(improvements)
	}

	// Parse top regressions
	var regressions []dbmodel.PerformanceChange
	if err := summary.TopRegressions.UnmarshalTo(&regressions); err == nil {
		response.Performance.TopRegressions = convertPerformanceChanges(regressions)
	}

	// Parse contributors
	var contributors []dbmodel.ContributorSummary
	if err := summary.TopContributors.UnmarshalTo(&contributors); err == nil {
		response.Contributors = convertContributors(contributors)
	}

	// Parse alerts
	var alerts []dbmodel.AlertInfo
	if err := summary.ActiveAlerts.UnmarshalTo(&alerts); err == nil {
		response.Anomalies = &AnomaliesInfo{}
		for _, alert := range alerts {
			if alert.Type == "regression" {
				response.Anomalies.RegressionAlerts++
			} else if alert.Type == "new_metric" {
				response.Anomalies.NewMetrics++
			}
		}
	}

	return response
}

// generateBasicDashboardSummary generates a basic dashboard summary from the latest run
func generateBasicDashboardSummary(ctx *gin.Context, clusterName string, config *dbmodel.GithubWorkflowConfigs, summaryDate time.Time) *DashboardSummaryResponse {
	response := &DashboardSummaryResponse{
		Config: &ConfigInfo{
			ID:          config.ID,
			Name:        config.Name,
			GithubOwner: config.GithubOwner,
			GithubRepo:  config.GithubRepo,
		},
		SummaryDate: summaryDate.Format("2006-01-02"),
		GeneratedAt: time.Now(),
	}

	// Get latest completed run
	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	runs, _, err := runFacade.List(ctx.Request.Context(), &database.GithubWorkflowRunFilter{
		ConfigID: config.ID,
		Status:   database.WorkflowRunStatusCompleted,
		Limit:    1,
	})

	if err != nil || len(runs) == 0 {
		response.Build = &BuildInfo{
			Status: "no_data",
		}
		response.CodeChanges = &CodeChangesInfo{}
		response.Performance = &PerformanceInfo{
			TopImprovements: []PerformanceChangeResponse{},
			TopRegressions:  []PerformanceChangeResponse{},
		}
		response.Contributors = []ContributorInfo{}
		response.Anomalies = &AnomaliesInfo{}
		return response
	}

	latestRun := runs[0]
	response.Build = &BuildInfo{
		CurrentRunID:    &latestRun.ID,
		Status:          latestRun.Status,
		GithubRunNumber: latestRun.GithubRunNumber,
		HeadSHA:         latestRun.HeadSha,
		HeadBranch:      latestRun.HeadBranch,
	}
	if !latestRun.WorkloadCompletedAt.IsZero() {
		response.Build.CompletedAt = &latestRun.WorkloadCompletedAt
	}
	if !latestRun.WorkloadStartedAt.IsZero() && !latestRun.WorkloadCompletedAt.IsZero() {
		duration := int(latestRun.WorkloadCompletedAt.Sub(latestRun.WorkloadStartedAt).Seconds())
		response.Build.DurationSeconds = &duration
	}

	// Get commit info
	commitFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowCommit()
	commit, _ := commitFacade.GetByRunID(ctx.Request.Context(), latestRun.ID)
	if commit != nil {
		response.CodeChanges = &CodeChangesInfo{
			CommitCount:      1,
			ContributorCount: 1,
			Additions:        commit.Additions,
			Deletions:        commit.Deletions,
		}
		response.Contributors = []ContributorInfo{
			{
				Author:    commit.AuthorName,
				Email:     commit.AuthorEmail,
				Commits:   1,
				Additions: commit.Additions,
				Deletions: commit.Deletions,
			},
		}
	} else {
		response.CodeChanges = &CodeChangesInfo{}
		response.Contributors = []ContributorInfo{}
	}

	// Performance data will be populated in Phase 2
	response.Performance = &PerformanceInfo{
		TopImprovements: []PerformanceChangeResponse{},
		TopRegressions:  []PerformanceChangeResponse{},
	}
	response.Anomalies = &AnomaliesInfo{}

	return response
}

// convertPerformanceChanges converts model performance changes to response format
func convertPerformanceChanges(changes []dbmodel.PerformanceChange) []PerformanceChangeResponse {
	result := make([]PerformanceChangeResponse, 0, len(changes))
	for _, c := range changes {
		change := PerformanceChangeResponse{
			Metric:        c.Metric,
			Dimensions:    c.Dimensions,
			CurrentValue:  c.CurrentValue,
			PreviousValue: c.PreviousValue,
			ChangePercent: c.ChangePercent,
			Unit:          c.Unit,
		}
		if c.LikelyCommit != nil {
			change.LikelyCommit = &CommitInfoResponse{
				SHA:     c.LikelyCommit.SHA,
				Author:  c.LikelyCommit.Author,
				Message: c.LikelyCommit.Message,
			}
		}
		result = append(result, change)
	}
	return result
}

// convertContributors converts model contributors to response format
func convertContributors(contributors []dbmodel.ContributorSummary) []ContributorInfo {
	result := make([]ContributorInfo, 0, len(contributors))
	for _, c := range contributors {
		result = append(result, ContributorInfo{
			Author:    c.Author,
			Email:     c.Email,
			Commits:   c.Commits,
			Additions: c.Additions,
			Deletions: c.Deletions,
			PRs:       c.PRs,
		})
	}
	return result
}
