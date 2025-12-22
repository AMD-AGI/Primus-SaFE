package transformer

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBaseTransformer(t *testing.T) {
	config := DefaultVLLMConfig()
	transformer := NewBaseTransformer(config)

	assert.Equal(t, "vllm", transformer.GetFramework())
	assert.NotEmpty(t, transformer.mappingIdx)
}

func TestBaseTransformer_Transform(t *testing.T) {
	config := DefaultVLLMConfig()
	transformer := NewBaseTransformer(config)

	t.Run("transform mapped metric", func(t *testing.T) {
		counterType := dto.MetricType_COUNTER
		metrics := []*dto.MetricFamily{
			{
				Name: strPtr("generation_tokens_total"),
				Type: &counterType,
				Metric: []*dto.Metric{
					{
						Counter: &dto.Counter{Value: float64Ptr(1000)},
					},
				},
			},
		}

		workloadLabels := map[string]string{
			"workload_uid": "test-uid",
			"namespace":    "ml-serving",
		}

		result, err := transformer.Transform(metrics, workloadLabels)
		require.NoError(t, err)
		require.Len(t, result, 1)

		// Check that the metric was renamed
		assert.Equal(t, UnifiedMetricNames.TokensGeneratedTotal, *result[0].Name)

		// Check that labels were added
		labelMap := labelsToMap(result[0].Metric[0].Label)
		assert.Equal(t, "test-uid", labelMap["workload_uid"])
		assert.Equal(t, "ml-serving", labelMap["namespace"])
		assert.Equal(t, "vllm", labelMap["framework"])
		assert.Equal(t, "inference", labelMap["framework_type"])
	})

	t.Run("passthrough unmapped metric", func(t *testing.T) {
		gaugeType := dto.MetricType_GAUGE
		metrics := []*dto.MetricFamily{
			{
				Name: strPtr("custom_metric"),
				Type: &gaugeType,
				Metric: []*dto.Metric{
					{
						Gauge: &dto.Gauge{Value: float64Ptr(42)},
					},
				},
			},
		}

		result, err := transformer.Transform(metrics, map[string]string{"workload_uid": "uid1"})
		require.NoError(t, err)
		require.Len(t, result, 1)

		// Name should be preserved
		assert.Equal(t, "custom_metric", *result[0].Name)

		// Labels should still be added
		labelMap := labelsToMap(result[0].Metric[0].Label)
		assert.Equal(t, "uid1", labelMap["workload_uid"])
	})
}

func TestBaseTransformer_ApplyTransform(t *testing.T) {
	transformer := NewBaseTransformer(&FrameworkMetricsConfig{})

	tests := []struct {
		name      string
		value     float64
		transform string
		expected  float64
	}{
		{"divide_by_100", 50.0, "divide_by_100", 0.5},
		{"divide_by_1000", 5000.0, "divide_by_1000", 5.0},
		{"multiply_by_1000", 0.005, "multiply_by_1000", 5.0},
		{"microseconds_to_seconds", 1000000.0, "microseconds_to_seconds", 1.0},
		{"milliseconds_to_seconds", 1500.0, "milliseconds_to_seconds", 1.5},
		{"empty transform", 42.0, "", 42.0},
		{"unknown transform", 42.0, "unknown", 42.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := transformer.applyTransform(tt.value, tt.transform)
			assert.InDelta(t, tt.expected, result, 0.0001)
		})
	}
}

func TestBaseTransformer_MergeLabels(t *testing.T) {
	config := &FrameworkMetricsConfig{
		LabelsAlwaysAdd: map[string]string{
			"framework": "test",
			"env":       "prod",
		},
	}
	transformer := NewBaseTransformer(config)

	metricLabels := []*dto.LabelPair{
		{Name: strPtr("method"), Value: strPtr("POST")},
		{Name: strPtr("env"), Value: strPtr("staging")}, // Should override always-add
	}

	workloadLabels := map[string]string{
		"workload_uid": "uid-123",
		"framework":    "custom", // Should override always-add
	}

	result := transformer.mergeLabels(metricLabels, workloadLabels)
	labelMap := labelsToMap(result)

	// Check label priority: metric labels > workload labels > always-add
	assert.Equal(t, "staging", labelMap["env"])      // from metric labels
	assert.Equal(t, "custom", labelMap["framework"]) // from workload labels (overrides always-add)
	assert.Equal(t, "uid-123", labelMap["workload_uid"])
	assert.Equal(t, "POST", labelMap["method"])
}

func TestBaseTransformer_TransformHistogram(t *testing.T) {
	config := &FrameworkMetricsConfig{
		Framework: "test",
		Mappings: []MetricMapping{
			{Source: "latency_us", Target: "latency_seconds", Type: "histogram", Transform: "microseconds_to_seconds"},
		},
	}
	transformer := NewBaseTransformer(config)

	histType := dto.MetricType_HISTOGRAM
	count := uint64(100)
	sum := float64(5000000) // 5 million microseconds = 5 seconds
	metrics := []*dto.MetricFamily{
		{
			Name: strPtr("latency_us"),
			Type: &histType,
			Metric: []*dto.Metric{
				{
					Histogram: &dto.Histogram{
						SampleCount: &count,
						SampleSum:   &sum,
						Bucket: []*dto.Bucket{
							{CumulativeCount: uint64Ptr(50), UpperBound: float64Ptr(1000)},    // 1ms
							{CumulativeCount: uint64Ptr(80), UpperBound: float64Ptr(10000)},   // 10ms
							{CumulativeCount: uint64Ptr(100), UpperBound: float64Ptr(100000)}, // 100ms
						},
					},
				},
			},
		},
	}

	result, err := transformer.Transform(metrics, nil)
	require.NoError(t, err)
	require.Len(t, result, 1)

	assert.Equal(t, "latency_seconds", *result[0].Name)

	hist := result[0].Metric[0].Histogram
	require.NotNil(t, hist)

	// Check sum was transformed
	assert.InDelta(t, 5.0, *hist.SampleSum, 0.0001)

	// Check buckets were transformed
	assert.InDelta(t, 0.001, *hist.Bucket[0].UpperBound, 0.0001)  // 1ms -> 0.001s
	assert.InDelta(t, 0.01, *hist.Bucket[1].UpperBound, 0.0001)   // 10ms -> 0.01s
	assert.InDelta(t, 0.1, *hist.Bucket[2].UpperBound, 0.0001)    // 100ms -> 0.1s
}

func TestTransformerRegistry(t *testing.T) {
	registry := NewTransformerRegistry()

	t.Run("register and get", func(t *testing.T) {
		transformer := NewBaseTransformer(DefaultVLLMConfig())
		registry.Register("vllm", transformer)

		retrieved, ok := registry.Get("vllm")
		assert.True(t, ok)
		assert.Equal(t, "vllm", retrieved.GetFramework())
	})

	t.Run("get non-existent", func(t *testing.T) {
		_, ok := registry.Get("nonexistent")
		assert.False(t, ok)
	})

	t.Run("get or create", func(t *testing.T) {
		t1 := registry.GetOrCreate("tgi")
		assert.Equal(t, "tgi", t1.GetFramework())

		t2 := registry.GetOrCreate("tgi")
		assert.Equal(t, t1, t2) // Same instance
	})

	t.Run("get or create unknown framework", func(t *testing.T) {
		tr := registry.GetOrCreate("unknown_framework")
		assert.Equal(t, "unknown_framework", tr.GetFramework())
	})

	t.Run("list frameworks", func(t *testing.T) {
		frameworks := registry.Frameworks()
		assert.Contains(t, frameworks, "vllm")
		assert.Contains(t, frameworks, "tgi")
	})
}

func TestBuildWorkloadLabels(t *testing.T) {
	labels := BuildWorkloadLabels("uid-123", "ml-serving", "vllm-0", "cluster-1")

	assert.Equal(t, "uid-123", labels["workload_uid"])
	assert.Equal(t, "ml-serving", labels["namespace"])
	assert.Equal(t, "vllm-0", labels["pod"])
	assert.Equal(t, "cluster-1", labels["cluster"])
}

func TestValidateMapping(t *testing.T) {
	t.Run("valid mapping", func(t *testing.T) {
		m := &MetricMapping{
			Source:    "source_metric",
			Target:    "target_metric",
			Type:      "counter",
			Transform: "divide_by_100",
		}
		err := ValidateMapping(m)
		assert.NoError(t, err)
	})

	t.Run("empty source", func(t *testing.T) {
		m := &MetricMapping{Target: "target"}
		err := ValidateMapping(m)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "source cannot be empty")
	})

	t.Run("empty target", func(t *testing.T) {
		m := &MetricMapping{Source: "source"}
		err := ValidateMapping(m)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "target cannot be empty")
	})

	t.Run("invalid type", func(t *testing.T) {
		m := &MetricMapping{Source: "s", Target: "t", Type: "invalid"}
		err := ValidateMapping(m)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid metric type")
	})

	t.Run("invalid transform", func(t *testing.T) {
		m := &MetricMapping{Source: "s", Target: "t", Transform: "invalid"}
		err := ValidateMapping(m)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid transform")
	})
}

func TestLabelInjector(t *testing.T) {
	injector := NewLabelInjector()

	gaugeType := dto.MetricType_GAUGE
	mf := &dto.MetricFamily{
		Name: strPtr("test_metric"),
		Type: &gaugeType,
		Metric: []*dto.Metric{
			{
				Gauge: &dto.Gauge{Value: float64Ptr(42)},
				Label: []*dto.LabelPair{
					{Name: strPtr("existing"), Value: strPtr("value")},
				},
			},
		},
	}

	labels := map[string]string{
		"workload_uid": "uid-123",
		"namespace":    "default",
	}

	result := injector.InjectLabels(mf, labels)
	labelMap := labelsToMap(result.Metric[0].Label)

	assert.Equal(t, "value", labelMap["existing"])
	assert.Equal(t, "uid-123", labelMap["workload_uid"])
	assert.Equal(t, "default", labelMap["namespace"])
}

func TestTransformWithStats(t *testing.T) {
	config := DefaultVLLMConfig()
	transformer := NewBaseTransformer(config)

	counterType := dto.MetricType_COUNTER
	gaugeType := dto.MetricType_GAUGE
	metrics := []*dto.MetricFamily{
		{
			Name:   strPtr("generation_tokens_total"), // mapped
			Type:   &counterType,
			Metric: []*dto.Metric{{Counter: &dto.Counter{Value: float64Ptr(100)}}},
		},
		{
			Name:   strPtr("custom_metric"), // not mapped
			Type:   &gaugeType,
			Metric: []*dto.Metric{{Gauge: &dto.Gauge{Value: float64Ptr(50)}}},
		},
	}

	stats, transformed, err := transformer.TransformWithStats(metrics, nil)
	require.NoError(t, err)

	assert.Equal(t, "vllm", stats.Framework)
	assert.Equal(t, 2, stats.SourceMetrics)
	assert.Equal(t, 2, stats.TransformedMetrics)
	assert.Equal(t, 1, stats.MappedMetrics)
	assert.Equal(t, 1, stats.PassthroughMetrics)
	assert.Len(t, transformed, 2)
}

// Helper functions
func labelsToMap(labels []*dto.LabelPair) map[string]string {
	m := make(map[string]string)
	for _, lp := range labels {
		if lp.Name != nil && lp.Value != nil {
			m[*lp.Name] = *lp.Value
		}
	}
	return m
}

func uint64Ptr(v uint64) *uint64 {
	return &v
}

