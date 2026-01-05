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
		TopicGithubMetricsExtract:
		return true
	default:
		return false
	}
}

