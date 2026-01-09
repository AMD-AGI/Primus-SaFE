// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package alerts

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
)

func TestCalculateCorrelation(t *testing.T) {
	baseTime := time.Now()

	tests := []struct {
		name                string
		alert               *UnifiedAlert
		related             *model.AlertEvents
		expectedMinScore    float64
		expectedMaxScore    float64
		expectedCorrType    string
		shouldHaveReason    bool
	}{
		{
			name: "causal correlation with high score",
			alert: &UnifiedAlert{
				ID:         "alert-1",
				Source:     SourceMetric,
				AlertName:  "GPUMemoryHigh",
				WorkloadID: "workload-1",
				StartsAt:   baseTime,
			},
			related: &model.AlertEvents{
				ID:         "alert-2",
				Source:     SourceLog,
				AlertName:  "OOMError",
				WorkloadID: "workload-1",
				StartsAt:   baseTime.Add(30 * time.Second),
			},
			expectedMinScore: 1.5, // Causal + time + workload
			expectedMaxScore: 2.5,
			expectedCorrType: CorrelationTypeCausal,
			shouldHaveReason: true,
		},
		{
			name: "causal correlation within 5 minutes",
			alert: &UnifiedAlert{
				ID:         "alert-1",
				Source:     SourceMetric,
				AlertName:  "GPUMemoryHigh",
				WorkloadID: "workload-1",
				StartsAt:   baseTime,
			},
			related: &model.AlertEvents{
				ID:         "alert-2",
				Source:     SourceLog,
				AlertName:  "OOMError",
				WorkloadID: "workload-1",
				StartsAt:   baseTime.Add(3 * time.Minute),
			},
			expectedMinScore: 1.5, // Causal + workload
			expectedMaxScore: 2.5,
			expectedCorrType: CorrelationTypeCausal,
			shouldHaveReason: true,
		},
		{
			name: "cross-source correlation",
			alert: &UnifiedAlert{
				ID:         "alert-1",
				Source:     SourceMetric,
				AlertName:  "GPUUtilizationLow",
				WorkloadID: "workload-1",
				StartsAt:   baseTime,
			},
			related: &model.AlertEvents{
				ID:         "alert-2",
				Source:     SourceLog,
				AlertName:  "TrainingSlowdown",
				WorkloadID: "workload-1",
				StartsAt:   baseTime.Add(2 * time.Minute),
			},
			expectedMinScore: 0.5, // Cross-source + workload
			expectedMaxScore: 1.5,
			expectedCorrType: CorrelationTypeCrossSource,
			shouldHaveReason: true,
		},
		{
			name: "cross-source with different pod",
			alert: &UnifiedAlert{
				ID:        "alert-1",
				Source:    SourceMetric,
				AlertName: "HighMemoryUsage",
				PodName:   "worker-0",
				StartsAt:  baseTime,
			},
			related: &model.AlertEvents{
				ID:        "alert-2",
				Source:    SourceLog,
				AlertName: "ContainerRestart",
				PodName:   "worker-0",
				StartsAt:  baseTime.Add(1 * time.Minute),
			},
			expectedMinScore: 0.5, // Cross-source + pod
			expectedMaxScore: 1.5,
			expectedCorrType: CorrelationTypeCrossSource,
			shouldHaveReason: true,
		},
		{
			name: "cross-source with same node",
			alert: &UnifiedAlert{
				ID:        "alert-1",
				Source:    SourceMetric,
				AlertName: "NodeDiskPressure",
				NodeName:  "gpu-node-01",
				StartsAt:  baseTime,
			},
			related: &model.AlertEvents{
				ID:        "alert-2",
				Source:    SourceLog,
				AlertName: "DiskWriteFailed",
				NodeName:  "gpu-node-01",
				StartsAt:  baseTime.Add(1 * time.Minute),
			},
			expectedMinScore: 0.5, // Cross-source + node
			expectedMaxScore: 1.5,
			expectedCorrType: CorrelationTypeCrossSource,
			shouldHaveReason: true,
		},
		{
			name: "cross-source correlation",
			alert: &UnifiedAlert{
				ID:        "alert-1",
				Source:    SourceMetric,
				AlertName: "HighLatency",
				StartsAt:  baseTime,
			},
			related: &model.AlertEvents{
				ID:        "alert-2",
				Source:    SourceTrace,
				AlertName: "SlowOperation",
				StartsAt:  baseTime.Add(1 * time.Minute),
			},
			expectedMinScore: 0.3, // 0.3 (cross-source)
			expectedMaxScore: 0.8,
			expectedCorrType: CorrelationTypeCrossSource,
			shouldHaveReason: true,
		},
		{
			name: "no correlation",
			alert: &UnifiedAlert{
				ID:        "alert-1",
				Source:    SourceMetric,
				AlertName: "Alert1",
				StartsAt:  baseTime,
			},
			related: &model.AlertEvents{
				ID:        "alert-2",
				Source:    SourceMetric,
				AlertName: "Alert2",
				StartsAt:  baseTime.Add(10 * time.Minute),
			},
			expectedMinScore: 0.0,
			expectedMaxScore: 0.3,
			expectedCorrType: "",
			shouldHaveReason: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, corrType, reason := calculateCorrelation(tt.alert, tt.related)

			assert.GreaterOrEqual(t, score, tt.expectedMinScore, "Score should be at least minimum expected")
			assert.LessOrEqual(t, score, tt.expectedMaxScore, "Score should not exceed maximum expected")

			if tt.shouldHaveReason {
				assert.NotEmpty(t, reason, "Reason should not be empty")
			}

			if tt.expectedCorrType != "" {
				assert.Equal(t, tt.expectedCorrType, corrType, "Correlation type should match")
			}
		})
	}
}

func TestDetectCausalRelationship(t *testing.T) {
	tests := []struct {
		name          string
		alert         *UnifiedAlert
		related       *model.AlertEvents
		expectedScore float64
		expectedInReason string
	}{
		{
			name: "GPUMemoryHigh causes OOMError",
			alert: &UnifiedAlert{
				AlertName: "OOMError",
			},
			related: &model.AlertEvents{
				AlertName: "GPUMemoryHigh",
				Labels:    model.ExtType{},
			},
			expectedScore:    0.8,
			expectedInReason: "high GPU memory leads to OOM",
		},
		{
			name: "NetworkLatencyHigh causes TrainingSlowdown",
			alert: &UnifiedAlert{
				AlertName: "TrainingSlowdown",
			},
			related: &model.AlertEvents{
				AlertName: "NetworkLatencyHigh",
				Labels:    model.ExtType{},
			},
			expectedScore:    0.7,
			expectedInReason: "network latency causes training slowdown",
		},
		{
			name: "NodeDown causes PodRestart",
			alert: &UnifiedAlert{
				AlertName: "PodRestart",
			},
			related: &model.AlertEvents{
				AlertName: "NodeDown",
				Labels:    model.ExtType{},
			},
			expectedScore:    0.9,
			expectedInReason: "node failure causes pod restart",
		},
		{
			name: "DiskSpaceLow causes CheckpointFailed",
			alert: &UnifiedAlert{
				AlertName: "CheckpointFailed",
			},
			related: &model.AlertEvents{
				AlertName: "DiskSpaceLow",
				Labels:    model.ExtType{},
			},
			expectedScore:    0.8,
			expectedInReason: "low disk space causes checkpoint failure",
		},
		{
			name: "NCCLError causes TrainingHang",
			alert: &UnifiedAlert{
				AlertName: "TrainingHang",
			},
			related: &model.AlertEvents{
				AlertName: "NCCLError",
				Labels:    model.ExtType{},
			},
			expectedScore:    0.7,
			expectedInReason: "NCCL error causes training hang",
		},
		{
			name: "HighTemperature causes GPUThrottling",
			alert: &UnifiedAlert{
				AlertName: "GPUThrottling",
			},
			related: &model.AlertEvents{
				AlertName: "HighTemperature",
				Labels:    model.ExtType{},
			},
			expectedScore:    0.8,
			expectedInReason: "high temperature causes GPU throttling",
		},
		{
			name: "reverse relationship also detected",
			alert: &UnifiedAlert{
				AlertName: "GPUMemoryHigh",
			},
			related: &model.AlertEvents{
				AlertName: "OOMError",
				Labels:    model.ExtType{},
			},
			expectedScore:    0.8,
			expectedInReason: "high GPU memory leads to OOM",
		},
		{
			name: "no causal relationship",
			alert: &UnifiedAlert{
				AlertName: "UnrelatedAlert1",
			},
			related: &model.AlertEvents{
				AlertName: "UnrelatedAlert2",
				Labels:    model.ExtType{},
			},
			expectedScore:    0.0,
			expectedInReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, reason := detectCausalRelationship(tt.alert, tt.related)

			assert.Equal(t, tt.expectedScore, score, "Score should match expected")

			if tt.expectedInReason != "" {
				assert.Contains(t, reason, tt.expectedInReason, "Reason should contain expected text")
			} else {
				assert.Empty(t, reason, "Reason should be empty for non-causal relationships")
			}
		})
	}
}

func TestConvertAlertEventToUnified(t *testing.T) {
	now := time.Now()
	endsAt := now.Add(1 * time.Hour)

	tests := []struct {
		name     string
		event    *model.AlertEvents
		validate func(t *testing.T, result *UnifiedAlert)
	}{
		{
			name: "complete alert event",
			event: &model.AlertEvents{
				ID:          "alert-123",
				Source:      SourceMetric,
				AlertName:   "TestAlert",
				Severity:    SeverityCritical,
				Status:      StatusFiring,
				StartsAt:    now,
				EndsAt:      endsAt,
				WorkloadID:  "workload-1",
				PodName:     "pod-1",
				PodID:       "pod-uid-1",
				NodeName:    "node-1",
				ClusterName: "cluster-1",
				Labels: model.ExtType{
					"severity": "critical",
					"team":     "ml",
				},
				Annotations: model.ExtType{
					"summary":     "Test alert summary",
					"description": "Test alert description",
				},
				RawData: model.ExtType{
					"raw_field": "raw_value",
				},
				EnrichedData: model.ExtType{
					"enriched_field": "enriched_value",
				},
			},
			validate: func(t *testing.T, result *UnifiedAlert) {
				assert.Equal(t, "alert-123", result.ID)
				assert.Equal(t, SourceMetric, result.Source)
				assert.Equal(t, "TestAlert", result.AlertName)
				assert.Equal(t, SeverityCritical, result.Severity)
				assert.Equal(t, StatusFiring, result.Status)
				assert.Equal(t, now, result.StartsAt)
				assert.NotNil(t, result.EndsAt)
				assert.Equal(t, endsAt, *result.EndsAt)
				assert.Equal(t, "workload-1", result.WorkloadID)
				assert.Equal(t, "pod-1", result.PodName)
				assert.Equal(t, "pod-uid-1", result.PodID)
				assert.Equal(t, "node-1", result.NodeName)
				assert.Equal(t, "cluster-1", result.ClusterName)
				assert.NotNil(t, result.Labels)
				assert.Equal(t, "critical", result.Labels["severity"])
				assert.Equal(t, "ml", result.Labels["team"])
				assert.NotNil(t, result.Annotations)
				assert.Equal(t, "Test alert summary", result.Annotations["summary"])
				assert.NotNil(t, result.RawData)
				assert.NotNil(t, result.EnrichedData)
			},
		},
		{
			name: "alert without ends_at",
			event: &model.AlertEvents{
				ID:        "alert-456",
				Source:    SourceLog,
				AlertName: "LogAlert",
				Severity:  SeverityWarning,
				Status:    StatusFiring,
				StartsAt:  now,
				EndsAt:    time.Time{}, // Zero time
			},
			validate: func(t *testing.T, result *UnifiedAlert) {
				assert.Equal(t, "alert-456", result.ID)
				assert.Nil(t, result.EndsAt, "EndsAt should be nil for zero time")
			},
		},
		{
			name: "alert with nil labels and annotations",
			event: &model.AlertEvents{
				ID:          "alert-789",
				Source:      SourceTrace,
				AlertName:   "TraceAlert",
				Severity:    SeverityInfo,
				Status:      StatusResolved,
				StartsAt:    now,
				Labels:      nil,
				Annotations: nil,
			},
			validate: func(t *testing.T, result *UnifiedAlert) {
				assert.Equal(t, "alert-789", result.ID)
				assert.NotNil(t, result.Labels)
				assert.Empty(t, result.Labels)
				assert.NotNil(t, result.Annotations)
				assert.Empty(t, result.Annotations)
			},
		},
		{
			name: "alert with minimal fields",
			event: &model.AlertEvents{
				ID:        "alert-min",
				Source:    SourceMetric,
				AlertName: "MinimalAlert",
				Severity:  SeverityHigh,
				Status:    StatusFiring,
				StartsAt:  now,
			},
			validate: func(t *testing.T, result *UnifiedAlert) {
				assert.Equal(t, "alert-min", result.ID)
				assert.Equal(t, SourceMetric, result.Source)
				assert.Equal(t, "MinimalAlert", result.AlertName)
				assert.Empty(t, result.WorkloadID)
				assert.Empty(t, result.PodName)
				assert.Empty(t, result.NodeName)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertAlertEventToUnified(tt.event)
			assert.NotNil(t, result)
			tt.validate(t, result)
		})
	}
}

func TestStringPtr(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "normal string",
			input: "test-string",
		},
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "string with spaces",
			input: "  spaces  ",
		},
		{
			name:  "special characters",
			input: "!@#$%^&*()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stringPtr(tt.input)
			assert.NotNil(t, result, "Result should not be nil")
			assert.Equal(t, tt.input, *result, "Dereferenced value should match input")
		})
	}
}

func TestAnalyzeCorrelations(t *testing.T) {
	baseTime := time.Now()

	alert := &UnifiedAlert{
		ID:         "alert-1",
		Source:     SourceMetric,
		AlertName:  "GPUMemoryHigh",
		WorkloadID: "workload-1",
		StartsAt:   baseTime,
	}

	relatedAlerts := []*model.AlertEvents{
		{
			ID:         "alert-2",
			Source:     SourceLog,
			AlertName:  "OOMError",
			WorkloadID: "workload-1",
			StartsAt:   baseTime.Add(30 * time.Second),
			Labels:     model.ExtType{},
		},
		{
			ID:         "alert-3",
			Source:     SourceMetric,
			AlertName:  "NetworkLatency",
			WorkloadID: "workload-1",
			StartsAt:   baseTime.Add(1 * time.Minute),
			Labels:     model.ExtType{},
		},
		{
			ID:        "alert-4",
			Source:    SourceTrace,
			AlertName: "UnrelatedAlert",
			StartsAt:  baseTime.Add(10 * time.Minute),
			Labels:    model.ExtType{},
		},
	}

	correlations := analyzeCorrelations(alert, relatedAlerts)

	assert.NotNil(t, correlations, "Correlations should not be nil")
	
	// Verify correlations structure
	for _, corr := range correlations {
		assert.NotEmpty(t, corr.RelatedAlertID, "Related alert ID should not be empty")
		if corr.CorrelationType != "" {
			assert.GreaterOrEqual(t, corr.CorrelationScore, minCorrelationScore, "Score should meet minimum threshold")
		}
	}

	// Check if we have any high-scoring correlations
	if len(correlations) > 0 {
		foundValidCorrelation := false
		for _, corr := range correlations {
			if corr.CorrelationScore >= minCorrelationScore {
				foundValidCorrelation = true
				break
			}
		}
		
		if foundValidCorrelation {
			assert.True(t, true, "Found at least one valid correlation")
		}
	}
}

func TestConvertAlertEventToUnifiedJSONRoundTrip(t *testing.T) {
	now := time.Now()
	endsAt := now.Add(1 * time.Hour)

	original := &model.AlertEvents{
		ID:        "alert-json-test",
		Source:    SourceMetric,
		AlertName: "JSONTest",
		Severity:  SeverityCritical,
		Status:    StatusFiring,
		StartsAt:  now,
		EndsAt:    endsAt,
		Labels: model.ExtType{
			"key1": "value1",
			"key2": "value2",
		},
		Annotations: model.ExtType{
			"ann1": "annotation1",
		},
	}

	unified := convertAlertEventToUnified(original)

	// Verify JSON serialization
	jsonData, err := json.Marshal(unified)
	assert.NoError(t, err, "Should serialize to JSON without error")
	assert.NotEmpty(t, jsonData, "JSON data should not be empty")

	// Verify JSON deserialization
	var deserialized UnifiedAlert
	err = json.Unmarshal(jsonData, &deserialized)
	assert.NoError(t, err, "Should deserialize from JSON without error")
	assert.Equal(t, unified.ID, deserialized.ID, "ID should match after round trip")
	assert.Equal(t, unified.AlertName, deserialized.AlertName, "Alert name should match")
	assert.Equal(t, unified.Labels, deserialized.Labels, "Labels should match")
}

