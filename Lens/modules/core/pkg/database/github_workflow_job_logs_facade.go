// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm/clause"
)

// GithubWorkflowJobLogsFacade provides database operations for github_workflow_job_logs table
type GithubWorkflowJobLogsFacade struct {
	BaseFacade
}

// NewGithubWorkflowJobLogsFacade creates a new facade instance
func NewGithubWorkflowJobLogsFacade() *GithubWorkflowJobLogsFacade {
	return &GithubWorkflowJobLogsFacade{}
}

// WithCluster returns a new facade instance for the specified cluster
func (f *GithubWorkflowJobLogsFacade) WithCluster(clusterName string) *GithubWorkflowJobLogsFacade {
	return &GithubWorkflowJobLogsFacade{
		BaseFacade: BaseFacade{clusterName: clusterName},
	}
}

// GetByJobID retrieves logs by GitHub job ID
func (f *GithubWorkflowJobLogsFacade) GetByJobID(ctx context.Context, runID, githubJobID int64) (*model.GithubWorkflowJobLogs, error) {
	var log model.GithubWorkflowJobLogs
	result := f.getDB().WithContext(ctx).
		Where("run_id = ? AND github_job_id = ?", runID, githubJobID).
		First(&log)
	if result.Error != nil {
		if result.Error.Error() == "record not found" {
			return nil, nil
		}
		return nil, result.Error
	}
	return &log, nil
}

// GetByRunID retrieves all logs for a workflow run
func (f *GithubWorkflowJobLogsFacade) GetByRunID(ctx context.Context, runID int64) ([]*model.GithubWorkflowJobLogs, error) {
	var logs []*model.GithubWorkflowJobLogs
	result := f.getDB().WithContext(ctx).
		Where("run_id = ?", runID).
		Find(&logs)
	if result.Error != nil {
		return nil, result.Error
	}
	return logs, nil
}

// UpsertLogs creates or updates log entry
func (f *GithubWorkflowJobLogsFacade) UpsertLogs(ctx context.Context, log *model.GithubWorkflowJobLogs) error {
	log.UpdatedAt = time.Now()
	return f.getDB().WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "run_id"}, {Name: "github_job_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"logs", "logs_size_bytes", "fetched_at", "fetch_status", "fetch_error", "updated_at"}),
		}).
		Create(log).Error
}

// CreatePendingLogs creates a pending log entry for a job
func (f *GithubWorkflowJobLogsFacade) CreatePendingLogs(ctx context.Context, runID, githubJobID int64, jobName string) error {
	log := &model.GithubWorkflowJobLogs{
		RunID:       runID,
		GithubJobID: githubJobID,
		JobName:     jobName,
		FetchStatus: model.LogFetchStatusPending,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	return f.UpsertLogs(ctx, log)
}

// UpdateLogsContent updates the logs content and status
func (f *GithubWorkflowJobLogsFacade) UpdateLogsContent(ctx context.Context, runID, githubJobID int64, logs string) error {
	now := time.Now()
	return f.getDB().WithContext(ctx).
		Model(&model.GithubWorkflowJobLogs{}).
		Where("run_id = ? AND github_job_id = ?", runID, githubJobID).
		Updates(map[string]interface{}{
			"logs":            logs,
			"logs_size_bytes": len(logs),
			"fetched_at":      now,
			"fetch_status":    model.LogFetchStatusFetched,
			"updated_at":      now,
		}).Error
}

// UpdateFetchError marks log fetch as failed
func (f *GithubWorkflowJobLogsFacade) UpdateFetchError(ctx context.Context, runID, githubJobID int64, errMsg string) error {
	return f.getDB().WithContext(ctx).
		Model(&model.GithubWorkflowJobLogs{}).
		Where("run_id = ? AND github_job_id = ?", runID, githubJobID).
		Updates(map[string]interface{}{
			"fetch_status": model.LogFetchStatusFailed,
			"fetch_error":  errMsg,
			"updated_at":   time.Now(),
		}).Error
}

// GetPendingLogs retrieves all pending log entries
func (f *GithubWorkflowJobLogsFacade) GetPendingLogs(ctx context.Context, limit int) ([]*model.GithubWorkflowJobLogs, error) {
	var logs []*model.GithubWorkflowJobLogs
	result := f.getDB().WithContext(ctx).
		Where("fetch_status = ?", model.LogFetchStatusPending).
		Limit(limit).
		Find(&logs)
	if result.Error != nil {
		return nil, result.Error
	}
	return logs, nil
}
