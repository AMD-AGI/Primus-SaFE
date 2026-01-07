package aitopics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTopicRegistry(t *testing.T) {
	assert.NotNil(t, TopicRegistry)

	// Should contain all defined topics
	expectedTopics := []string{
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

	for _, topic := range expectedTopics {
		t.Run(topic, func(t *testing.T) {
			def, ok := TopicRegistry[topic]
			assert.True(t, ok, "Topic %s should be in registry", topic)
			assert.Equal(t, topic, def.Name)
			assert.Equal(t, CurrentVersion, def.Version)
			assert.NotEmpty(t, def.Description)
			assert.True(t, def.Timeout > 0)
		})
	}
}

func TestGetTopicDefinition(t *testing.T) {
	t.Run("existing topic", func(t *testing.T) {
		def, ok := GetTopicDefinition(TopicAlertAdvisorAggregateWorkloads)
		assert.True(t, ok)
		assert.Equal(t, TopicAlertAdvisorAggregateWorkloads, def.Name)
		assert.Equal(t, CurrentVersion, def.Version)
		assert.Contains(t, def.Description, "Aggregate")
		assert.Equal(t, 30*time.Second, def.Timeout)
		assert.False(t, def.Async)
	})

	t.Run("non-existing topic", func(t *testing.T) {
		_, ok := GetTopicDefinition("unknown.topic")
		assert.False(t, ok)
	})
}

func TestGetTopicTimeout(t *testing.T) {
	tests := []struct {
		topic   string
		want    time.Duration
	}{
		{TopicAlertAdvisorAggregateWorkloads, 30 * time.Second},
		{TopicAlertAdvisorGenerateSuggestions, 60 * time.Second},
		{TopicAlertAdvisorAnalyzeCoverage, 60 * time.Second},
		{TopicAlertHandlerAnalyze, 120 * time.Second},
		{TopicAlertHandlerCorrelate, 60 * time.Second},
		{TopicAlertHandlerExecuteAction, 30 * time.Second},
		{TopicReportGenerateSummary, 120 * time.Second},
		{TopicReportGenerateInsights, 180 * time.Second},
		{TopicScanIdentifyComponent, 10 * time.Second},
		{TopicScanSuggestGrouping, 30 * time.Second},
		{"unknown.topic", 30 * time.Second}, // Default
	}

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			timeout := GetTopicTimeout(tt.topic)
			assert.Equal(t, tt.want, timeout)
		})
	}
}

func TestIsAsyncTopic(t *testing.T) {
	tests := []struct {
		topic string
		want  bool
	}{
		{TopicAlertAdvisorAggregateWorkloads, false},
		{TopicAlertAdvisorGenerateSuggestions, true},
		{TopicAlertAdvisorAnalyzeCoverage, true},
		{TopicAlertHandlerAnalyze, true},
		{TopicAlertHandlerCorrelate, true},
		{TopicAlertHandlerExecuteAction, false},
		{TopicReportGenerateSummary, true},
		{TopicReportGenerateInsights, true},
		{TopicScanIdentifyComponent, false},
		{TopicScanSuggestGrouping, false},
		{"unknown.topic", false}, // Default
	}

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			isAsync := IsAsyncTopic(tt.topic)
			assert.Equal(t, tt.want, isAsync)
		})
	}
}

func TestListTopics(t *testing.T) {
	topics := ListTopics()
	assert.NotEmpty(t, topics)
	assert.Equal(t, len(TopicRegistry), len(topics))

	// Should contain all known topics
	topicSet := make(map[string]bool)
	for _, topic := range topics {
		topicSet[topic] = true
	}

	assert.True(t, topicSet[TopicAlertAdvisorAggregateWorkloads])
	assert.True(t, topicSet[TopicAlertAdvisorGenerateSuggestions])
	assert.True(t, topicSet[TopicAlertHandlerAnalyze])
	assert.True(t, topicSet[TopicScanIdentifyComponent])
}

func TestTopicDefinition_AlertAdvisorAggregateWorkloads(t *testing.T) {
	def := TopicRegistry[TopicAlertAdvisorAggregateWorkloads]

	assert.Equal(t, TopicAlertAdvisorAggregateWorkloads, def.Name)
	assert.Equal(t, CurrentVersion, def.Version)
	assert.NotEmpty(t, def.Description)
	assert.NotNil(t, def.InputType)
	assert.NotNil(t, def.OutputType)
	assert.Equal(t, 30*time.Second, def.Timeout)
	assert.False(t, def.Async)
}

func TestTopicDefinition_AlertAdvisorGenerateSuggestions(t *testing.T) {
	def := TopicRegistry[TopicAlertAdvisorGenerateSuggestions]

	assert.Equal(t, TopicAlertAdvisorGenerateSuggestions, def.Name)
	assert.Equal(t, 60*time.Second, def.Timeout)
	assert.True(t, def.Async)
}

func TestTopicDefinition_AlertHandlerAnalyze(t *testing.T) {
	def := TopicRegistry[TopicAlertHandlerAnalyze]

	assert.Equal(t, TopicAlertHandlerAnalyze, def.Name)
	assert.Equal(t, 120*time.Second, def.Timeout)
	assert.True(t, def.Async)
}

func TestTopicDefinition_ScanIdentifyComponent(t *testing.T) {
	def := TopicRegistry[TopicScanIdentifyComponent]

	assert.Equal(t, TopicScanIdentifyComponent, def.Name)
	assert.Equal(t, 10*time.Second, def.Timeout)
	assert.False(t, def.Async)
}

func TestTopicDefinition_Fields(t *testing.T) {
	// Verify all required fields are set for each topic
	for name, def := range TopicRegistry {
		t.Run(name, func(t *testing.T) {
			assert.NotEmpty(t, def.Name)
			assert.NotEmpty(t, def.Version)
			assert.NotEmpty(t, def.Description)
			assert.True(t, def.Timeout > 0)
		})
	}
}

