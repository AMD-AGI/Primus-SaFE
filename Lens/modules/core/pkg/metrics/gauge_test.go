package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewGaugeVec tests the NewGaugeVec constructor
func TestNewGaugeVec(t *testing.T) {
	gauge := NewGaugeVec("test_gauge_new", "test gauge help", []string{"label1"})
	require.NotNil(t, gauge)
	require.NotNil(t, gauge.gauges)
}

// TestNewGaugeVec_WithOptions tests NewGaugeVec with various options
func TestNewGaugeVec_WithOptions(t *testing.T) {
	tests := []struct {
		name string
		opts []OptsFunc
	}{
		{
			name: "with namespace",
			opts: []OptsFunc{WithNamespace("gauge_namespace")},
		},
		{
			name: "with labels",
			opts: []OptsFunc{WithLabels(map[string]string{"env": "prod"})},
		},
		{
			name: "without suffix",
			opts: []OptsFunc{WithoutSuffix()},
		},
		{
			name: "multiple options",
			opts: []OptsFunc{
				WithNamespace("gauge_multi"),
				WithLabels(map[string]string{"cluster": "prod"}),
			},
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metricName := "test_gauge_opts_" + string(rune('a'+i))
			gauge := NewGaugeVec(metricName, "test help", []string{"label"}, tt.opts...)
			require.NotNil(t, gauge)
			require.NotNil(t, gauge.gauges)
		})
	}
}

// TestGaugeVec_Inc tests the Inc method
func TestGaugeVec_Inc(t *testing.T) {
	gauge := NewGaugeVec("test_gauge_inc", "test gauge inc", []string{"host"})

	gauge.Inc("server1")
	gauge.Inc("server1")
	gauge.Inc("server2")

	// Verify the gauge values
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_gauge_inc_g" {
			found = true
			metrics := mf.GetMetric()
			require.Len(t, metrics, 2)

			for _, m := range metrics {
				labels := m.GetLabel()
				require.Len(t, labels, 1)

				if labels[0].GetValue() == "server1" {
					assert.Equal(t, float64(2), m.GetGauge().GetValue())
				} else if labels[0].GetValue() == "server2" {
					assert.Equal(t, float64(1), m.GetGauge().GetValue())
				}
			}
		}
	}
	assert.True(t, found, "Metric should be registered")
}

// TestGaugeVec_Dec tests the Dec method
func TestGaugeVec_Dec(t *testing.T) {
	gauge := NewGaugeVec("test_gauge_dec", "test gauge dec", []string{"queue"})

	gauge.Set(10, "jobs")
	gauge.Dec("jobs")
	gauge.Dec("jobs")

	// Verify the gauge value
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_gauge_dec_g" {
			metrics := mf.GetMetric()
			require.Len(t, metrics, 1)
			assert.Equal(t, float64(8), metrics[0].GetGauge().GetValue())
		}
	}
}

// TestGaugeVec_Add tests the Add method
func TestGaugeVec_Add(t *testing.T) {
	gauge := NewGaugeVec("test_gauge_add", "test gauge add", []string{"metric"})

	gauge.Add(5.5, "temperature")
	gauge.Add(2.5, "temperature")
	gauge.Add(-3.0, "temperature")

	// Verify the gauge value
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_gauge_add_g" {
			metrics := mf.GetMetric()
			require.Len(t, metrics, 1)
			assert.InDelta(t, 5.0, metrics[0].GetGauge().GetValue(), 0.01)
		}
	}
}

// TestGaugeVec_Sub tests the Sub method
func TestGaugeVec_Sub(t *testing.T) {
	gauge := NewGaugeVec("test_gauge_sub", "test gauge sub", []string{"resource"})

	gauge.Set(100, "memory")
	gauge.Sub(25.5, "memory")
	gauge.Sub(10.5, "memory")

	// Verify the gauge value
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_gauge_sub_g" {
			metrics := mf.GetMetric()
			require.Len(t, metrics, 1)
			assert.InDelta(t, 64.0, metrics[0].GetGauge().GetValue(), 0.01)
		}
	}
}

// TestGaugeVec_Set tests the Set method
func TestGaugeVec_Set(t *testing.T) {
	gauge := NewGaugeVec("test_gauge_set", "test gauge set", []string{"node"})

	gauge.Set(50, "node1")
	gauge.Set(75, "node1")
	gauge.Set(100, "node2")

	// Verify the gauge values
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_gauge_set_g" {
			metrics := mf.GetMetric()
			require.Len(t, metrics, 2)

			for _, m := range metrics {
				labels := m.GetLabel()
				if labels[0].GetValue() == "node1" {
					assert.Equal(t, float64(75), m.GetGauge().GetValue())
				} else if labels[0].GetValue() == "node2" {
					assert.Equal(t, float64(100), m.GetGauge().GetValue())
				}
			}
		}
	}
}

// TestGaugeVec_Delete tests the Delete method
func TestGaugeVec_Delete(t *testing.T) {
	gauge := NewGaugeVec("test_gauge_delete", "test gauge delete", []string{"service"})

	gauge.Set(10, "api")
	gauge.Set(20, "web")
	gauge.Set(30, "worker")

	// Delete one metric
	gauge.Delete("web")

	// Verify only two metrics remain
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_gauge_delete_g" {
			metrics := mf.GetMetric()
			require.Len(t, metrics, 2)

			// Verify "web" is deleted
			for _, m := range metrics {
				labels := m.GetLabel()
				assert.NotEqual(t, "web", labels[0].GetValue())
			}
		}
	}
}

// TestGaugeVec_MultipleLabels tests gauge with multiple labels
func TestGaugeVec_MultipleLabels(t *testing.T) {
	gauge := NewGaugeVec("test_gauge_multi", "test gauge multi labels", []string{"region", "az", "instance"})

	gauge.Set(100, "us-west", "az1", "i-001")
	gauge.Set(200, "us-west", "az2", "i-002")
	gauge.Set(150, "us-east", "az1", "i-003")

	// Verify metrics
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_gauge_multi_g" {
			found = true
			metrics := mf.GetMetric()
			require.Len(t, metrics, 3)

			for _, m := range metrics {
				labels := m.GetLabel()
				require.Len(t, labels, 3)

				labelMap := getLabelsMap(labels)
				if labelMap["region"] == "us-west" && labelMap["az"] == "az1" {
					assert.Equal(t, float64(100), m.GetGauge().GetValue())
				}
			}
		}
	}
	assert.True(t, found, "Metric should be registered")
}

// TestGaugeVec_NegativeValues tests gauge with negative values
func TestGaugeVec_NegativeValues(t *testing.T) {
	gauge := NewGaugeVec("test_gauge_negative", "test gauge negative", []string{"metric"})

	gauge.Set(10, "value")
	gauge.Add(-5, "value")
	gauge.Sub(8, "value")

	// Verify the gauge value is negative
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_gauge_negative_g" {
			metrics := mf.GetMetric()
			require.Len(t, metrics, 1)
			assert.Equal(t, float64(-3), metrics[0].GetGauge().GetValue())
		}
	}
}

// TestGaugeVec_ConcurrentAccess tests concurrent access to gauge
func TestGaugeVec_ConcurrentAccess(t *testing.T) {
	gauge := NewGaugeVec("test_gauge_concurrent", "test gauge concurrent", []string{"counter"})

	gauge.Set(0, "concurrent")

	// Launch multiple goroutines
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				gauge.Inc("concurrent")
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify total count
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "primus_lens_test_gauge_concurrent_g" {
			metrics := mf.GetMetric()
			require.Len(t, metrics, 1)
			assert.Equal(t, float64(1000), metrics[0].GetGauge().GetValue())
		}
	}
}

// TestGaugeVec_Describe tests the Describe method
func TestGaugeVec_Describe(t *testing.T) {
	gauge := NewGaugeVec("test_gauge_describe", "test gauge describe", []string{"label"})

	descChan := make(chan *prometheus.Desc, 10)
	go func() {
		gauge.Describe(descChan)
		close(descChan)
	}()

	// Verify we get a description
	descs := []prometheus.Desc{}
	for desc := range descChan {
		descs = append(descs, *desc)
	}
	assert.NotEmpty(t, descs)
}

// TestGaugeVec_Collect tests the Collect method
func TestGaugeVec_Collect(t *testing.T) {
	gauge := NewGaugeVec("test_gauge_collect", "test gauge collect", []string{"label"})
	gauge.Set(42, "test")

	metricChan := make(chan prometheus.Metric, 10)
	go func() {
		gauge.Collect(metricChan)
		close(metricChan)
	}()

	// Verify we get metrics
	metrics := []prometheus.Metric{}
	for metric := range metricChan {
		metrics = append(metrics, metric)
	}
	assert.NotEmpty(t, metrics)
}

// Benchmarks - create metrics once to avoid duplicate registration
var (
	benchGaugeSet   *GaugeVec
	benchGaugeInc   *GaugeVec
	benchGaugeAdd   *GaugeVec
	benchGaugeMulti *GaugeVec
)

func init() {
	benchGaugeSet = NewGaugeVec("bench_gauge_set", "benchmark gauge set", []string{"label"})
	benchGaugeInc = NewGaugeVec("bench_gauge_inc", "benchmark gauge inc", []string{"label"})
	benchGaugeAdd = NewGaugeVec("bench_gauge_add", "benchmark gauge add", []string{"label"})
	benchGaugeMulti = NewGaugeVec("bench_gauge_multi", "benchmark gauge multi", []string{"label1", "label2", "label3"})
}

// BenchmarkGaugeVec_Set benchmarks the Set operation
func BenchmarkGaugeVec_Set(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchGaugeSet.Set(float64(i), "test")
	}
}

// BenchmarkGaugeVec_Inc benchmarks the Inc operation
func BenchmarkGaugeVec_Inc(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchGaugeInc.Inc("test")
	}
}

// BenchmarkGaugeVec_Add benchmarks the Add operation
func BenchmarkGaugeVec_Add(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchGaugeAdd.Add(1.5, "test")
	}
}

// BenchmarkGaugeVec_MultipleLabels benchmarks operations with multiple labels
func BenchmarkGaugeVec_MultipleLabels(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchGaugeMulti.Set(float64(i), "val1", "val2", "val3")
	}
}

