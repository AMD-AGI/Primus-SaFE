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
