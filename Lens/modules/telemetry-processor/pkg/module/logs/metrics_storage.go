package logs

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// InMemoryMetricsStorage 内存指标存储实现
type InMemoryMetricsStorage struct {
	mu      sync.RWMutex
	metrics map[string][]*StoredMetric // workloadUID -> metrics
	maxSize int                        // 每个 workload 最多保存的指标数量
}

// NewInMemoryMetricsStorage 创建内存指标存储
func NewInMemoryMetricsStorage(maxSize int) *InMemoryMetricsStorage {
	if maxSize <= 0 {
		maxSize = 10000 // 默认最多保存 10000 条
	}
	return &InMemoryMetricsStorage{
		metrics: make(map[string][]*StoredMetric),
		maxSize: maxSize,
	}
}

// Store 存储指标
func (s *InMemoryMetricsStorage) Store(ctx context.Context, metric *StoredMetric) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 获取或创建该 workload 的指标列表
	metrics, exists := s.metrics[metric.WorkloadUID]
	if !exists {
		metrics = make([]*StoredMetric, 0, 100)
	}

	// 添加新指标
	metrics = append(metrics, metric)

	// 如果超过最大大小，删除最旧的
	if len(metrics) > s.maxSize {
		metrics = metrics[len(metrics)-s.maxSize:]
	}

	s.metrics[metric.WorkloadUID] = metrics

	logrus.Debugf("Stored metric %s for workload %s (total: %d)", 
		metric.Name, metric.WorkloadUID, len(metrics))

	return nil
}

// Query 查询指标
func (s *InMemoryMetricsStorage) Query(ctx context.Context, workloadUID string, metricName string) ([]*StoredMetric, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	allMetrics, exists := s.metrics[workloadUID]
	if !exists {
		return []*StoredMetric{}, nil
	}

	// 如果指定了指标名称，过滤
	if metricName != "" {
		result := make([]*StoredMetric, 0)
		for _, m := range allMetrics {
			if m.Name == metricName {
				result = append(result, m)
			}
		}
		return result, nil
	}

	// 返回所有指标
	return allMetrics, nil
}

// GetMetricsCount 获取指标数量（用于监控）
func (s *InMemoryMetricsStorage) GetMetricsCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := 0
	for _, metrics := range s.metrics {
		total += len(metrics)
	}
	return total
}

// CleanupOldMetrics 清理超过指定时间的旧指标
func (s *InMemoryMetricsStorage) CleanupOldMetrics(maxAge time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoffTime := time.Now().Add(-maxAge)
	deletedCount := 0

	for workloadUID, metrics := range s.metrics {
		// 过滤出仍然有效的指标
		validMetrics := make([]*StoredMetric, 0, len(metrics))
		for _, m := range metrics {
			if m.Timestamp.After(cutoffTime) {
				validMetrics = append(validMetrics, m)
			} else {
				deletedCount++
			}
		}

		if len(validMetrics) == 0 {
			// 如果该 workload 没有有效指标了，删除整个条目
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

