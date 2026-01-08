// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package metrics

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/VictoriaMetrics/VictoriaMetrics/lib/prompb"
)

// DebugConfig debug configuration
type DebugConfig struct {
	Enabled        bool              `json:"enabled"`         // whether to enable debugging
	MetricPattern  string            `json:"metric_pattern"`  // metric name pattern (supports regex)
	LabelSelectors map[string]string `json:"label_selectors"` // label selectors in key=value format
	MaxRecords     int               `json:"max_records"`     // maximum number of records to prevent memory overflow
}

// DebugRecord debug record
type DebugRecord struct {
	Timestamp   time.Time         `json:"timestamp"`
	MetricName  string            `json:"metric_name"`
	Labels      map[string]string `json:"labels"`
	PodName     string            `json:"pod_name"`
	PodUID      string            `json:"pod_uid"`
	Status      string            `json:"status"`       // "passed" or "filtered"
	Reason      string            `json:"reason"`       // reason for rejection or information about passing
	SampleCount int               `json:"sample_count"` // number of samples
}

// DebugManager debug manager
type DebugManager struct {
	mu      sync.RWMutex
	config  *DebugConfig
	records []DebugRecord
	stats   DebugStats
}

// DebugStats debug statistics
type DebugStats struct {
	TotalMatched   int       `json:"total_matched"`  // total number of matches
	TotalPassed    int       `json:"total_passed"`   // number passed
	TotalFiltered  int       `json:"total_filtered"` // number filtered
	LastUpdateTime time.Time `json:"last_update_time"`
}

var debugManager = &DebugManager{
	config: &DebugConfig{
		Enabled:    false,
		MaxRecords: 1000, // default maximum of 1000 records
	},
	records: make([]DebugRecord, 0),
}

// SetDebugConfig sets debug configuration
func SetDebugConfig(config *DebugConfig) {
	debugManager.mu.Lock()
	defer debugManager.mu.Unlock()

	if config.MaxRecords <= 0 {
		config.MaxRecords = 1000
	}
	debugManager.config = config

	// Clear previous records if debugging is enabled
	if config.Enabled {
		debugManager.records = make([]DebugRecord, 0)
		debugManager.stats = DebugStats{
			LastUpdateTime: time.Now(),
		}
	}
}

// GetDebugConfig gets debug configuration
func GetDebugConfig() *DebugConfig {
	debugManager.mu.RLock()
	defer debugManager.mu.RUnlock()

	configCopy := *debugManager.config
	return &configCopy
}

// GetDebugRecords gets debug records
func GetDebugRecords() ([]DebugRecord, DebugStats) {
	debugManager.mu.RLock()
	defer debugManager.mu.RUnlock()

	// Return a copy to avoid concurrency issues
	recordsCopy := make([]DebugRecord, len(debugManager.records))
	copy(recordsCopy, debugManager.records)

	return recordsCopy, debugManager.stats
}

// ClearDebugRecords clears debug records
func ClearDebugRecords() {
	debugManager.mu.Lock()
	defer debugManager.mu.Unlock()

	debugManager.records = make([]DebugRecord, 0)
	debugManager.stats = DebugStats{
		LastUpdateTime: time.Now(),
	}
}

// shouldDebug determines whether to debug this time series
func shouldDebug(labels []prompb.Label) bool {
	debugManager.mu.RLock()
	defer debugManager.mu.RUnlock()

	if !debugManager.config.Enabled {
		return false
	}

	// Check metric name
	metricName := getName(labels)
	if debugManager.config.MetricPattern != "" {
		matched, err := regexp.MatchString(debugManager.config.MetricPattern, metricName)
		if err != nil || !matched {
			return false
		}
	}

	// Check label selectors
	if len(debugManager.config.LabelSelectors) > 0 {
		labelMap := labelsToMap(labels)
		for key, value := range debugManager.config.LabelSelectors {
			// Support simple wildcard matching
			if !matchLabelValue(labelMap[key], value) {
				return false
			}
		}
	}

	return true
}

// recordDebug records debug information
func recordDebug(record DebugRecord) {
	debugManager.mu.Lock()
	defer debugManager.mu.Unlock()

	// Update statistics
	debugManager.stats.TotalMatched++
	if record.Status == "passed" {
		debugManager.stats.TotalPassed++
	} else {
		debugManager.stats.TotalFiltered++
	}
	debugManager.stats.LastUpdateTime = time.Now()

	// Add record, remove oldest if exceeding max records
	if len(debugManager.records) >= debugManager.config.MaxRecords {
		// Remove oldest 10% of records to avoid frequent operations
		removeCount := debugManager.config.MaxRecords / 10
		if removeCount < 1 {
			removeCount = 1
		}
		debugManager.records = debugManager.records[removeCount:]
	}

	debugManager.records = append(debugManager.records, record)
}

// labelsToMap converts prompb.Label array to map
func labelsToMap(labels []prompb.Label) map[string]string {
	result := make(map[string]string)
	for _, label := range labels {
		result[label.Name] = label.Value
	}
	return result
}

// matchLabelValue matches label values, supports wildcard *
func matchLabelValue(actual, pattern string) bool {
	if pattern == "*" {
		return actual != ""
	}

	// Use regex matching if contains *
	if strings.Contains(pattern, "*") {
		// Convert wildcard to regular expression
		regexPattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), "\\*", ".*") + "$"
		matched, err := regexp.MatchString(regexPattern, actual)
		return err == nil && matched
	}

	// Exact match
	return actual == pattern
}

// formatDebugReason formats debug reason
func formatDebugReason(reason string, details ...interface{}) string {
	if len(details) > 0 {
		return fmt.Sprintf("%s: %v", reason, details)
	}
	return reason
}
