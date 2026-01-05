package aitopics

import (
	"reflect"
	"time"
)

// TopicDefinition contains metadata about a topic
type TopicDefinition struct {
	Name        string        // Topic name
	Version     string        // API version
	Description string        // Human-readable description
	InputType   reflect.Type  // Input payload type
	OutputType  reflect.Type  // Output payload type
	Timeout     time.Duration // Suggested timeout
	Async       bool          // Whether this topic typically runs async
}

// TopicRegistry contains all registered topics with their metadata
var TopicRegistry = map[string]TopicDefinition{
	// Alert Advisor topics
	TopicAlertAdvisorAggregateWorkloads: {
		Name:        TopicAlertAdvisorAggregateWorkloads,
		Version:     CurrentVersion,
		Description: "Aggregate workloads into logical component groups using AI",
		InputType:   reflect.TypeOf(AggregateWorkloadsInput{}),
		OutputType:  reflect.TypeOf(AggregateWorkloadsOutput{}),
		Timeout:     30 * time.Second,
		Async:       false,
	},
	TopicAlertAdvisorGenerateSuggestions: {
		Name:        TopicAlertAdvisorGenerateSuggestions,
		Version:     CurrentVersion,
		Description: "Generate alert rule suggestions for a component",
		InputType:   reflect.TypeOf(GenerateSuggestionsInput{}),
		OutputType:  reflect.TypeOf(GenerateSuggestionsOutput{}),
		Timeout:     60 * time.Second,
		Async:       true,
	},
	TopicAlertAdvisorAnalyzeCoverage: {
		Name:        TopicAlertAdvisorAnalyzeCoverage,
		Version:     CurrentVersion,
		Description: "Analyze alert coverage gaps across components",
		InputType:   reflect.TypeOf(AnalyzeCoverageInput{}),
		OutputType:  reflect.TypeOf(AnalyzeCoverageOutput{}),
		Timeout:     60 * time.Second,
		Async:       true,
	},

	// Alert Handler topics
	TopicAlertHandlerAnalyze: {
		Name:        TopicAlertHandlerAnalyze,
		Version:     CurrentVersion,
		Description: "Analyze firing alert and provide root cause analysis",
		InputType:   reflect.TypeOf(AnalyzeAlertInput{}),
		OutputType:  reflect.TypeOf(AnalyzeAlertOutput{}),
		Timeout:     120 * time.Second,
		Async:       true,
	},
	TopicAlertHandlerCorrelate: {
		Name:        TopicAlertHandlerCorrelate,
		Version:     CurrentVersion,
		Description: "Correlate multiple alerts to find common cause",
		InputType:   reflect.TypeOf(CorrelateAlertsInput{}),
		OutputType:  reflect.TypeOf(CorrelateAlertsOutput{}),
		Timeout:     60 * time.Second,
		Async:       true,
	},
	TopicAlertHandlerExecuteAction: {
		Name:        TopicAlertHandlerExecuteAction,
		Version:     CurrentVersion,
		Description: "Execute a remediation action",
		InputType:   reflect.TypeOf(ExecuteActionInput{}),
		OutputType:  reflect.TypeOf(ExecuteActionOutput{}),
		Timeout:     30 * time.Second,
		Async:       false,
	},

	// Report topics
	TopicReportGenerateSummary: {
		Name:        TopicReportGenerateSummary,
		Version:     CurrentVersion,
		Description: "Generate a summary report",
		InputType:   nil, // Define as needed
		OutputType:  nil,
		Timeout:     120 * time.Second,
		Async:       true,
	},
	TopicReportGenerateInsights: {
		Name:        TopicReportGenerateInsights,
		Version:     CurrentVersion,
		Description: "Generate insights from historical data",
		InputType:   nil,
		OutputType:  nil,
		Timeout:     180 * time.Second,
		Async:       true,
	},

	// Scan topics
	TopicScanIdentifyComponent: {
		Name:        TopicScanIdentifyComponent,
		Version:     CurrentVersion,
		Description: "Identify component type for unknown workloads",
		InputType:   reflect.TypeOf(IdentifyComponentInput{}),
		OutputType:  reflect.TypeOf(IdentifyComponentOutput{}),
		Timeout:     10 * time.Second,
		Async:       false,
	},
	TopicScanSuggestGrouping: {
		Name:        TopicScanSuggestGrouping,
		Version:     CurrentVersion,
		Description: "Suggest grouping for ungrouped workloads",
		InputType:   reflect.TypeOf(SuggestGroupingInput{}),
		OutputType:  reflect.TypeOf(SuggestGroupingOutput{}),
		Timeout:     30 * time.Second,
		Async:       false,
	},
}

// GetTopicDefinition returns the definition for a topic
func GetTopicDefinition(topic string) (TopicDefinition, bool) {
	def, ok := TopicRegistry[topic]
	return def, ok
}

// GetTopicTimeout returns the suggested timeout for a topic
func GetTopicTimeout(topic string) time.Duration {
	if def, ok := TopicRegistry[topic]; ok {
		return def.Timeout
	}
	return 30 * time.Second // Default timeout
}

// IsAsyncTopic returns whether a topic should run asynchronously
func IsAsyncTopic(topic string) bool {
	if def, ok := TopicRegistry[topic]; ok {
		return def.Async
	}
	return false
}

// ListTopics returns all registered topic names
func ListTopics() []string {
	topics := make([]string, 0, len(TopicRegistry))
	for topic := range TopicRegistry {
		topics = append(topics, topic)
	}
	return topics
}
