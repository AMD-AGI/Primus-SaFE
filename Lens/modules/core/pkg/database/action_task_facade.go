// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// ActionTask status constants
const (
	ActionTaskStatusPending   = "pending"
	ActionTaskStatusRunning   = "running"
	ActionTaskStatusCompleted = "completed"
	ActionTaskStatusFailed    = "failed"
	ActionTaskStatusTimeout   = "timeout"
)

// ActionTaskFacadeInterface defines database operations for action_tasks
type ActionTaskFacadeInterface interface {
	Create(ctx context.Context, task *model.ActionTasks) error
	GetByID(ctx context.Context, id int64) (*model.ActionTasks, error)
	GetPendingTasks(ctx context.Context, clusterName string, limit int) ([]*model.ActionTasks, error)
	UpdateStatus(ctx context.Context, id int64, status string) error
	MarkRunning(ctx context.Context, id int64) error
	MarkCompleted(ctx context.Context, id int64, result model.ExtType) error
	MarkFailed(ctx context.Context, id int64, errorMsg string) error
	MarkTimeout(ctx context.Context, id int64, errorMsg string) error
	CleanupOldTasks(ctx context.Context, olderThan time.Duration) (int64, error)
	WithCluster(clusterName string) ActionTaskFacadeInterface
}

// ActionTaskFacade provides database operations for action_tasks table
type ActionTaskFacade struct {
	BaseFacade
}

// NewActionTaskFacade creates a new ActionTaskFacade instance
func NewActionTaskFacade() ActionTaskFacadeInterface {
	return &ActionTaskFacade{}
}

// NewActionTaskFacadeForCluster creates a new ActionTaskFacade instance for a specific cluster
func NewActionTaskFacadeForCluster(clusterName string) ActionTaskFacadeInterface {
	return &ActionTaskFacade{
		BaseFacade: BaseFacade{clusterName: clusterName},
	}
}

// WithCluster returns a new ActionTaskFacade instance using the specified cluster
func (f *ActionTaskFacade) WithCluster(clusterName string) ActionTaskFacadeInterface {
	return &ActionTaskFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// Create creates a new action task
func (f *ActionTaskFacade) Create(ctx context.Context, task *model.ActionTasks) error {
	return f.getDAL().ActionTasks.WithContext(ctx).Create(task)
}

// GetByID retrieves an action task by ID
func (f *ActionTaskFacade) GetByID(ctx context.Context, id int64) (*model.ActionTasks, error) {
	q := f.getDAL().ActionTasks
	return q.WithContext(ctx).Where(q.ID.Eq(id)).First()
}

// GetPendingTasks retrieves pending tasks for a specific cluster
func (f *ActionTaskFacade) GetPendingTasks(ctx context.Context, clusterName string, limit int) ([]*model.ActionTasks, error) {
	q := f.getDAL().ActionTasks
	return q.WithContext(ctx).
		Where(q.ClusterName.Eq(clusterName)).
		Where(q.Status.Eq(ActionTaskStatusPending)).
		Order(q.CreatedAt).
		Limit(limit).
		Find()
}

// UpdateStatus updates the status of an action task
func (f *ActionTaskFacade) UpdateStatus(ctx context.Context, id int64, status string) error {
	q := f.getDAL().ActionTasks
	_, err := q.WithContext(ctx).
		Where(q.ID.Eq(id)).
		Update(q.Status, status)
	return err
}

// MarkRunning marks a task as running and sets started_at
func (f *ActionTaskFacade) MarkRunning(ctx context.Context, id int64) error {
	q := f.getDAL().ActionTasks
	now := time.Now()
	_, err := q.WithContext(ctx).
		Where(q.ID.Eq(id)).
		Updates(map[string]interface{}{
			"status":     ActionTaskStatusRunning,
			"started_at": now,
		})
	return err
}

// MarkCompleted marks a task as completed with result
func (f *ActionTaskFacade) MarkCompleted(ctx context.Context, id int64, result model.ExtType) error {
	q := f.getDAL().ActionTasks
	now := time.Now()
	_, err := q.WithContext(ctx).
		Where(q.ID.Eq(id)).
		Updates(map[string]interface{}{
			"status":       ActionTaskStatusCompleted,
			"result":       result,
			"completed_at": now,
		})
	return err
}

// MarkFailed marks a task as failed with error message
func (f *ActionTaskFacade) MarkFailed(ctx context.Context, id int64, errorMsg string) error {
	q := f.getDAL().ActionTasks
	now := time.Now()
	_, err := q.WithContext(ctx).
		Where(q.ID.Eq(id)).
		Updates(map[string]interface{}{
			"status":        ActionTaskStatusFailed,
			"error_message": errorMsg,
			"completed_at":  now,
		})
	return err
}

// MarkTimeout marks a task as timeout
func (f *ActionTaskFacade) MarkTimeout(ctx context.Context, id int64, errorMsg string) error {
	q := f.getDAL().ActionTasks
	now := time.Now()
	_, err := q.WithContext(ctx).
		Where(q.ID.Eq(id)).
		Updates(map[string]interface{}{
			"status":        ActionTaskStatusTimeout,
			"error_message": errorMsg,
			"completed_at":  now,
		})
	return err
}

// CleanupOldTasks deletes completed/failed/timeout tasks older than the specified duration
func (f *ActionTaskFacade) CleanupOldTasks(ctx context.Context, olderThan time.Duration) (int64, error) {
	q := f.getDAL().ActionTasks
	cutoff := time.Now().Add(-olderThan)
	result, err := q.WithContext(ctx).
		Where(q.Status.In(ActionTaskStatusCompleted, ActionTaskStatusFailed, ActionTaskStatusTimeout)).
		Where(q.CreatedAt.Lt(cutoff)).
		Delete()
	if err != nil {
		return 0, err
	}
	return result.RowsAffected, nil
}
