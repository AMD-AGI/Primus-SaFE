package aitopics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTopicConstants(t *testing.T) {
	// Alert Advisor Topics
	assert.Equal(t, "alert.advisor.aggregate-workloads", TopicAlertAdvisorAggregateWorkloads)
	assert.Equal(t, "alert.advisor.generate-suggestions", TopicAlertAdvisorGenerateSuggestions)
	assert.Equal(t, "alert.advisor.analyze-coverage", TopicAlertAdvisorAnalyzeCoverage)

	// Alert Handler Topics
	assert.Equal(t, "alert.handler.analyze", TopicAlertHandlerAnalyze)
	assert.Equal(t, "alert.handler.correlate", TopicAlertHandlerCorrelate)
	assert.Equal(t, "alert.handler.execute-action", TopicAlertHandlerExecuteAction)

	// Report Topics
	assert.Equal(t, "report.generate-summary", TopicReportGenerateSummary)
	assert.Equal(t, "report.generate-insights", TopicReportGenerateInsights)

	// Scan Topics
	assert.Equal(t, "scan.identify-component", TopicScanIdentifyComponent)
	assert.Equal(t, "scan.suggest-grouping", TopicScanSuggestGrouping)
}

func TestCurrentVersion(t *testing.T) {
	assert.Equal(t, "v1", CurrentVersion)
}

func TestTopicDomains(t *testing.T) {
	assert.Len(t, TopicDomains, 3)
	assert.Contains(t, TopicDomains, "alert")
	assert.Contains(t, TopicDomains, "report")
	assert.Contains(t, TopicDomains, "scan")
}

func TestIsValidTopic(t *testing.T) {
	validTopics := []string{
		TopicAlertAdvisorAggregateWorkloads,
		TopicAlertAdvisorGenerateSuggestions,
		TopicAlertAdvisorAnalyzeCoverage,
		TopicAlertHandlerAnalyze,
		TopicAlertHandlerCorrelate,
		TopicAlertHandlerExecuteAction,
		TopicReportGenerateSummary,
		TopicReportGenerateInsights,
		TopicScanIdentifyComponent,
		TopicScanSuggestGrouping,
	}

	for _, topic := range validTopics {
		t.Run(topic, func(t *testing.T) {
			assert.True(t, IsValidTopic(topic))
		})
	}
}

func TestIsValidTopic_Invalid(t *testing.T) {
	invalidTopics := []string{
		"unknown.topic",
		"alert.advisor",
		"alert",
		"",
		"alert.advisor.unknown",
		"some.random.topic",
	}

	for _, topic := range invalidTopics {
		t.Run(topic, func(t *testing.T) {
			assert.False(t, IsValidTopic(topic))
		})
	}
}
