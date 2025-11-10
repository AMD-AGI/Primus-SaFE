package containers

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	containerEventRecvCnt = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "primus_lens",
		Subsystem: "telemetry_processor",
		Name:      "container_event_recv_total",
		Help:      "Total number of container events received",
	}, []string{"source", "node"})

	containerEventErrorCnt = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "primus_lens",
		Subsystem: "telemetry_processor",
		Name:      "container_event_error_total",
		Help:      "Total number of container event errors",
	}, []string{"source", "node", "error_type"})

	containerEventProcessingDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "primus_lens",
		Subsystem: "telemetry_processor",
		Name:      "container_event_processing_duration_seconds",
		Help:      "Duration of container event processing in seconds",
		Buckets:   prometheus.ExponentialBuckets(0.001, 2, 15), // from 1ms to ~16s
	}, []string{"source", "node"})

	containerEventBatchSize = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "primus_lens",
		Subsystem: "telemetry_processor",
		Name:      "container_event_batch_size",
		Help:      "Number of events in each batch request",
		Buckets:   prometheus.LinearBuckets(1, 5, 20), // 1, 6, 11, ..., 96
	})
)

func init() {
	prometheus.MustRegister(containerEventRecvCnt)
	prometheus.MustRegister(containerEventErrorCnt)
	prometheus.MustRegister(containerEventProcessingDuration)
	prometheus.MustRegister(containerEventBatchSize)
}
