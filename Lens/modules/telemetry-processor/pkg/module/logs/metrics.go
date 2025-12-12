package logs

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	logConsumeLatencySummary   *prometheus.SummaryVec
	logConsumeLatencyHistogram *prometheus.HistogramVec
)

func init() {
	logConsumeLatencySummary = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "log_consume_latency_seconds",
			Help:       "The latency of log consume",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.95: 0.005, 0.99: 0.001},
		}, []string{"node"})
	prometheus.MustRegister(logConsumeLatencySummary)
	logConsumeLatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "log_consume_latency_histogram_seconds",
			Help:    "The latency of log consume",
			Buckets: []float64{0.1, 0.5, 1, 2, 3, 5, 8, 10, 15, 30, 60, 120, 300},
		}, []string{"node"})
	prometheus.MustRegister(logConsumeLatencyHistogram)
}

// WandB metrics
var (
	wandbRequestCount          *prometheus.CounterVec
	wandbRequestErrorCount     *prometheus.CounterVec
	wandbRequestDuration       *prometheus.HistogramVec
	wandbMetricsDataPointCount *prometheus.HistogramVec
	wandbMetricsStoreCount     *prometheus.CounterVec
	wandbMetricsStoreErrors    *prometheus.CounterVec
	wandbLogsDataPointCount    *prometheus.HistogramVec
)

func init() {
	// WandB request count (by request type: metrics/logs/detection)
	wandbRequestCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "wandb",
			Name:      "request_total",
			Help:      "Total number of WandB requests processed",
		},
		[]string{"type"}, // type: metrics, logs, detection
	)
	prometheus.MustRegister(wandbRequestCount)

	// WandB request error count
	wandbRequestErrorCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "wandb",
			Name:      "request_error_total",
			Help:      "Total number of WandB request errors",
		},
		[]string{"type", "error_type"}, // type: metrics/logs/detection, error_type: validation/storage/other
	)
	prometheus.MustRegister(wandbRequestErrorCount)

	// WandB request processing duration
	wandbRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: "wandb",
			Name:      "request_duration_seconds",
			Help:      "Duration of WandB request processing in seconds",
			Buckets:   prometheus.DefBuckets, // 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10
		},
		[]string{"type"},
	)
	prometheus.MustRegister(wandbRequestDuration)

	// WandB metrics data point count distribution
	wandbMetricsDataPointCount = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: "wandb",
			Name:      "metrics_datapoint_count",
			Help:      "Number of metrics data points per request",
			Buckets:   []float64{1, 5, 10, 20, 50, 100, 200, 500, 1000},
		},
		[]string{"workload_uid"},
	)
	prometheus.MustRegister(wandbMetricsDataPointCount)

	// WandB metrics storage success count
	wandbMetricsStoreCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "wandb",
			Name:      "metrics_store_total",
			Help:      "Total number of WandB metrics stored successfully",
		},
		[]string{"workload_uid"},
	)
	prometheus.MustRegister(wandbMetricsStoreCount)

	// WandB metrics storage error count
	wandbMetricsStoreErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "wandb",
			Name:      "metrics_store_error_total",
			Help:      "Total number of WandB metrics storage errors",
		},
		[]string{"workload_uid"},
	)
	prometheus.MustRegister(wandbMetricsStoreErrors)

	// WandB logs data point count distribution
	wandbLogsDataPointCount = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: "wandb",
			Name:      "logs_datapoint_count",
			Help:      "Number of log data points per request",
			Buckets:   []float64{1, 5, 10, 20, 50, 100, 200, 500},
		},
		[]string{"workload_uid"},
	)
	prometheus.MustRegister(wandbLogsDataPointCount)
}

// Framework detection metrics
var (
	frameworkDetectionCount      *prometheus.CounterVec
	frameworkDetectionConfidence *prometheus.HistogramVec
	frameworkDetectionErrors     *prometheus.CounterVec
	frameworkUsageCount          *prometheus.CounterVec
)

func init() {
	// Framework detection count (by framework name and detection method)
	frameworkDetectionCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "log_framework_detection",
			Name:      "total",
			Help:      "Total number of framework detections from log processing",
		},
		[]string{"framework", "method", "source"}, // method: env_vars/config/modules/project_name/log_pattern, source: wandb/log
	)
	prometheus.MustRegister(frameworkDetectionCount)

	// Framework detection confidence distribution
	frameworkDetectionConfidence = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: "log_framework_detection",
			Name:      "confidence",
			Help:      "Framework detection confidence distribution from log processing",
			Buckets:   []float64{0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 0.95, 1.0},
		},
		[]string{"framework", "method"},
	)
	prometheus.MustRegister(frameworkDetectionConfidence)

	// Framework detection error count
	frameworkDetectionErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "log_framework_detection",
			Name:      "error_total",
			Help:      "Total number of framework detection errors from log processing",
		},
		[]string{"source", "error_type"}, // source: wandb/log, error_type: no_match/report_failed
	)
	prometheus.MustRegister(frameworkDetectionErrors)

	// Framework usage statistics (from AI Advisor detection results)
	frameworkUsageCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "framework_usage",
			Name:      "total",
			Help:      "Total number of workloads using each framework (from AI Advisor detection)",
		},
		[]string{"framework", "detection_source"}, // framework: primus/pytorch/megatron/etc, detection_source: wrapper/base/primary
	)
	prometheus.MustRegister(frameworkUsageCount)
}

// Log pattern matching metrics
var (
	logPatternMatchCount  *prometheus.CounterVec
	logPatternMatchErrors *prometheus.CounterVec
)

func init() {
	// Log pattern match count (by pattern type, framework, and specific pattern name)
	logPatternMatchCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "log_pattern",
			Name:      "match_total",
			Help:      "Total number of successful log pattern matches",
		},
		[]string{"framework", "pattern_type", "pattern_name"}, // pattern_type: performance/training_event/checkpoint_event/identify, pattern_name: specific regex pattern name
	)
	prometheus.MustRegister(logPatternMatchCount)

	// Log pattern match errors
	logPatternMatchErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "log_pattern",
			Name:      "match_error_total",
			Help:      "Total number of log pattern match errors",
		},
		[]string{"framework", "pattern_type", "error_type"},
	)
	prometheus.MustRegister(logPatternMatchErrors)
}

// Training performance metrics
var (
	trainingPerformanceSaveCount  *prometheus.CounterVec
	trainingPerformanceSaveErrors *prometheus.CounterVec
)

func init() {
	// Training performance data save count
	trainingPerformanceSaveCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "training_performance",
			Name:      "save_total",
			Help:      "Total number of training performance data saved",
		},
		[]string{"workload_uid", "source"}, // source: log/wandb
	)
	prometheus.MustRegister(trainingPerformanceSaveCount)

	// Training performance data save errors
	trainingPerformanceSaveErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "training_performance",
			Name:      "save_error_total",
			Help:      "Total number of training performance save errors",
		},
		[]string{"workload_uid", "source", "error_type"},
	)
	prometheus.MustRegister(trainingPerformanceSaveErrors)
}

// Checkpoint metrics
var (
	checkpointEventCount  *prometheus.CounterVec
	checkpointEventErrors *prometheus.CounterVec
)

func init() {
	// Checkpoint event count
	checkpointEventCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "checkpoint",
			Name:      "event_total",
			Help:      "Total number of checkpoint events",
		},
		[]string{"event_type", "framework"}, // event_type: start_saving/end_saving/loading
	)
	prometheus.MustRegister(checkpointEventCount)

	// Checkpoint event errors
	checkpointEventErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "checkpoint",
			Name:      "event_error_total",
			Help:      "Total number of checkpoint event errors",
		},
		[]string{"event_type", "framework", "error_type"},
	)
	prometheus.MustRegister(checkpointEventErrors)
}

var (
	logAnalysisCount              *prometheus.CounterVec
	createWorkloadEventCount      *prometheus.CounterVec
	createWorkloadEventErrorCount *prometheus.CounterVec
	createEventStreamCount        *prometheus.CounterVec
	createEventStreamErrorCount   *prometheus.CounterVec
)

func init() {
	logAnalysisCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem:   "log_analysis",
			Name:        "consume_count",
			Help:        "consume count of log analysis",
			ConstLabels: nil,
		},
		[]string{
			"type",
		})
	prometheus.MustRegister(logAnalysisCount)
	createEventStreamCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem:   "event_stream",
			Name:        "create_count",
			Help:        "create count of event stream",
			ConstLabels: nil,
		}, []string{
			"operation",
		})
	prometheus.MustRegister(createEventStreamCount)
	createEventStreamErrorCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem:   "event_stream",
			Name:        "create_error_count",
			Help:        "create error count of event stream",
			ConstLabels: nil,
		}, []string{
			"operation",
		},
	)
	prometheus.MustRegister(createEventStreamErrorCount)
	createWorkloadEventCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem:   "workload_event",
			Name:        "create_count",
			Help:        "create count of workload event",
			ConstLabels: nil,
		}, []string{
			"operation",
		},
	)
	prometheus.MustRegister(createWorkloadEventCount)
	createWorkloadEventErrorCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem:   "workload_event",
			Name:        "create_error_count",
			Help:        "create error count of workload event",
			ConstLabels: nil,
		}, []string{
			"operation",
		},
	)
}

func IncLogAnalysisCount(typ string) {
	logAnalysisCount.WithLabelValues(typ).Inc()
}

func IncCreateEventStreamCount(operation string) {
	createEventStreamCount.WithLabelValues(operation).Inc()
}

func IncCreateEventStreamErrorCount(operation string) {
	createEventStreamErrorCount.WithLabelValues(operation).Inc()
}

func IncCreateWorkloadEventCount(operation string) {
	createWorkloadEventCount.WithLabelValues(operation).Inc()
}

func IncCreateWorkloadEventErrorCount(operation string) {
	createWorkloadEventErrorCount.WithLabelValues(operation).Inc()
}

// WandB metrics helper functions
func IncWandBRequestCount(requestType string) {
	wandbRequestCount.WithLabelValues(requestType).Inc()
}

func IncWandBRequestErrorCount(requestType, errorType string) {
	wandbRequestErrorCount.WithLabelValues(requestType, errorType).Inc()
}

func ObserveWandBRequestDuration(requestType string, durationSeconds float64) {
	wandbRequestDuration.WithLabelValues(requestType).Observe(durationSeconds)
}

func ObserveWandBMetricsDataPointCount(workloadUID string, count int) {
	wandbMetricsDataPointCount.WithLabelValues(workloadUID).Observe(float64(count))
}

func IncWandBMetricsStoreCount(workloadUID string) {
	wandbMetricsStoreCount.WithLabelValues(workloadUID).Inc()
}

func IncWandBMetricsStoreErrors(workloadUID string) {
	wandbMetricsStoreErrors.WithLabelValues(workloadUID).Inc()
}

func ObserveWandBLogsDataPointCount(workloadUID string, count int) {
	wandbLogsDataPointCount.WithLabelValues(workloadUID).Observe(float64(count))
}

// Framework detection helper functions
func IncFrameworkDetectionCount(framework, method, source string) {
	frameworkDetectionCount.WithLabelValues(framework, method, source).Inc()
}

func ObserveFrameworkDetectionConfidence(framework, method string, confidence float64) {
	frameworkDetectionConfidence.WithLabelValues(framework, method).Observe(confidence)
}

func IncFrameworkDetectionErrors(source, errorType string) {
	frameworkDetectionErrors.WithLabelValues(source, errorType).Inc()
}

func IncFrameworkUsageCount(framework, detectionSource string) {
	frameworkUsageCount.WithLabelValues(framework, detectionSource).Inc()
}

// Log pattern matching helper functions
func IncLogPatternMatchCount(framework, patternType, patternName string) {
	logPatternMatchCount.WithLabelValues(framework, patternType, patternName).Inc()
}

func IncLogPatternMatchErrors(framework, patternType, errorType string) {
	logPatternMatchErrors.WithLabelValues(framework, patternType, errorType).Inc()
}

// Training performance helper functions
func IncTrainingPerformanceSaveCount(workloadUID, source string) {
	trainingPerformanceSaveCount.WithLabelValues(workloadUID, source).Inc()
}

func IncTrainingPerformanceSaveErrors(workloadUID, source, errorType string) {
	trainingPerformanceSaveErrors.WithLabelValues(workloadUID, source, errorType).Inc()
}

// Checkpoint helper functions
func IncCheckpointEventCount(eventType, framework string) {
	checkpointEventCount.WithLabelValues(eventType, framework).Inc()
}

func IncCheckpointEventErrors(eventType, framework, errorType string) {
	checkpointEventErrors.WithLabelValues(eventType, framework, errorType).Inc()
}
