package aitopics

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAggregateWorkloadsInput(t *testing.T) {
	input := AggregateWorkloadsInput{
		Workloads: []WorkloadInfo{
			{
				UID:       "uid-1",
				Name:      "nginx-deployment",
				Namespace: "default",
				Kind:      "Deployment",
				Labels:    map[string]string{"app": "nginx"},
				Images:    []string{"nginx:latest"},
				Replicas:  3,
			},
		},
		Options: &AggregateOptions{
			MaxGroups:     10,
			MinConfidence: 0.8,
		},
	}

	// Test JSON serialization
	data, err := json.Marshal(input)
	require.NoError(t, err)

	var decoded AggregateWorkloadsInput
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Len(t, decoded.Workloads, 1)
	assert.Equal(t, "nginx-deployment", decoded.Workloads[0].Name)
	assert.Equal(t, 10, decoded.Options.MaxGroups)
}

func TestWorkloadInfo(t *testing.T) {
	workload := WorkloadInfo{
		UID:                      "uid-123",
		Name:                     "web-app",
		Namespace:                "production",
		Kind:                     "Deployment",
		Labels:                   map[string]string{"app": "web", "tier": "frontend"},
		Annotations:              map[string]string{"description": "Web frontend"},
		Images:                   []string{"nginx:1.19", "envoy:1.18"},
		Replicas:                 5,
		IdentifiedType:           "web-server",
		IdentificationConfidence: 0.95,
	}

	assert.Equal(t, "uid-123", workload.UID)
	assert.Equal(t, "web-app", workload.Name)
	assert.Equal(t, "production", workload.Namespace)
	assert.Equal(t, "Deployment", workload.Kind)
	assert.Len(t, workload.Labels, 2)
	assert.Len(t, workload.Images, 2)
	assert.Equal(t, 5, workload.Replicas)
	assert.Equal(t, 0.95, workload.IdentificationConfidence)
}

func TestAggregateWorkloadsOutput(t *testing.T) {
	output := AggregateWorkloadsOutput{
		Groups: []ComponentGroup{
			{
				GroupID:           "group-1",
				Name:              "Web Servers",
				ComponentType:     "web-server",
				Category:          "frontend",
				Members:           []string{"workload-1", "workload-2"},
				AggregationReason: "Similar container images and labels",
				Confidence:        0.92,
			},
		},
		Ungrouped: []string{"workload-3"},
		Stats: AggregateStats{
			TotalWorkloads:   3,
			GroupedWorkloads: 2,
			TotalGroups:      1,
		},
	}

	assert.Len(t, output.Groups, 1)
	assert.Equal(t, "Web Servers", output.Groups[0].Name)
	assert.Len(t, output.Ungrouped, 1)
	assert.Equal(t, 3, output.Stats.TotalWorkloads)
}

func TestComponentGroup(t *testing.T) {
	group := ComponentGroup{
		GroupID:           "group-abc",
		Name:              "Database Cluster",
		ComponentType:     "database",
		Category:          "storage",
		Members:           []string{"db-1", "db-2", "db-3"},
		AggregationReason: "StatefulSet with postgres image",
		Confidence:        0.98,
	}

	assert.Equal(t, "group-abc", group.GroupID)
	assert.Equal(t, "Database Cluster", group.Name)
	assert.Equal(t, "database", group.ComponentType)
	assert.Len(t, group.Members, 3)
	assert.Equal(t, 0.98, group.Confidence)
}

func TestGenerateSuggestionsInput(t *testing.T) {
	input := GenerateSuggestionsInput{
		Component: ComponentInfo{
			GroupID:       "group-1",
			Name:          "API Gateway",
			ComponentType: "api-gateway",
			Category:      "networking",
			Members: []WorkloadInfo{
				{Name: "gateway-1"},
			},
		},
		ExistingRules: []ExistingRule{
			{
				Name:     "HighLatency",
				Expr:     "histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m])) > 1",
				Severity: "warning",
			},
		},
		Options: &SuggestionOptions{
			IncludeBestPractices: true,
			MaxSuggestions:       5,
			SeverityFilter:       []string{"warning", "critical"},
		},
	}

	assert.Equal(t, "API Gateway", input.Component.Name)
	assert.Len(t, input.ExistingRules, 1)
	assert.True(t, input.Options.IncludeBestPractices)
}

func TestGenerateSuggestionsOutput(t *testing.T) {
	output := GenerateSuggestionsOutput{
		Suggestions: []AlertSuggestion{
			{
				SuggestionID: "sug-1",
				RuleName:     "HighErrorRate",
				Description:  "Alert when error rate exceeds threshold",
				Category:     "reliability",
				Severity:     "critical",
				PrometheusRule: &PrometheusRule{
					Expr: "rate(http_requests_total{status=~\"5..\"}[5m]) / rate(http_requests_total[5m]) > 0.05",
					For:  "5m",
					Labels: map[string]string{
						"severity": "critical",
					},
					Annotations: map[string]string{
						"summary": "High error rate detected",
					},
				},
				Rationale:  "Best practice for API monitoring",
				Confidence: 0.9,
				Priority:   1,
			},
		},
		CoverageAnalysis: &CoverageAnalysis{
			ExistingCoverage: []string{"latency", "availability"},
			MissingCoverage:  []string{"error-rate", "throughput"},
			CoverageScore:    0.6,
		},
	}

	assert.Len(t, output.Suggestions, 1)
	assert.Equal(t, "HighErrorRate", output.Suggestions[0].RuleName)
	assert.NotNil(t, output.CoverageAnalysis)
	assert.Equal(t, 0.6, output.CoverageAnalysis.CoverageScore)
}

func TestPrometheusRule(t *testing.T) {
	rule := PrometheusRule{
		Expr: "up == 0",
		For:  "1m",
		Labels: map[string]string{
			"severity": "critical",
			"team":     "platform",
		},
		Annotations: map[string]string{
			"summary":     "Instance is down",
			"description": "{{ $labels.instance }} has been down for more than 1 minute",
		},
	}

	assert.Equal(t, "up == 0", rule.Expr)
	assert.Equal(t, "1m", rule.For)
	assert.Equal(t, "critical", rule.Labels["severity"])
	assert.Contains(t, rule.Annotations["description"], "$labels.instance")
}

func TestAnalyzeCoverageInput(t *testing.T) {
	input := AnalyzeCoverageInput{
		Components: []ComponentInfo{
			{Name: "Web Server", ComponentType: "web-server"},
			{Name: "Database", ComponentType: "database"},
		},
		ExistingRules: []ExistingRule{
			{Name: "Rule1", Expr: "expr1", Severity: "warning"},
		},
	}

	assert.Len(t, input.Components, 2)
	assert.Len(t, input.ExistingRules, 1)
}

func TestAnalyzeCoverageOutput(t *testing.T) {
	output := AnalyzeCoverageOutput{
		TotalComponents:   10,
		CoveredComponents: 7,
		OverallScore:      0.7,
		ByCategory: map[string]CategoryCoverage{
			"frontend": {
				TotalComponents: 3,
				Covered:         2,
				Score:           0.67,
			},
			"backend": {
				TotalComponents: 5,
				Covered:         4,
				Score:           0.8,
			},
		},
		Gaps: []CoverageGap{
			{
				ComponentName: "Redis Cache",
				ComponentType: "cache",
				MissingAreas:  []string{"availability", "memory-usage"},
				Priority:      1,
			},
		},
	}

	assert.Equal(t, 10, output.TotalComponents)
	assert.Equal(t, 7, output.CoveredComponents)
	assert.Equal(t, 0.7, output.OverallScore)
	assert.Len(t, output.ByCategory, 2)
	assert.Len(t, output.Gaps, 1)
}

func TestAlertSuggestion(t *testing.T) {
	suggestion := AlertSuggestion{
		SuggestionID:   "sug-abc",
		RuleName:       "MemoryPressure",
		Description:    "Alert on high memory usage",
		Category:       "resources",
		Severity:       "warning",
		PrometheusRule: &PrometheusRule{Expr: "memory_usage > 0.9"},
		Rationale:      "Memory pressure can cause OOM kills",
		Confidence:     0.85,
		Priority:       2,
	}

	assert.Equal(t, "sug-abc", suggestion.SuggestionID)
	assert.Equal(t, "MemoryPressure", suggestion.RuleName)
	assert.Equal(t, 0.85, suggestion.Confidence)
	assert.Equal(t, 2, suggestion.Priority)
}

func TestCoverageGap(t *testing.T) {
	gap := CoverageGap{
		ComponentName: "Message Queue",
		ComponentType: "messaging",
		MissingAreas:  []string{"queue-depth", "consumer-lag", "dead-letters"},
		Priority:      1,
	}

	assert.Equal(t, "Message Queue", gap.ComponentName)
	assert.Len(t, gap.MissingAreas, 3)
	assert.Equal(t, 1, gap.Priority)
}
