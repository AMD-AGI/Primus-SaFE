// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aiclient"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// ========== Constants ==========

const (
	// Performance thresholds
	regressionThresholdPercent  = -3.0  // Changes worse than -3% are regressions
	improvementThresholdPercent = 3.0   // Changes better than +3% are improvements
	topChangesLimit             = 5     // Max number of top improvements/regressions
	topContributorsLimit        = 10    // Max number of top contributors

	// AI confidence thresholds
	aiConfidenceThreshold = 0.5 // Minimum confidence for AI results
)

// ========== DashboardService ==========

// DashboardService handles dashboard data aggregation and generation
type DashboardService struct {
	clusterName string
	aiClient    aiclient.Client
}

// NewDashboardService creates a new DashboardService
func NewDashboardService(clusterName string) *DashboardService {
	return &DashboardService{
		clusterName: clusterName,
		aiClient:    aiclient.GetGlobalClient(),
	}
}

// hasAIClient returns true if AI client is available
func (s *DashboardService) hasAIClient() bool {
	return s.aiClient != nil
}

// GenerateDashboardSummary generates a complete dashboard summary for a config
func (s *DashboardService) GenerateDashboardSummary(
	ctx context.Context,
	config *dbmodel.GithubWorkflowConfigs,
	summaryDate time.Time,
) (*DashboardSummaryResponse, error) {
	// 1. Get current and previous runs
	currentRun, previousRun, err := s.getCurrentAndPreviousRun(ctx, config.ID, summaryDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get runs: %w", err)
	}

	if currentRun == nil {
		return s.buildEmptySummary(config, summaryDate), nil
	}

	// 2. Get code changes summary
	codeChanges, contributors, err := s.getCodeChangesSummary(ctx, currentRun, previousRun)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Warningf("Failed to get code changes: %v", err)
		codeChanges = &CodeChangesInfo{}
		contributors = []ContributorInfo{}
	}

	// 3. Calculate performance changes
	perfSummary, err := s.calculatePerformanceChanges(ctx, currentRun, previousRun)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Warningf("Failed to calculate performance: %v", err)
		perfSummary = &PerformanceInfo{
			TopImprovements: []PerformanceChangeResponse{},
			TopRegressions:  []PerformanceChangeResponse{},
		}
	}

	// 4. Analyze regressions and find likely commits
	if len(perfSummary.TopRegressions) > 0 && currentRun != nil {
		commits, _ := s.getCommitsBetweenRuns(ctx, currentRun, previousRun)
		s.analyzeRegressions(ctx, perfSummary, commits)
	}

	// 5. Detect anomalies
	anomalies := s.detectAnomalies(perfSummary)

	// 6. Build response
	response := &DashboardSummaryResponse{
		Config: &ConfigInfo{
			ID:          config.ID,
			Name:        config.Name,
			GithubOwner: config.GithubOwner,
			GithubRepo:  config.GithubRepo,
		},
		SummaryDate:  summaryDate.Format("2006-01-02"),
		Build:        s.buildBuildInfo(currentRun),
		CodeChanges:  codeChanges,
		Performance:  perfSummary,
		Contributors: contributors,
		Anomalies:    anomalies,
		GeneratedAt:  time.Now(),
	}

	// 7. Cache the summary
	if err := s.cacheSummary(ctx, config.ID, summaryDate, response); err != nil {
		log.GlobalLogger().WithContext(ctx).Warningf("Failed to cache summary: %v", err)
	}

	return response, nil
}

// ========== Run Retrieval ==========

// getCurrentAndPreviousRun gets the current and previous completed runs
func (s *DashboardService) getCurrentAndPreviousRun(
	ctx context.Context,
	configID int64,
	date time.Time,
) (*dbmodel.GithubWorkflowRuns, *dbmodel.GithubWorkflowRuns, error) {
	facade := database.GetFacadeForCluster(s.clusterName).GetGithubWorkflowRun()

	// Get completed runs for this config, ordered by completion time
	runs, _, err := facade.List(ctx, &database.GithubWorkflowRunFilter{
		ConfigID: configID,
		Status:   database.WorkflowRunStatusCompleted,
		Limit:    10, // Get recent runs
	})
	if err != nil {
		return nil, nil, err
	}

	if len(runs) == 0 {
		return nil, nil, nil
	}

	// Find the most recent run on or before the target date
	var currentRun, previousRun *dbmodel.GithubWorkflowRuns
	targetDate := time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 0, date.Location())

	for _, run := range runs {
		if run.WorkloadCompletedAt.Before(targetDate) || run.WorkloadCompletedAt.Equal(targetDate) {
			if currentRun == nil {
				currentRun = run
			} else if previousRun == nil {
				previousRun = run
				break
			}
		}
	}

	return currentRun, previousRun, nil
}

// ========== Code Changes ==========

// getCodeChangesSummary aggregates code change information between runs
func (s *DashboardService) getCodeChangesSummary(
	ctx context.Context,
	currentRun, previousRun *dbmodel.GithubWorkflowRuns,
) (*CodeChangesInfo, []ContributorInfo, error) {
	if currentRun == nil {
		return &CodeChangesInfo{}, []ContributorInfo{}, nil
	}

	commitFacade := database.GetFacadeForCluster(s.clusterName).GetGithubWorkflowCommit()

	// Get commit for current run (one commit per run)
	commit, err := commitFacade.GetByRunID(ctx, currentRun.ID)
	if err != nil {
		return nil, nil, err
	}

	// Convert single commit to slice for processing
	var commits []*dbmodel.GithubWorkflowCommits
	if commit != nil {
		commits = []*dbmodel.GithubWorkflowCommits{commit}
	}

	codeChanges := &CodeChangesInfo{
		CommitCount:      len(commits),
		ContributorCount: 0,
		Additions:        0,
		Deletions:        0,
	}

	// Aggregate contributor stats
	contributorMap := make(map[string]*ContributorInfo)
	for _, commit := range commits {
		codeChanges.Additions += int(commit.Additions)
		codeChanges.Deletions += int(commit.Deletions)

		key := commit.AuthorEmail
		if key == "" {
			key = commit.AuthorName
		}

		if existing, ok := contributorMap[key]; ok {
			existing.Commits++
			existing.Additions += int(commit.Additions)
			existing.Deletions += int(commit.Deletions)
		} else {
			contributorMap[key] = &ContributorInfo{
				Author:    commit.AuthorName,
				Email:     commit.AuthorEmail,
				Commits:   1,
				Additions: int(commit.Additions),
				Deletions: int(commit.Deletions),
			}
		}
	}

	codeChanges.ContributorCount = len(contributorMap)

	// Set commit range
	if len(commits) > 0 && previousRun != nil {
		codeChanges.CommitRange = &CommitRange{
			From: previousRun.HeadSha,
			To:   currentRun.HeadSha,
		}
	}

	// Sort contributors by commit count
	contributors := make([]ContributorInfo, 0, len(contributorMap))
	for _, c := range contributorMap {
		contributors = append(contributors, *c)
	}
	sort.Slice(contributors, func(i, j int) bool {
		return contributors[i].Commits > contributors[j].Commits
	})

	// Limit to top contributors
	if len(contributors) > topContributorsLimit {
		contributors = contributors[:topContributorsLimit]
	}

	return codeChanges, contributors, nil
}

// ========== Performance Calculation ==========

// calculatePerformanceChanges calculates performance changes between two runs
func (s *DashboardService) calculatePerformanceChanges(
	ctx context.Context,
	currentRun, previousRun *dbmodel.GithubWorkflowRuns,
) (*PerformanceInfo, error) {
	if currentRun == nil {
		return &PerformanceInfo{
			TopImprovements: []PerformanceChangeResponse{},
			TopRegressions:  []PerformanceChangeResponse{},
		}, nil
	}

	metricsFacade := database.GetFacadeForCluster(s.clusterName).GetGithubWorkflowMetrics()

	// Get metrics for current run
	currentMetrics, err := metricsFacade.ListByRun(ctx, currentRun.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current metrics: %w", err)
	}

	// If no previous run, all metrics are new
	if previousRun == nil {
		return &PerformanceInfo{
			NewMetricCount:  len(currentMetrics),
			TopImprovements: []PerformanceChangeResponse{},
			TopRegressions:  []PerformanceChangeResponse{},
		}, nil
	}

	// Get metrics for previous run
	previousMetrics, err := metricsFacade.ListByRun(ctx, previousRun.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get previous metrics: %w", err)
	}

	// Build previous metrics map by dimension key
	prevMetricsMap := s.buildMetricsMap(previousMetrics)

	// Compare metrics
	var improvements, regressions []PerformanceChangeResponse
	var regressionCount, improvementCount, newMetricCount int
	var totalChangePercent float64
	var comparedCount int

	for _, curr := range currentMetrics {
		dimKey := s.buildDimensionKey(curr)
		prev, exists := prevMetricsMap[dimKey]

		if !exists {
			newMetricCount++
			continue
		}

		// Compare numeric metric values
		changes := s.compareMetricValues(curr, prev)
		for _, change := range changes {
			comparedCount++
			totalChangePercent += change.ChangePercent

			if change.ChangePercent <= regressionThresholdPercent {
				regressionCount++
				regressions = append(regressions, change)
			} else if change.ChangePercent >= improvementThresholdPercent {
				improvementCount++
				improvements = append(improvements, change)
			}
		}
	}

	// Sort and limit
	sort.Slice(improvements, func(i, j int) bool {
		return improvements[i].ChangePercent > improvements[j].ChangePercent
	})
	sort.Slice(regressions, func(i, j int) bool {
		return regressions[i].ChangePercent < regressions[j].ChangePercent
	})

	if len(improvements) > topChangesLimit {
		improvements = improvements[:topChangesLimit]
	}
	if len(regressions) > topChangesLimit {
		regressions = regressions[:topChangesLimit]
	}

	// Calculate overall change
	var overallChange *float64
	if comparedCount > 0 {
		avg := totalChangePercent / float64(comparedCount)
		overallChange = &avg
	}

	return &PerformanceInfo{
		OverallChangePercent: overallChange,
		RegressionCount:      regressionCount,
		ImprovementCount:     improvementCount,
		NewMetricCount:       newMetricCount,
		TopImprovements:      improvements,
		TopRegressions:       regressions,
	}, nil
}

// buildMetricsMap builds a map of metrics by dimension key
func (s *DashboardService) buildMetricsMap(metrics []*dbmodel.GithubWorkflowMetrics) map[string]*dbmodel.GithubWorkflowMetrics {
	result := make(map[string]*dbmodel.GithubWorkflowMetrics)
	for _, m := range metrics {
		key := s.buildDimensionKey(m)
		result[key] = m
	}
	return result
}

// buildDimensionKey creates a unique key from dimensions
func (s *DashboardService) buildDimensionKey(m *dbmodel.GithubWorkflowMetrics) string {
	dims := m.Dimensions
	if dims == nil {
		return ""
	}

	// Sort dimension keys for consistent ordering
	keys := make([]string, 0, len(dims))
	for k := range dims {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, dims[k]))
	}
	return strings.Join(parts, ",")
}

// compareMetricValues compares numeric values between two metrics
func (s *DashboardService) compareMetricValues(curr, prev *dbmodel.GithubWorkflowMetrics) []PerformanceChangeResponse {
	var changes []PerformanceChangeResponse

	currMetrics := curr.Metrics
	prevMetrics := prev.Metrics

	for key, currVal := range currMetrics {
		prevVal, exists := prevMetrics[key]
		if !exists {
			continue
		}

		currFloat, currOk := s.toFloat64(currVal)
		prevFloat, prevOk := s.toFloat64(prevVal)

		if !currOk || !prevOk || prevFloat == 0 {
			continue
		}

		changePercent := ((currFloat - prevFloat) / math.Abs(prevFloat)) * 100

		changes = append(changes, PerformanceChangeResponse{
			Metric:        key,
			Dimensions:    curr.Dimensions,
			CurrentValue:  currFloat,
			PreviousValue: prevFloat,
			ChangePercent: math.Round(changePercent*100) / 100, // Round to 2 decimal places
		})
	}

	return changes
}

// toFloat64 converts an interface to float64
func (s *DashboardService) toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case int32:
		return float64(val), true
	case json.Number:
		f, err := val.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// ========== Regression Analysis ==========

// analyzeRegressions analyzes regressions and finds likely commits using AI
func (s *DashboardService) analyzeRegressions(
	ctx context.Context,
	perfSummary *PerformanceInfo,
	commits []*dbmodel.GithubWorkflowCommits,
) {
	if len(commits) == 0 || !s.hasAIClient() {
		return
	}

	for i := range perfSummary.TopRegressions {
		regression := &perfSummary.TopRegressions[i]

		// Use AI analysis (calls remote Primus-Conductor API)
		aiResult := s.analyzeRegressionWithAI(ctx, regression, commits)
		if aiResult != nil && aiResult.Confidence >= aiConfidenceThreshold && aiResult.LikelyCommit != nil {
			regression.LikelyCommit = &CommitInfoResponse{
				SHA:     aiResult.LikelyCommit.SHA,
				Author:  aiResult.LikelyCommit.Author,
				Message: aiResult.LikelyCommit.Message,
			}
		}
	}
}

// analyzeRegressionWithAI calls the remote AI API for regression analysis
func (s *DashboardService) analyzeRegressionWithAI(
	ctx context.Context,
	regression *PerformanceChangeResponse,
	commits []*dbmodel.GithubWorkflowCommits,
) *aitopics.RegressionAnalysisOutput {
	if s.aiClient == nil {
		return nil
	}

	// Build input for AI API
	input := aitopics.RegressionAnalysisInput{
		Regression: aitopics.PerformanceChange{
			Metric:        regression.Metric,
			Dimensions:    regression.Dimensions,
			CurrentValue:  regression.CurrentValue,
			PreviousValue: regression.PreviousValue,
			ChangePercent: regression.ChangePercent,
		},
		Commits: make([]aitopics.CommitInfo, 0, len(commits)),
	}

	for _, c := range commits {
		var files []string
		if err := c.Files.UnmarshalTo(&files); err == nil {
			input.Commits = append(input.Commits, aitopics.CommitInfo{
				SHA:          c.SHA,
				Author:       c.AuthorName,
				Message:      c.Message,
				FilesChanged: files,
				Additions:    c.Additions,
				Deletions:    c.Deletions,
			})
		}
	}

	// Call AI API
	resp, err := s.aiClient.InvokeSync(ctx, aitopics.TopicGithubDashboardRegressionAnalyze, input)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Debugf("AI regression analysis failed: %v", err)
		return nil
	}

	// Parse response
	var output aitopics.RegressionAnalysisOutput
	if len(resp.Payload) > 0 {
		if err := json.Unmarshal(resp.Payload, &output); err != nil {
			log.GlobalLogger().WithContext(ctx).Debugf("Failed to parse AI response: %v", err)
			return nil
		}
		return &output
	}

	return nil
}

// getCommitsBetweenRuns gets commits between two runs
func (s *DashboardService) getCommitsBetweenRuns(
	ctx context.Context,
	currentRun, previousRun *dbmodel.GithubWorkflowRuns,
) ([]*dbmodel.GithubWorkflowCommits, error) {
	if currentRun == nil {
		return nil, nil
	}

	commitFacade := database.GetFacadeForCluster(s.clusterName).GetGithubWorkflowCommit()
	commit, err := commitFacade.GetByRunID(ctx, currentRun.ID)
	if err != nil {
		return nil, err
	}
	if commit == nil {
		return []*dbmodel.GithubWorkflowCommits{}, nil
	}
	return []*dbmodel.GithubWorkflowCommits{commit}, nil
}

// ========== Anomaly Detection ==========

// detectAnomalies detects anomalies from performance data
func (s *DashboardService) detectAnomalies(perfSummary *PerformanceInfo) *AnomaliesInfo {
	return &AnomaliesInfo{
		RegressionAlerts: perfSummary.RegressionCount,
		NewMetrics:       perfSummary.NewMetricCount,
		FlakyTests:       0, // Not implemented yet
	}
}

// ========== Build Info ==========

// buildBuildInfo creates BuildInfo from a run
func (s *DashboardService) buildBuildInfo(run *dbmodel.GithubWorkflowRuns) *BuildInfo {
	if run == nil {
		return &BuildInfo{Status: "no_data"}
	}

	info := &BuildInfo{
		CurrentRunID:    &run.ID,
		Status:          run.Status,
		GithubRunNumber: run.GithubRunNumber,
		HeadSHA:         run.HeadSha,
		HeadBranch:      run.HeadBranch,
	}

	if !run.WorkloadCompletedAt.IsZero() {
		info.CompletedAt = &run.WorkloadCompletedAt
	}

	if !run.WorkloadStartedAt.IsZero() && !run.WorkloadCompletedAt.IsZero() {
		duration := int(run.WorkloadCompletedAt.Sub(run.WorkloadStartedAt).Seconds())
		info.DurationSeconds = &duration
	}

	return info
}

// ========== Caching ==========

// cacheSummary caches the dashboard summary
func (s *DashboardService) cacheSummary(
	ctx context.Context,
	configID int64,
	summaryDate time.Time,
	response *DashboardSummaryResponse,
) error {
	facade := database.GetFacadeForCluster(s.clusterName).GetDashboardSummary()

	// Convert response to model
	summary := &dbmodel.DashboardSummaries{
		ConfigID:    configID,
		SummaryDate: time.Date(summaryDate.Year(), summaryDate.Month(), summaryDate.Day(), 0, 0, 0, 0, time.UTC),
		BuildStatus: response.Build.Status,
		GeneratedAt: response.GeneratedAt,
		IsStale:     false,
	}

	if response.Build.CurrentRunID != nil {
		summary.CurrentRunID = response.Build.CurrentRunID
	}
	if response.Build.DurationSeconds != nil {
		summary.BuildDurationSeconds = response.Build.DurationSeconds
	}

	summary.CommitCount = response.CodeChanges.CommitCount
	summary.PRCount = response.CodeChanges.PRCount
	summary.ContributorCount = response.CodeChanges.ContributorCount
	summary.TotalAdditions = response.CodeChanges.Additions
	summary.TotalDeletions = response.CodeChanges.Deletions

	summary.RegressionCount = response.Performance.RegressionCount
	summary.ImprovementCount = response.Performance.ImprovementCount
	summary.NewMetricCount = response.Performance.NewMetricCount
	summary.OverallPerfChangePercent = response.Performance.OverallChangePercent

	// Serialize JSONB fields
	if improvements, err := json.Marshal(response.Performance.TopImprovements); err == nil {
		summary.TopImprovements = improvements
	}
	if regressions, err := json.Marshal(response.Performance.TopRegressions); err == nil {
		summary.TopRegressions = regressions
	}
	if contributors, err := json.Marshal(response.Contributors); err == nil {
		summary.TopContributors = contributors
	}

	return facade.Upsert(ctx, summary)
}

// buildEmptySummary builds an empty summary when no data is available
func (s *DashboardService) buildEmptySummary(config *dbmodel.GithubWorkflowConfigs, date time.Time) *DashboardSummaryResponse {
	return &DashboardSummaryResponse{
		Config: &ConfigInfo{
			ID:          config.ID,
			Name:        config.Name,
			GithubOwner: config.GithubOwner,
			GithubRepo:  config.GithubRepo,
		},
		SummaryDate: date.Format("2006-01-02"),
		Build:       &BuildInfo{Status: "no_data"},
		CodeChanges: &CodeChangesInfo{},
		Performance: &PerformanceInfo{
			TopImprovements: []PerformanceChangeResponse{},
			TopRegressions:  []PerformanceChangeResponse{},
		},
		Contributors: []ContributorInfo{},
		Anomalies:    &AnomaliesInfo{},
		GeneratedAt:  time.Now(),
	}
}

// ========== AI Insights Generation ==========

// GeneratePerformanceInsights generates AI-powered performance insights
// Returns nil if AI is not available
func (s *DashboardService) GeneratePerformanceInsights(
	ctx context.Context,
	config *dbmodel.GithubWorkflowConfigs,
	summary *DashboardSummaryResponse,
) *aitopics.PerformanceInsightOutput {
	if !s.hasAIClient() {
		log.GlobalLogger().WithContext(ctx).Warningf("AI client not available for insights generation")
		return nil
	}

	if summary == nil {
		return nil
	}

	// Build input for AI API
	input := aitopics.PerformanceInsightInput{
		ConfigID:         config.ID,
		ConfigName:       config.Name,
		SummaryDate:      summary.SummaryDate,
		BuildStatus:      summary.Build.Status,
		PerfChange:       summary.Performance.OverallChangePercent,
		CommitCount:      summary.CodeChanges.CommitCount,
		ContributorCount: summary.CodeChanges.ContributorCount,
	}

	// Add top improvements
	for _, imp := range summary.Performance.TopImprovements {
		input.TopImprovements = append(input.TopImprovements, aitopics.PerformanceChange{
			Metric:        imp.Metric,
			Dimensions:    imp.Dimensions,
			CurrentValue:  imp.CurrentValue,
			PreviousValue: imp.PreviousValue,
			ChangePercent: imp.ChangePercent,
		})
	}

	// Add top regressions
	for _, reg := range summary.Performance.TopRegressions {
		input.TopRegressions = append(input.TopRegressions, aitopics.PerformanceChange{
			Metric:        reg.Metric,
			Dimensions:    reg.Dimensions,
			CurrentValue:  reg.CurrentValue,
			PreviousValue: reg.PreviousValue,
			ChangePercent: reg.ChangePercent,
		})
	}

	// Call AI API
	resp, err := s.aiClient.InvokeSync(ctx, aitopics.TopicGithubDashboardInsightsGenerate, input)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("AI insight generation failed: %v", err)
		return nil
	}

	// Parse response
	var output aitopics.PerformanceInsightOutput
	if len(resp.Payload) > 0 {
		if err := json.Unmarshal(resp.Payload, &output); err != nil {
			log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse AI response: %v", err)
			return nil
		}
		return &output
	}

	return nil
}

// AnalyzeCommitImpact analyzes a commit's potential performance impact using AI
// Returns nil if AI is not available
func (s *DashboardService) AnalyzeCommitImpact(
	ctx context.Context,
	configID int64,
	commit *dbmodel.GithubWorkflowCommits,
	affectedMetrics []PerformanceChangeResponse,
) *aitopics.CommitImpactOutput {
	if !s.hasAIClient() {
		log.GlobalLogger().WithContext(ctx).Warningf("AI client not available for commit impact analysis")
		return nil
	}

	if commit == nil {
		return nil
	}

	// Get files from commit
	var files []string
	if err := commit.Files.UnmarshalTo(&files); err != nil {
		files = []string{}
	}

	// Build input for AI API
	input := aitopics.CommitImpactInput{
		ConfigID: configID,
		Commit: aitopics.CommitInfo{
			SHA:          commit.SHA,
			Author:       commit.AuthorName,
			Message:      commit.Message,
			FilesChanged: files,
			Additions:    commit.Additions,
			Deletions:    commit.Deletions,
		},
	}

	for _, m := range affectedMetrics {
		input.AffectedMetrics = append(input.AffectedMetrics, aitopics.PerformanceChange{
			Metric:        m.Metric,
			Dimensions:    m.Dimensions,
			CurrentValue:  m.CurrentValue,
			PreviousValue: m.PreviousValue,
			ChangePercent: m.ChangePercent,
		})
	}

	// Call AI API
	resp, err := s.aiClient.InvokeSync(ctx, aitopics.TopicGithubDashboardCommitImpact, input)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("AI commit impact analysis failed: %v", err)
		return nil
	}

	// Parse response
	var output aitopics.CommitImpactOutput
	if len(resp.Payload) > 0 {
		if err := json.Unmarshal(resp.Payload, &output); err != nil {
			log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse AI response: %v", err)
			return nil
		}
		return &output
	}

	return nil
}

// ========== Helper Functions ==========

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
