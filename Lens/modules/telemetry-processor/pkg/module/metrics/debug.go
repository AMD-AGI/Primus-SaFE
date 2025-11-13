package metrics

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/VictoriaMetrics/VictoriaMetrics/lib/prompb"
)

// DebugConfig 调试配置
type DebugConfig struct {
	Enabled        bool              `json:"enabled"`         // 是否开启调试
	MetricPattern  string            `json:"metric_pattern"`  // 指标名称模式（支持正则）
	LabelSelectors map[string]string `json:"label_selectors"` // 标签选择器，key=value 格式
	MaxRecords     int               `json:"max_records"`     // 最大记录数，防止内存溢出
}

// DebugRecord 调试记录
type DebugRecord struct {
	Timestamp   time.Time         `json:"timestamp"`
	MetricName  string            `json:"metric_name"`
	Labels      map[string]string `json:"labels"`
	PodName     string            `json:"pod_name"`
	PodUID      string            `json:"pod_uid"`
	Status      string            `json:"status"`       // "passed" 或 "filtered"
	Reason      string            `json:"reason"`       // 不通过的原因或通过的信息
	SampleCount int               `json:"sample_count"` // 样本数量
}

// DebugManager 调试管理器
type DebugManager struct {
	mu      sync.RWMutex
	config  *DebugConfig
	records []DebugRecord
	stats   DebugStats
}

// DebugStats 调试统计
type DebugStats struct {
	TotalMatched   int       `json:"total_matched"`  // 总匹配数
	TotalPassed    int       `json:"total_passed"`   // 通过数
	TotalFiltered  int       `json:"total_filtered"` // 过滤数
	LastUpdateTime time.Time `json:"last_update_time"`
}

var debugManager = &DebugManager{
	config: &DebugConfig{
		Enabled:    false,
		MaxRecords: 1000, // 默认最多保存 1000 条记录
	},
	records: make([]DebugRecord, 0),
}

// SetDebugConfig 设置调试配置
func SetDebugConfig(config *DebugConfig) {
	debugManager.mu.Lock()
	defer debugManager.mu.Unlock()

	if config.MaxRecords <= 0 {
		config.MaxRecords = 1000
	}
	debugManager.config = config

	// 如果开启了调试，清空之前的记录
	if config.Enabled {
		debugManager.records = make([]DebugRecord, 0)
		debugManager.stats = DebugStats{
			LastUpdateTime: time.Now(),
		}
	}
}

// GetDebugConfig 获取调试配置
func GetDebugConfig() *DebugConfig {
	debugManager.mu.RLock()
	defer debugManager.mu.RUnlock()

	configCopy := *debugManager.config
	return &configCopy
}

// GetDebugRecords 获取调试记录
func GetDebugRecords() ([]DebugRecord, DebugStats) {
	debugManager.mu.RLock()
	defer debugManager.mu.RUnlock()

	// 返回副本，避免并发问题
	recordsCopy := make([]DebugRecord, len(debugManager.records))
	copy(recordsCopy, debugManager.records)

	return recordsCopy, debugManager.stats
}

// ClearDebugRecords 清空调试记录
func ClearDebugRecords() {
	debugManager.mu.Lock()
	defer debugManager.mu.Unlock()

	debugManager.records = make([]DebugRecord, 0)
	debugManager.stats = DebugStats{
		LastUpdateTime: time.Now(),
	}
}

// shouldDebug 判断是否需要调试这个时间序列
func shouldDebug(labels []prompb.Label) bool {
	debugManager.mu.RLock()
	defer debugManager.mu.RUnlock()

	if !debugManager.config.Enabled {
		return false
	}

	// 检查 metric name
	metricName := getName(labels)
	if debugManager.config.MetricPattern != "" {
		matched, err := regexp.MatchString(debugManager.config.MetricPattern, metricName)
		if err != nil || !matched {
			return false
		}
	}

	// 检查 label selectors
	if len(debugManager.config.LabelSelectors) > 0 {
		labelMap := labelsToMap(labels)
		for key, value := range debugManager.config.LabelSelectors {
			// 支持简单的通配符匹配
			if !matchLabelValue(labelMap[key], value) {
				return false
			}
		}
	}

	return true
}

// recordDebug 记录调试信息
func recordDebug(record DebugRecord) {
	debugManager.mu.Lock()
	defer debugManager.mu.Unlock()

	// 更新统计
	debugManager.stats.TotalMatched++
	if record.Status == "passed" {
		debugManager.stats.TotalPassed++
	} else {
		debugManager.stats.TotalFiltered++
	}
	debugManager.stats.LastUpdateTime = time.Now()

	// 添加记录，如果超过最大记录数，删除最旧的
	if len(debugManager.records) >= debugManager.config.MaxRecords {
		// 删除最旧的 10% 记录，避免频繁操作
		removeCount := debugManager.config.MaxRecords / 10
		if removeCount < 1 {
			removeCount = 1
		}
		debugManager.records = debugManager.records[removeCount:]
	}

	debugManager.records = append(debugManager.records, record)
}

// labelsToMap 将 prompb.Label 数组转换为 map
func labelsToMap(labels []prompb.Label) map[string]string {
	result := make(map[string]string)
	for _, label := range labels {
		result[label.Name] = label.Value
	}
	return result
}

// matchLabelValue 匹配标签值，支持通配符 *
func matchLabelValue(actual, pattern string) bool {
	if pattern == "*" {
		return actual != ""
	}

	// 如果包含 *，使用正则匹配
	if strings.Contains(pattern, "*") {
		// 将通配符转换为正则表达式
		regexPattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), "\\*", ".*") + "$"
		matched, err := regexp.MatchString(regexPattern, actual)
		return err == nil && matched
	}

	// 精确匹配
	return actual == pattern
}

// formatDebugReason 格式化调试原因
func formatDebugReason(reason string, details ...interface{}) string {
	if len(details) > 0 {
		return fmt.Sprintf("%s: %v", reason, details)
	}
	return reason
}
