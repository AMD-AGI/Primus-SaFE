package alerts

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlertConstants(t *testing.T) {
	t.Run("alert sources", func(t *testing.T) {
		assert.Equal(t, "metric", SourceMetric)
		assert.Equal(t, "log", SourceLog)
		assert.Equal(t, "trace", SourceTrace)
	})

	t.Run("alert status", func(t *testing.T) {
		assert.Equal(t, "firing", StatusFiring)
		assert.Equal(t, "resolved", StatusResolved)
		assert.Equal(t, "silenced", StatusSilenced)
	})

	t.Run("alert severity", func(t *testing.T) {
		assert.Equal(t, "critical", SeverityCritical)
		assert.Equal(t, "high", SeverityHigh)
		assert.Equal(t, "warning", SeverityWarning)
		assert.Equal(t, "info", SeverityInfo)
	})

	t.Run("correlation types", func(t *testing.T) {
		assert.Equal(t, "time", CorrelationTypeTime)
		assert.Equal(t, "entity", CorrelationTypeEntity)
		assert.Equal(t, "causal", CorrelationTypeCausal)
		assert.Equal(t, "cross_source", CorrelationTypeCrossSource)
	})

	t.Run("notification channels", func(t *testing.T) {
		assert.Equal(t, "webhook", ChannelWebhook)
		assert.Equal(t, "email", ChannelEmail)
		assert.Equal(t, "dingtalk", ChannelDingTalk)
		assert.Equal(t, "wechat", ChannelWeChat)
		assert.Equal(t, "slack", ChannelSlack)
		assert.Equal(t, "alertmanager", ChannelAlertManager)
	})

	t.Run("notification status", func(t *testing.T) {
		assert.Equal(t, "pending", NotificationStatusPending)
		assert.Equal(t, "sent", NotificationStatusSent)
		assert.Equal(t, "failed", NotificationStatusFailed)
	})
}

func TestUnifiedAlertJSON(t *testing.T) {
	now := time.Now()
	endsAt := now.Add(1 * time.Hour)

	tests := []struct {
		name  string
		alert UnifiedAlert
	}{
		{
			name: "complete unified alert",
			alert: UnifiedAlert{
				ID:        "alert-1",
				Source:    SourceMetric,
				AlertName: "GPUMemoryHigh",
				Severity:  SeverityCritical,
				Status:    StatusFiring,
				StartsAt:  now,
				EndsAt:    &endsAt,
				Labels: map[string]string{
					"severity": "critical",
					"team":     "ml",
				},
				Annotations: map[string]string{
					"summary":     "GPU memory is high",
					"description": "GPU 0 memory usage above 90%",
				},
				WorkloadID:  "workload-123",
				PodName:     "worker-0",
				PodID:       "pod-uid-123",
				NodeName:    "gpu-node-01",
				ClusterName: "prod-cluster",
				RawData:     json.RawMessage(`{"raw":"data"}`),
				EnrichedData: map[string]interface{}{
					"enriched_field": "enriched_value",
				},
			},
		},
		{
			name: "minimal unified alert",
			alert: UnifiedAlert{
				ID:        "alert-2",
				Source:    SourceLog,
				AlertName: "LogAlert",
				Severity:  SeverityWarning,
				Status:    StatusFiring,
				StartsAt:  now,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling
			jsonData, err := json.Marshal(tt.alert)
			require.NoError(t, err, "Should marshal to JSON without error")
			assert.NotEmpty(t, jsonData, "JSON data should not be empty")

			// Test JSON unmarshaling
			var deserialized UnifiedAlert
			err = json.Unmarshal(jsonData, &deserialized)
			require.NoError(t, err, "Should unmarshal from JSON without error")

			// Verify key fields
			assert.Equal(t, tt.alert.ID, deserialized.ID)
			assert.Equal(t, tt.alert.Source, deserialized.Source)
			assert.Equal(t, tt.alert.AlertName, deserialized.AlertName)
			assert.Equal(t, tt.alert.Severity, deserialized.Severity)
			assert.Equal(t, tt.alert.Status, deserialized.Status)
		})
	}
}

func TestVMAlertWebhookJSON(t *testing.T) {
	webhook := VMAlertWebhook{
		Alerts: []VMAlertItem{
			{
				Status: StatusFiring,
				Labels: map[string]string{
					"alertname": "GPUMemoryHigh",
					"severity":  "critical",
				},
				Annotations: map[string]string{
					"summary": "GPU memory is high",
				},
				StartsAt:     "2025-01-01T00:00:00Z",
				EndsAt:       "0001-01-01T00:00:00Z",
				GeneratorURL: "http://vmalert/alerts/1",
				Fingerprint:  "abc123",
			},
		},
		GroupLabels: map[string]string{
			"alertname": "GPUMemoryHigh",
		},
		CommonLabels: map[string]string{
			"cluster": "prod",
		},
		CommonAnnotations: map[string]string{
			"runbook": "http://runbook/gpu-memory",
		},
		ExternalURL: "http://vmalert",
		Version:     "4",
		GroupKey:    "group-key-1",
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(webhook)
	require.NoError(t, err, "Should marshal to JSON without error")
	assert.NotEmpty(t, jsonData, "JSON data should not be empty")

	// Test JSON unmarshaling
	var deserialized VMAlertWebhook
	err = json.Unmarshal(jsonData, &deserialized)
	require.NoError(t, err, "Should unmarshal from JSON without error")

	assert.Equal(t, len(webhook.Alerts), len(deserialized.Alerts))
	assert.Equal(t, webhook.Alerts[0].Status, deserialized.Alerts[0].Status)
	assert.Equal(t, webhook.GroupKey, deserialized.GroupKey)
}

func TestLogAlertRequestJSON(t *testing.T) {
	now := time.Now()

	request := LogAlertRequest{
		RuleName:   "OOMDetected",
		Severity:   SeverityCritical,
		Message:    "Out of memory error detected",
		Pattern:    "OOM|OutOfMemoryError",
		WorkloadID: "workload-123",
		PodName:    "worker-0",
		PodID:      "pod-uid-123",
		NodeName:   "node-1",
		LogTime:    now,
		Labels: map[string]string{
			"error_type": "oom",
		},
		Annotations: map[string]string{
			"action": "restart pod",
		},
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(request)
	require.NoError(t, err, "Should marshal to JSON without error")
	assert.NotEmpty(t, jsonData, "JSON data should not be empty")

	// Test JSON unmarshaling
	var deserialized LogAlertRequest
	err = json.Unmarshal(jsonData, &deserialized)
	require.NoError(t, err, "Should unmarshal from JSON without error")

	assert.Equal(t, request.RuleName, deserialized.RuleName)
	assert.Equal(t, request.Severity, deserialized.Severity)
	assert.Equal(t, request.Message, deserialized.Message)
	assert.Equal(t, request.WorkloadID, deserialized.WorkloadID)
}

func TestTraceAlertRequestJSON(t *testing.T) {
	request := TraceAlertRequest{
		RuleName:    "SlowOperation",
		Severity:    SeverityWarning,
		Message:     "Operation exceeded threshold",
		TraceID:     "trace-123",
		SpanID:      "span-456",
		ServiceName: "dataloader",
		Operation:   "load_batch",
		Duration:    5000.5,
		WorkloadID:  "workload-123",
		PodName:     "worker-0",
		Labels: map[string]string{
			"operation_type": "io",
		},
		Annotations: map[string]string{
			"threshold": "1000ms",
		},
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(request)
	require.NoError(t, err, "Should marshal to JSON without error")
	assert.NotEmpty(t, jsonData, "JSON data should not be empty")

	// Test JSON unmarshaling
	var deserialized TraceAlertRequest
	err = json.Unmarshal(jsonData, &deserialized)
	require.NoError(t, err, "Should unmarshal from JSON without error")

	assert.Equal(t, request.RuleName, deserialized.RuleName)
	assert.Equal(t, request.TraceID, deserialized.TraceID)
	assert.Equal(t, request.SpanID, deserialized.SpanID)
	assert.Equal(t, request.Duration, deserialized.Duration)
}

func TestAlertQueryRequest(t *testing.T) {
	now := time.Now()
	later := now.Add(1 * time.Hour)

	request := AlertQueryRequest{
		Source:       SourceMetric,
		AlertName:    "GPUMemoryHigh",
		Severity:     SeverityCritical,
		Status:       StatusFiring,
		WorkloadID:   "workload-123",
		PodName:      "worker-0",
		NodeName:     "node-1",
		ClusterName:  "prod",
		StartsAfter:  &now,
		StartsBefore: &later,
		Offset:       0,
		Limit:        100,
	}

	jsonData, err := json.Marshal(request)
	require.NoError(t, err, "Should marshal to JSON without error")

	var deserialized AlertQueryRequest
	err = json.Unmarshal(jsonData, &deserialized)
	require.NoError(t, err, "Should unmarshal from JSON without error")

	assert.Equal(t, request.Source, deserialized.Source)
	assert.Equal(t, request.AlertName, deserialized.AlertName)
	assert.Equal(t, request.Limit, deserialized.Limit)
}

func TestSilenceRequest(t *testing.T) {
	now := time.Now()
	later := now.Add(4 * time.Hour)

	request := SilenceRequest{
		Matchers: []Matcher{
			{Name: "alertname", Value: "GPUMemoryHigh"},
			{Name: "severity", Value: "warning"},
		},
		StartsAt:  now,
		EndsAt:    later,
		Comment:   "Maintenance window",
		CreatedBy: "admin@example.com",
	}

	jsonData, err := json.Marshal(request)
	require.NoError(t, err, "Should marshal to JSON without error")

	var deserialized SilenceRequest
	err = json.Unmarshal(jsonData, &deserialized)
	require.NoError(t, err, "Should unmarshal from JSON without error")

	assert.Equal(t, len(request.Matchers), len(deserialized.Matchers))
	assert.Equal(t, request.Comment, deserialized.Comment)
	assert.Equal(t, request.CreatedBy, deserialized.CreatedBy)
}

func TestAlertRuleRequest(t *testing.T) {
	request := AlertRuleRequest{
		Name:     "NCCLError",
		Source:   SourceLog,
		Enabled:  true,
		RuleType: "pattern",
		RuleConfig: map[string]interface{}{
			"pattern": "NCCL error|NCCL WARN",
			"threshold": map[string]interface{}{
				"count":  3,
				"window": "10m",
			},
		},
		Severity: SeverityWarning,
		Labels: map[string]string{
			"category": "network",
		},
		Annotations: map[string]string{
			"summary":     "NCCL communication errors detected",
			"description": "Multiple NCCL errors indicate network issues",
		},
		RouteConfig: &RouteConfig{
			Matchers: []Matcher{
				{Name: "severity", Value: "warning"},
			},
			Channels: []ChannelConfig{
				{
					Type: ChannelWebhook,
					Config: map[string]interface{}{
						"url": "http://alerts.example.com",
					},
				},
			},
		},
	}

	jsonData, err := json.Marshal(request)
	require.NoError(t, err, "Should marshal to JSON without error")

	var deserialized AlertRuleRequest
	err = json.Unmarshal(jsonData, &deserialized)
	require.NoError(t, err, "Should unmarshal from JSON without error")

	assert.Equal(t, request.Name, deserialized.Name)
	assert.Equal(t, request.Source, deserialized.Source)
	assert.Equal(t, request.Enabled, deserialized.Enabled)
	assert.Equal(t, request.RuleType, deserialized.RuleType)
	assert.NotNil(t, deserialized.RouteConfig)
}

func TestAlertCorrelationResponse(t *testing.T) {
	now := time.Now()

	response := AlertCorrelationResponse{
		CorrelationID: "corr-123",
		Alerts: []*UnifiedAlert{
			{
				ID:        "alert-1",
				Source:    SourceMetric,
				AlertName: "GPUMemoryHigh",
				StartsAt:  now,
			},
			{
				ID:        "alert-2",
				Source:    SourceLog,
				AlertName: "OOMError",
				StartsAt:  now.Add(30 * time.Second),
			},
		},
		CorrelationType:  CorrelationTypeCausal,
		CorrelationScore: 0.85,
		Reason:           "high GPU memory leads to OOM",
	}

	jsonData, err := json.Marshal(response)
	require.NoError(t, err, "Should marshal to JSON without error")

	var deserialized AlertCorrelationResponse
	err = json.Unmarshal(jsonData, &deserialized)
	require.NoError(t, err, "Should unmarshal from JSON without error")

	assert.Equal(t, response.CorrelationID, deserialized.CorrelationID)
	assert.Equal(t, len(response.Alerts), len(deserialized.Alerts))
	assert.Equal(t, response.CorrelationType, deserialized.CorrelationType)
	assert.Equal(t, response.CorrelationScore, deserialized.CorrelationScore)
}

func TestMatcherJSON(t *testing.T) {
	matchers := []Matcher{
		{Name: "severity", Value: "critical", IsRegex: false},
		{Name: "workload_id", Value: "prod-.*", IsRegex: true},
	}

	jsonData, err := json.Marshal(matchers)
	require.NoError(t, err)

	var deserialized []Matcher
	err = json.Unmarshal(jsonData, &deserialized)
	require.NoError(t, err)

	assert.Equal(t, len(matchers), len(deserialized))
	assert.Equal(t, matchers[0].Name, deserialized[0].Name)
	assert.Equal(t, matchers[1].IsRegex, deserialized[1].IsRegex)
}

func TestChannelConfigJSON(t *testing.T) {
	configs := []ChannelConfig{
		{
			Type: ChannelWebhook,
			Config: map[string]interface{}{
				"url": "http://webhook.example.com",
			},
		},
		{
			Type: ChannelEmail,
			Config: map[string]interface{}{
				"to":   "team@example.com",
				"from": "noreply@example.com",
			},
		},
	}

	jsonData, err := json.Marshal(configs)
	require.NoError(t, err)

	var deserialized []ChannelConfig
	err = json.Unmarshal(jsonData, &deserialized)
	require.NoError(t, err)

	assert.Equal(t, len(configs), len(deserialized))
	assert.Equal(t, configs[0].Type, deserialized[0].Type)
	assert.NotNil(t, deserialized[1].Config)
}

