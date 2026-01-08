// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewExecutionStats(t *testing.T) {
	stats := NewExecutionStats()

	assert.NotNil(t, stats, "NewExecutionStats should return non-nil stats")
	assert.NotNil(t, stats.CustomMetrics, "CustomMetrics should be initialized")
	assert.NotNil(t, stats.Messages, "Messages should be initialized")
	assert.Empty(t, stats.CustomMetrics, "CustomMetrics should be empty initially")
	assert.Empty(t, stats.Messages, "Messages should be empty initially")
	assert.Equal(t, int64(0), stats.RecordsProcessed, "RecordsProcessed should be 0")
	assert.Equal(t, int64(0), stats.BytesTransferred, "BytesTransferred should be 0")
	assert.Equal(t, int64(0), stats.ItemsCreated, "ItemsCreated should be 0")
	assert.Equal(t, int64(0), stats.ItemsUpdated, "ItemsUpdated should be 0")
	assert.Equal(t, int64(0), stats.ItemsDeleted, "ItemsDeleted should be 0")
	assert.Equal(t, int64(0), stats.CacheHits, "CacheHits should be 0")
	assert.Equal(t, int64(0), stats.CacheMisses, "CacheMisses should be 0")
	assert.Equal(t, int64(0), stats.ErrorCount, "ErrorCount should be 0")
	assert.Equal(t, int64(0), stats.WarningCount, "WarningCount should be 0")
	assert.Equal(t, float64(0), stats.QueryDuration, "QueryDuration should be 0")
	assert.Equal(t, float64(0), stats.ProcessDuration, "ProcessDuration should be 0")
	assert.Equal(t, float64(0), stats.SaveDuration, "SaveDuration should be 0")
}

func TestAddMessage(t *testing.T) {
	tests := []struct {
		name     string
		messages []string
		expected []string
	}{
		{
			name:     "add single message",
			messages: []string{"test message"},
			expected: []string{"test message"},
		},
		{
			name:     "add multiple messages",
			messages: []string{"message 1", "message 2", "message 3"},
			expected: []string{"message 1", "message 2", "message 3"},
		},
		{
			name:     "add empty message",
			messages: []string{""},
			expected: []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := NewExecutionStats()

			for _, msg := range tt.messages {
				stats.AddMessage(msg)
			}

			assert.Equal(t, tt.expected, stats.Messages, "Messages should match expected")
		})
	}
}

func TestAddMessageWithNilMessages(t *testing.T) {
	stats := &ExecutionStats{
		Messages: nil,
	}

	stats.AddMessage("test message")

	assert.NotNil(t, stats.Messages, "Messages should be initialized")
	assert.Len(t, stats.Messages, 1, "Should have one message")
	assert.Equal(t, "test message", stats.Messages[0], "Message content should match")
}

func TestAddCustomMetric(t *testing.T) {
	tests := []struct {
		name     string
		metrics  map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "add string metric",
			metrics: map[string]interface{}{
				"status": "success",
			},
			expected: map[string]interface{}{
				"status": "success",
			},
		},
		{
			name: "add int metric",
			metrics: map[string]interface{}{
				"count": 42,
			},
			expected: map[string]interface{}{
				"count": 42,
			},
		},
		{
			name: "add float metric",
			metrics: map[string]interface{}{
				"rate": 3.14,
			},
			expected: map[string]interface{}{
				"rate": 3.14,
			},
		},
		{
			name: "add multiple metrics",
			metrics: map[string]interface{}{
				"count":  10,
				"rate":   2.5,
				"status": "ok",
			},
			expected: map[string]interface{}{
				"count":  10,
				"rate":   2.5,
				"status": "ok",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := NewExecutionStats()

			for key, value := range tt.metrics {
				stats.AddCustomMetric(key, value)
			}

			assert.Equal(t, tt.expected, stats.CustomMetrics, "CustomMetrics should match expected")
		})
	}
}

func TestAddCustomMetricWithNilMetrics(t *testing.T) {
	stats := &ExecutionStats{
		CustomMetrics: nil,
	}

	stats.AddCustomMetric("test_key", "test_value")

	assert.NotNil(t, stats.CustomMetrics, "CustomMetrics should be initialized")
	assert.Len(t, stats.CustomMetrics, 1, "Should have one metric")
	assert.Equal(t, "test_value", stats.CustomMetrics["test_key"], "Metric value should match")
}

func TestAddCustomMetricOverwrite(t *testing.T) {
	stats := NewExecutionStats()

	stats.AddCustomMetric("key", "value1")
	assert.Equal(t, "value1", stats.CustomMetrics["key"], "First value should be set")

	stats.AddCustomMetric("key", "value2")
	assert.Equal(t, "value2", stats.CustomMetrics["key"], "Value should be overwritten")
}

func TestMerge(t *testing.T) {
	tests := []struct {
		name     string
		base     *ExecutionStats
		other    *ExecutionStats
		expected *ExecutionStats
	}{
		{
			name: "merge with populated stats",
			base: &ExecutionStats{
				RecordsProcessed: 10,
				BytesTransferred: 100,
				ItemsCreated:     5,
				ItemsUpdated:     3,
				ItemsDeleted:     2,
				CacheHits:        20,
				CacheMisses:      5,
				QueryDuration:    1.5,
				ProcessDuration:  2.5,
				SaveDuration:     0.5,
				ErrorCount:       1,
				WarningCount:     2,
				CustomMetrics: map[string]interface{}{
					"metric1": 100,
				},
				Messages: []string{"message1"},
			},
			other: &ExecutionStats{
				RecordsProcessed: 5,
				BytesTransferred: 50,
				ItemsCreated:     2,
				ItemsUpdated:     1,
				ItemsDeleted:     1,
				CacheHits:        10,
				CacheMisses:      2,
				QueryDuration:    0.5,
				ProcessDuration:  1.0,
				SaveDuration:     0.2,
				ErrorCount:       0,
				WarningCount:     1,
				CustomMetrics: map[string]interface{}{
					"metric2": 200,
				},
				Messages: []string{"message2"},
			},
			expected: &ExecutionStats{
				RecordsProcessed: 15,
				BytesTransferred: 150,
				ItemsCreated:     7,
				ItemsUpdated:     4,
				ItemsDeleted:     3,
				CacheHits:        30,
				CacheMisses:      7,
				QueryDuration:    2.0,
				ProcessDuration:  3.5,
				SaveDuration:     0.7,
				ErrorCount:       1,
				WarningCount:     3,
				CustomMetrics: map[string]interface{}{
					"metric1": 100,
					"metric2": 200,
				},
				Messages: []string{"message1", "message2"},
			},
		},
		{
			name: "merge with nil other",
			base: &ExecutionStats{
				RecordsProcessed: 10,
				CustomMetrics: map[string]interface{}{
					"key": "value",
				},
				Messages: []string{"message"},
			},
			other: nil,
			expected: &ExecutionStats{
				RecordsProcessed: 10,
				CustomMetrics: map[string]interface{}{
					"key": "value",
				},
				Messages: []string{"message"},
			},
		},
		{
			name:  "merge with empty base",
			base:  NewExecutionStats(),
			other: &ExecutionStats{
				RecordsProcessed: 5,
				ItemsCreated:     2,
				CustomMetrics: map[string]interface{}{
					"key": "value",
				},
				Messages: []string{"message"},
			},
			expected: &ExecutionStats{
				RecordsProcessed: 5,
				ItemsCreated:     2,
				CustomMetrics: map[string]interface{}{
					"key": "value",
				},
				Messages: []string{"message"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.base.Merge(tt.other)

			assert.Equal(t, tt.expected.RecordsProcessed, tt.base.RecordsProcessed)
			assert.Equal(t, tt.expected.BytesTransferred, tt.base.BytesTransferred)
			assert.Equal(t, tt.expected.ItemsCreated, tt.base.ItemsCreated)
			assert.Equal(t, tt.expected.ItemsUpdated, tt.base.ItemsUpdated)
			assert.Equal(t, tt.expected.ItemsDeleted, tt.base.ItemsDeleted)
			assert.Equal(t, tt.expected.CacheHits, tt.base.CacheHits)
			assert.Equal(t, tt.expected.CacheMisses, tt.base.CacheMisses)
			assert.InDelta(t, tt.expected.QueryDuration, tt.base.QueryDuration, 0.0001)
			assert.InDelta(t, tt.expected.ProcessDuration, tt.base.ProcessDuration, 0.0001)
			assert.InDelta(t, tt.expected.SaveDuration, tt.base.SaveDuration, 0.0001)
			assert.Equal(t, tt.expected.ErrorCount, tt.base.ErrorCount)
			assert.Equal(t, tt.expected.WarningCount, tt.base.WarningCount)
			assert.Equal(t, tt.expected.CustomMetrics, tt.base.CustomMetrics)
			assert.Equal(t, tt.expected.Messages, tt.base.Messages)
		})
	}
}

func TestMergeCustomMetricsOverwrite(t *testing.T) {
	base := &ExecutionStats{
		CustomMetrics: map[string]interface{}{
			"shared_key": "original",
			"base_key":   "base",
		},
	}

	other := &ExecutionStats{
		CustomMetrics: map[string]interface{}{
			"shared_key": "overwritten",
			"other_key":  "other",
		},
	}

	base.Merge(other)

	assert.Equal(t, "overwritten", base.CustomMetrics["shared_key"], "Shared key should be overwritten")
	assert.Equal(t, "base", base.CustomMetrics["base_key"], "Base key should remain")
	assert.Equal(t, "other", base.CustomMetrics["other_key"], "Other key should be added")
}

func TestMergeWithNilCustomMetrics(t *testing.T) {
	base := &ExecutionStats{
		CustomMetrics: nil,
	}

	other := &ExecutionStats{
		CustomMetrics: map[string]interface{}{
			"key": "value",
		},
	}

	base.Merge(other)

	assert.NotNil(t, base.CustomMetrics, "CustomMetrics should be initialized")
	assert.Equal(t, "value", base.CustomMetrics["key"], "Metric should be merged")
}

func TestMergeWithNilMessages(t *testing.T) {
	base := &ExecutionStats{
		Messages: nil,
	}

	other := &ExecutionStats{
		Messages: []string{"message1", "message2"},
	}

	base.Merge(other)

	assert.NotNil(t, base.Messages, "Messages should be initialized")
	assert.Equal(t, []string{"message1", "message2"}, base.Messages, "Messages should be merged")
}

