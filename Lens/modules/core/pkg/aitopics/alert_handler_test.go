package aitopics

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyzeAlertInput(t *testing.T) {
	now := time.Now()
	input := AnalyzeAlertInput{
		Alert: AlertInfo{
			AlertID:   "alert-123",
			Name:      "HighCPUUsage",
			Status:    "firing",
			Severity:  "warning",
			StartedAt: now,
			Labels: map[string]string{
				"instance": "node-1",
				"job":      "kubernetes-nodes",
			},
			Annotations: map[string]string{
				"summary": "High CPU usage detected",
			},
			Value: 95.5,
		},
		RelatedAlerts: []AlertInfo{
			{AlertID: "alert-124", Name: "MemoryPressure", Status: "firing"},
		},
		Options: &AnalyzeOptions{
			FetchLogs:    true,
			FetchMetrics: true,
			TimeRange:    "1h",
		},
	}

	assert.Equal(t, "alert-123", input.Alert.AlertID)
	assert.Equal(t, "HighCPUUsage", input.Alert.Name)
	assert.Len(t, input.RelatedAlerts, 1)
	assert.True(t, input.Options.FetchLogs)
}

func TestAlertInfo(t *testing.T) {
	now := time.Now()
	alert := AlertInfo{
		AlertID:   "alert-abc",
		Name:      "PodCrashLooping",
		Status:    "firing",
		Severity:  "critical",
		StartedAt: now,
		Labels: map[string]string{
			"pod":       "app-123",
			"namespace": "production",
		},
		Annotations: map[string]string{
			"summary":     "Pod is crash looping",
			"description": "Pod app-123 has restarted 5 times in the last hour",
		},
		Value: 5,
	}

	assert.Equal(t, "alert-abc", alert.AlertID)
	assert.Equal(t, "PodCrashLooping", alert.Name)
	assert.Equal(t, "firing", alert.Status)
	assert.Equal(t, "critical", alert.Severity)
	assert.Equal(t, now, alert.StartedAt)
	assert.Equal(t, "app-123", alert.Labels["pod"])
	assert.Equal(t, 5, alert.Value)
}

func TestAnalyzeAlertOutput(t *testing.T) {
	output := AnalyzeAlertOutput{
		Analysis: AlertAnalysis{
			RootCause: &RootCauseAnalysis{
				Summary:    "Memory leak in application causing OOM",
				Confidence: 0.85,
				Evidence:   []string{"Memory usage increasing linearly", "No GC activity"},
			},
			Impact: &ImpactAssessment{
				AffectedServices:   []string{"api-gateway", "auth-service"},
				SeverityAssessment: "High - affects user-facing services",
			},
			Recommendations: []Recommendation{
				{
					Action:         "Restart the pod",
					Command:        "kubectl rollout restart deployment/app",
					Priority:       1,
					AutoExecutable: true,
				},
				{
					Action:         "Increase memory limit",
					Priority:       2,
					AutoExecutable: false,
				},
			},
		},
		RelatedKnowledge: []KnowledgeItem{
			{
				Title: "Troubleshooting Memory Issues",
				URL:   "https://docs.example.com/memory-troubleshooting",
			},
		},
	}

	assert.NotNil(t, output.Analysis.RootCause)
	assert.Equal(t, 0.85, output.Analysis.RootCause.Confidence)
	assert.Len(t, output.Analysis.Recommendations, 2)
	assert.Len(t, output.RelatedKnowledge, 1)
}

func TestRootCauseAnalysis(t *testing.T) {
	rca := RootCauseAnalysis{
		Summary:    "Network partition between nodes",
		Confidence: 0.92,
		Evidence: []string{
			"Network latency spikes observed",
			"Partial connectivity between nodes",
			"Similar issues in the past 24 hours",
		},
	}

	assert.Equal(t, "Network partition between nodes", rca.Summary)
	assert.Equal(t, 0.92, rca.Confidence)
	assert.Len(t, rca.Evidence, 3)
}

func TestImpactAssessment(t *testing.T) {
	impact := ImpactAssessment{
		AffectedServices:   []string{"service-a", "service-b", "service-c"},
		SeverityAssessment: "Critical - complete service outage",
	}

	assert.Len(t, impact.AffectedServices, 3)
	assert.Contains(t, impact.SeverityAssessment, "Critical")
}

func TestRecommendation(t *testing.T) {
	rec := Recommendation{
		Action:         "Scale up replicas",
		Command:        "kubectl scale deployment/app --replicas=5",
		Priority:       1,
		AutoExecutable: true,
	}

	assert.Equal(t, "Scale up replicas", rec.Action)
	assert.Contains(t, rec.Command, "kubectl")
	assert.Equal(t, 1, rec.Priority)
	assert.True(t, rec.AutoExecutable)
}

func TestKnowledgeItem(t *testing.T) {
	item := KnowledgeItem{
		Title: "Kubernetes Troubleshooting Guide",
		URL:   "https://kubernetes.io/docs/troubleshooting",
	}

	assert.Equal(t, "Kubernetes Troubleshooting Guide", item.Title)
	assert.Contains(t, item.URL, "kubernetes.io")
}

func TestCorrelateAlertsInput(t *testing.T) {
	input := CorrelateAlertsInput{
		Alerts: []AlertInfo{
			{AlertID: "alert-1", Name: "HighCPU"},
			{AlertID: "alert-2", Name: "HighMemory"},
			{AlertID: "alert-3", Name: "SlowResponse"},
		},
		Options: &CorrelateOptions{
			TimeWindow:    "5m",
			MinConfidence: 0.7,
		},
	}

	assert.Len(t, input.Alerts, 3)
	assert.Equal(t, "5m", input.Options.TimeWindow)
	assert.Equal(t, 0.7, input.Options.MinConfidence)
}

func TestCorrelateAlertsOutput(t *testing.T) {
	output := CorrelateAlertsOutput{
		CorrelationGroups: []CorrelationGroup{
			{
				GroupID:        "group-1",
				PrimaryAlertID: "alert-1",
				RelatedAlerts:  []string{"alert-2", "alert-3"},
				CommonCause:    "Resource exhaustion on node-1",
				Confidence:     0.88,
			},
		},
		Uncorrelated: []string{"alert-4", "alert-5"},
	}

	assert.Len(t, output.CorrelationGroups, 1)
	assert.Equal(t, "group-1", output.CorrelationGroups[0].GroupID)
	assert.Len(t, output.Uncorrelated, 2)
}

func TestCorrelationGroup(t *testing.T) {
	group := CorrelationGroup{
		GroupID:        "corr-abc",
		PrimaryAlertID: "alert-primary",
		RelatedAlerts:  []string{"alert-sec-1", "alert-sec-2"},
		CommonCause:    "Disk I/O saturation",
		Confidence:     0.95,
	}

	assert.Equal(t, "corr-abc", group.GroupID)
	assert.Equal(t, "alert-primary", group.PrimaryAlertID)
	assert.Len(t, group.RelatedAlerts, 2)
	assert.Equal(t, 0.95, group.Confidence)
}

func TestExecuteActionInput(t *testing.T) {
	input := ExecuteActionInput{
		ActionType: "restart",
		Target: ActionTarget{
			Kind:      "Deployment",
			Name:      "app-server",
			Namespace: "production",
		},
		Parameters: map[string]string{
			"strategy": "rolling",
		},
		DryRun: true,
	}

	assert.Equal(t, "restart", input.ActionType)
	assert.Equal(t, "Deployment", input.Target.Kind)
	assert.Equal(t, "app-server", input.Target.Name)
	assert.True(t, input.DryRun)
}

func TestActionTarget(t *testing.T) {
	target := ActionTarget{
		Kind:      "StatefulSet",
		Name:      "database",
		Namespace: "data",
	}

	assert.Equal(t, "StatefulSet", target.Kind)
	assert.Equal(t, "database", target.Name)
	assert.Equal(t, "data", target.Namespace)
}

func TestExecuteActionOutput(t *testing.T) {
	output := ExecuteActionOutput{
		Success:    true,
		Message:    "Action executed successfully",
		ActionID:   "action-12345",
		ExecutedAt: "2025-01-15T10:30:00Z",
	}

	assert.True(t, output.Success)
	assert.Contains(t, output.Message, "successfully")
	assert.Equal(t, "action-12345", output.ActionID)
	assert.NotEmpty(t, output.ExecutedAt)
}

func TestAnalyzeOptions(t *testing.T) {
	opts := AnalyzeOptions{
		FetchLogs:    true,
		FetchMetrics: true,
		TimeRange:    "30m",
	}

	assert.True(t, opts.FetchLogs)
	assert.True(t, opts.FetchMetrics)
	assert.Equal(t, "30m", opts.TimeRange)
}

func TestCorrelateOptions(t *testing.T) {
	opts := CorrelateOptions{
		TimeWindow:    "10m",
		MinConfidence: 0.8,
	}

	assert.Equal(t, "10m", opts.TimeWindow)
	assert.Equal(t, 0.8, opts.MinConfidence)
}

func TestAlertHandler_JSON_Serialization(t *testing.T) {
	input := AnalyzeAlertInput{
		Alert: AlertInfo{
			AlertID:  "alert-1",
			Name:     "TestAlert",
			Status:   "firing",
			Severity: "warning",
		},
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var decoded AnalyzeAlertInput
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, input.Alert.AlertID, decoded.Alert.AlertID)
	assert.Equal(t, input.Alert.Name, decoded.Alert.Name)
}
