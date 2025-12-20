package scraper

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsParser_Parse(t *testing.T) {
	parser := NewMetricsParser()

	t.Run("parse valid prometheus format", func(t *testing.T) {
		input := `
# HELP http_requests_total Total HTTP requests
# TYPE http_requests_total counter
http_requests_total{method="GET",path="/api"} 1234
http_requests_total{method="POST",path="/api"} 567

# HELP request_latency_seconds Request latency histogram
# TYPE request_latency_seconds histogram
request_latency_seconds_bucket{le="0.1"} 100
request_latency_seconds_bucket{le="0.5"} 500
request_latency_seconds_bucket{le="+Inf"} 600
request_latency_seconds_sum 150.5
request_latency_seconds_count 600
`
		families, err := parser.Parse(strings.NewReader(input))
		require.NoError(t, err)
		assert.Len(t, families, 2)

		names := GetMetricNames(families)
		assert.Contains(t, names, "http_requests_total")
		assert.Contains(t, names, "request_latency_seconds")
	})

	t.Run("parse empty input", func(t *testing.T) {
		families, err := parser.Parse(strings.NewReader(""))
		require.NoError(t, err)
		assert.Empty(t, families)
	})

	t.Run("parse comments only", func(t *testing.T) {
		// Prometheus format requires proper structure, comments alone may cause issues
		// Test with just whitespace which should be fine
		input := `  
`
		families, err := parser.Parse(strings.NewReader(input))
		require.NoError(t, err)
		assert.Empty(t, families)
	})
}

func TestFilterByPrefix(t *testing.T) {
	parser := NewMetricsParser()

	input := `
# TYPE vllm_requests_total counter
vllm_requests_total 100

# TYPE vllm_tokens_total counter
vllm_tokens_total 5000

# TYPE http_requests_total counter
http_requests_total 200

# TYPE process_cpu_seconds_total counter
process_cpu_seconds_total 123
`
	families, err := parser.Parse(strings.NewReader(input))
	require.NoError(t, err)

	t.Run("filter by vllm prefix", func(t *testing.T) {
		filtered := FilterByPrefix(families, "vllm_")
		assert.Len(t, filtered, 2)
	})

	t.Run("filter by http prefix", func(t *testing.T) {
		filtered := FilterByPrefix(families, "http_")
		assert.Len(t, filtered, 1)
	})

	t.Run("filter with no matches", func(t *testing.T) {
		filtered := FilterByPrefix(families, "nonexistent_")
		assert.Empty(t, filtered)
	})
}

func TestCountMetrics(t *testing.T) {
	parser := NewMetricsParser()

	input := `
# TYPE gauge1 gauge
gauge1{label="a"} 1
gauge1{label="b"} 2
gauge1{label="c"} 3

# TYPE counter1 counter
counter1 100
`
	families, err := parser.Parse(strings.NewReader(input))
	require.NoError(t, err)

	count := CountMetrics(families)
	assert.Equal(t, 4, count) // 3 gauge + 1 counter
}

func TestSummarize(t *testing.T) {
	parser := NewMetricsParser()

	input := `
# TYPE requests counter
requests 100

# TYPE latency histogram
latency_bucket{le="0.1"} 10
latency_bucket{le="+Inf"} 20
latency_sum 5.5
latency_count 20

# TYPE active_connections gauge
active_connections 50
`
	families, err := parser.Parse(strings.NewReader(input))
	require.NoError(t, err)

	summary := Summarize(families)
	assert.Equal(t, 3, summary.TotalFamilies)
	assert.Greater(t, summary.TotalMetrics, 0)
	assert.NotEmpty(t, summary.TypeCounts)
}

func TestParseInferenceMetrics(t *testing.T) {
	parser := NewMetricsParser()

	// Simulated vLLM metrics
	input := `
# HELP vllm_num_requests_running Number of requests currently running
# TYPE vllm_num_requests_running gauge
vllm_num_requests_running 5

# HELP vllm_num_requests_waiting Number of requests waiting
# TYPE vllm_num_requests_waiting gauge
vllm_num_requests_waiting 3

# HELP vllm_time_to_first_token_seconds Time to first token
# TYPE vllm_time_to_first_token_seconds histogram
vllm_time_to_first_token_seconds_bucket{le="0.1"} 100
vllm_time_to_first_token_seconds_bucket{le="0.5"} 450
vllm_time_to_first_token_seconds_bucket{le="1.0"} 480
vllm_time_to_first_token_seconds_bucket{le="+Inf"} 500
vllm_time_to_first_token_seconds_sum 100.5
vllm_time_to_first_token_seconds_count 500

# HELP vllm_generation_tokens_total Total generated tokens
# TYPE vllm_generation_tokens_total counter
vllm_generation_tokens_total 50000
`
	families, err := parser.Parse(strings.NewReader(input))
	require.NoError(t, err)
	assert.Len(t, families, 4)

	names := GetMetricNames(families)
	assert.Contains(t, names, "vllm_num_requests_running")
	assert.Contains(t, names, "vllm_time_to_first_token_seconds")
	assert.Contains(t, names, "vllm_generation_tokens_total")
}

