// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
)

// DataplaneInstallTaskFacadeInterface defines the interface for dataplane install task operations
type DataplaneInstallTaskFacadeInterface interface {
	// Create creates a new install task
	Create(ctx context.Context, task *model.DataplaneInstallTask) error
	// GetByID gets a task by ID
	GetByID(ctx context.Context, id int32) (*model.DataplaneInstallTask, error)
	// GetActiveTask gets the active (pending/running) task for a cluster
	GetActiveTask(ctx context.Context, clusterName string) (*model.DataplaneInstallTask, error)
	// GetPendingTasks gets pending or running tasks
	GetPendingTasks(ctx context.Context, limit int) ([]*model.DataplaneInstallTask, error)
	// MarkRunning marks a task as running
	MarkRunning(ctx context.Context, id int32) error
	// UpdateStage updates the current stage
	UpdateStage(ctx context.Context, id int32, stage string) error
	// MarkCompleted marks a task as completed
	MarkCompleted(ctx context.Context, id int32) error
	// MarkFailed marks a task as failed
	MarkFailed(ctx context.Context, id int32, errorMsg string) error
	// IncrementRetry increments the retry count
	IncrementRetry(ctx context.Context, id int32, errorMsg string) error
	// ListByCluster lists tasks for a cluster
	ListByCluster(ctx context.Context, clusterName string, limit int) ([]*model.DataplaneInstallTask, error)
}

// DataplaneInstallTaskFacade implements DataplaneInstallTaskFacadeInterface
type DataplaneInstallTaskFacade struct {
	db *gorm.DB
}

// NewDataplaneInstallTaskFacade creates a new DataplaneInstallTaskFacade
func NewDataplaneInstallTaskFacade(db *gorm.DB) *DataplaneInstallTaskFacade {
	return &DataplaneInstallTaskFacade{db: db}
}

// Create creates a new install task
func (f *DataplaneInstallTaskFacade) Create(ctx context.Context, task *model.DataplaneInstallTask) error {
	return f.db.WithContext(ctx).Create(task).Error
}

// GetByID gets a task by ID
func (f *DataplaneInstallTaskFacade) GetByID(ctx context.Context, id int32) (*model.DataplaneInstallTask, error) {
	var task model.DataplaneInstallTask
	err := f.db.WithContext(ctx).Where("id = ?", id).First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// GetActiveTask gets the active (pending/running) task for a cluster
func (f *DataplaneInstallTaskFacade) GetActiveTask(ctx context.Context, clusterName string) (*model.DataplaneInstallTask, error) {
	var task model.DataplaneInstallTask
	err := f.db.WithContext(ctx).
		Where("cluster_name = ? AND status IN ?", clusterName, []string{model.TaskStatusPending, model.TaskStatusRunning}).
		First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// GetPendingTasks gets pending or running tasks
func (f *DataplaneInstallTaskFacade) GetPendingTasks(ctx context.Context, limit int) ([]*model.DataplaneInstallTask, error) {
	var tasks []*model.DataplaneInstallTask
	query := f.db.WithContext(ctx).
		Where("status IN ?", []string{model.TaskStatusPending, model.TaskStatusRunning}).
		Order("created_at ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

// MarkRunning marks a task as running
func (f *DataplaneInstallTaskFacade) MarkRunning(ctx context.Context, id int32) error {
	now := time.Now()
	return f.db.WithContext(ctx).
		Model(&model.DataplaneInstallTask{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     model.TaskStatusRunning,
			"started_at": now,
			"updated_at": now,
		}).Error
}

// UpdateStage updates the current stage
func (f *DataplaneInstallTaskFacade) UpdateStage(ctx context.Context, id int32, stage string) error {
	return f.db.WithContext(ctx).
		Model(&model.DataplaneInstallTask{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"current_stage": stage,
			"updated_at":    time.Now(),
			"error_message": "", // Clear error on successful stage update
		}).Error
}

// MarkCompleted marks a task as completed
func (f *DataplaneInstallTaskFacade) MarkCompleted(ctx context.Context, id int32) error {
	now := time.Now()
	return f.db.WithContext(ctx).
		Model(&model.DataplaneInstallTask{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        model.TaskStatusCompleted,
			"current_stage": model.StageCompleted,
			"completed_at":  now,
			"updated_at":    now,
		}).Error
}

// MarkFailed marks a task as failed
func (f *DataplaneInstallTaskFacade) MarkFailed(ctx context.Context, id int32, errorMsg string) error {
	now := time.Now()
	return f.db.WithContext(ctx).
		Model(&model.DataplaneInstallTask{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        model.TaskStatusFailed,
			"error_message": errorMsg,
			"completed_at":  now,
			"updated_at":    now,
		}).Error
}

// IncrementRetry increments the retry count
func (f *DataplaneInstallTaskFacade) IncrementRetry(ctx context.Context, id int32, errorMsg string) error {
	return f.db.WithContext(ctx).
		Model(&model.DataplaneInstallTask{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"retry_count":   gorm.Expr("retry_count + 1"),
			"error_message": errorMsg,
			"updated_at":    time.Now(),
		}).Error
}

// ListByCluster lists tasks for a cluster
func (f *DataplaneInstallTaskFacade) ListByCluster(ctx context.Context, clusterName string, limit int) ([]*model.DataplaneInstallTask, error) {
	var tasks []*model.DataplaneInstallTask
	query := f.db.WithContext(ctx).
		Where("cluster_name = ?", clusterName).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}
