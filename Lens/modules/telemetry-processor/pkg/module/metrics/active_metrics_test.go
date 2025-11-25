package metrics

import (
	"testing"
	"time"
)

func TestActiveMetricsCache(t *testing.T) {
	// 创建新的缓存实例用于测试
	cache := &ActiveMetricsCache{
		metrics:         make(map[string]*MetricActivity),
		ttl:             1 * time.Second, // 使用较短的TTL便于测试
		cleanupInterval: 500 * time.Millisecond,
		stopChan:        make(chan struct{}),
	}
	defer cache.Stop()

	// 测试记录metrics
	t.Run("RecordMetrics", func(t *testing.T) {
		metricNames := map[string]bool{
			"workload_gpu_utilization": true,
			"workload_gpu_memory_used": true,
		}
		cache.RecordMetrics(metricNames, 2)

		stats := cache.GetStats()
		if stats["total_active_metrics"].(int) != 2 {
			t.Errorf("Expected 2 active metrics, got %d", stats["total_active_metrics"].(int))
		}
	})

	// 测试获取活跃metrics
	t.Run("GetActiveMetrics", func(t *testing.T) {
		metrics := cache.GetActiveMetrics()
		if len(metrics) != 2 {
			t.Errorf("Expected 2 metrics, got %d", len(metrics))
		}

		// 验证metric名称
		names := make(map[string]bool)
		for _, m := range metrics {
			names[m.MetricName] = true
		}
		if !names["workload_gpu_utilization"] || !names["workload_gpu_memory_used"] {
			t.Error("Expected metrics not found")
		}
	})

	// 测试重复记录会增加计数
	t.Run("IncrementSeenCount", func(t *testing.T) {
		metricNames := map[string]bool{
			"workload_gpu_utilization": true,
		}
		cache.RecordMetrics(metricNames, 1)

		metrics := cache.GetActiveMetrics()
		for _, m := range metrics {
			if m.MetricName == "workload_gpu_utilization" {
				if m.SeenCount != 2 {
					t.Errorf("Expected seen count 2, got %d", m.SeenCount)
				}
			}
		}
	})

	// 测试TTL过期清理
	t.Run("TTLExpiration", func(t *testing.T) {
		// 等待TTL过期
		time.Sleep(1500 * time.Millisecond)
		cache.cleanup()

		metrics := cache.GetActiveMetrics()
		if len(metrics) != 0 {
			t.Errorf("Expected 0 metrics after TTL, got %d", len(metrics))
		}
	})
}

func TestGlobalFunctions(t *testing.T) {
	// 测试全局函数
	t.Run("GlobalFunctions", func(t *testing.T) {
		metricNames := map[string]bool{
			"workload_test_metric": true,
		}
		RecordActiveMetrics(metricNames, 1)

		metrics := GetActiveMetricsList()
		found := false
		for _, m := range metrics {
			if m.MetricName == "workload_test_metric" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected metric not found in global cache")
		}

		stats := GetActiveMetricsStats()
		if stats["total_active_metrics"].(int) < 1 {
			t.Error("Expected at least 1 active metric in global cache")
		}
	})
}
