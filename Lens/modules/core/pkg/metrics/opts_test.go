// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMOpts_GetCounterOpts tests the GetCounterOpts method
func TestMOpts_GetCounterOpts(t *testing.T) {
	tests := []struct {
		name           string
		opts           *mOpts
		expectedName   string
		expectedNS     string
		expectedHelp   string
		expectedLabels map[string]string
	}{
		{
			name: "basic counter opts",
			opts: &mOpts{
				name: "requests",
				help: "Total requests",
			},
			expectedName:   "requests_c",
			expectedNS:     "primus_lens",
			expectedHelp:   "Total requests (counters)",
			expectedLabels: nil,
		},
		{
			name: "with custom namespace",
			opts: &mOpts{
				name:      "errors",
				help:      "Error count",
				namespace: stringPtr("custom_ns"),
			},
			expectedName:   "errors_c",
			expectedNS:     "custom_ns",
			expectedHelp:   "Error count (counters)",
			expectedLabels: nil,
		},
		{
			name: "with const labels",
			opts: &mOpts{
				name:   "connections",
				help:   "Active connections",
				labels: map[string]string{"env": "prod", "region": "us-west"},
			},
			expectedName:   "connections_c",
			expectedNS:     "primus_lens",
			expectedHelp:   "Active connections (counters)",
			expectedLabels: map[string]string{"env": "prod", "region": "us-west"},
		},
		{
			name: "without suffix",
			opts: &mOpts{
				name:          "raw_metric",
				help:          "Raw metric",
				withoutSuffix: true,
			},
			expectedName:   "raw_metric",
			expectedNS:     "primus_lens",
			expectedHelp:   "Raw metric (counters)",
			expectedLabels: nil,
		},
		{
			name: "empty help uses name",
			opts: &mOpts{
				name: "test_metric",
				help: "",
			},
			expectedName:   "test_metric_c",
			expectedNS:     "primus_lens",
			expectedHelp:   "test_metric (counters)",
			expectedLabels: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.opts.GetCounterOpts()
			assert.Equal(t, tt.expectedName, result.Name)
			assert.Equal(t, tt.expectedNS, result.Namespace)
			assert.Equal(t, tt.expectedHelp, result.Help)
			// Compare as maps since prometheus.Labels is an alias for map[string]string
			if tt.expectedLabels == nil {
				assert.Nil(t, result.ConstLabels)
			} else {
				assert.Equal(t, map[string]string(tt.expectedLabels), map[string]string(result.ConstLabels))
			}
		})
	}
}

// TestMOpts_GetHistogramOpts tests the GetHistogramOpts method
func TestMOpts_GetHistogramOpts(t *testing.T) {
	tests := []struct {
		name            string
		opts            *mOpts
		expectedName    string
		expectedNS      string
		expectedHelp    string
		expectedBuckets []float64
	}{
		{
			name: "basic histogram opts",
			opts: &mOpts{
				name:    "duration",
				help:    "Request duration",
				buckets: []float64{0.1, 0.5, 1.0, 5.0},
			},
			expectedName:    "duration_h",
			expectedNS:      "primus_lens",
			expectedHelp:    "Request duration (histogram)",
			expectedBuckets: []float64{0.1, 0.5, 1.0, 5.0},
		},
		{
			name: "with custom namespace",
			opts: &mOpts{
				name:      "latency",
				help:      "API latency",
				namespace: stringPtr("api_ns"),
				buckets:   []float64{1, 5, 10},
			},
			expectedName:    "latency_h",
			expectedNS:      "api_ns",
			expectedHelp:    "API latency (histogram)",
			expectedBuckets: []float64{1, 5, 10},
		},
		{
			name: "without suffix",
			opts: &mOpts{
				name:          "raw_histogram",
				help:          "Raw histogram",
				buckets:       []float64{1, 10, 100},
				withoutSuffix: true,
			},
			expectedName:    "raw_histogram",
			expectedNS:      "primus_lens",
			expectedHelp:    "Raw histogram (histogram)",
			expectedBuckets: []float64{1, 10, 100},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.opts.GetHistogramOpts()
			assert.Equal(t, tt.expectedName, result.Name)
			assert.Equal(t, tt.expectedNS, result.Namespace)
			assert.Equal(t, tt.expectedHelp, result.Help)
			assert.Equal(t, tt.expectedBuckets, result.Buckets)
		})
	}
}

// TestMOpts_GetSummaryOpts tests the GetSummaryOpts method
func TestMOpts_GetSummaryOpts(t *testing.T) {
	tests := []struct {
		name             string
		opts             *mOpts
		expectedName     string
		expectedNS       string
		expectedHelp     string
		expectedQuantile map[float64]float64
	}{
		{
			name: "basic summary opts",
			opts: &mOpts{
				name:     "response_time",
				help:     "Response time",
				quantile: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			},
			expectedName:     "response_time_s",
			expectedNS:       "primus_lens",
			expectedHelp:     "Response time (summary)",
			expectedQuantile: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		{
			name: "with custom namespace",
			opts: &mOpts{
				name:      "processing_time",
				help:      "Processing time",
				namespace: stringPtr("worker_ns"),
				quantile:  map[float64]float64{0.5: 0.05, 0.95: 0.01},
			},
			expectedName:     "processing_time_s",
			expectedNS:       "worker_ns",
			expectedHelp:     "Processing time (summary)",
			expectedQuantile: map[float64]float64{0.5: 0.05, 0.95: 0.01},
		},
		{
			name: "without suffix",
			opts: &mOpts{
				name:          "raw_summary",
				help:          "Raw summary",
				quantile:      map[float64]float64{0.5: 0.05},
				withoutSuffix: true,
			},
			expectedName:     "raw_summary",
			expectedNS:       "primus_lens",
			expectedHelp:     "Raw summary (summary)",
			expectedQuantile: map[float64]float64{0.5: 0.05},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.opts.GetSummaryOpts()
			assert.Equal(t, tt.expectedName, result.Name)
			assert.Equal(t, tt.expectedNS, result.Namespace)
			assert.Equal(t, tt.expectedHelp, result.Help)
			assert.Equal(t, tt.expectedQuantile, result.Objectives)
		})
	}
}

// TestMOpts_GetGaugeOpts tests the GetGaugeOpts method
func TestMOpts_GetGaugeOpts(t *testing.T) {
	tests := []struct {
		name           string
		opts           *mOpts
		expectedName   string
		expectedNS     string
		expectedHelp   string
		expectedLabels map[string]string
	}{
		{
			name: "basic gauge opts",
			opts: &mOpts{
				name: "memory_usage",
				help: "Memory usage",
			},
			expectedName:   "memory_usage_g",
			expectedNS:     "primus_lens",
			expectedHelp:   "Memory usage (gauge)",
			expectedLabels: nil,
		},
		{
			name: "with custom namespace",
			opts: &mOpts{
				name:      "cpu_usage",
				help:      "CPU usage",
				namespace: stringPtr("system_ns"),
			},
			expectedName:   "cpu_usage_g",
			expectedNS:     "system_ns",
			expectedHelp:   "CPU usage (gauge)",
			expectedLabels: nil,
		},
		{
			name: "with const labels",
			opts: &mOpts{
				name:   "disk_space",
				help:   "Disk space",
				labels: map[string]string{"mount": "/data"},
			},
			expectedName:   "disk_space_g",
			expectedNS:     "primus_lens",
			expectedHelp:   "Disk space (gauge)",
			expectedLabels: map[string]string{"mount": "/data"},
		},
		{
			name: "without suffix",
			opts: &mOpts{
				name:          "raw_gauge",
				help:          "Raw gauge",
				withoutSuffix: true,
			},
			expectedName:   "raw_gauge",
			expectedNS:     "primus_lens",
			expectedHelp:   "Raw gauge (gauge)",
			expectedLabels: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.opts.GetGaugeOpts()
			assert.Equal(t, tt.expectedName, result.Name)
			assert.Equal(t, tt.expectedNS, result.Namespace)
			assert.Equal(t, tt.expectedHelp, result.Help)
			// Compare as maps since prometheus.Labels is an alias for map[string]string
			if tt.expectedLabels == nil {
				assert.Nil(t, result.ConstLabels)
			} else {
				assert.Equal(t, map[string]string(tt.expectedLabels), map[string]string(result.ConstLabels))
			}
		})
	}
}

// TestWithNamespace tests the WithNamespace option function
func TestWithNamespace(t *testing.T) {
	opts := &mOpts{name: "test", help: "test"}
	WithNamespace("custom_namespace")(opts)

	require.NotNil(t, opts.namespace)
	assert.Equal(t, "custom_namespace", *opts.namespace)
}

// TestWithBuckets tests the WithBuckets option function
func TestWithBuckets(t *testing.T) {
	buckets := []float64{0.1, 1.0, 10.0}
	opts := &mOpts{name: "test", help: "test"}
	WithBuckets(buckets)(opts)

	assert.Equal(t, buckets, opts.buckets)
}

// TestWithLabels tests the WithLabels option function
func TestWithLabels(t *testing.T) {
	labels := map[string]string{"env": "prod", "region": "us"}
	opts := &mOpts{name: "test", help: "test"}
	WithLabels(labels)(opts)

	assert.Equal(t, labels, opts.labels)
}

// TestWithQuantile tests the WithQuantile option function
func TestWithQuantile(t *testing.T) {
	quantile := map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
	opts := &mOpts{name: "test", help: "test"}
	WithQuantile(quantile)(opts)

	assert.Equal(t, quantile, opts.quantile)
}

// TestWithoutSuffix tests the WithoutSuffix option function
func TestWithoutSuffix(t *testing.T) {
	opts := &mOpts{name: "test", help: "test"}
	WithoutSuffix()(opts)

	assert.True(t, opts.withoutSuffix)
}

// TestMultipleOptions tests applying multiple option functions
func TestMultipleOptions(t *testing.T) {
	opts := &mOpts{name: "test", help: "test"}

	optFuncs := []OptsFunc{
		WithNamespace("multi_ns"),
		WithBuckets([]float64{1, 5, 10}),
		WithLabels(map[string]string{"cluster": "prod"}),
		WithQuantile(map[float64]float64{0.5: 0.05}),
		WithoutSuffix(),
	}

	for _, fn := range optFuncs {
		fn(opts)
	}

	require.NotNil(t, opts.namespace)
	assert.Equal(t, "multi_ns", *opts.namespace)
	assert.Equal(t, []float64{1, 5, 10}, opts.buckets)
	assert.Equal(t, map[string]string{"cluster": "prod"}, opts.labels)
	assert.Equal(t, map[float64]float64{0.5: 0.05}, opts.quantile)
	assert.True(t, opts.withoutSuffix)
}

// TestOptsWithNilNamespace tests behavior with nil namespace
func TestOptsWithNilNamespace(t *testing.T) {
	opts := &mOpts{
		name:      "test_metric",
		help:      "test help",
		namespace: nil,
	}

	counterOpts := opts.GetCounterOpts()
	assert.Equal(t, DefaultMetricsNamespace, counterOpts.Namespace)

	gaugeOpts := opts.GetGaugeOpts()
	assert.Equal(t, DefaultMetricsNamespace, gaugeOpts.Namespace)

	histogramOpts := opts.GetHistogramOpts()
	assert.Equal(t, DefaultMetricsNamespace, histogramOpts.Namespace)

	summaryOpts := opts.GetSummaryOpts()
	assert.Equal(t, DefaultMetricsNamespace, summaryOpts.Namespace)
}

// TestOptsWithEmptyBuckets tests behavior with empty buckets
func TestOptsWithEmptyBuckets(t *testing.T) {
	opts := &mOpts{
		name:    "test_metric",
		help:    "test help",
		buckets: []float64{},
	}

	histogramOpts := opts.GetHistogramOpts()
	assert.Empty(t, histogramOpts.Buckets)
}

// TestOptsWithEmptyLabels tests behavior with empty labels map
func TestOptsWithEmptyLabels(t *testing.T) {
	opts := &mOpts{
		name:   "test_metric",
		help:   "test help",
		labels: map[string]string{},
	}

	counterOpts := opts.GetCounterOpts()
	assert.Empty(t, counterOpts.ConstLabels)
}

// TestOptsWithEmptyQuantile tests behavior with empty quantile map
func TestOptsWithEmptyQuantile(t *testing.T) {
	opts := &mOpts{
		name:     "test_metric",
		help:     "test help",
		quantile: map[float64]float64{},
	}

	summaryOpts := opts.GetSummaryOpts()
	assert.Empty(t, summaryOpts.Objectives)
}

// BenchmarkGetCounterOpts benchmarks GetCounterOpts
func BenchmarkGetCounterOpts(b *testing.B) {
	opts := &mOpts{
		name: "test_counter",
		help: "test counter",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = opts.GetCounterOpts()
	}
}

// BenchmarkGetHistogramOpts benchmarks GetHistogramOpts
func BenchmarkGetHistogramOpts(b *testing.B) {
	opts := &mOpts{
		name:    "test_histogram",
		help:    "test histogram",
		buckets: []float64{0.1, 0.5, 1.0, 5.0},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = opts.GetHistogramOpts()
	}
}

// BenchmarkGetSummaryOpts benchmarks GetSummaryOpts
func BenchmarkGetSummaryOpts(b *testing.B) {
	opts := &mOpts{
		name:     "test_summary",
		help:     "test summary",
		quantile: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = opts.GetSummaryOpts()
	}
}

// BenchmarkGetGaugeOpts benchmarks GetGaugeOpts
func BenchmarkGetGaugeOpts(b *testing.B) {
	opts := &mOpts{
		name: "test_gauge",
		help: "test gauge",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = opts.GetGaugeOpts()
	}
}

// Helper function to create a string pointer
func stringPtr(s string) *string {
	return &s
}

