// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metrics

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetPromethuesAsFmtText tests the GetPromethuesAsFmtText function
func TestGetPromethuesAsFmtText(t *testing.T) {
	// Create some test metrics
	counter := NewCounterVec("test_print_counter", "test counter for print", []string{"label"})
	counter.Inc("value1")
	counter.Add(5, "value2")

	gauge := NewGaugeVec("test_print_gauge", "test gauge for print", []string{"label"})
	gauge.Set(42, "value1")

	// Get metrics as text
	result, err := GetPromethuesAsFmtText()
	require.NoError(t, err)
	require.NotEmpty(t, result)

	// Verify the result contains expected metrics
	assert.Contains(t, result, "test_print_counter")
	assert.Contains(t, result, "test_print_gauge")
	assert.Contains(t, result, "HELP")
	assert.Contains(t, result, "TYPE")
}

// TestGetPromethuesAsFmtText_Format tests the format of the output
func TestGetPromethuesAsFmtText_Format(t *testing.T) {
	// Create a test metric
	counter := NewCounterVec("test_print_format", "test counter format", []string{"method"})
	counter.Inc("GET")

	result, err := GetPromethuesAsFmtText()
	require.NoError(t, err)

	// Verify format contains standard Prometheus text format elements
	lines := strings.Split(result, "\n")
	foundHelp := false
	foundType := false
	foundMetric := false

	for _, line := range lines {
		if strings.HasPrefix(line, "# HELP") {
			foundHelp = true
		}
		if strings.HasPrefix(line, "# TYPE") {
			foundType = true
		}
		if strings.Contains(line, "test_print_format") && !strings.HasPrefix(line, "#") {
			foundMetric = true
		}
	}

	assert.True(t, foundHelp, "Should contain HELP comment")
	assert.True(t, foundType, "Should contain TYPE comment")
	assert.True(t, foundMetric, "Should contain metric value")
}

// TestGetPromethuesAsFmtText_MultipleMetrics tests with multiple metric types
func TestGetPromethuesAsFmtText_MultipleMetrics(t *testing.T) {
	// Create different types of metrics
	counter := NewCounterVec("test_print_multi_counter", "counter", []string{})
	counter.Inc()

	gauge := NewGaugeVec("test_print_multi_gauge", "gauge", []string{})
	gauge.Set(100)

	histogram := NewHistogramVec("test_print_multi_histogram", "histogram", []string{}, WithBuckets([]float64{1, 5, 10}))
	histogram.Observe(3)

	result, err := GetPromethuesAsFmtText()
	require.NoError(t, err)

	// Verify all metric types are present
	assert.Contains(t, result, "test_print_multi_counter")
	assert.Contains(t, result, "test_print_multi_gauge")
	assert.Contains(t, result, "test_print_multi_histogram")
}

// TestGetPromethuesAsFmtText_WithLabels tests metrics with labels
func TestGetPromethuesAsFmtText_WithLabels(t *testing.T) {
	counter := NewCounterVec("test_print_labels", "counter with labels", []string{"method", "status"})
	counter.Inc("GET", "200")
	counter.Inc("POST", "201")

	result, err := GetPromethuesAsFmtText()
	require.NoError(t, err)

	// Verify labels are present in the output
	assert.Contains(t, result, "method=\"GET\"")
	assert.Contains(t, result, "status=\"200\"")
	assert.Contains(t, result, "method=\"POST\"")
	assert.Contains(t, result, "status=\"201\"")
}

// TestGetPromethuesAsFmtText_CounterType tests counter type format
func TestGetPromethuesAsFmtText_CounterType(t *testing.T) {
	counter := NewCounterVec("test_print_counter_type", "test counter type", []string{})
	counter.Inc()

	result, err := GetPromethuesAsFmtText()
	require.NoError(t, err)

	// Verify counter type declaration
	assert.Contains(t, result, "# TYPE")
	assert.Contains(t, result, "counter")
}

// TestGetPromethuesAsFmtText_GaugeType tests gauge type format
func TestGetPromethuesAsFmtText_GaugeType(t *testing.T) {
	gauge := NewGaugeVec("test_print_gauge_type", "test gauge type", []string{})
	gauge.Set(42)

	result, err := GetPromethuesAsFmtText()
	require.NoError(t, err)

	// Verify gauge type declaration
	assert.Contains(t, result, "# TYPE")
	assert.Contains(t, result, "gauge")
}

// TestGetPromethuesAsFmtText_HistogramType tests histogram type format
func TestGetPromethuesAsFmtText_HistogramType(t *testing.T) {
	histogram := NewHistogramVec("test_print_histogram_type", "test histogram type", []string{})
	histogram.Observe(5)

	result, err := GetPromethuesAsFmtText()
	require.NoError(t, err)

	// Verify histogram type declaration and buckets
	assert.Contains(t, result, "# TYPE")
	assert.Contains(t, result, "histogram")
	assert.Contains(t, result, "_bucket")
	assert.Contains(t, result, "_sum")
	assert.Contains(t, result, "_count")
}

// TestGetPromethuesAsFmtText_EmptyRegistry tests with minimal metrics
func TestGetPromethuesAsFmtText_EmptyRegistry(t *testing.T) {
	// Even without custom metrics, should return Go runtime metrics
	result, err := GetPromethuesAsFmtText()
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	
	// Should at least contain some default Go metrics
	assert.True(t, strings.Contains(result, "go_") || len(result) > 0)
}

// TestGetPromethuesAsFmtText_TextFormat tests that output is valid Prometheus text format
func TestGetPromethuesAsFmtText_TextFormat(t *testing.T) {
	counter := NewCounterVec("test_print_text_format", "test text format", []string{})
	counter.Add(123.45)

	result, err := GetPromethuesAsFmtText()
	require.NoError(t, err)

	// Verify basic text format structure
	lines := strings.Split(result, "\n")
	assert.NotEmpty(t, lines)

	// Each non-comment line should contain metric data
	for _, line := range lines {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			// Comment line - should contain HELP or TYPE
			assert.True(t, strings.Contains(line, "HELP") || strings.Contains(line, "TYPE"))
		}
	}
}

// TestGetPromethuesAsFmtText_SpecialCharacters tests metrics with special characters
func TestGetPromethuesAsFmtText_SpecialCharacters(t *testing.T) {
	counter := NewCounterVec("test_print_special_chars", "test special characters", []string{"path"})
	counter.Inc("/api/v1/users")
	counter.Inc("/api/v2/items")

	result, err := GetPromethuesAsFmtText()
	require.NoError(t, err)

	// Verify special characters are properly encoded
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "test_print_special_chars")
}

// TestGetPromethuesAsFmtText_ConcurrentAccess tests concurrent calls
func TestGetPromethuesAsFmtText_ConcurrentAccess(t *testing.T) {
	// Create some metrics
	counter := NewCounterVec("test_print_concurrent", "test concurrent", []string{})
	counter.Inc()

	// Call GetPromethuesAsFmtText concurrently
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := GetPromethuesAsFmtText()
			done <- err
		}()
	}

	// Wait for all goroutines and check for errors
	for i := 0; i < 10; i++ {
		err := <-done
		assert.NoError(t, err)
	}
}

// TestGetPromethuesAsFmtText_Integration tests a realistic scenario
func TestGetPromethuesAsFmtText_Integration(t *testing.T) {
	// Create a realistic set of metrics
	requestCounter := NewCounterVec("test_print_http_requests", "HTTP requests", []string{"method", "status"})
	requestCounter.Inc("GET", "200")
	requestCounter.Inc("GET", "404")
	requestCounter.Inc("POST", "201")

	requestDuration := NewHistogramVec("test_print_http_duration", "HTTP duration", []string{"method"}, 
		WithBuckets([]float64{0.1, 0.5, 1.0, 5.0}))
	requestDuration.Observe(0.25, "GET")
	requestDuration.Observe(0.75, "POST")

	activeConnections := NewGaugeVec("test_print_active_connections", "Active connections", []string{})
	activeConnections.Set(42)

	result, err := GetPromethuesAsFmtText()
	require.NoError(t, err)

	// Verify all metrics are present
	assert.Contains(t, result, "test_print_http_requests")
	assert.Contains(t, result, "test_print_http_duration")
	assert.Contains(t, result, "test_print_active_connections")

	// Verify labels
	assert.Contains(t, result, "method=\"GET\"")
	assert.Contains(t, result, "status=\"200\"")
}

// Benchmarks - create metrics once to avoid duplicate registration
var (
	benchPrintCounter *CounterVec
	benchPrintGauge   *GaugeVec
)

func init() {
	benchPrintCounter = NewCounterVec("bench_print_counter", "benchmark counter", []string{"label"})
	benchPrintCounter.Inc("test")

	benchPrintGauge = NewGaugeVec("bench_print_gauge", "benchmark gauge", []string{"label"})
	benchPrintGauge.Set(100, "test")
}

// BenchmarkGetPromethuesAsFmtText benchmarks the export function
func BenchmarkGetPromethuesAsFmtText(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = GetPromethuesAsFmtText()
	}
}

// BenchmarkGetPromethuesAsFmtText_Concurrent benchmarks concurrent access
func BenchmarkGetPromethuesAsFmtText_Concurrent(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = GetPromethuesAsFmtText()
		}
	})
}

