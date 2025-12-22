package transformer

// MetricMapping defines how a source metric maps to a unified target metric
type MetricMapping struct {
	Source    string `json:"source"`              // Source metric name (e.g., "vllm:time_to_first_token_seconds")
	Target    string `json:"target"`              // Target unified metric name
	Type      string `json:"type"`                // Metric type: counter, gauge, histogram, summary
	Help      string `json:"help,omitempty"`      // Help text for the metric
	Transform string `json:"transform,omitempty"` // Optional transformation: divide_by_100, multiply_by_1000, etc.
}

// FrameworkMetricsConfig defines the metrics configuration for a framework
type FrameworkMetricsConfig struct {
	Framework       string            `json:"framework"`
	DefaultPort     int               `json:"default_port"`
	DefaultPath     string            `json:"default_path"`
	ScrapeInterval  int               `json:"scrape_interval"` // seconds
	Mappings        []MetricMapping   `json:"mappings"`
	LabelsAlwaysAdd map[string]string `json:"labels_always_add,omitempty"`
}

// UnifiedMetricNames defines the standard metric names used across all frameworks
var UnifiedMetricNames = struct {
	// Request metrics
	RequestsTotal       string
	RequestLatency      string
	RequestErrors       string
	QueueDepth          string
	BatchSize           string

	// Token metrics (LLM specific)
	TimeToFirstToken    string
	TimePerOutputToken  string
	TokensGeneratedTotal string
	PromptTokensTotal   string
	TokenThroughput     string

	// Resource metrics
	KVCacheUtilization  string
	GPUMemoryUsed       string
	ModelLoadTime       string
}{
	// Request metrics
	RequestsTotal:       "inference_requests_total",
	RequestLatency:      "inference_request_latency_seconds",
	RequestErrors:       "inference_request_errors_total",
	QueueDepth:          "inference_queue_depth",
	BatchSize:           "inference_batch_size",

	// Token metrics (LLM specific)
	TimeToFirstToken:    "inference_time_to_first_token_seconds",
	TimePerOutputToken:  "inference_time_per_output_token_seconds",
	TokensGeneratedTotal: "inference_tokens_generated_total",
	PromptTokensTotal:   "inference_prompt_tokens_total",
	TokenThroughput:     "inference_token_throughput",

	// Resource metrics
	KVCacheUtilization:  "inference_kv_cache_utilization",
	GPUMemoryUsed:       "inference_gpu_memory_used_bytes",
	ModelLoadTime:       "inference_model_load_time_seconds",
}

// DefaultVLLMConfig returns the default metrics configuration for vLLM
func DefaultVLLMConfig() *FrameworkMetricsConfig {
	return &FrameworkMetricsConfig{
		Framework:      "vllm",
		DefaultPort:    8000,
		DefaultPath:    "/metrics",
		ScrapeInterval: 15,
		Mappings: []MetricMapping{
			{Source: "vllm:time_to_first_token_seconds", Target: UnifiedMetricNames.TimeToFirstToken, Type: "histogram", Help: "Time to first token (TTFT)"},
			{Source: "vllm:time_per_output_token_seconds", Target: UnifiedMetricNames.TimePerOutputToken, Type: "histogram", Help: "Time per output token (TPOT)"},
			{Source: "vllm:generation_tokens_total", Target: UnifiedMetricNames.TokensGeneratedTotal, Type: "counter", Help: "Total tokens generated"},
			{Source: "vllm:prompt_tokens_total", Target: UnifiedMetricNames.PromptTokensTotal, Type: "counter", Help: "Total prompt tokens"},
			{Source: "vllm:e2e_request_latency_seconds", Target: UnifiedMetricNames.RequestLatency, Type: "histogram", Help: "End-to-end request latency"},
			{Source: "vllm:num_requests_running", Target: UnifiedMetricNames.BatchSize, Type: "gauge", Help: "Number of requests currently running"},
			{Source: "vllm:num_requests_waiting", Target: UnifiedMetricNames.QueueDepth, Type: "gauge", Help: "Number of requests waiting in queue"},
			{Source: "vllm:gpu_cache_usage_perc", Target: UnifiedMetricNames.KVCacheUtilization, Type: "gauge", Help: "KV cache utilization", Transform: "divide_by_100"},
		},
		LabelsAlwaysAdd: map[string]string{
			"framework":      "vllm",
			"framework_type": "inference",
		},
	}
}

// DefaultTGIConfig returns the default metrics configuration for TGI
func DefaultTGIConfig() *FrameworkMetricsConfig {
	return &FrameworkMetricsConfig{
		Framework:      "tgi",
		DefaultPort:    80,
		DefaultPath:    "/metrics",
		ScrapeInterval: 15,
		Mappings: []MetricMapping{
			{Source: "tgi_request_duration_seconds", Target: UnifiedMetricNames.RequestLatency, Type: "histogram", Help: "Request duration"},
			{Source: "tgi_request_count", Target: UnifiedMetricNames.RequestsTotal, Type: "counter", Help: "Total requests"},
			{Source: "tgi_queue_size", Target: UnifiedMetricNames.QueueDepth, Type: "gauge", Help: "Queue size"},
			{Source: "tgi_batch_current_size", Target: UnifiedMetricNames.BatchSize, Type: "gauge", Help: "Current batch size"},
			{Source: "tgi_request_generated_tokens_total", Target: UnifiedMetricNames.TokensGeneratedTotal, Type: "counter", Help: "Total generated tokens"},
			{Source: "tgi_time_per_token_seconds", Target: UnifiedMetricNames.TimePerOutputToken, Type: "histogram", Help: "Time per token"},
		},
		LabelsAlwaysAdd: map[string]string{
			"framework":      "tgi",
			"framework_type": "inference",
		},
	}
}

// DefaultTritonConfig returns the default metrics configuration for Triton
func DefaultTritonConfig() *FrameworkMetricsConfig {
	return &FrameworkMetricsConfig{
		Framework:      "triton",
		DefaultPort:    8002,
		DefaultPath:    "/metrics",
		ScrapeInterval: 15,
		Mappings: []MetricMapping{
			{Source: "nv_inference_request_duration_us", Target: UnifiedMetricNames.RequestLatency, Type: "histogram", Help: "Request duration", Transform: "microseconds_to_seconds"},
			{Source: "nv_inference_request_success", Target: UnifiedMetricNames.RequestsTotal, Type: "counter", Help: "Successful requests"},
			{Source: "nv_inference_request_failure", Target: UnifiedMetricNames.RequestErrors, Type: "counter", Help: "Failed requests"},
			{Source: "nv_inference_queue_duration_us", Target: "inference_queue_wait_seconds", Type: "histogram", Help: "Queue wait time", Transform: "microseconds_to_seconds"},
			{Source: "nv_inference_pending_request_count", Target: UnifiedMetricNames.QueueDepth, Type: "gauge", Help: "Pending requests"},
			{Source: "nv_gpu_memory_used_bytes", Target: UnifiedMetricNames.GPUMemoryUsed, Type: "gauge", Help: "GPU memory used"},
		},
		LabelsAlwaysAdd: map[string]string{
			"framework":      "triton",
			"framework_type": "inference",
		},
	}
}

// GetDefaultConfig returns the default config for a framework
func GetDefaultConfig(framework string) *FrameworkMetricsConfig {
	switch framework {
	case "vllm":
		return DefaultVLLMConfig()
	case "tgi":
		return DefaultTGIConfig()
	case "triton":
		return DefaultTritonConfig()
	default:
		return nil
	}
}

