package logs

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// InMemoryMetricsStorage in-memory metrics storage implementation
type InMemoryMetricsStorage struct {
	mu      sync.RWMutex
	metrics map[string][]*StoredMetric // workloadUID -> metrics
	maxSize int                        // Maximum number of metrics saved per workload
}

// NewInMemoryMetricsStorage creates in-memory metrics storage
func NewInMemoryMetricsStorage(maxSize int) *InMemoryMetricsStorage {
	if maxSize <= 0 {
		maxSize = 10000 // Default maximum 10000 entries
	}
	return &InMemoryMetricsStorage{
		metrics: make(map[string][]*StoredMetric),
		maxSize: maxSize,
	}
}

// Store stores metric
func (s *InMemoryMetricsStorage) Store(ctx context.Context, metric *StoredMetric) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get or create metrics list for this workload
	metrics, exists := s.metrics[metric.WorkloadUID]
	if !exists {
		metrics = make([]*StoredMetric, 0, 100)
	}

	// Add new metric
	metrics = append(metrics, metric)

	// Delete oldest if exceeding max size
	if len(metrics) > s.maxSize {
		metrics = metrics[len(metrics)-s.maxSize:]
	}

	s.metrics[metric.WorkloadUID] = metrics

	logrus.Debugf("Stored metric %s for workload %s (total: %d)", 
		metric.Name, metric.WorkloadUID, len(metrics))

	return nil
}

// Query queries metrics
func (s *InMemoryMetricsStorage) Query(ctx context.Context, workloadUID string, metricName string) ([]*StoredMetric, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	allMetrics, exists := s.metrics[workloadUID]
	if !exists {
		return []*StoredMetric{}, nil
	}

	// Filter by metric name if specified
	if metricName != "" {
		result := make([]*StoredMetric, 0)
		for _, m := range allMetrics {
			if m.Name == metricName {
				result = append(result, m)
			}
		}
		return result, nil
	}

	// Return all metrics
	return allMetrics, nil
}

// GetMetricsCount gets metrics count (for monitoring)
func (s *InMemoryMetricsStorage) GetMetricsCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := 0
	for _, metrics := range s.metrics {
		total += len(metrics)
	}
	return total
}

// CleanupOldMetrics cleans up old metrics exceeding specified time
func (s *InMemoryMetricsStorage) CleanupOldMetrics(maxAge time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoffTime := time.Now().Add(-maxAge)
	deletedCount := 0

	for workloadUID, metrics := range s.metrics {
		// Filter out still valid metrics
		validMetrics := make([]*StoredMetric, 0, len(metrics))
		for _, m := range metrics {
			if m.Timestamp.After(cutoffTime) {
				validMetrics = append(validMetrics, m)
			} else {
				deletedCount++
			}
		}

		if len(validMetrics) == 0 {
			// If this workload has no valid metrics left, delete the entire entry
			delete(s.metrics, workloadUID)
		} else {
			s.metrics[workloadUID] = validMetrics
		}
	}

	if deletedCount > 0 {
		logrus.Infof("Cleaned up %d old metrics (older than %v)", deletedCount, maxAge)
	}

	return deletedCount
}

