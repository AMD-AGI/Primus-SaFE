package common

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime/debug"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	dbmodel "github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
)

// Job 接口定义（最小化定义以避免循环导入）
type Job interface {
	Schedule() string
}

// HistoryService 历史记录服务
type HistoryService struct {
	clusterName string
	hostname    string
}

// NewHistoryService 创建新的历史记录服务
func NewHistoryService() *HistoryService {
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	hostname, _ := os.Hostname()

	return &HistoryService{
		clusterName: clusterName,
		hostname:    hostname,
	}
}

// RecordJobExecution 记录Job执行历史
func (h *HistoryService) RecordJobExecution(
	ctx context.Context,
	job Job,
	result *ExecutionResult,
) error {
	jobName := getJobName(job)
	jobType := getJobType(job)
	schedule := job.Schedule()

	// 确定状态
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

	// 构建历史记录
	history := &dbmodel.JobExecutionHistory{
		JobName:         jobName,
		JobType:         jobType,
		Schedule:        schedule,
		Status:          status,
		StartedAt:       result.StartTime,
		EndedAt:         result.EndTime,
		DurationSeconds: result.Duration,
		ClusterName:     h.clusterName,
		Hostname:        h.hostname,
	}

	// 设置错误信息
	if result.Error != nil {
		history.ErrorMessage = result.Error.Error()
		history.ErrorStack = string(debug.Stack())
	}

	// 序列化元数据
	metadata := dbmodel.ExtType{
		"cluster_name": h.clusterName,
		"hostname":     h.hostname,
		"schedule":     schedule,
	}
	history.Metadata = metadata

	// 序列化执行统计信息 - 将统计数据转换为 map
	if result.Stats != nil {
		statsBytes, err := json.Marshal(result.Stats)
		if err == nil {
			var statsMap map[string]interface{}
			if err := json.Unmarshal(statsBytes, &statsMap); err == nil {
				history.ExecutionStats = dbmodel.ExtType(statsMap)
			}
		}
	}

	// 保存到数据库
	db := clientsets.GetClusterManager().GetCurrentClusterClients().StorageClientSet.DB
	if err := db.Create(history).Error; err != nil {
		log.Errorf("Failed to save job execution history for %s: %v", jobName, err)
		return fmt.Errorf("failed to save job execution history: %w", err)
	}

	log.Debugf("Saved job execution history for %s (ID: %d, Status: %s, Duration: %.2fs)",
		jobName, history.ID, status, result.Duration)

	return nil
}

// getJobType 获取Job的完整类型名称
func getJobType(job Job) string {
	jobType := reflect.TypeOf(job)
	if jobType.Kind() == reflect.Ptr {
		jobType = jobType.Elem()
	}

	// 返回包名+类型名
	pkgPath := jobType.PkgPath()
	typeName := jobType.Name()

	if pkgPath != "" {
		return pkgPath + "." + typeName
	}
	return typeName
}

// getJobName 返回job的名称（使用类型名）
func getJobName(job Job) string {
	jobType := reflect.TypeOf(job)
	if jobType.Kind() == reflect.Ptr {
		jobType = jobType.Elem()
	}
	return jobType.Name()
}

// QueryJobHistory 查询Job执行历史
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

// QueryJobHistoryByTimeRange 查询指定时间范围的Job执行历史
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

// QueryAllJobsHistory 查询所有Job的最近执行历史
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

// GetJobStatistics 获取Job的统计信息
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

// JobStatistics Job统计信息
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

// CleanupOldHistory 清理旧的历史记录
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
