package framework

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metrics for metadata reuse
var (
	// reuseAttemptTotal counts the total number of reuse attempts
	reuseAttemptTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "lens",
			Subsystem: "metadata_reuse",
			Name:      "attempt_total",
			Help:      "Total number of metadata reuse attempts",
		},
		[]string{"result"}, // result: success, no_candidate, below_threshold, error
	)

	// reuseSuccessTotal counts successful metadata reuses
	reuseSuccessTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "lens",
			Subsystem: "metadata_reuse",
			Name:      "success_total",
			Help:      "Total number of successful metadata reuses",
		},
		[]string{"framework", "namespace"},
	)

	// reuseSimilarityScore records the similarity scores
	reuseSimilarityScore = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "lens",
			Subsystem: "metadata_reuse",
			Name:      "similarity_score",
			Help:      "Similarity scores for reuse attempts",
			Buckets:   []float64{0.5, 0.6, 0.7, 0.75, 0.8, 0.85, 0.9, 0.95, 0.98, 1.0},
		},
	)

	// reuseConfidenceDecay records confidence after decay
	reuseConfidenceDecay = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "lens",
			Subsystem: "metadata_reuse",
			Name:      "confidence_decay",
			Help:      "Confidence values after decay",
			Buckets:   []float64{0.5, 0.6, 0.7, 0.75, 0.8, 0.85, 0.9, 0.95, 1.0},
		},
	)

	// reuseDuration measures the time taken for reuse operation
	reuseDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "lens",
			Subsystem: "metadata_reuse",
			Name:      "duration_seconds",
			Help:      "Duration of reuse operation in seconds",
			Buckets:   prometheus.DefBuckets, // 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10
		},
	)

	// candidateCount records the number of candidates found
	candidateCount = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "lens",
			Subsystem: "metadata_reuse",
			Name:      "candidate_count",
			Help:      "Number of candidate workloads found for reuse",
			Buckets:   []float64{0, 1, 5, 10, 20, 50, 100, 200},
		},
	)

	// cacheHitTotal counts cache hits
	cacheHitTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "lens",
			Subsystem: "metadata_reuse",
			Name:      "cache_hit_total",
			Help:      "Total number of cache hits",
		},
		[]string{"cache_type"}, // cache_type: signature, similarity
	)

	// cacheMissTotal counts cache misses
	cacheMissTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "lens",
			Subsystem: "metadata_reuse",
			Name:      "cache_miss_total",
			Help:      "Total number of cache misses",
		},
		[]string{"cache_type"}, // cache_type: signature, similarity
	)

	// timeSavedSeconds estimates time saved by reuse (vs. normal detection)
	timeSavedSeconds = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "lens",
			Subsystem: "metadata_reuse",
			Name:      "time_saved_seconds_total",
			Help:      "Total estimated time saved by metadata reuse in seconds",
		},
	)
)

// MetricsRecorder handles metrics recording for reuse operations
type MetricsRecorder struct{}

// NewMetricsRecorder creates a new metrics recorder
func NewMetricsRecorder() *MetricsRecorder {
	return &MetricsRecorder{}
}

// RecordAttempt records a reuse attempt
func (m *MetricsRecorder) RecordAttempt(result string) {
	reuseAttemptTotal.WithLabelValues(result).Inc()
}

// RecordSuccess records a successful reuse
func (m *MetricsRecorder) RecordSuccess(framework, namespace string, similarityScore, confidence float64) {
	reuseSuccessTotal.WithLabelValues(framework, namespace).Inc()
	reuseSimilarityScore.Observe(similarityScore)
	reuseConfidenceDecay.Observe(confidence)
	
	// Estimate time saved (assuming normal detection takes ~360 seconds)
	// Reuse takes ~0.3 seconds, so we save ~359.7 seconds
	timeSavedSeconds.Add(360.0)
}

// RecordDuration records the duration of a reuse operation
func (m *MetricsRecorder) RecordDuration(duration time.Duration) {
	reuseDuration.Observe(duration.Seconds())
}

// RecordCandidateCount records the number of candidates found
func (m *MetricsRecorder) RecordCandidateCount(count int) {
	candidateCount.Observe(float64(count))
}

// RecordCacheHit records a cache hit
func (m *MetricsRecorder) RecordCacheHit(cacheType string) {
	cacheHitTotal.WithLabelValues(cacheType).Inc()
}

// RecordCacheMiss records a cache miss
func (m *MetricsRecorder) RecordCacheMiss(cacheType string) {
	cacheMissTotal.WithLabelValues(cacheType).Inc()
}

// RecordSimilarityScore records a similarity score (for monitoring distribution)
func (m *MetricsRecorder) RecordSimilarityScore(score float64) {
	reuseSimilarityScore.Observe(score)
}

