package aitopics

import "time"

// ========== alert.handler.analyze ==========

// AnalyzeAlertInput is the input payload for alert analysis
type AnalyzeAlertInput struct {
	Alert         AlertInfo       `json:"alert"`
	RelatedAlerts []AlertInfo     `json:"related_alerts,omitempty"`
	Options       *AnalyzeOptions `json:"options,omitempty"`
}

// AlertInfo represents an alert to analyze
type AlertInfo struct {
	AlertID     string            `json:"alert_id"`
	Name        string            `json:"name"`
	Status      string            `json:"status"` // firing, resolved
	Severity    string            `json:"severity"`
	StartedAt   time.Time         `json:"started_at"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Value       interface{}       `json:"value,omitempty"`
}

// AnalyzeOptions contains optional settings for alert analysis
type AnalyzeOptions struct {
	FetchLogs    bool   `json:"fetch_logs,omitempty"`
	FetchMetrics bool   `json:"fetch_metrics,omitempty"`
	TimeRange    string `json:"time_range,omitempty"` // e.g., "1h", "30m"
}

// AnalyzeAlertOutput is the output payload for alert analysis
type AnalyzeAlertOutput struct {
	Analysis         AlertAnalysis   `json:"analysis"`
	RelatedKnowledge []KnowledgeItem `json:"related_knowledge,omitempty"`
}

// AlertAnalysis contains the analysis results
type AlertAnalysis struct {
	RootCause       *RootCauseAnalysis `json:"root_cause,omitempty"`
	Impact          *ImpactAssessment  `json:"impact,omitempty"`
	Recommendations []Recommendation   `json:"recommendations,omitempty"`
}

// RootCauseAnalysis contains root cause information
type RootCauseAnalysis struct {
	Summary    string   `json:"summary"`
	Confidence float64  `json:"confidence"`
	Evidence   []string `json:"evidence,omitempty"`
}

// ImpactAssessment contains impact information
type ImpactAssessment struct {
	AffectedServices   []string `json:"affected_services,omitempty"`
	SeverityAssessment string   `json:"severity_assessment"`
}

// Recommendation represents a recommended action
type Recommendation struct {
	Action         string `json:"action"`
	Command        string `json:"command,omitempty"`
	Priority       int    `json:"priority"`
	AutoExecutable bool   `json:"auto_executable"`
}

// KnowledgeItem represents related documentation or resources
type KnowledgeItem struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// ========== alert.handler.correlate ==========

// CorrelateAlertsInput is the input payload for alert correlation
type CorrelateAlertsInput struct {
	Alerts  []AlertInfo       `json:"alerts"`
	Options *CorrelateOptions `json:"options,omitempty"`
}

// CorrelateOptions contains optional settings for correlation
type CorrelateOptions struct {
	TimeWindow    string  `json:"time_window,omitempty"` // e.g., "5m"
	MinConfidence float64 `json:"min_confidence,omitempty"`
}

// CorrelateAlertsOutput is the output payload for alert correlation
type CorrelateAlertsOutput struct {
	CorrelationGroups []CorrelationGroup `json:"correlation_groups"`
	Uncorrelated      []string           `json:"uncorrelated,omitempty"`
}

// CorrelationGroup represents a group of correlated alerts
type CorrelationGroup struct {
	GroupID        string   `json:"group_id"`
	PrimaryAlertID string   `json:"primary_alert_id"`
	RelatedAlerts  []string `json:"related_alerts"`
	CommonCause    string   `json:"common_cause,omitempty"`
	Confidence     float64  `json:"confidence"`
}

// ========== alert.handler.execute-action ==========

// ExecuteActionInput is the input payload for action execution
type ExecuteActionInput struct {
	ActionType string            `json:"action_type"`
	Target     ActionTarget      `json:"target"`
	Parameters map[string]string `json:"parameters,omitempty"`
	DryRun     bool              `json:"dry_run,omitempty"`
}

// ActionTarget represents the target of an action
type ActionTarget struct {
	Kind      string `json:"kind"` // Pod, Deployment, StatefulSet, etc.
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// ExecuteActionOutput is the output payload for action execution
type ExecuteActionOutput struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	ActionID   string `json:"action_id,omitempty"`
	ExecutedAt string `json:"executed_at,omitempty"`
}
