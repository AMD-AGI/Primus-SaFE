package common

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime/debug"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// Job interface definition (minimized to avoid circular imports)
type Job interface {
	Schedule() string
}

// HistoryService is the job execution history recording service
type HistoryService struct {
	clusterName string
	hostname    string
}

// NewHistoryService creates a new history service
func NewHistoryService() *HistoryService {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	hostname, _ := os.Hostname()

	return &HistoryService{
		clusterName: clusterName,
		hostname:    hostname,
	}
}

// RecordJobExecution records job execution history
func (h *HistoryService) RecordJobExecution(
	ctx context.Context,
	job Job,
	result *ExecutionResult,
) error {
	jobName := getJobName(job)
	jobType := getJobType(job)
	schedule := job.Schedule()

	// Determine status
	status := "success"
	if !result.Success {
		if ctx.Err() == context.Canceled {
			status = "cancelled"
		} else if ctx.Err() == context.DeadlineExceeded {
			status = "timeout"
		} else {
			status = "failed"
		}
	}

	// Truncate fields to fit database column limits
	clusterName := truncateString(h.clusterName, 100) // cluster_name VARCHAR(100)
	hostname := truncateString(h.hostname, 255)       // hostname VARCHAR(255)

	// Build history record
	history := &dbmodel.JobExecutionHistory{
		JobName:         jobName,
		JobType:         jobType,
		Schedule:        schedule,
		Status:          status,
		StartedAt:       result.StartTime,
		EndedAt:         result.EndTime,
		DurationSeconds: result.Duration,
		ClusterName:     clusterName,
		Hostname:        hostname,
	}

	// Set error information
	if result.Error != nil {
		history.ErrorMessage = result.Error.Error()
		history.ErrorStack = string(debug.Stack())
	}

	// Serialize metadata
	metadata := dbmodel.ExtType{
		"cluster_name": h.clusterName,
		"hostname":     h.hostname,
		"schedule":     schedule,
	}
	history.Metadata = metadata

	// Serialize execution statistics - convert stats to map
	if result.Stats != nil {
		statsBytes, err := json.Marshal(result.Stats)
		if err == nil {
			var statsMap map[string]interface{}
			if err := json.Unmarshal(statsBytes, &statsMap); err == nil {
				history.ExecutionStats = dbmodel.ExtType(statsMap)
			}
		}
	}

	// Save to database
	db := clientsets.GetClusterManager().GetCurrentClusterClients().StorageClientSet.DB
	if err := db.Create(history).Error; err != nil {
		log.Errorf("Failed to save job execution history for %s: %v", jobName, err)
		return fmt.Errorf("failed to save job execution history: %w", err)
	}

	log.Debugf("Saved job execution history for %s (ID: %d, Status: %s, Duration: %.2fs)",
		jobName, history.ID, status, result.Duration)

	return nil
}

// getJobType gets the type name of the Job (shortened to fit database column limit)
func getJobType(job Job) string {
	jobType := reflect.TypeOf(job)
	if jobType.Kind() == reflect.Ptr {
		jobType = jobType.Elem()
	}

	// Return shortened package name + type name to fit VARCHAR(100) column
	pkgPath := jobType.PkgPath()
	typeName := jobType.Name()

	if pkgPath != "" {
		// Extract only the last part of the package path (e.g., "gpu_aggregation_backfill")
		// to avoid exceeding the 100 character limit
		parts := splitPath(pkgPath)
		if len(parts) > 0 {
			shortPkg := parts[len(parts)-1]
			result := shortPkg + "." + typeName
			// Ensure result fits in 100 characters
			if len(result) > 100 {
				return typeName // Fall back to just the type name
			}
			return result
		}
	}
	return typeName
}

// splitPath splits a path by "/" separator
func splitPath(path string) []string {
	result := make([]string, 0)
	start := 0
	for i, c := range path {
		if c == '/' {
			if i > start {
				result = append(result, path[start:i])
			}
			start = i + 1
		}
	}
	if start < len(path) {
		result = append(result, path[start:])
	}
	return result
}

// truncateString truncates a string to the specified maximum length
// This is used to ensure strings fit within database column limits
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// getJobName returns the job name (using type name)
func getJobName(job Job) string {
	jobType := reflect.TypeOf(job)
	if jobType.Kind() == reflect.Ptr {
		jobType = jobType.Elem()
	}
	return jobType.Name()
}

// QueryJobHistory queries job execution history
func (h *HistoryService) QueryJobHistory(
	jobName string,
	limit int,
) ([]*dbmodel.JobExecutionHistory, error) {
	db := clientsets.GetClusterManager().GetCurrentClusterClients().StorageClientSet.DB

	var histories []*dbmodel.JobExecutionHistory
	query := db.Where("job_name = ?", jobName).
		Order("started_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&histories).Error; err != nil {
		return nil, fmt.Errorf("failed to query job history: %w", err)
	}

	return histories, nil
}

// QueryJobHistoryByTimeRange queries job execution history within a specified time range
func (h *HistoryService) QueryJobHistoryByTimeRange(
	jobName string,
	startTime, endTime time.Time,
) ([]*dbmodel.JobExecutionHistory, error) {
	db := clientsets.GetClusterManager().GetCurrentClusterClients().StorageClientSet.DB

	var histories []*dbmodel.JobExecutionHistory
	query := db.Where("job_name = ? AND started_at >= ? AND started_at <= ?",
		jobName, startTime, endTime).
		Order("started_at DESC")

	if err := query.Find(&histories).Error; err != nil {
		return nil, fmt.Errorf("failed to query job history by time range: %w", err)
	}

	return histories, nil
}

// QueryAllJobsHistory queries recent execution history for all jobs
func (h *HistoryService) QueryAllJobsHistory(limit int) ([]*dbmodel.JobExecutionHistory, error) {
	db := clientsets.GetClusterManager().GetCurrentClusterClients().StorageClientSet.DB

	var histories []*dbmodel.JobExecutionHistory
	query := db.Order("started_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&histories).Error; err != nil {
		return nil, fmt.Errorf("failed to query all jobs history: %w", err)
	}

	return histories, nil
}

// GetJobStatistics gets job statistics
func (h *HistoryService) GetJobStatistics(jobName string, days int) (*JobStatistics, error) {
	startTime := time.Now().AddDate(0, 0, -days)

	histories, err := h.QueryJobHistoryByTimeRange(jobName, startTime, time.Now())
	if err != nil {
		return nil, err
	}

	stats := &JobStatistics{
		JobName:       jobName,
		TotalRuns:     len(histories),
		SuccessCount:  0,
		FailureCount:  0,
		TimeoutCount:  0,
		TotalDuration: 0,
	}

	for _, h := range histories {
		switch h.Status {
		case "success":
			stats.SuccessCount++
		case "failed":
			stats.FailureCount++
		case "timeout":
			stats.TimeoutCount++
		}

		stats.TotalDuration += h.DurationSeconds
	}

	if stats.TotalRuns > 0 {
		stats.AverageDuration = stats.TotalDuration / float64(stats.TotalRuns)
		stats.SuccessRate = float64(stats.SuccessCount) / float64(stats.TotalRuns) * 100
	}

	return stats, nil
}

// JobStatistics holds job statistics information
type JobStatistics struct {
	JobName         string  `json:"job_name"`
	TotalRuns       int     `json:"total_runs"`
	SuccessCount    int     `json:"success_count"`
	FailureCount    int     `json:"failure_count"`
	TimeoutCount    int     `json:"timeout_count"`
	SuccessRate     float64 `json:"success_rate"`
	AverageDuration float64 `json:"average_duration"`
	TotalDuration   float64 `json:"total_duration"`
}

// CleanupOldHistory cleans up old history records
func (h *HistoryService) CleanupOldHistory(retentionDays int) (int64, error) {
	db := clientsets.GetClusterManager().GetCurrentClusterClients().StorageClientSet.DB

	cutoffTime := time.Now().AddDate(0, 0, -retentionDays)

	result := db.Where("started_at < ?", cutoffTime).
		Delete(&dbmodel.JobExecutionHistory{})

	if result.Error != nil {
		return 0, fmt.Errorf("failed to cleanup old history: %w", result.Error)
	}

	log.Infof("Cleaned up %d old job execution history records (older than %d days)",
		result.RowsAffected, retentionDays)

	return result.RowsAffected, nil
}
