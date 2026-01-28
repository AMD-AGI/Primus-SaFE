// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WorkloadTaskFacade provides database operations for workload task management
type WorkloadTaskFacade struct {
	db          *gorm.DB
	clusterName string // empty means default cluster
}

// WorkloadTaskFacadeInterface defines the interface for task management
type WorkloadTaskFacadeInterface interface {
	// Task CRUD operations
	UpsertTask(ctx context.Context, task *model.WorkloadTaskState) error
	GetTask(ctx context.Context, workloadUID, taskType string) (*model.WorkloadTaskState, error)
	DeleteTask(ctx context.Context, workloadUID, taskType string) error
	
	// Task queries
	ListTasksByWorkload(ctx context.Context, workloadUID string) ([]*model.WorkloadTaskState, error)
	ListTasksByStatus(ctx context.Context, status string) ([]*model.WorkloadTaskState, error)
	ListTasksByStatusAndTypes(ctx context.Context, status string, taskTypes []string) ([]*model.WorkloadTaskState, error)
	ListRecoverableTasks(ctx context.Context) ([]*model.WorkloadTaskState, error)
	
	// Status updates
	UpdateTaskStatus(ctx context.Context, workloadUID, taskType, status string) error
	UpdateTaskExt(ctx context.Context, workloadUID, taskType string, extData model.ExtType) error
	
	// Distributed lock operations
	TryAcquireLock(ctx context.Context, workloadUID, taskType, lockOwner string, lockDuration time.Duration) (bool, error)
	ExtendLock(ctx context.Context, workloadUID, taskType, lockOwner string, lockDuration time.Duration) (bool, error)
	ReleaseLock(ctx context.Context, workloadUID, taskType, lockOwner string) error
	ReleaseStaleLocks(ctx context.Context) (int64, error)
	
	// Cleanup
	CleanupOldTasks(ctx context.Context, retentionDays int) (int64, error)
}

// NewWorkloadTaskFacade creates a new task facade with lazy initialization
func NewWorkloadTaskFacade() *WorkloadTaskFacade {
	return &WorkloadTaskFacade{
		clusterName: "", // empty means default cluster
	}
}

// NewWorkloadTaskFacadeForCluster creates a new task facade for a specific cluster with lazy initialization
func NewWorkloadTaskFacadeForCluster(clusterName string) *WorkloadTaskFacade {
	return &WorkloadTaskFacade{
		clusterName: clusterName,
	}
}

// getDB returns the database connection, initializing it lazily if needed
func (f *WorkloadTaskFacade) getDB() *gorm.DB {
	if f.db == nil {
		if f.clusterName == "" {
			f.db = GetFacade().GetSystemConfig().GetDB()
		} else {
			f.db = GetFacadeForCluster(f.clusterName).GetSystemConfig().GetDB()
		}
	}
	return f.db
}

// UpsertTask creates or updates a task
func (f *WorkloadTaskFacade) UpsertTask(ctx context.Context, task *model.WorkloadTaskState) error {
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}
	task.UpdatedAt = time.Now()
	
	return f.getDB().WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "workload_uid"}, {Name: "task_type"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"status":     gorm.Expr("EXCLUDED.status"),
				"ext":        gorm.Expr("workload_task_state.ext || EXCLUDED.ext"),
				"updated_at": gorm.Expr("NOW()"),
			}),
		}).
		Create(task).Error
}

// GetTask retrieves a task
func (f *WorkloadTaskFacade) GetTask(ctx context.Context, workloadUID, taskType string) (*model.WorkloadTaskState, error) {
	var task model.WorkloadTaskState
	err := f.getDB().WithContext(ctx).
		Where("workload_uid = ? AND task_type = ?", workloadUID, taskType).
		First(&task).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &task, err
}

// DeleteTask deletes a task
func (f *WorkloadTaskFacade) DeleteTask(ctx context.Context, workloadUID, taskType string) error {
	return f.getDB().WithContext(ctx).
		Where("workload_uid = ? AND task_type = ?", workloadUID, taskType).
		Delete(&model.WorkloadTaskState{}).Error
}

// ListTasksByWorkload lists all tasks for a workload
func (f *WorkloadTaskFacade) ListTasksByWorkload(ctx context.Context, workloadUID string) ([]*model.WorkloadTaskState, error) {
	var tasks []*model.WorkloadTaskState
	err := f.getDB().WithContext(ctx).
		Where("workload_uid = ?", workloadUID).
		Order("created_at ASC").
		Find(&tasks).Error
	return tasks, err
}

// ListTasksByStatus lists tasks by status
func (f *WorkloadTaskFacade) ListTasksByStatus(ctx context.Context, status string) ([]*model.WorkloadTaskState, error) {
	var tasks []*model.WorkloadTaskState
	err := f.getDB().WithContext(ctx).
		Where("status = ?", status).
		Order("created_at ASC").
		Find(&tasks).Error
	return tasks, err
}

// ListTasksByStatusAndTypes lists tasks by status and filtered by task types.
// If taskTypes is empty, returns all tasks with the given status (same as ListTasksByStatus).
// If taskTypes is not empty, only returns tasks whose task_type is in the list.
func (f *WorkloadTaskFacade) ListTasksByStatusAndTypes(ctx context.Context, status string, taskTypes []string) ([]*model.WorkloadTaskState, error) {
	var tasks []*model.WorkloadTaskState
	query := f.getDB().WithContext(ctx).Where("status = ?", status)
	
	if len(taskTypes) > 0 {
		query = query.Where("task_type IN ?", taskTypes)
	}
	
	err := query.Order("created_at ASC").Find(&tasks).Error
	return tasks, err
}

// ListPendingTasksByType lists pending tasks by task type
func (f *WorkloadTaskFacade) ListPendingTasksByType(ctx context.Context, taskType string) ([]*model.WorkloadTaskState, error) {
	var tasks []*model.WorkloadTaskState
	err := f.getDB().WithContext(ctx).
		Where("task_type = ? AND status = ?", taskType, constant.TaskStatusPending).
		Order("created_at ASC").
		Find(&tasks).Error
	return tasks, err
}

// ListTasksByTypeAndStatus lists tasks by type and status
func (f *WorkloadTaskFacade) ListTasksByTypeAndStatus(ctx context.Context, taskType, status string) ([]*model.WorkloadTaskState, error) {
	var tasks []*model.WorkloadTaskState
	err := f.getDB().WithContext(ctx).
		Where("task_type = ? AND status = ?", taskType, status).
		Order("created_at ASC").
		Find(&tasks).Error
	return tasks, err
}

// ListTasksByTypeAndStatuses lists tasks by type and multiple statuses
func (f *WorkloadTaskFacade) ListTasksByTypeAndStatuses(ctx context.Context, taskType string, statuses []string) ([]*model.WorkloadTaskState, error) {
	var tasks []*model.WorkloadTaskState
	err := f.getDB().WithContext(ctx).
		Where("task_type = ? AND status IN ?", taskType, statuses).
		Order("created_at ASC").
		Find(&tasks).Error
	return tasks, err
}

// ListRecoverableTasks lists tasks that should be recovered on restart
func (f *WorkloadTaskFacade) ListRecoverableTasks(ctx context.Context) ([]*model.WorkloadTaskState, error) {
	var tasks []*model.WorkloadTaskState
	err := f.getDB().WithContext(ctx).
		Where("status IN (?) AND (ext->>'auto_restart')::boolean IS NOT FALSE",
			[]string{constant.TaskStatusRunning, constant.TaskStatusPending}).
		Order("created_at ASC").
		Find(&tasks).Error
	return tasks, err
}

// UpdateTaskStatus updates task status
func (f *WorkloadTaskFacade) UpdateTaskStatus(ctx context.Context, workloadUID, taskType, status string) error {
	return f.getDB().WithContext(ctx).
		Model(&model.WorkloadTaskState{}).
		Where("workload_uid = ? AND task_type = ?", workloadUID, taskType).
		Update("status", status).Error
}

// UpdateTaskExt updates or merges ext field
func (f *WorkloadTaskFacade) UpdateTaskExt(ctx context.Context, workloadUID, taskType string, extData model.ExtType) error {
	return f.getDB().WithContext(ctx).
		Model(&model.WorkloadTaskState{}).
		Where("workload_uid = ? AND task_type = ?", workloadUID, taskType).
		Update("ext", gorm.Expr("ext || ?", extData)).Error
}

// ============================================================================
// Distributed Lock Operations (Application-Level Implementation)
// ============================================================================

// TryAcquireLock attempts to acquire a distributed lock for a task
// Returns true if lock was acquired successfully
func (f *WorkloadTaskFacade) TryAcquireLock(
	ctx context.Context,
	workloadUID, taskType, lockOwner string,
	lockDuration time.Duration,
) (bool, error) {
	expiresAt := time.Now().Add(lockDuration)
	
	result := f.getDB().WithContext(ctx).
		Model(&model.WorkloadTaskState{}).
		Where("workload_uid = ? AND task_type = ?", workloadUID, taskType).
		Where("lock_owner IS NULL OR lock_expires_at < NOW() OR lock_owner = ?", lockOwner).
		Updates(map[string]interface{}{
			"lock_owner":       lockOwner,
			"lock_acquired_at": time.Now(),
			"lock_expires_at":  expiresAt,
			"lock_version":     gorm.Expr("lock_version + 1"),
			"status":           constant.TaskStatusRunning,
		})
	
	if result.Error != nil {
		return false, result.Error
	}
	
	return result.RowsAffected > 0, nil
}

// ExtendLock extends the lock expiration time (heartbeat)
// Returns true if lock was extended successfully
func (f *WorkloadTaskFacade) ExtendLock(
	ctx context.Context,
	workloadUID, taskType, lockOwner string,
	lockDuration time.Duration,
) (bool, error) {
	expiresAt := time.Now().Add(lockDuration)
	
	result := f.getDB().WithContext(ctx).
		Model(&model.WorkloadTaskState{}).
		Where("workload_uid = ? AND task_type = ? AND lock_owner = ?", workloadUID, taskType, lockOwner).
		Updates(map[string]interface{}{
			"lock_expires_at": expiresAt,
			"lock_version":    gorm.Expr("lock_version + 1"),
		})
	
	if result.Error != nil {
		return false, result.Error
	}
	
	return result.RowsAffected > 0, nil
}

// ReleaseLock releases the lock for a task
func (f *WorkloadTaskFacade) ReleaseLock(ctx context.Context, workloadUID, taskType, lockOwner string) error {
	return f.getDB().WithContext(ctx).
		Model(&model.WorkloadTaskState{}).
		Where("workload_uid = ? AND task_type = ? AND lock_owner = ?", workloadUID, taskType, lockOwner).
		Updates(map[string]interface{}{
			"lock_owner":      nil,
			"lock_acquired_at": nil,
			"lock_expires_at":  nil,
		}).Error
}

// ReleaseStaleLocks releases all expired locks and returns count
func (f *WorkloadTaskFacade) ReleaseStaleLocks(ctx context.Context) (int64, error) {
	result := f.getDB().WithContext(ctx).
		Model(&model.WorkloadTaskState{}).
		Where("lock_expires_at < NOW() AND lock_owner IS NOT NULL").
		Updates(map[string]interface{}{
			"lock_owner":       nil,
			"lock_acquired_at": nil,
			"lock_expires_at":  nil,
			"status":           constant.TaskStatusPending,
		})
	
	return result.RowsAffected, result.Error
}

// CleanupOldTasks removes old completed tasks
func (f *WorkloadTaskFacade) CleanupOldTasks(ctx context.Context, retentionDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	
	result := f.getDB().WithContext(ctx).
		Where("status IN (?) AND updated_at < ?",
			[]string{constant.TaskStatusCompleted, constant.TaskStatusCancelled}, cutoff).
		Delete(&model.WorkloadTaskState{})
	
	return result.RowsAffected, result.Error
}

// ============================================================================
// Helper methods for ext field manipulation
// ============================================================================

// GetExtValue gets a value from ext field
func GetExtValue(task *model.WorkloadTaskState, key string) (interface{}, bool) {
	if task.Ext == nil {
		return nil, false
	}
	val, ok := task.Ext[key]
	return val, ok
}

// SetExtValue sets a value in ext field
func SetExtValue(task *model.WorkloadTaskState, key string, value interface{}) {
	if task.Ext == nil {
		task.Ext = make(model.ExtType)
	}
	task.Ext[key] = value
}

// GetExtString gets a string value from ext field
func GetExtString(task *model.WorkloadTaskState, key string) string {
	if val, ok := GetExtValue(task, key); ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// GetExtInt gets an int value from ext field
func GetExtInt(task *model.WorkloadTaskState, key string) int {
	if val, ok := GetExtValue(task, key); ok {
		if num, ok := val.(float64); ok {
			return int(num)
		}
	}
	return 0
}

// GetExtBool gets a bool value from ext field
func GetExtBool(task *model.WorkloadTaskState, key string) bool {
	if val, ok := GetExtValue(task, key); ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

// GetExtMap gets a map value from ext field
func GetExtMap(task *model.WorkloadTaskState, key string) map[string]interface{} {
	if val, ok := GetExtValue(task, key); ok {
		if m, ok := val.(map[string]interface{}); ok {
			return m
		}
	}
	return nil
}
