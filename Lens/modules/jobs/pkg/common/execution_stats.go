package common

import (
	"time"
)

// ExecutionStats represents job execution statistics
type ExecutionStats struct {
	// RecordsProcessed is the number of records processed
	RecordsProcessed int64 `json:"records_processed,omitempty"`
	
	// BytesTransferred is the number of bytes transferred
	BytesTransferred int64 `json:"bytes_transferred,omitempty"`
	
	// ItemsCreated is the number of items created
	ItemsCreated int64 `json:"items_created,omitempty"`
	
	// ItemsUpdated is the number of items updated
	ItemsUpdated int64 `json:"items_updated,omitempty"`
	
	// ItemsDeleted is the number of items deleted
	ItemsDeleted int64 `json:"items_deleted,omitempty"`
	
	// CacheHits is the number of cache hits
	CacheHits int64 `json:"cache_hits,omitempty"`
	
	// CacheMisses is the number of cache misses
	CacheMisses int64 `json:"cache_misses,omitempty"`
	
	// QueryDuration is the query duration in seconds
	QueryDuration float64 `json:"query_duration,omitempty"`
	
	// ProcessDuration is the processing duration in seconds
	ProcessDuration float64 `json:"process_duration,omitempty"`
	
	// SaveDuration is the save duration in seconds
	SaveDuration float64 `json:"save_duration,omitempty"`
	
	// ErrorCount is the error count
	ErrorCount int64 `json:"error_count,omitempty"`
	
	// WarningCount is the warning count
	WarningCount int64 `json:"warning_count,omitempty"`
	
	// CustomMetrics allows jobs to add specific statistics
	CustomMetrics map[string]interface{} `json:"custom_metrics,omitempty"`
	
	// Messages is the list of messages during execution
	Messages []string `json:"messages,omitempty"`
}

// ExecutionResult represents the result of job execution
type ExecutionResult struct {
	// Success indicates whether the execution was successful
	Success bool `json:"success"`
	
	// Error contains error information if any
	Error error `json:"error,omitempty"`
	
	// Stats contains execution statistics
	Stats *ExecutionStats `json:"stats,omitempty"`
	
	// StartTime is the start time
	StartTime time.Time `json:"start_time"`
	
	// EndTime is the end time
	EndTime time.Time `json:"end_time"`
	
	// Duration is the execution duration in seconds
	Duration float64 `json:"duration"`
}

// NewExecutionStats creates a new execution statistics instance
func NewExecutionStats() *ExecutionStats {
	return &ExecutionStats{
		CustomMetrics: make(map[string]interface{}),
		Messages:      make([]string, 0),
	}
}

// AddMessage adds a message to the statistics
func (s *ExecutionStats) AddMessage(message string) {
	if s.Messages == nil {
		s.Messages = make([]string, 0)
	}
	s.Messages = append(s.Messages, message)
}

// AddCustomMetric adds a custom metric to the statistics
func (s *ExecutionStats) AddCustomMetric(key string, value interface{}) {
	if s.CustomMetrics == nil {
		s.CustomMetrics = make(map[string]interface{})
	}
	s.CustomMetrics[key] = value
}

// Merge merges data from another ExecutionStats
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
	
	// Merge custom metrics
	if other.CustomMetrics != nil {
		if s.CustomMetrics == nil {
			s.CustomMetrics = make(map[string]interface{})
		}
		for k, v := range other.CustomMetrics {
			s.CustomMetrics[k] = v
		}
	}
	
	// Merge messages
	if other.Messages != nil {
		if s.Messages == nil {
			s.Messages = make([]string, 0)
		}
		s.Messages = append(s.Messages, other.Messages...)
	}
}

