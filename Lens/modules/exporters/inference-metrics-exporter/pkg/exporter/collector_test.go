package exporter

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInferenceMetricsCollector(t *testing.T) {
	collector := NewInferenceMetricsCollector()
	assert.NotNil(t, collector)
	assert.NotNil(t, collector.workloadMetrics)
	assert.NotNil(t, collector.desc)
}

func TestInferenceMetricsCollector_UpdateAndRemoveMetrics(t *testing.T) {
	collector := NewInferenceMetricsCollector()

	// Add metrics
	name := "test_metric"
	help := "Test metric help"
	mf := &dto.MetricFamily{
		Name: &name,
		Help: &help,
		Type: dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{Name: strPtr("label1"), Value: strPtr("value1")},
				},
				Gauge: &dto.Gauge{Value: float64Ptr(42.0)},
			},
		},
	}

	collector.UpdateMetrics("workload-1", []*dto.MetricFamily{mf})
	assert.Equal(t, 1, collector.GetWorkloadCount())

	// Add more
	collector.UpdateMetrics("workload-2", []*dto.MetricFamily{mf})
	assert.Equal(t, 2, collector.GetWorkloadCount())

	// Remove one
	collector.RemoveMetrics("workload-1")
	assert.Equal(t, 1, collector.GetWorkloadCount())

	// Remove all
	collector.RemoveMetrics("workload-2")
	assert.Equal(t, 0, collector.GetWorkloadCount())
}

func TestInferenceMetricsCollector_GetAllMetrics(t *testing.T) {
	collector := NewInferenceMetricsCollector()

	name1 := "metric1"
	name2 := "metric2"
	mf1 := &dto.MetricFamily{Name: &name1, Type: dto.MetricType_COUNTER.Enum()}
	mf2 := &dto.MetricFamily{Name: &name2, Type: dto.MetricType_GAUGE.Enum()}

	collector.UpdateMetrics("workload-1", []*dto.MetricFamily{mf1})
	collector.UpdateMetrics("workload-2", []*dto.MetricFamily{mf2})

	all := collector.GetAllMetrics()
	assert.Len(t, all, 2)
	assert.Contains(t, all, "workload-1")
	assert.Contains(t, all, "workload-2")
}

func TestInferenceMetricsCollector_SerializeToText(t *testing.T) {
	collector := NewInferenceMetricsCollector()

	name := "test_gauge"
	help := "A test gauge"
	mf := &dto.MetricFamily{
		Name: &name,
		Help: &help,
		Type: dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{
			{
				Gauge: &dto.Gauge{Value: float64Ptr(123.45)},
			},
		},
	}

	collector.UpdateMetrics("workload-1", []*dto.MetricFamily{mf})

	text, err := collector.SerializeToText()
	require.NoError(t, err)
	assert.Contains(t, string(text), "test_gauge")
	assert.Contains(t, string(text), "123.45")
}

func TestInferenceMetricsCollector_GetStats(t *testing.T) {
	collector := NewInferenceMetricsCollector()

	name1 := "metric1"
	name2 := "metric2"
	mf1 := &dto.MetricFamily{
		Name: &name1,
		Type: dto.MetricType_COUNTER.Enum(),
		Metric: []*dto.Metric{
			{Counter: &dto.Counter{Value: float64Ptr(1)}},
			{Counter: &dto.Counter{Value: float64Ptr(2)}},
		},
	}
	mf2 := &dto.MetricFamily{
		Name: &name2,
		Type: dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{
			{Gauge: &dto.Gauge{Value: float64Ptr(3)}},
		},
	}

	collector.UpdateMetrics("workload-1", []*dto.MetricFamily{mf1})
	collector.UpdateMetrics("workload-2", []*dto.MetricFamily{mf2})

	stats := collector.GetStats()
	assert.Equal(t, 2, stats.WorkloadCount)
	assert.Equal(t, 2, stats.MetricFamilies)
	assert.Equal(t, 3, stats.TotalMetrics) // 2 from mf1 + 1 from mf2
	assert.Equal(t, 2, stats.ByWorkload["workload-1"])
	assert.Equal(t, 1, stats.ByWorkload["workload-2"])
}

func TestInferenceMetricsCollector_ConvertMetric(t *testing.T) {
	collector := NewInferenceMetricsCollector()

	t.Run("counter", func(t *testing.T) {
		name := "counter_metric"
		help := "Counter help"
		mf := &dto.MetricFamily{
			Name: &name,
			Help: &help,
			Type: dto.MetricType_COUNTER.Enum(),
		}
		m := &dto.Metric{
			Label:   []*dto.LabelPair{{Name: strPtr("l1"), Value: strPtr("v1")}},
			Counter: &dto.Counter{Value: float64Ptr(100)},
		}

		metric, err := collector.convertMetric(mf, m, "workload-1")
		require.NoError(t, err)
		assert.NotNil(t, metric)
	})

	t.Run("gauge", func(t *testing.T) {
		name := "gauge_metric"
		help := "Gauge help"
		mf := &dto.MetricFamily{
			Name: &name,
			Help: &help,
			Type: dto.MetricType_GAUGE.Enum(),
		}
		m := &dto.Metric{
			Gauge: &dto.Gauge{Value: float64Ptr(50.5)},
		}

		metric, err := collector.convertMetric(mf, m, "workload-1")
		require.NoError(t, err)
		assert.NotNil(t, metric)
	})

	t.Run("histogram", func(t *testing.T) {
		name := "histogram_metric"
		help := "Histogram help"
		mf := &dto.MetricFamily{
			Name: &name,
			Help: &help,
			Type: dto.MetricType_HISTOGRAM.Enum(),
		}
		m := &dto.Metric{
			Histogram: &dto.Histogram{
				SampleCount: uint64Ptr(10),
				SampleSum:   float64Ptr(100.0),
				Bucket: []*dto.Bucket{
					{UpperBound: float64Ptr(0.1), CumulativeCount: uint64Ptr(2)},
					{UpperBound: float64Ptr(0.5), CumulativeCount: uint64Ptr(7)},
					{UpperBound: float64Ptr(1.0), CumulativeCount: uint64Ptr(10)},
				},
			},
		}

		metric, err := collector.convertMetric(mf, m, "workload-1")
		require.NoError(t, err)
		assert.NotNil(t, metric)
	})

	t.Run("summary", func(t *testing.T) {
		name := "summary_metric"
		help := "Summary help"
		mf := &dto.MetricFamily{
			Name: &name,
			Help: &help,
			Type: dto.MetricType_SUMMARY.Enum(),
		}
		m := &dto.Metric{
			Summary: &dto.Summary{
				SampleCount: uint64Ptr(10),
				SampleSum:   float64Ptr(100.0),
				Quantile: []*dto.Quantile{
					{Quantile: float64Ptr(0.5), Value: float64Ptr(5.0)},
					{Quantile: float64Ptr(0.9), Value: float64Ptr(9.0)},
				},
			},
		}

		metric, err := collector.convertMetric(mf, m, "workload-1")
		require.NoError(t, err)
		assert.NotNil(t, metric)
	})
}

// Helper functions
func strPtr(s string) *string {
	return &s
}

func float64Ptr(f float64) *float64 {
	return &f
}

func uint64Ptr(u uint64) *uint64 {
	return &u
}

