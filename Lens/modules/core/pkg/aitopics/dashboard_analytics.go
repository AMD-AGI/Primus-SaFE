// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package aitopics

// ========== Input Types ==========

// CommitInfo represents information about a commit
type CommitInfo struct {
	SHA          string   `json:"sha"`
	Author       string   `json:"author"`
	Message      string   `json:"message"`
	FilesChanged []string `json:"files_changed,omitempty"`
	Additions    int      `json:"additions,omitempty"`
	Deletions    int      `json:"deletions,omitempty"`
}

// PerformanceChange represents a performance change entry
type PerformanceChange struct {
	Metric        string                 `json:"metric"`
	Dimensions    map[string]interface{} `json:"dimensions,omitempty"`
	CurrentValue  float64                `json:"current_value"`
	PreviousValue float64                `json:"previous_value"`
	ChangePercent float64                `json:"change_percent"`
	Unit          string                 `json:"unit,omitempty"`
}

// HistoricalDataPoint represents a historical data point
type HistoricalDataPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// RegressionAnalysisInput is the input for regression analysis
type RegressionAnalysisInput struct {
	ConfigID        int64                 `json:"config_id"`
	ConfigName      string                `json:"config_name,omitempty"`
	Regression      PerformanceChange     `json:"regression"`
	Commits         []CommitInfo          `json:"commits"`
	HistoricalTrend []HistoricalDataPoint `json:"historical_trend,omitempty"`
}

// CommitImpactInput is the input for commit impact analysis
type CommitImpactInput struct {
	ConfigID        int64               `json:"config_id"`
	Commit          CommitInfo          `json:"commit"`
	AffectedMetrics []PerformanceChange `json:"affected_metrics,omitempty"`
	CodeDiff        string              `json:"code_diff,omitempty"`
}

// PerformanceInsightInput is the input for performance insight generation
type PerformanceInsightInput struct {
	ConfigID         int64               `json:"config_id"`
	ConfigName       string              `json:"config_name"`
	SummaryDate      string              `json:"summary_date"`
	BuildStatus      string              `json:"build_status"`
	PerfChange       *float64            `json:"perf_change,omitempty"`
	TopImprovements  []PerformanceChange `json:"top_improvements,omitempty"`
	TopRegressions   []PerformanceChange `json:"top_regressions,omitempty"`
	CommitCount      int                 `json:"commit_count,omitempty"`
	ContributorCount int                 `json:"contributor_count,omitempty"`
}

// ========== Output Types ==========

// LikelyCommit represents a likely commit that caused a regression
type LikelyCommit struct {
	SHA     string `json:"sha"`
	Author  string `json:"author"`
	Message string `json:"message"`
}

// RegressionAnalysisOutput is the output from regression analysis
type RegressionAnalysisOutput struct {
	LikelyCommit     *LikelyCommit `json:"likely_commit,omitempty"`
	Confidence       float64       `json:"confidence"`
	Reasoning        string        `json:"reasoning,omitempty"`
	SuggestedActions []string      `json:"suggested_actions,omitempty"`
	RelatedMetrics   []string      `json:"related_metrics,omitempty"`
	IsAIGenerated    bool          `json:"is_ai_generated"`
}

// CommitImpactOutput is the output from commit impact analysis
type CommitImpactOutput struct {
	ImpactScore      float64  `json:"impact_score"`
	ImpactCategories []string `json:"impact_categories,omitempty"`
	RiskLevel        string   `json:"risk_level"`
	Explanation      string   `json:"explanation,omitempty"`
	Recommendations  []string `json:"recommendations,omitempty"`
	IsAIGenerated    bool     `json:"is_ai_generated"`
}

// PerformanceInsightOutput is the output from performance insight generation
type PerformanceInsightOutput struct {
	Summary         string   `json:"summary"`
	Highlights      []string `json:"highlights,omitempty"`
	Concerns        []string `json:"concerns,omitempty"`
	Recommendations []string `json:"recommendations,omitempty"`
	TrendAnalysis   string   `json:"trend_analysis,omitempty"`
	IsAIGenerated   bool     `json:"is_ai_generated"`
}
