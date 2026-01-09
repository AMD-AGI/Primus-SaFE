// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package aitopics

// ========== alert.advisor.aggregate-workloads ==========

// AggregateWorkloadsInput is the input payload for aggregate-workloads topic
type AggregateWorkloadsInput struct {
	Workloads []WorkloadInfo    `json:"workloads"`
	Options   *AggregateOptions `json:"options,omitempty"`
}

// WorkloadInfo represents a workload to be aggregated
type WorkloadInfo struct {
	UID                      string            `json:"uid"`
	Name                     string            `json:"name"`
	Namespace                string            `json:"namespace"`
	Kind                     string            `json:"kind"`
	Labels                   map[string]string `json:"labels,omitempty"`
	Annotations              map[string]string `json:"annotations,omitempty"`
	Images                   []string          `json:"images,omitempty"`
	Replicas                 int               `json:"replicas,omitempty"`
	IdentifiedType           string            `json:"identified_type,omitempty"`
	IdentificationConfidence float64           `json:"identification_confidence,omitempty"`
}

// AggregateOptions contains optional settings for aggregation
type AggregateOptions struct {
	MaxGroups     int     `json:"max_groups,omitempty"`
	MinConfidence float64 `json:"min_confidence,omitempty"`
}

// AggregateWorkloadsOutput is the output payload for aggregate-workloads topic
type AggregateWorkloadsOutput struct {
	Groups    []ComponentGroup `json:"groups"`
	Ungrouped []string         `json:"ungrouped,omitempty"`
	Stats     AggregateStats   `json:"stats"`
}

// ComponentGroup represents a logical component group
type ComponentGroup struct {
	GroupID           string   `json:"group_id"`
	Name              string   `json:"name"`
	ComponentType     string   `json:"component_type"`
	Category          string   `json:"category"`
	Members           []string `json:"members"`
	AggregationReason string   `json:"aggregation_reason,omitempty"`
	Confidence        float64  `json:"confidence"`
}

// AggregateStats contains statistics about the aggregation
type AggregateStats struct {
	TotalWorkloads   int `json:"total_workloads"`
	GroupedWorkloads int `json:"grouped_workloads"`
	TotalGroups      int `json:"total_groups"`
}

// ========== alert.advisor.generate-suggestions ==========

// GenerateSuggestionsInput is the input payload for generate-suggestions topic
type GenerateSuggestionsInput struct {
	Component     ComponentInfo      `json:"component"`
	ExistingRules []ExistingRule     `json:"existing_rules,omitempty"`
	Options       *SuggestionOptions `json:"options,omitempty"`
}

// ComponentInfo represents a component for which to generate suggestions
type ComponentInfo struct {
	GroupID       string         `json:"group_id"`
	Name          string         `json:"name"`
	ComponentType string         `json:"component_type"`
	Category      string         `json:"category"`
	Members       []WorkloadInfo `json:"members"`
}

// ExistingRule represents an existing alert rule
type ExistingRule struct {
	Name     string `json:"name"`
	Expr     string `json:"expr"`
	Severity string `json:"severity"`
}

// SuggestionOptions contains optional settings for suggestion generation
type SuggestionOptions struct {
	IncludeBestPractices bool     `json:"include_best_practices,omitempty"`
	MaxSuggestions       int      `json:"max_suggestions,omitempty"`
	SeverityFilter       []string `json:"severity_filter,omitempty"`
}

// GenerateSuggestionsOutput is the output payload for generate-suggestions topic
type GenerateSuggestionsOutput struct {
	Suggestions      []AlertSuggestion `json:"suggestions"`
	CoverageAnalysis *CoverageAnalysis `json:"coverage_analysis,omitempty"`
}

// AlertSuggestion represents a suggested alert rule
type AlertSuggestion struct {
	SuggestionID   string          `json:"suggestion_id"`
	RuleName       string          `json:"rule_name"`
	Description    string          `json:"description"`
	Category       string          `json:"category"`
	Severity       string          `json:"severity"`
	PrometheusRule *PrometheusRule `json:"prometheus_rule"`
	Rationale      string          `json:"rationale,omitempty"`
	Confidence     float64         `json:"confidence"`
	Priority       int             `json:"priority"`
}

// PrometheusRule represents a ready-to-use Prometheus alerting rule
type PrometheusRule struct {
	Expr        string            `json:"expr"`
	For         string            `json:"for,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// CoverageAnalysis contains analysis of alert coverage
type CoverageAnalysis struct {
	ExistingCoverage []string `json:"existing_coverage"`
	MissingCoverage  []string `json:"missing_coverage"`
	CoverageScore    float64  `json:"coverage_score"`
}

// ========== alert.advisor.analyze-coverage ==========

// AnalyzeCoverageInput is the input payload for analyze-coverage topic
type AnalyzeCoverageInput struct {
	Components    []ComponentInfo `json:"components"`
	ExistingRules []ExistingRule  `json:"existing_rules,omitempty"`
}

// AnalyzeCoverageOutput is the output payload for analyze-coverage topic
type AnalyzeCoverageOutput struct {
	TotalComponents   int                     `json:"total_components"`
	CoveredComponents int                     `json:"covered_components"`
	OverallScore      float64                 `json:"overall_score"`
	ByCategory        map[string]CategoryCoverage `json:"by_category"`
	Gaps              []CoverageGap           `json:"gaps"`
}

// CategoryCoverage represents coverage for a category
type CategoryCoverage struct {
	TotalComponents int     `json:"total_components"`
	Covered         int     `json:"covered"`
	Score           float64 `json:"score"`
}

// CoverageGap represents a gap in alert coverage
type CoverageGap struct {
	ComponentName string   `json:"component_name"`
	ComponentType string   `json:"component_type"`
	MissingAreas  []string `json:"missing_areas"`
	Priority      int      `json:"priority"`
}

