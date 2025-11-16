package model

import (
	"encoding/json"
	"testing"

	promModel "github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTimePoint tests the TimePoint struct
func TestTimePoint(t *testing.T) {
	tp := TimePoint{
		Timestamp: 1609459200,
		Value:     42.5,
	}

	assert.Equal(t, int64(1609459200), tp.Timestamp)
	assert.Equal(t, 42.5, tp.Value)
}

// TestTimePoint_JSONMarshal tests JSON marshaling
func TestTimePoint_JSONMarshal(t *testing.T) {
	tp := TimePoint{
		Timestamp: 1234567890,
		Value:     99.99,
	}

	data, err := json.Marshal(tp)
	require.NoError(t, err)

	var decoded TimePoint
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, tp.Timestamp, decoded.Timestamp)
	assert.InDelta(t, tp.Value, decoded.Value, 0.01)
}

// TestTimePoint_ZeroValues tests TimePoint with zero values
func TestTimePoint_ZeroValues(t *testing.T) {
	tp := TimePoint{}

	assert.Equal(t, int64(0), tp.Timestamp)
	assert.Equal(t, 0.0, tp.Value)
}

// TestMetricsSeries tests the MetricsSeries struct
func TestMetricsSeries(t *testing.T) {
	series := MetricsSeries{
		Labels: promModel.Metric{
			"__name__": "cpu_usage",
			"host":     "server1",
		},
		Values: []TimePoint{
			{Timestamp: 1000, Value: 10.0},
			{Timestamp: 2000, Value: 20.0},
		},
	}

	assert.Len(t, series.Labels, 2)
	assert.Equal(t, "cpu_usage", string(series.Labels["__name__"]))
	assert.Len(t, series.Values, 2)
}

// TestMetricsSeries_JSONMarshal tests JSON marshaling
func TestMetricsSeries_JSONMarshal(t *testing.T) {
	series := MetricsSeries{
		Labels: promModel.Metric{
			"metric": "test",
		},
		Values: []TimePoint{
			{Timestamp: 1000, Value: 1.0},
		},
	}

	data, err := json.Marshal(series)
	require.NoError(t, err)

	var decoded MetricsSeries
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.NotNil(t, decoded.Labels)
	assert.Len(t, decoded.Values, 1)
}

// TestMetricsSeries_EmptyValues tests series with empty values
func TestMetricsSeries_EmptyValues(t *testing.T) {
	series := MetricsSeries{
		Labels: promModel.Metric{
			"metric": "empty",
		},
		Values: []TimePoint{},
	}

	assert.NotNil(t, series.Values)
	assert.Len(t, series.Values, 0)
}

// TestMetricsGraph tests the MetricsGraph struct
func TestMetricsGraph(t *testing.T) {
	graph := MetricsGraph{
		Serial: 1,
		Series: []MetricsSeries{
			{
				Labels: promModel.Metric{"metric": "test1"},
				Values: []TimePoint{{Timestamp: 1000, Value: 10}},
			},
		},
		Config: MetricsGraphConfig{
			YAxisUnit: "percent",
		},
	}

	assert.Equal(t, 1, graph.Serial)
	assert.Len(t, graph.Series, 1)
	assert.Equal(t, "percent", graph.Config.YAxisUnit)
}

// TestMetricsGraph_JSONMarshal tests JSON marshaling
func TestMetricsGraph_JSONMarshal(t *testing.T) {
	graph := MetricsGraph{
		Serial: 2,
		Series: []MetricsSeries{},
		Config: MetricsGraphConfig{
			YAxisUnit: "bytes",
		},
	}

	data, err := json.Marshal(graph)
	require.NoError(t, err)

	var decoded MetricsGraph
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, graph.Serial, decoded.Serial)
	assert.Equal(t, graph.Config.YAxisUnit, decoded.Config.YAxisUnit)
}

// TestMetricsGraphConfig tests the MetricsGraphConfig struct
func TestMetricsGraphConfig(t *testing.T) {
	config := MetricsGraphConfig{
		YAxisUnit: "milliseconds",
	}

	assert.Equal(t, "milliseconds", config.YAxisUnit)
}

// TestMetricsGraphConfig_JSONMarshal tests JSON marshaling
func TestMetricsGraphConfig_JSONMarshal(t *testing.T) {
	config := MetricsGraphConfig{
		YAxisUnit: "GB",
	}

	data, err := json.Marshal(config)
	require.NoError(t, err)

	var decoded MetricsGraphConfig
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, config.YAxisUnit, decoded.YAxisUnit)
}

// TestMetricsGraph_MultipleSeries tests graph with multiple series
func TestMetricsGraph_MultipleSeries(t *testing.T) {
	graph := MetricsGraph{
		Serial: 1,
		Series: []MetricsSeries{
			{
				Labels: promModel.Metric{"host": "server1"},
				Values: []TimePoint{{Timestamp: 1000, Value: 10}},
			},
			{
				Labels: promModel.Metric{"host": "server2"},
				Values: []TimePoint{{Timestamp: 1000, Value: 20}},
			},
			{
				Labels: promModel.Metric{"host": "server3"},
				Values: []TimePoint{{Timestamp: 1000, Value: 30}},
			},
		},
		Config: MetricsGraphConfig{YAxisUnit: "count"},
	}

	assert.Len(t, graph.Series, 3)
	assert.Equal(t, "server1", string(graph.Series[0].Labels["host"]))
	assert.Equal(t, "server2", string(graph.Series[1].Labels["host"]))
	assert.Equal(t, "server3", string(graph.Series[2].Labels["host"]))
}

// TestMetricsGraph_EmptyConfig tests graph with empty config
func TestMetricsGraph_EmptyConfig(t *testing.T) {
	graph := MetricsGraph{
		Serial: 1,
		Series: []MetricsSeries{},
		Config: MetricsGraphConfig{},
	}

	assert.Equal(t, "", graph.Config.YAxisUnit)
}

// BenchmarkTimePoint_JSONMarshal benchmarks TimePoint marshaling
func BenchmarkTimePoint_JSONMarshal(b *testing.B) {
	tp := TimePoint{
		Timestamp: 1609459200,
		Value:     42.5,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(tp)
	}
}

// BenchmarkMetricsSeries_JSONMarshal benchmarks MetricsSeries marshaling
func BenchmarkMetricsSeries_JSONMarshal(b *testing.B) {
	series := MetricsSeries{
		Labels: promModel.Metric{"metric": "test"},
		Values: []TimePoint{
			{Timestamp: 1000, Value: 10},
			{Timestamp: 2000, Value: 20},
			{Timestamp: 3000, Value: 30},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(series)
	}
}

// BenchmarkMetricsGraph_JSONMarshal benchmarks MetricsGraph marshaling
func BenchmarkMetricsGraph_JSONMarshal(b *testing.B) {
	graph := MetricsGraph{
		Serial: 1,
		Series: []MetricsSeries{
			{
				Labels: promModel.Metric{"host": "server1"},
				Values: []TimePoint{{Timestamp: 1000, Value: 10}},
			},
		},
		Config: MetricsGraphConfig{YAxisUnit: "percent"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(graph)
	}
}

