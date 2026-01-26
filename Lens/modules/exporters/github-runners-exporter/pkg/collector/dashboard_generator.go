// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

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
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	regressionThresholdPercent  = -3.0 // Changes worse than -3% are regressions
	improvementThresholdPercent = 3.0  // Changes better than +3% are improvements
	topChangesLimit             = 5    // Max number of top improvements/regressions
	aiConfidenceThreshold       = 0.5  // Minimum confidence for AI results
)

// generateDashboardSummary generates a dashboard summary with AI-powered regression analysis
// This is called automatically after metrics extraction and GitHub data fetching
func (c *WorkflowCollector) generateDashboardSummary(
	ctx context.Context,
	config *model.GithubWorkflowConfigs,
	currentRun *model.GithubWorkflowRuns,
) error {
	log.Infof("WorkflowCollector: generating dashboard summary for run %d", currentRun.ID)

	facade := database.GetFacade()
	runFacade := facade.GetGithubWorkflowRun()

	// Get previous completed run
	previousRuns, _, err := runFacade.List(ctx, &database.GithubWorkflowRunFilter{
		ConfigID: config.ID,
		Status:   database.WorkflowRunStatusCompleted,
		Limit:    10,
	})
	if err != nil {
		return fmt.Errorf("failed to get previous runs: %w", err)
	}

	var previousRun *model.GithubWorkflowRuns
	for _, run := range previousRuns {
		if run.ID != currentRun.ID && run.WorkloadCompletedAt.Before(currentRun.WorkloadCompletedAt) {
			previousRun = run
			break
		}
	}

	// Calculate performance changes and code changes
	perfSummary, codeChanges, err := c.analyzeBuildComparison(ctx, currentRun, previousRun)
	if err != nil {
		return fmt.Errorf("failed to analyze build comparison: %w", err)
	}

	// Run AI regression analysis if there are regressions
	if len(perfSummary.TopRegressions) > 0 && previousRun != nil {
		c.analyzeRegressionsWithAI(ctx, config, currentRun, previousRun, perfSummary)
	}

	// Save dashboard summary to database
	if err := c.saveDashboardSummary(ctx, config, currentRun, perfSummary, codeChanges); err != nil {
		return fmt.Errorf("failed to save dashboard summary: %w", err)
	}

	log.Infof("WorkflowCollector: dashboard summary generated for run %d", currentRun.ID)
	return nil
}

// analyzeBuildComparison compares current and previous builds
func (c *WorkflowCollector) analyzeBuildComparison(
	ctx context.Context,
	currentRun, previousRun *model.GithubWorkflowRuns,
) (*performanceSummary, *codeChangesSummary, error) {
	facade := database.GetFacade()
	metricsFacade := facade.GetGithubWorkflowMetrics()
	commitFacade := facade.GetGithubWorkflowCommit()

	// Get current metrics
	currentMetrics, err := metricsFacade.ListByRun(ctx, currentRun.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get current metrics: %w", err)
	}

	perfSummary := &performanceSummary{}

	// Get code changes
	commits, err := commitFacade.ListByRunID(ctx, currentRun.ID)
	if err != nil {
		log.Warnf("Failed to get commits for run %d: %v", currentRun.ID, err)
		commits = []*model.GithubWorkflowCommits{}
	}

	codeChanges := &codeChangesSummary{
		CommitCount:      len(commits),
		ContributorCount: countUniqueContributors(commits),
	}
	for _, c := range commits {
		codeChanges.Additions += int(c.Additions)
		codeChanges.Deletions += int(c.Deletions)
	}

	// If no previous run, all metrics are new
	if previousRun == nil {
		perfSummary.NewMetricCount = len(currentMetrics)
		return perfSummary, codeChanges, nil
	}

	// Get previous metrics
	previousMetrics, err := metricsFacade.ListByRun(ctx, previousRun.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get previous metrics: %w", err)
	}

	// Compare metrics
	perfSummary = compareMetrics(currentMetrics, previousMetrics)

	return perfSummary, codeChanges, nil
}

// analyzeRegressionsWithAI uses AI to analyze regressions and find likely causes
func (c *WorkflowCollector) analyzeRegressionsWithAI(
	ctx context.Context,
	config *model.GithubWorkflowConfigs,
	currentRun, previousRun *model.GithubWorkflowRuns,
	perfSummary *performanceSummary,
) {
	aiClient := aiclient.GetGlobalClient()
	if aiClient == nil {
		log.Debug("AI client not available, skipping regression analysis")
		return
	}

	commitFacade := database.GetFacade().GetGithubWorkflowCommit()
	commits, err := commitFacade.ListByRunID(ctx, currentRun.ID)
	if err != nil || len(commits) == 0 {
		log.Debugf("No commits found for regression analysis: %v", err)
		return
	}

	// Analyze each regression
	for i := range perfSummary.TopRegressions {
		regression := &perfSummary.TopRegressions[i]

		// Build AI input
		input := aitopics.RegressionAnalysisInput{
			ConfigID:   config.ID,
			ConfigName: config.Name,
			Regression: aitopics.PerformanceChange{
				Metric:        regression.Metric,
				Dimensions:    regression.Dimensions,
				CurrentValue:  regression.CurrentValue,
				PreviousValue: regression.PreviousValue,
				ChangePercent: regression.ChangePercent,
			},
			Commits: make([]aitopics.CommitInfo, 0, len(commits)),
		}

		for _, commit := range commits {
			var files []string
			if commit.Files != nil {
				if filesData, ok := commit.Files["files"].([]interface{}); ok {
					for _, f := range filesData {
						if s, ok := f.(string); ok {
							files = append(files, s)
						}
					}
				}
			}
			input.Commits = append(input.Commits, aitopics.CommitInfo{
				SHA:          commit.Sha,
				Author:       commit.AuthorName,
				Message:      commit.Message,
				FilesChanged: files,
				Additions:    int(commit.Additions),
				Deletions:    int(commit.Deletions),
			})
		}

		// Call AI API
		resp, err := aiClient.InvokeSync(ctx, aitopics.TopicGithubDashboardRegressionAnalyze, input)
		if err != nil {
			log.Debugf("AI regression analysis failed for metric %s: %v", regression.Metric, err)
			continue
		}

		// Parse response
		var output aitopics.RegressionAnalysisOutput
		if err := resp.UnmarshalPayload(&output); err == nil {
			if output.Confidence >= aiConfidenceThreshold && output.LikelyCommit != nil {
				regression.LikelyCommitSHA = output.LikelyCommit.SHA
				regression.LikelyCommitAuthor = output.LikelyCommit.Author
				regression.LikelyCommitMessage = output.LikelyCommit.Message
				regression.AIConfidence = output.Confidence
				regression.AIReasoning = output.Reasoning
			}
		}
	}
}

// saveDashboardSummary saves the dashboard summary to database
func (c *WorkflowCollector) saveDashboardSummary(
	ctx context.Context,
	config *model.GithubWorkflowConfigs,
	run *model.GithubWorkflowRuns,
	perfSummary *performanceSummary,
	codeChanges *codeChangesSummary,
) error {
	facade := database.GetFacade().GetDashboardSummary()

	summaryDate := time.Date(run.WorkloadCompletedAt.Year(), run.WorkloadCompletedAt.Month(),
		run.WorkloadCompletedAt.Day(), 0, 0, 0, 0, time.UTC)

	var overallChange float64
	if perfSummary.OverallChangePercent != nil {
		overallChange = *perfSummary.OverallChangePercent
	}

	summary := &model.DashboardSummaries{
		ConfigID:         config.ID,
		Date:             summaryDate,
		BuildStatus:      run.Status,
		CommitCount:      int32(codeChanges.CommitCount),
		ContributorCount: int32(codeChanges.ContributorCount),
		PerfChange:       overallChange,
	}

	// Serialize JSONB fields
	if improvements, err := json.Marshal(perfSummary.TopImprovements); err == nil {
		summary.TopImprovements = improvements
	}
	if regressions, err := json.Marshal(perfSummary.TopRegressions); err == nil {
		summary.TopRegressions = regressions
	}

	return facade.Upsert(ctx, summary)
}

// ========== Helper Types ==========

type performanceSummary struct {
	OverallChangePercent *float64
	RegressionCount      int
	ImprovementCount     int
	NewMetricCount       int
	TopImprovements      []performanceChange
	TopRegressions       []performanceChange
}

type performanceChange struct {
	Metric              string                 `json:"metric"`
	Dimensions          map[string]interface{} `json:"dimensions,omitempty"`
	CurrentValue        float64                `json:"current_value"`
	PreviousValue       float64                `json:"previous_value"`
	ChangePercent       float64                `json:"change_percent"`
	LikelyCommitSHA     string                 `json:"likely_commit_sha,omitempty"`
	LikelyCommitAuthor  string                 `json:"likely_commit_author,omitempty"`
	LikelyCommitMessage string                 `json:"likely_commit_message,omitempty"`
	AIConfidence        float64                `json:"ai_confidence,omitempty"`
	AIReasoning         string                 `json:"ai_reasoning,omitempty"`
}

type codeChangesSummary struct {
	CommitCount      int
	ContributorCount int
	Additions        int
	Deletions        int
}

// ========== Helper Functions ==========

func compareMetrics(current, previous []*model.GithubWorkflowMetrics) *performanceSummary {
	// Build previous metrics map by dimension key
	prevMap := make(map[string]*model.GithubWorkflowMetrics)
	for _, m := range previous {
		key := buildDimensionKey(m)
		prevMap[key] = m
	}

	var improvements, regressions []performanceChange
	var totalChangePercent float64
	var comparedCount int

	for _, curr := range current {
		dimKey := buildDimensionKey(curr)
		prev, exists := prevMap[dimKey]

		if !exists {
			continue
		}

		// Compare numeric metric values
		changes := compareMetricValues(curr, prev)
		for _, change := range changes {
			comparedCount++
			totalChangePercent += change.ChangePercent

			if change.ChangePercent <= regressionThresholdPercent {
				regressions = append(regressions, change)
			} else if change.ChangePercent >= improvementThresholdPercent {
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

	return &performanceSummary{
		OverallChangePercent: overallChange,
		RegressionCount:      len(regressions),
		ImprovementCount:     len(improvements),
		NewMetricCount:       len(current) - comparedCount,
		TopImprovements:      improvements,
		TopRegressions:       regressions,
	}
}

func buildDimensionKey(m *model.GithubWorkflowMetrics) string {
	dims := m.Dimensions
	if dims == nil {
		return ""
	}

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

func compareMetricValues(curr, prev *model.GithubWorkflowMetrics) []performanceChange {
	var changes []performanceChange

	currMetrics := curr.Metrics
	prevMetrics := prev.Metrics

	for key, currVal := range currMetrics {
		prevVal, exists := prevMetrics[key]
		if !exists {
			continue
		}

		currFloat, currOk := toFloat64(currVal)
		prevFloat, prevOk := toFloat64(prevVal)

		if !currOk || !prevOk || prevFloat == 0 {
			continue
		}

		changePercent := ((currFloat - prevFloat) / math.Abs(prevFloat)) * 100

		changes = append(changes, performanceChange{
			Metric:        key,
			Dimensions:    curr.Dimensions,
			CurrentValue:  currFloat,
			PreviousValue: prevFloat,
			ChangePercent: math.Round(changePercent*100) / 100,
		})
	}

	return changes
}

func toFloat64(v interface{}) (float64, bool) {
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

func countUniqueContributors(commits []*model.GithubWorkflowCommits) int {
	contributors := make(map[string]bool)
	for _, c := range commits {
		key := c.AuthorEmail
		if key == "" {
			key = c.AuthorName
		}
		contributors[key] = true
	}
	return len(contributors)
}
