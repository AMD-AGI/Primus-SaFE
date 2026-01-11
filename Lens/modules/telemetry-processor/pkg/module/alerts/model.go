// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package alerts

import (
	"encoding/json"
	"time"
)

const (
	// Alert sources
	SourceMetric = "metric"
	SourceLog    = "log"
	SourceTrace  = "trace"
	
	// Alert status
	StatusFiring   = "firing"
	StatusResolved = "resolved"
	StatusSilenced = "silenced"
	
	// Alert severity
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
	
	// Correlation types
	CorrelationTypeTime        = "time"
	CorrelationTypeEntity      = "entity"
	CorrelationTypeCausal      = "causal"
	CorrelationTypeCrossSource = "cross_source"
	
	// Notification channels
	ChannelWebhook   = "webhook"
	ChannelEmail     = "email"
	ChannelDingTalk  = "dingtalk"
	ChannelWeChat    = "wechat"
	ChannelSlack     = "slack"
	ChannelAlertManager = "alertmanager"
	
	// Notification status
	NotificationStatusPending = "pending"
	NotificationStatusSent    = "sent"
	NotificationStatusFailed  = "failed"
)

// UnifiedAlert represents a standardized alert from any source
type UnifiedAlert struct {
	ID          string                 `json:"id"`
	Source      string                 `json:"source"`
	AlertName   string                 `json:"alert_name"`
	Severity    string                 `json:"severity"`
	Status      string                 `json:"status"`
	StartsAt    time.Time              `json:"starts_at"`
	EndsAt      *time.Time             `json:"ends_at,omitempty"`
	Labels      map[string]string      `json:"labels"`
	Annotations map[string]string      `json:"annotations"`
	WorkloadID  string                 `json:"workload_id,omitempty"`
	PodName     string                 `json:"pod_name,omitempty"`
	PodID       string                 `json:"pod_id,omitempty"`
	NodeName    string                 `json:"node_name,omitempty"`
	ClusterName string                 `json:"cluster_name,omitempty"`
	RawData     json.RawMessage        `json:"raw_data,omitempty"`
	EnrichedData map[string]interface{} `json:"enriched_data,omitempty"`
}

// VMAlertWebhook represents the webhook payload from VMAlert
type VMAlertWebhook struct {
	Alerts            []VMAlertItem     `json:"alerts"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
}

// VMAlertItem represents a single alert from VMAlert
type VMAlertItem struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     string            `json:"startsAt"`
	EndsAt       string            `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
}

// LogAlertRequest represents a log-based alert trigger
type LogAlertRequest struct {
	RuleName    string            `json:"rule_name"`
	Severity    string            `json:"severity"`
	Message     string            `json:"message"`
	Pattern     string            `json:"pattern"`
	WorkloadID  string            `json:"workload_id,omitempty"`
	PodName     string            `json:"pod_name,omitempty"`
	PodID       string            `json:"pod_id,omitempty"`
	NodeName    string            `json:"node_name,omitempty"`
	LogTime     time.Time         `json:"log_time"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// TraceAlertRequest represents a trace-based alert trigger
type TraceAlertRequest struct {
	RuleName    string            `json:"rule_name"`
	Severity    string            `json:"severity"`
	Message     string            `json:"message"`
	TraceID     string            `json:"trace_id"`
	SpanID      string            `json:"span_id"`
	ServiceName string            `json:"service_name"`
	Operation   string            `json:"operation"`
	Duration    float64           `json:"duration"`
	WorkloadID  string            `json:"workload_id,omitempty"`
	PodName     string            `json:"pod_name,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// AlertQueryRequest represents a query request for alerts
type AlertQueryRequest struct {
	Source       string     `json:"source,omitempty" form:"source"`
	AlertName    string     `json:"alert_name,omitempty" form:"alert_name"`
	Severity     string     `json:"severity,omitempty" form:"severity"`
	Status       string     `json:"status,omitempty" form:"status"`
	WorkloadID   string     `json:"workload_id,omitempty" form:"workload_id"`
	PodName      string     `json:"pod_name,omitempty" form:"pod_name"`
	NodeName     string     `json:"node_name,omitempty" form:"node_name"`
	ClusterName  string     `json:"cluster_name,omitempty" form:"cluster_name"`
	StartsAfter  *time.Time `json:"starts_after,omitempty" form:"starts_after"`
	StartsBefore *time.Time `json:"starts_before,omitempty" form:"starts_before"`
	Offset       int        `json:"offset,omitempty" form:"offset"`
	Limit        int        `json:"limit,omitempty" form:"limit"`
}

// AlertStatisticsRequest represents a query request for alert statistics
type AlertStatisticsRequest struct {
	DateFrom    *time.Time `json:"date_from,omitempty" form:"date_from"`
	DateTo      *time.Time `json:"date_to,omitempty" form:"date_to"`
	AlertName   string     `json:"alert_name,omitempty" form:"alert_name"`
	Source      string     `json:"source,omitempty" form:"source"`
	WorkloadID  string     `json:"workload_id,omitempty" form:"workload_id"`
	ClusterName string     `json:"cluster_name,omitempty" form:"cluster_name"`
	GroupBy     string     `json:"group_by,omitempty" form:"group_by"` // hour/day
	Offset      int        `json:"offset,omitempty" form:"offset"`
	Limit       int        `json:"limit,omitempty" form:"limit"`
}

// AlertCorrelationResponse represents correlated alerts
type AlertCorrelationResponse struct {
	CorrelationID string          `json:"correlation_id"`
	Alerts        []*UnifiedAlert `json:"alerts"`
	CorrelationType string        `json:"correlation_type"`
	CorrelationScore float64      `json:"correlation_score,omitempty"`
	Reason        string          `json:"reason,omitempty"`
}

// RouteConfig represents alert routing configuration
type RouteConfig struct {
	Matchers    []Matcher       `json:"matchers"`
	Channels    []ChannelConfig `json:"channels"`
	GroupBy     []string        `json:"group_by,omitempty"`
	GroupWait   string          `json:"group_wait,omitempty"`
	GroupInterval string        `json:"group_interval,omitempty"`
	RepeatInterval string       `json:"repeat_interval,omitempty"`
}

// Matcher represents a label matcher for routing
type Matcher struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"is_regex,omitempty"`
}

// ChannelConfig represents notification channel configuration
type ChannelConfig struct {
	Type   string                 `json:"type"`
	Config map[string]interface{} `json:"config"`
}

// SilenceRequest represents a request to create a silence
type SilenceRequest struct {
	Matchers  []Matcher `json:"matchers"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Comment   string    `json:"comment,omitempty"`
	CreatedBy string    `json:"created_by,omitempty"`
}

// AlertRuleRequest represents a request to create or update an alert rule
type AlertRuleRequest struct {
	Name        string                 `json:"name"`
	Source      string                 `json:"source"`
	Enabled     bool                   `json:"enabled"`
	RuleType    string                 `json:"rule_type"`
	RuleConfig  map[string]interface{} `json:"rule_config"`
	Severity    string                 `json:"severity"`
	Labels      map[string]string      `json:"labels,omitempty"`
	Annotations map[string]string      `json:"annotations,omitempty"`
	RouteConfig *RouteConfig           `json:"route_config,omitempty"`
}

