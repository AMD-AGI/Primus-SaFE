// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package aitopics

// Topic constants for AI Agent invocation
// Format: {domain}.{agent}.{action}

const (
	// Alert Advisor Topics
	TopicAlertAdvisorAggregateWorkloads  = "alert.advisor.aggregate-workloads"
	TopicAlertAdvisorGenerateSuggestions = "alert.advisor.generate-suggestions"
	TopicAlertAdvisorAnalyzeCoverage     = "alert.advisor.analyze-coverage"

	// Alert Handler Topics
	TopicAlertHandlerAnalyze       = "alert.handler.analyze"
	TopicAlertHandlerCorrelate     = "alert.handler.correlate"
	TopicAlertHandlerExecuteAction = "alert.handler.execute-action"

	// Report Topics
	TopicReportGenerateSummary  = "report.generate-summary"
	TopicReportGenerateInsights = "report.generate-insights"

	// Scan Topics
	TopicScanIdentifyComponent = "scan.identify-component"
	TopicScanSuggestGrouping   = "scan.suggest-grouping"

	// GitHub Metrics Topics
	TopicGithubMetricsExtract = "github.metrics.extract"
	TopicGithubSchemaAnalyze  = "github.schema.analyze"

	// GitHub Dashboard Analytics Topics
	TopicGithubDashboardRegressionAnalyze = "github.dashboard.regression.analyze"
	TopicGithubDashboardCommitImpact      = "github.dashboard.commit.impact"
	TopicGithubDashboardInsightsGenerate  = "github.dashboard.insights.generate"
)

// API Version
const CurrentVersion = "v1"

// TopicDomains defines the valid topic domains
var TopicDomains = []string{
	"alert",
	"report",
	"scan",
	"github",
}

// IsValidTopic checks if a topic string is a known topic
func IsValidTopic(topic string) bool {
	switch topic {
	case TopicAlertAdvisorAggregateWorkloads,
		TopicAlertAdvisorGenerateSuggestions,
		TopicAlertAdvisorAnalyzeCoverage,
		TopicAlertHandlerAnalyze,
		TopicAlertHandlerCorrelate,
		TopicAlertHandlerExecuteAction,
		TopicReportGenerateSummary,
		TopicReportGenerateInsights,
		TopicScanIdentifyComponent,
		TopicScanSuggestGrouping,
		TopicGithubMetricsExtract,
		TopicGithubSchemaAnalyze,
		TopicGithubDashboardRegressionAnalyze,
		TopicGithubDashboardCommitImpact,
		TopicGithubDashboardInsightsGenerate:
		return true
	default:
		return false
	}
}

