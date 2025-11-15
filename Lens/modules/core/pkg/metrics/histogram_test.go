package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewHistogramVec tests the NewHistogramVec constructor
func TestNewHistogramVec(t *testing.T) {
	histogram := NewHistogramVec("test_histogram_new", "test histogram help", []string{"label1"})
	require.NotNil(t, histogram)
	require.NotNil(t, histogram.histogram)
}

// TestNewHistogramVec_WithCustomBuckets tests NewHistogramVec with custom buckets
func TestNewHistogramVec_WithCustomBuckets(t *testing.T) {
	customBuckets := []float64{0.1, 0.5, 1.0, 2.5, 5.0, 10.0}
	histogram := NewHistogramVec("test_histogram_custom_buckets", "test histogram custom", []string{"endpoint"}, WithBuckets(customBuckets))
	require.NotNil(t, histogram)

	// Observe some values
	histogram.Observe(0.3, "/api/v1")
	histogram.Observe(1.5, "/api/v1")
	histogram.Observe(8.0, "/api/v1")

	// Verify buckets are used
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_histogram_custom_buckets_h" {
			metrics := mf.GetMetric()
			require.Len(t, metrics, 1)

			buckets := metrics[0].GetHistogram().GetBucket()
			// Should have observations in different buckets
			assert.NotEmpty(t, buckets)
		}
	}
}

// TestNewHistogramVec_WithDefaultBuckets tests that default buckets are applied
func TestNewHistogramVec_WithDefaultBuckets(t *testing.T) {
	histogram := NewHistogramVec("test_histogram_default_buckets", "test histogram default", []string{"method"})
	require.NotNil(t, histogram)

	histogram.Observe(0.5, "GET")

	// Verify default buckets are present
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_histogram_default_buckets_h" {
			metrics := mf.GetMetric()
			require.Len(t, metrics, 1)

			buckets := metrics[0].GetHistogram().GetBucket()
			// Default buckets: .0001, .0005, .001, .005, .01, .025, .05, .1, .5, 1, 2.5, 5, 10, 60, 600, 3600
			assert.Len(t, buckets, 16)
		}
	}
}

// TestNewHistogramVec_WithOptions tests NewHistogramVec with various options
func TestNewHistogramVec_WithOptions(t *testing.T) {
	tests := []struct {
		name string
		opts []OptsFunc
	}{
		{
			name: "with namespace",
			opts: []OptsFunc{WithNamespace("histogram_ns")},
		},
		{
			name: "with labels",
			opts: []OptsFunc{WithLabels(map[string]string{"env": "staging"})},
		},
		{
			name: "without suffix",
			opts: []OptsFunc{WithoutSuffix()},
		},
		{
			name: "with custom buckets",
			opts: []OptsFunc{WithBuckets([]float64{1, 5, 10, 50, 100})},
		},
		{
			name: "multiple options",
			opts: []OptsFunc{
				WithNamespace("histo_multi"),
				WithBuckets([]float64{0.5, 1.0, 2.0}),
				WithLabels(map[string]string{"region": "eu"}),
			},
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metricName := "test_histogram_opts_" + string(rune('a'+i))
			histogram := NewHistogramVec(metricName, "test help", []string{"label"}, tt.opts...)
			require.NotNil(t, histogram)
			require.NotNil(t, histogram.histogram)
		})
	}
}

// TestHistogramVec_Observe tests the Observe method
func TestHistogramVec_Observe(t *testing.T) {
	histogram := NewHistogramVec("test_histogram_observe", "test histogram observe", []string{"api"})

	// Observe multiple values
	histogram.Observe(0.1, "v1")
	histogram.Observe(0.5, "v1")
	histogram.Observe(1.5, "v1")
	histogram.Observe(10.0, "v2")

	// Verify observations
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_histogram_observe_h" {
			found = true
			metrics := mf.GetMetric()
			require.Len(t, metrics, 2) // Two label values

			for _, m := range metrics {
				h := m.GetHistogram()
				if m.GetLabel()[0].GetValue() == "v1" {
					assert.Equal(t, uint64(3), h.GetSampleCount())
					assert.InDelta(t, 2.1, h.GetSampleSum(), 0.01)
				} else if m.GetLabel()[0].GetValue() == "v2" {
					assert.Equal(t, uint64(1), h.GetSampleCount())
					assert.InDelta(t, 10.0, h.GetSampleSum(), 0.01)
				}
			}
		}
	}
	assert.True(t, found, "Metric should be registered")
}

// TestHistogramVec_ObserveDistribution tests observation distribution across buckets
func TestHistogramVec_ObserveDistribution(t *testing.T) {
	buckets := []float64{1.0, 5.0, 10.0, 50.0}
	histogram := NewHistogramVec("test_histogram_distribution", "test histogram distribution", []string{"operation"}, WithBuckets(buckets))

	// Observe values in different buckets
	histogram.Observe(0.5, "op1")  // bucket <= 1.0
	histogram.Observe(3.0, "op1")  // bucket <= 5.0
	histogram.Observe(7.0, "op1")  // bucket <= 10.0
	histogram.Observe(30.0, "op1") // bucket <= 50.0
	histogram.Observe(100.0, "op1") // bucket > 50.0

	// Verify bucket distribution
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_histogram_distribution_h" {
			metrics := mf.GetMetric()
			require.Len(t, metrics, 1)

			h := metrics[0].GetHistogram()
			assert.Equal(t, uint64(5), h.GetSampleCount())
			assert.InDelta(t, 140.5, h.GetSampleSum(), 0.01)

			bucketList := h.GetBucket()
			assert.NotEmpty(t, bucketList)
			
			// Verify cumulative counts
			var lastCount uint64 = 0
			for _, b := range bucketList {
				// Cumulative count should be non-decreasing
				assert.GreaterOrEqual(t, b.GetCumulativeCount(), lastCount)
				lastCount = b.GetCumulativeCount()
			}
		}
	}
}

// TestHistogramVec_Delete tests the Delete method
func TestHistogramVec_Delete(t *testing.T) {
	histogram := NewHistogramVec("test_histogram_delete", "test histogram delete", []string{"route"})

	histogram.Observe(1.0, "/health")
	histogram.Observe(2.0, "/metrics")
	histogram.Observe(3.0, "/health")

	// Delete one label
	histogram.Delete("/health")

	// Verify only one metric remains
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_histogram_delete_h" {
			metrics := mf.GetMetric()
			require.Len(t, metrics, 1)
			assert.Equal(t, "/metrics", metrics[0].GetLabel()[0].GetValue())
		}
	}
}

// TestHistogramVec_MultipleLabels tests histogram with multiple labels
func TestHistogramVec_MultipleLabels(t *testing.T) {
	histogram := NewHistogramVec("test_histogram_multi", "test histogram multi labels", []string{"method", "status", "path"})

	histogram.Observe(0.1, "GET", "200", "/api/users")
	histogram.Observe(0.2, "GET", "200", "/api/users")
	histogram.Observe(0.5, "POST", "201", "/api/users")
	histogram.Observe(1.0, "GET", "404", "/api/notfound")

	// Verify metrics
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_histogram_multi_h" {
			found = true
			metrics := mf.GetMetric()
			require.Len(t, metrics, 3) // Three unique label combinations

			for _, m := range metrics {
				labels := m.GetLabel()
				require.Len(t, labels, 3)

				labelMap := getLabelsMap(labels)
				if labelMap["method"] == "GET" && labelMap["status"] == "200" {
					h := m.GetHistogram()
					assert.Equal(t, uint64(2), h.GetSampleCount())
					assert.InDelta(t, 0.3, h.GetSampleSum(), 0.01)
				}
			}
		}
	}
	assert.True(t, found, "Metric should be registered")
}

// TestHistogramVec_ZeroValue tests observing zero values
func TestHistogramVec_ZeroValue(t *testing.T) {
	histogram := NewHistogramVec("test_histogram_zero", "test histogram zero", []string{"metric"})

	histogram.Observe(0, "test")
	histogram.Observe(0, "test")

	// Verify observations
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_histogram_zero_h" {
			metrics := mf.GetMetric()
			require.Len(t, metrics, 1)
			h := metrics[0].GetHistogram()
			assert.Equal(t, uint64(2), h.GetSampleCount())
			assert.Equal(t, float64(0), h.GetSampleSum())
		}
	}
}

// TestHistogramVec_LargeValues tests observing large values
func TestHistogramVec_LargeValues(t *testing.T) {
	histogram := NewHistogramVec("test_histogram_large", "test histogram large", []string{"operation"})

	histogram.Observe(1000.0, "batch")
	histogram.Observe(5000.0, "batch")
	histogram.Observe(10000.0, "batch")

	// Verify observations
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_histogram_large_h" {
			metrics := mf.GetMetric()
			require.Len(t, metrics, 1)
			h := metrics[0].GetHistogram()
			assert.Equal(t, uint64(3), h.GetSampleCount())
			assert.InDelta(t, 16000.0, h.GetSampleSum(), 0.01)
		}
	}
}

// TestHistogramVec_ConcurrentObserve tests concurrent observations
func TestHistogramVec_ConcurrentObserve(t *testing.T) {
	histogram := NewHistogramVec("test_histogram_concurrent", "test histogram concurrent", []string{"worker"})

	// Launch multiple goroutines
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				histogram.Observe(0.5, "worker1")
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify total observations
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_histogram_concurrent_h" {
			metrics := mf.GetMetric()
			require.Len(t, metrics, 1)
			h := metrics[0].GetHistogram()
			assert.Equal(t, uint64(1000), h.GetSampleCount())
			assert.InDelta(t, 500.0, h.GetSampleSum(), 0.01)
		}
	}
}

// Benchmarks - create metrics once to avoid duplicate registration
var (
	benchHistogramObserve *HistogramVec
	benchHistogramMulti   *HistogramVec
	benchHistogramVaried  *HistogramVec
)

func init() {
	benchHistogramObserve = NewHistogramVec("bench_histogram_observe", "benchmark histogram observe", []string{"label"})
	benchHistogramMulti = NewHistogramVec("bench_histogram_multi", "benchmark histogram multi", []string{"label1", "label2", "label3"})
	benchHistogramVaried = NewHistogramVec("bench_histogram_varied", "benchmark histogram varied", []string{"label"})
}

// BenchmarkHistogramVec_Observe benchmarks the Observe operation
func BenchmarkHistogramVec_Observe(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchHistogramObserve.Observe(1.5, "test")
	}
}

// BenchmarkHistogramVec_ObserveMultipleLabels benchmarks Observe with multiple labels
func BenchmarkHistogramVec_ObserveMultipleLabels(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchHistogramMulti.Observe(1.5, "val1", "val2", "val3")
	}
}

// BenchmarkHistogramVec_ObserveVariedValues benchmarks Observe with varied values
func BenchmarkHistogramVec_ObserveVariedValues(b *testing.B) {
	values := []float64{0.001, 0.01, 0.1, 1.0, 10.0}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchHistogramVaried.Observe(values[i%len(values)], "test")
	}
}

