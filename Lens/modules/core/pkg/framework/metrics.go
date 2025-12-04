package framework

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

var (
	// detectionTotal counts total number of framework detections
	detectionTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "framework_detection_total",
			Help: "Total number of framework detection reports",
		},
		[]string{"source", "framework", "status"},
	)
	
	// detectionConfidence tracks distribution of detection confidence scores
	detectionConfidence = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "framework_detection_confidence",
			Help:    "Distribution of framework detection confidence scores",
			Buckets: []float64{0.3, 0.5, 0.6, 0.7, 0.8, 0.85, 0.9, 0.95, 1.0},
		},
		[]string{"framework"},
	)
	
	// detectionConflicts counts conflicts between detection sources
	detectionConflicts = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "framework_detection_conflicts_total",
			Help: "Total number of detection conflicts",
		},
		[]string{"source1", "source2"},
	)
	
	// detectionLatency measures detection processing latency
	detectionLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "framework_detection_latency_seconds",
			Help:    "Detection processing latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)
	
	// sourceReportCount counts reports per source
	sourceReportCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "framework_detection_source_reports_total",
			Help: "Total number of reports per detection source",
		},
		[]string{"source"},
	)
	
	// detectionStatusChanges tracks status transitions
	detectionStatusChanges = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "framework_detection_status_changes_total",
			Help: "Total number of detection status changes",
		},
		[]string{"from_status", "to_status"},
	)
	
	// cacheHits tracks cache hit/miss ratio
	cacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "framework_detection_cache_hits_total",
			Help: "Total number of cache hits",
		},
		[]string{"result"}, // hit or miss
	)
)

// RecordDetection records a detection event
func RecordDetection(source, framework string, status model.DetectionStatus, confidence float64) {
	detectionTotal.WithLabelValues(source, framework, string(status)).Inc()
	detectionConfidence.WithLabelValues(framework).Observe(confidence)
	sourceReportCount.WithLabelValues(source).Inc()
}

// RecordConflict records a conflict between two sources
func RecordConflict(source1, source2 string) {
	detectionConflicts.WithLabelValues(source1, source2).Inc()
}

// RecordLatency records operation latency
func RecordLatency(operation string, seconds float64) {
	detectionLatency.WithLabelValues(operation).Observe(seconds)
}

// RecordStatusChange records a status transition
func RecordStatusChange(fromStatus, toStatus model.DetectionStatus) {
	detectionStatusChanges.WithLabelValues(string(fromStatus), string(toStatus)).Inc()
}

// RecordCacheHit records a cache hit
func RecordCacheHit() {
	cacheHits.WithLabelValues("hit").Inc()
}

// RecordCacheMiss records a cache miss
func RecordCacheMiss() {
	cacheHits.WithLabelValues("miss").Inc()
}

