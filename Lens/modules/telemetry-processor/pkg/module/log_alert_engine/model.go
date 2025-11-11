package log_alert_engine

import (
	"time"
)

// LabelSelector defines a selector for matching logs based on labels
type LabelSelector struct {
	Type     string   `json:"type"`      // workload/namespace/pod/node/cluster/custom
	Key      string   `json:"key"`       // Label key
	Operator string   `json:"operator"`  // eq/ne/in/notin/exists/notexists/regex
	Values   []string `json:"values"`    // Label values
}

// MatchConfig defines the matching configuration for a rule
type MatchConfig struct {
	// Pattern matching
	Pattern    string `json:"pattern,omitempty"`     // Regex pattern
	IgnoreCase bool   `json:"ignore_case,omitempty"` // Case insensitive matching
	
	// Threshold matching
	Threshold *ThresholdConfig `json:"threshold,omitempty"`
	
	// Composite matching
	CompositeRules []CompositeRule `json:"composite_rules,omitempty"`
}

// ThresholdConfig defines threshold-based matching
type ThresholdConfig struct {
	CountThreshold int      `json:"count_threshold"` // Number of occurrences
	TimeWindow     int      `json:"time_window"`     // Time window in seconds
	AggregateBy    []string `json:"aggregate_by"`    // Aggregation dimensions
}

// CompositeRule defines a composite matching rule
type CompositeRule struct {
	Condition string      `json:"condition"` // AND/OR
	Rules     []RuleEntry `json:"rules"`
}

// RuleEntry represents a single rule in composite matching
type RuleEntry struct {
	Type     string      `json:"type"`     // pattern/threshold/field
	Pattern  string      `json:"pattern,omitempty"`
	Field    string      `json:"field,omitempty"`
	Operator string      `json:"operator,omitempty"` // lt/gt/eq/ne
	Value    interface{} `json:"value,omitempty"`
	TimeWindow int       `json:"time_window,omitempty"` // seconds
}

// AlertTemplate defines the template for generating alerts
type AlertTemplate struct {
	Summary     string            `json:"summary"`               // Summary template
	Description string            `json:"description"`           // Description template
	Labels      map[string]string `json:"labels,omitempty"`      // Additional labels
	Annotations map[string]string `json:"annotations,omitempty"` // Additional annotations
}

// RouteConfig defines routing configuration for alerts
type RouteConfig struct {
	Channels       []NotificationChannel `json:"channels,omitempty"`        // Notification channels
	SilenceOnMatch []string              `json:"silence_on_match,omitempty"` // Rules to silence
}

// NotificationChannel defines a notification channel
type NotificationChannel struct {
	Type   string                 `json:"type"`   // webhook/dingtalk/slack/wechat/email
	Config map[string]interface{} `json:"config"` // Channel-specific configuration
}

// RuleState tracks the runtime state of a rule
type RuleState struct {
	RuleID         int64
	LastEvaluation time.Time
	LastFiring     time.Time
	FiringCount    int64
	
	// Sliding window counters for threshold matching
	WindowCounters map[string]*WindowCounter
}

// WindowCounter tracks events within a time window
type WindowCounter struct {
	Events     []time.Time
	Count      int
	LastUpdate time.Time
}

// EvaluationContext provides context for rule evaluation
type EvaluationContext struct {
	Log         interface{} // The log being evaluated
	LogTime     time.Time
	Labels      map[string]string
	Message     string
	PodName     string
	PodID       string
	NodeName    string
	WorkloadID  string
	Namespace   string
	ClusterName string
}

// EvaluationResult represents the result of rule evaluation
type EvaluationResult struct {
	Matched       bool
	RuleID        int64
	RuleName      string
	Severity      string
	AlertTemplate AlertTemplate
	RouteConfig   RouteConfig
	Context       *EvaluationContext
	MatchReason   string
	EvalTimeMs    float64
}

