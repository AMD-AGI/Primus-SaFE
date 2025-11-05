package common

import (
	"time"
)

// ExecutionStats 表示Job执行的统计信息
type ExecutionStats struct {
	// RecordsProcessed 处理的记录数
	RecordsProcessed int64 `json:"records_processed,omitempty"`
	
	// BytesTransferred 传输的字节数
	BytesTransferred int64 `json:"bytes_transferred,omitempty"`
	
	// ItemsCreated 创建的项目数
	ItemsCreated int64 `json:"items_created,omitempty"`
	
	// ItemsUpdated 更新的项目数
	ItemsUpdated int64 `json:"items_updated,omitempty"`
	
	// ItemsDeleted 删除的项目数
	ItemsDeleted int64 `json:"items_deleted,omitempty"`
	
	// CacheHits 缓存命中数
	CacheHits int64 `json:"cache_hits,omitempty"`
	
	// CacheMisses 缓存未命中数
	CacheMisses int64 `json:"cache_misses,omitempty"`
	
	// QueryDuration 查询耗时（秒）
	QueryDuration float64 `json:"query_duration,omitempty"`
	
	// ProcessDuration 处理耗时（秒）
	ProcessDuration float64 `json:"process_duration,omitempty"`
	
	// SaveDuration 保存耗时（秒）
	SaveDuration float64 `json:"save_duration,omitempty"`
	
	// ErrorCount 错误计数
	ErrorCount int64 `json:"error_count,omitempty"`
	
	// WarningCount 警告计数
	WarningCount int64 `json:"warning_count,omitempty"`
	
	// CustomMetrics 自定义指标，允许job添加特定的统计信息
	CustomMetrics map[string]interface{} `json:"custom_metrics,omitempty"`
	
	// Messages 执行过程中的消息列表
	Messages []string `json:"messages,omitempty"`
}

// ExecutionResult 表示Job执行的结果
type ExecutionResult struct {
	// Success 是否成功
	Success bool `json:"success"`
	
	// Error 错误信息
	Error error `json:"error,omitempty"`
	
	// Stats 执行统计信息
	Stats *ExecutionStats `json:"stats,omitempty"`
	
	// StartTime 开始时间
	StartTime time.Time `json:"start_time"`
	
	// EndTime 结束时间
	EndTime time.Time `json:"end_time"`
	
	// Duration 执行时长（秒）
	Duration float64 `json:"duration"`
}

// NewExecutionStats 创建新的执行统计信息
func NewExecutionStats() *ExecutionStats {
	return &ExecutionStats{
		CustomMetrics: make(map[string]interface{}),
		Messages:      make([]string, 0),
	}
}

// AddMessage 添加消息
func (s *ExecutionStats) AddMessage(message string) {
	if s.Messages == nil {
		s.Messages = make([]string, 0)
	}
	s.Messages = append(s.Messages, message)
}

// AddCustomMetric 添加自定义指标
func (s *ExecutionStats) AddCustomMetric(key string, value interface{}) {
	if s.CustomMetrics == nil {
		s.CustomMetrics = make(map[string]interface{})
	}
	s.CustomMetrics[key] = value
}

// Merge 合并另一个ExecutionStats的数据
func (s *ExecutionStats) Merge(other *ExecutionStats) {
	if other == nil {
		return
	}
	
	s.RecordsProcessed += other.RecordsProcessed
	s.BytesTransferred += other.BytesTransferred
	s.ItemsCreated += other.ItemsCreated
	s.ItemsUpdated += other.ItemsUpdated
	s.ItemsDeleted += other.ItemsDeleted
	s.CacheHits += other.CacheHits
	s.CacheMisses += other.CacheMisses
	s.QueryDuration += other.QueryDuration
	s.ProcessDuration += other.ProcessDuration
	s.SaveDuration += other.SaveDuration
	s.ErrorCount += other.ErrorCount
	s.WarningCount += other.WarningCount
	
	// 合并自定义指标
	if other.CustomMetrics != nil {
		if s.CustomMetrics == nil {
			s.CustomMetrics = make(map[string]interface{})
		}
		for k, v := range other.CustomMetrics {
			s.CustomMetrics[k] = v
		}
	}
	
	// 合并消息
	if other.Messages != nil {
		if s.Messages == nil {
			s.Messages = make([]string, 0)
		}
		s.Messages = append(s.Messages, other.Messages...)
	}
}

