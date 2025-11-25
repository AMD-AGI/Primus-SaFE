package metrics

import (
	"sync"
	"time"
)

// MetricActivity 记录单个metric的活跃信息
type MetricActivity struct {
	MetricName    string    `json:"metric_name"`
	LastSeenTime  time.Time `json:"last_seen_time"`
	SeenCount     int64     `json:"seen_count"`
	FirstSeenTime time.Time `json:"first_seen_time"`
}

// ActiveMetricsCache LRU缓存，记录最近5分钟活跃的metrics
type ActiveMetricsCache struct {
	mu              sync.RWMutex
	metrics         map[string]*MetricActivity
	ttl             time.Duration
	cleanupInterval time.Duration
	stopChan        chan struct{}
}

var (
	activeMetricsCache *ActiveMetricsCache
	once               sync.Once
)

// initActiveMetricsCache 初始化活跃metrics缓存
func initActiveMetricsCache() {
	once.Do(func() {
		activeMetricsCache = &ActiveMetricsCache{
			metrics:         make(map[string]*MetricActivity),
			ttl:             5 * time.Minute,
			cleanupInterval: 1 * time.Minute,
			stopChan:        make(chan struct{}),
		}
		go activeMetricsCache.startCleanup()
	})
}

// startCleanup 定期清理过期的metrics记录
func (c *ActiveMetricsCache) startCleanup() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopChan:
			return
		}
	}
}

// cleanup 清理过期的metrics
func (c *ActiveMetricsCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for name, activity := range c.metrics {
		if now.Sub(activity.LastSeenTime) > c.ttl {
			delete(c.metrics, name)
		}
	}
}

// RecordMetrics 记录一批metrics的活跃信息
func (c *ActiveMetricsCache) RecordMetrics(metricNames map[string]bool, count int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for name := range metricNames {
		if activity, exists := c.metrics[name]; exists {
			// 更新已存在的记录
			activity.LastSeenTime = now
			activity.SeenCount++
		} else {
			// 创建新记录
			c.metrics[name] = &MetricActivity{
				MetricName:    name,
				LastSeenTime:  now,
				FirstSeenTime: now,
				SeenCount:     1,
			}
		}
	}
}

// GetActiveMetrics 获取当前活跃的metrics列表
func (c *ActiveMetricsCache) GetActiveMetrics() []*MetricActivity {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*MetricActivity, 0, len(c.metrics))
	now := time.Now()

	for _, activity := range c.metrics {
		// 只返回未过期的记录
		if now.Sub(activity.LastSeenTime) <= c.ttl {
			result = append(result, &MetricActivity{
				MetricName:    activity.MetricName,
				LastSeenTime:  activity.LastSeenTime,
				SeenCount:     activity.SeenCount,
				FirstSeenTime: activity.FirstSeenTime,
			})
		}
	}

	return result
}

// GetStats 获取缓存统计信息
func (c *ActiveMetricsCache) GetStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var totalSeenCount int64
	for _, activity := range c.metrics {
		totalSeenCount += activity.SeenCount
	}

	return map[string]interface{}{
		"total_active_metrics":     len(c.metrics),
		"total_seen_count":         totalSeenCount,
		"ttl_minutes":              c.ttl.Minutes(),
		"cleanup_interval_minutes": c.cleanupInterval.Minutes(),
	}
}

// Stop 停止后台清理任务
func (c *ActiveMetricsCache) Stop() {
	close(c.stopChan)
}

// RecordActiveMetrics 记录活跃的metrics（包初始化时调用）
func RecordActiveMetrics(metricNames map[string]bool, count int) {
	initActiveMetricsCache()
	activeMetricsCache.RecordMetrics(metricNames, count)
}

// GetActiveMetricsList 获取活跃metrics列表
func GetActiveMetricsList() []*MetricActivity {
	initActiveMetricsCache()
	return activeMetricsCache.GetActiveMetrics()
}

// GetActiveMetricsStats 获取活跃metrics统计信息
func GetActiveMetricsStats() map[string]interface{} {
	initActiveMetricsCache()
	return activeMetricsCache.GetStats()
}
