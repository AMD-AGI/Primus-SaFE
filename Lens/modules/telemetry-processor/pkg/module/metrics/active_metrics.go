package metrics

import (
	"sync"
	"time"
)

// MetricActivity records activity information for a single metric
type MetricActivity struct {
	MetricName    string    `json:"metric_name"`
	LastSeenTime  time.Time `json:"last_seen_time"`
	SeenCount     int64     `json:"seen_count"`
	FirstSeenTime time.Time `json:"first_seen_time"`
}

// ActiveMetricsCache LRU cache, records active metrics from the last 5 minutes
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

// initActiveMetricsCache initializes active metrics cache
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

// startCleanup periodically cleans up expired metrics records
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

// cleanup cleans up expired metrics
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

// RecordMetrics records activity information for a batch of metrics
func (c *ActiveMetricsCache) RecordMetrics(metricNames map[string]bool, count int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for name := range metricNames {
		if activity, exists := c.metrics[name]; exists {
			// Update existing record
			activity.LastSeenTime = now
			activity.SeenCount++
		} else {
			// Create new record
			c.metrics[name] = &MetricActivity{
				MetricName:    name,
				LastSeenTime:  now,
				FirstSeenTime: now,
				SeenCount:     1,
			}
		}
	}
}

// GetActiveMetrics gets list of currently active metrics
func (c *ActiveMetricsCache) GetActiveMetrics() []*MetricActivity {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*MetricActivity, 0, len(c.metrics))
	now := time.Now()

	for _, activity := range c.metrics {
		// Only return unexpired records
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

// GetStats gets cache statistics
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

// Stop stops background cleanup task
func (c *ActiveMetricsCache) Stop() {
	close(c.stopChan)
}

// RecordActiveMetrics records active metrics (called during package initialization)
func RecordActiveMetrics(metricNames map[string]bool, count int) {
	initActiveMetricsCache()
	activeMetricsCache.RecordMetrics(metricNames, count)
}

// GetActiveMetricsList gets active metrics list
func GetActiveMetricsList() []*MetricActivity {
	initActiveMetricsCache()
	return activeMetricsCache.GetActiveMetrics()
}

// GetActiveMetricsStats gets active metrics statistics
func GetActiveMetricsStats() map[string]interface{} {
	initActiveMetricsCache()
	return activeMetricsCache.GetStats()
}
