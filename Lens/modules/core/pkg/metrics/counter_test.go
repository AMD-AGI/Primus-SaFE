// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewCounterVec tests the NewCounterVec constructor
func TestNewCounterVec(t *testing.T) {
	counter := NewCounterVec("test_counter_new", "test counter help", []string{"label1", "label2"})
	require.NotNil(t, counter)
	require.NotNil(t, counter.counters)
}

// TestNewCounterVec_WithOptions tests NewCounterVec with various options
func TestNewCounterVec_WithOptions(t *testing.T) {
	tests := []struct {
		name string
		opts []OptsFunc
	}{
		{
			name: "with namespace",
			opts: []OptsFunc{WithNamespace("custom_namespace")},
		},
		{
			name: "with labels",
			opts: []OptsFunc{WithLabels(map[string]string{"env": "test", "service": "api"})},
		},
		{
			name: "without suffix",
			opts: []OptsFunc{WithoutSuffix()},
		},
		{
			name: "multiple options",
			opts: []OptsFunc{
				WithNamespace("multi_ns"),
				WithLabels(map[string]string{"region": "us-west"}),
				WithoutSuffix(),
			},
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metricName := "test_counter_opts_" + string(rune('a'+i))
			counter := NewCounterVec(metricName, "test help", []string{"label"}, tt.opts...)
			require.NotNil(t, counter)
			require.NotNil(t, counter.counters)
		})
	}
}

// TestCounterVec_Inc tests the Inc method
func TestCounterVec_Inc(t *testing.T) {
	counter := NewCounterVec("test_counter_inc", "test counter inc", []string{"status"})

	// Increment with label
	counter.Inc("200")
	counter.Inc("200")
	counter.Inc("500")

	// Verify the counter values
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_counter_inc_c" {
			found = true
			metrics := mf.GetMetric()
			require.Len(t, metrics, 2) // Two unique label values

			for _, m := range metrics {
				labels := m.GetLabel()
				require.Len(t, labels, 1)

				if labels[0].GetValue() == "200" {
					assert.Equal(t, float64(2), m.GetCounter().GetValue())
				} else if labels[0].GetValue() == "500" {
					assert.Equal(t, float64(1), m.GetCounter().GetValue())
				}
			}
		}
	}
	assert.True(t, found, "Metric should be registered")
}

// TestCounterVec_Add tests the Add method
func TestCounterVec_Add(t *testing.T) {
	counter := NewCounterVec("test_counter_add", "test counter add", []string{"method"})

	// Add different values
	counter.Add(10.5, "GET")
	counter.Add(5.25, "GET")
	counter.Add(100, "POST")

	// Verify the counter values
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_counter_add_c" {
			found = true
			metrics := mf.GetMetric()

			for _, m := range metrics {
				labels := m.GetLabel()
				require.Len(t, labels, 1)

				if labels[0].GetValue() == "GET" {
					assert.InDelta(t, 15.75, m.GetCounter().GetValue(), 0.01)
				} else if labels[0].GetValue() == "POST" {
					assert.InDelta(t, 100.0, m.GetCounter().GetValue(), 0.01)
				}
			}
		}
	}
	assert.True(t, found, "Metric should be registered")
}

// TestCounterVec_Delete tests the Delete method
func TestCounterVec_Delete(t *testing.T) {
	counter := NewCounterVec("test_counter_delete", "test counter delete", []string{"endpoint"})

	// Add some metrics
	counter.Inc("/api/v1")
	counter.Inc("/api/v2")
	counter.Inc("/api/v1")

	// Delete one label
	counter.Delete("/api/v1")

	// Verify only one metric remains
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_counter_delete_c" {
			metrics := mf.GetMetric()
			require.Len(t, metrics, 1)
			assert.Equal(t, "/api/v2", metrics[0].GetLabel()[0].GetValue())
		}
	}
}

// TestCounterVec_MultipleLabels tests counter with multiple labels
func TestCounterVec_MultipleLabels(t *testing.T) {
	counter := NewCounterVec("test_counter_multi", "test counter multi labels", []string{"method", "status", "path"})

	counter.Inc("GET", "200", "/api/users")
	counter.Inc("GET", "200", "/api/users")
	counter.Inc("POST", "201", "/api/users")
	counter.Inc("GET", "404", "/api/notfound")

	// Verify metrics
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_counter_multi_c" {
			found = true
			metrics := mf.GetMetric()
			require.Len(t, metrics, 3) // Three unique label combinations

			for _, m := range metrics {
				labels := m.GetLabel()
				require.Len(t, labels, 3)

				// Find the GET/200/users combination
				if getLabelsMap(labels)["method"] == "GET" &&
					getLabelsMap(labels)["status"] == "200" &&
					getLabelsMap(labels)["path"] == "/api/users" {
					assert.Equal(t, float64(2), m.GetCounter().GetValue())
				}
			}
		}
	}
	assert.True(t, found, "Metric should be registered")
}

// TestCounterVec_ZeroLabels tests counter with no labels
func TestCounterVec_ZeroLabels(t *testing.T) {
	counter := NewCounterVec("test_counter_zero_labels", "test counter no labels", []string{})

	counter.Inc()
	counter.Inc()
	counter.Add(5.5)

	// Verify metric value
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_counter_zero_labels_c" {
			metrics := mf.GetMetric()
			require.Len(t, metrics, 1)
			assert.InDelta(t, 7.5, metrics[0].GetCounter().GetValue(), 0.01)
		}
	}
}

// TestCounterVec_ConcurrentAccess tests concurrent access to counter
func TestCounterVec_ConcurrentAccess(t *testing.T) {
	counter := NewCounterVec("test_counter_concurrent", "test counter concurrent", []string{"worker"})

	// Launch multiple goroutines
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				counter.Inc("worker1")
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify total count
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_counter_concurrent_c" {
			metrics := mf.GetMetric()
			require.Len(t, metrics, 1)
			assert.Equal(t, float64(1000), metrics[0].GetCounter().GetValue())
		}
	}
}

// Benchmarks - create metrics once to avoid duplicate registration
var (
	benchCounter      *CounterVec
	benchCounterAdd   *CounterVec
	benchCounterMulti *CounterVec
)

func init() {
	benchCounter = NewCounterVec("bench_counter_inc", "benchmark counter inc", []string{"label"})
	benchCounterAdd = NewCounterVec("bench_counter_add", "benchmark counter add", []string{"label"})
	benchCounterMulti = NewCounterVec("bench_counter_multi", "benchmark counter multi", []string{"label1", "label2", "label3"})
}

// BenchmarkCounterVec_Inc benchmarks the Inc operation
func BenchmarkCounterVec_Inc(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchCounter.Inc("test")
	}
}

// BenchmarkCounterVec_Add benchmarks the Add operation
func BenchmarkCounterVec_Add(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchCounterAdd.Add(1.5, "test")
	}
}

// BenchmarkCounterVec_IncMultipleLabels benchmarks Inc with multiple labels
func BenchmarkCounterVec_IncMultipleLabels(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchCounterMulti.Inc("val1", "val2", "val3")
	}
}

// Helper function to convert label array to map
func getLabelsMap(labels []*dto.LabelPair) map[string]string {
	result := make(map[string]string)
	for _, label := range labels {
		result[label.GetName()] = label.GetValue()
	}
	return result
}
