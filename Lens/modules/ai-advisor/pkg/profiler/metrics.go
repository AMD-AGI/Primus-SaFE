package profiler

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// File discovery metrics
	profilerFilesDiscovered = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "profiler_files_discovered_total",
			Help: "Total number of profiler files discovered",
		},
		[]string{"workload_uid", "file_type", "confidence"},
	)

	// File archive metrics
	profilerFilesArchived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "profiler_files_archived_total",
			Help: "Total number of profiler files archived",
		},
		[]string{"workload_uid", "file_type", "storage_type", "status"},
	)

	// File size distribution
	profilerFileSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "profiler_file_size_bytes",
			Help:    "Size of profiler files",
			Buckets: prometheus.ExponentialBuckets(1024, 10, 8), // 1KB to ~100MB
		},
		[]string{"file_type"},
	)

	// Archive duration
	profilerUploadDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "profiler_upload_duration_seconds",
			Help:    "Time taken to upload profiler files",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s to ~100s
		},
		[]string{"file_type", "storage_type"},
	)

	// Task execution metrics
	profilerTaskExecutions = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "profiler_task_executions_total",
			Help: "Total number of profiler collection task executions",
		},
		[]string{"workload_uid", "status"}, // status: success, failure
	)

	profilerTaskDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "profiler_task_duration_seconds",
			Help:    "Duration of profiler collection task execution",
			Buckets: prometheus.ExponentialBuckets(1, 2, 10), // 1s to ~1000s
		},
		[]string{"workload_uid"},
	)

	// Cleanup job metrics
	profilerCleanupFilesDeleted = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "profiler_cleanup_files_deleted_total",
			Help: "Total number of files deleted by cleanup job",
		},
	)

	profilerCleanupBytesFreed = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "profiler_cleanup_bytes_freed_total",
			Help: "Total bytes freed by cleanup job",
		},
	)

	profilerCleanupDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "profiler_cleanup_duration_seconds",
			Help:    "Duration of cleanup job execution",
			Buckets: prometheus.LinearBuckets(1, 5, 10), // 1s to 50s
		},
	)

	// Storage usage metrics
	profilerStorageUsedBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "profiler_storage_used_bytes",
			Help: "Total storage used by profiler files",
		},
		[]string{"storage_type"},
	)

	profilerStorageFileCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "profiler_storage_file_count",
			Help: "Number of profiler files in storage",
		},
		[]string{"storage_type", "file_type"},
	)

	// Cache hit rate metrics
	profilerCacheHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "profiler_cache_hits_total",
			Help: "Total number of cache hits",
		},
	)

	profilerCacheMisses = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "profiler_cache_misses_total",
			Help: "Total number of cache misses",
		},
	)

	// Error metrics
	profilerErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "profiler_errors_total",
			Help: "Total number of errors by type",
		},
		[]string{"error_type", "operation"}, // operation: discover, archive, cleanup
	)
)

// RecordFileDiscovered records file discovery
func RecordFileDiscovered(workloadUID, fileType, confidence string) {
	profilerFilesDiscovered.WithLabelValues(workloadUID, fileType, confidence).Inc()
}

// RecordFileArchived records file archive
func RecordFileArchived(workloadUID, fileType, storageType, status string, size int64, duration float64) {
	profilerFilesArchived.WithLabelValues(workloadUID, fileType, storageType, status).Inc()
	profilerFileSize.WithLabelValues(fileType).Observe(float64(size))
	profilerUploadDuration.WithLabelValues(fileType, storageType).Observe(duration)
}

// RecordTaskExecution records task execution
func RecordTaskExecution(workloadUID, status string, duration float64) {
	profilerTaskExecutions.WithLabelValues(workloadUID, status).Inc()
	profilerTaskDuration.WithLabelValues(workloadUID).Observe(duration)
}

// RecordCleanup records cleanup operation
func RecordCleanup(filesDeleted int, bytesFreed int64, duration float64) {
	profilerCleanupFilesDeleted.Add(float64(filesDeleted))
	profilerCleanupBytesFreed.Add(float64(bytesFreed))
	profilerCleanupDuration.Observe(duration)
}

// UpdateStorageMetrics updates storage metrics
func UpdateStorageMetrics(storageType string, usedBytes int64, fileCount map[string]int) {
	profilerStorageUsedBytes.WithLabelValues(storageType).Set(float64(usedBytes))

	for fileType, count := range fileCount {
		profilerStorageFileCount.WithLabelValues(storageType, fileType).Set(float64(count))
	}
}

// RecordCacheHit records cache hit
func RecordCacheHit() {
	profilerCacheHits.Inc()
}

// RecordCacheMiss records cache miss
func RecordCacheMiss() {
	profilerCacheMisses.Inc()
}

// RecordError records error
func RecordError(errorType, operation string) {
	profilerErrors.WithLabelValues(errorType, operation).Inc()
}
