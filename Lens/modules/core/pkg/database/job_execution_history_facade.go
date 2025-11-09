package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// JobExecutionHistoryFacadeInterface defines the database operation interface for JobExecutionHistory
type JobExecutionHistoryFacadeInterface interface {
	// Query operations
	GetJobExecutionHistoryByID(ctx context.Context, id int64) (*model.JobExecutionHistory, error)
	ListJobExecutionHistories(ctx context.Context, filter *JobExecutionHistoryFilter) ([]*model.JobExecutionHistory, int64, error)
	GetRecentFailures(ctx context.Context, limit int) ([]*model.JobExecutionHistory, error)
	GetJobStatistics(ctx context.Context, jobName string) (*JobStatistics, error)
	GetDistinctJobTypes(ctx context.Context) ([]string, error)

	// WithCluster method
	WithCluster(clusterName string) JobExecutionHistoryFacadeInterface
}

// JobExecutionHistoryFilter defines filter conditions for querying job execution histories
type JobExecutionHistoryFilter struct {
	JobName     *string
	JobType     *string
	Status      *string
	ClusterName *string
	Hostname    *string
	StartTimeFrom *time.Time
	StartTimeTo   *time.Time
	MinDuration   *float64
	MaxDuration   *float64
	Offset      int
	Limit       int
	OrderBy     string // Default: "started_at DESC"
}

// JobStatistics contains statistics for a specific job
type JobStatistics struct {
	JobName       string  `json:"job_name"`
	TotalRuns     int64   `json:"total_runs"`
	SuccessCount  int64   `json:"success_count"`
	FailureCount  int64   `json:"failure_count"`
	CancelledCount int64  `json:"cancelled_count"`
	TimeoutCount  int64   `json:"timeout_count"`
	AvgDuration   float64 `json:"avg_duration"`
	MinDuration   float64 `json:"min_duration"`
	MaxDuration   float64 `json:"max_duration"`
	LastRunTime   time.Time `json:"last_run_time"`
	LastStatus    string  `json:"last_status"`
}

// JobExecutionHistoryFacade implements JobExecutionHistoryFacadeInterface
type JobExecutionHistoryFacade struct {
	BaseFacade
}

// NewJobExecutionHistoryFacade creates a new JobExecutionHistoryFacade instance
func NewJobExecutionHistoryFacade() JobExecutionHistoryFacadeInterface {
	return &JobExecutionHistoryFacade{}
}

func (f *JobExecutionHistoryFacade) WithCluster(clusterName string) JobExecutionHistoryFacadeInterface {
	return &JobExecutionHistoryFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// GetJobExecutionHistoryByID retrieves a job execution history record by ID
func (f *JobExecutionHistoryFacade) GetJobExecutionHistoryByID(ctx context.Context, id int64) (*model.JobExecutionHistory, error) {
	db := f.getDB().WithContext(ctx)
	var history model.JobExecutionHistory
	err := db.Where("id = ?", id).First(&history).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &history, nil
}

// ListJobExecutionHistories lists job execution histories with filtering and pagination
func (f *JobExecutionHistoryFacade) ListJobExecutionHistories(ctx context.Context, filter *JobExecutionHistoryFilter) ([]*model.JobExecutionHistory, int64, error) {
	db := f.getDB().WithContext(ctx)
	query := db.Model(&model.JobExecutionHistory{})

	// Apply filters
	if filter.JobName != nil && *filter.JobName != "" {
		query = query.Where("job_name LIKE ?", "%"+*filter.JobName+"%")
	}
	if filter.JobType != nil && *filter.JobType != "" {
		query = query.Where("job_type LIKE ?", "%"+*filter.JobType+"%")
	}
	if filter.Status != nil && *filter.Status != "" {
		query = query.Where("status = ?", *filter.Status)
	}
	if filter.ClusterName != nil && *filter.ClusterName != "" {
		query = query.Where("cluster_name = ?", *filter.ClusterName)
	}
	if filter.Hostname != nil && *filter.Hostname != "" {
		query = query.Where("hostname = ?", *filter.Hostname)
	}
	if filter.StartTimeFrom != nil {
		query = query.Where("started_at >= ?", *filter.StartTimeFrom)
	}
	if filter.StartTimeTo != nil {
		query = query.Where("started_at <= ?", *filter.StartTimeTo)
	}
	if filter.MinDuration != nil {
		query = query.Where("duration_seconds >= ?", *filter.MinDuration)
	}
	if filter.MaxDuration != nil {
		query = query.Where("duration_seconds <= ?", *filter.MaxDuration)
	}

	// Count total records
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply ordering
	orderBy := "started_at DESC"
	if filter.OrderBy != "" {
		orderBy = filter.OrderBy
	}
	query = query.Order(orderBy)

	// Apply pagination
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit).Offset(filter.Offset)
	}

	// Execute query
	var histories []*model.JobExecutionHistory
	err := query.Find(&histories).Error
	return histories, total, err
}

// GetRecentFailures retrieves recent failed job executions
func (f *JobExecutionHistoryFacade) GetRecentFailures(ctx context.Context, limit int) ([]*model.JobExecutionHistory, error) {
	db := f.getDB().WithContext(ctx)
	var histories []*model.JobExecutionHistory
	
	query := db.Where("status IN ?", []string{"failed", "timeout", "cancelled"}).
		Order("started_at DESC")
	
	if limit > 0 {
		query = query.Limit(limit)
	}
	
	err := query.Find(&histories).Error
	return histories, err
}

// GetJobStatistics retrieves statistics for a specific job
func (f *JobExecutionHistoryFacade) GetJobStatistics(ctx context.Context, jobName string) (*JobStatistics, error) {
	db := f.getDB().WithContext(ctx)
	
	var stats JobStatistics
	stats.JobName = jobName
	
	// Get total runs
	if err := db.Model(&model.JobExecutionHistory{}).
		Where("job_name = ?", jobName).
		Count(&stats.TotalRuns).Error; err != nil {
		return nil, err
	}
	
	// Get status counts
	type StatusCount struct {
		Status string
		Count  int64
	}
	var statusCounts []StatusCount
	if err := db.Model(&model.JobExecutionHistory{}).
		Select("status, COUNT(*) as count").
		Where("job_name = ?", jobName).
		Group("status").
		Scan(&statusCounts).Error; err != nil {
		return nil, err
	}
	
	for _, sc := range statusCounts {
		switch sc.Status {
		case "success":
			stats.SuccessCount = sc.Count
		case "failed":
			stats.FailureCount = sc.Count
		case "cancelled":
			stats.CancelledCount = sc.Count
		case "timeout":
			stats.TimeoutCount = sc.Count
		}
	}
	
	// Get duration statistics (only for completed jobs)
	type DurationStats struct {
		AvgDuration float64
		MinDuration float64
		MaxDuration float64
	}
	var durationStats DurationStats
	if err := db.Model(&model.JobExecutionHistory{}).
		Select("AVG(duration_seconds) as avg_duration, MIN(duration_seconds) as min_duration, MAX(duration_seconds) as max_duration").
		Where("job_name = ? AND duration_seconds IS NOT NULL AND duration_seconds > 0", jobName).
		Scan(&durationStats).Error; err != nil {
		return nil, err
	}
	stats.AvgDuration = durationStats.AvgDuration
	stats.MinDuration = durationStats.MinDuration
	stats.MaxDuration = durationStats.MaxDuration
	
	// Get last run info
	var lastRun model.JobExecutionHistory
	if err := db.Where("job_name = ?", jobName).
		Order("started_at DESC").
		First(&lastRun).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	} else {
		stats.LastRunTime = lastRun.StartedAt
		stats.LastStatus = lastRun.Status
	}
	
	return &stats, nil
}

// GetDistinctJobTypes retrieves all distinct job types from execution history
func (f *JobExecutionHistoryFacade) GetDistinctJobTypes(ctx context.Context) ([]string, error) {
	db := f.getDB().WithContext(ctx)
	
	var jobTypes []string
	err := db.Model(&model.JobExecutionHistory{}).
		Distinct("job_type").
		Where("job_type IS NOT NULL AND job_type != ?", "").
		Order("job_type ASC").
		Pluck("job_type", &jobTypes).Error
	
	if err != nil {
		return nil, err
	}
	
	return jobTypes, nil
}

