/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"time"

	"gorm.io/gorm/clause"
)

// OptimizationTaskFilter encapsulates optional filter predicates for listing
// optimization tasks. Zero-valued fields are ignored at query time.
type OptimizationTaskFilter struct {
	Workspace string
	Status    string
	ModelID   string
	UserID    string
	Search    string // matches display_name ILIKE %search%
	Limit     int
	Offset    int
}

// UpsertOptimizationTask creates or fully replaces an OptimizationTask row by id.
func (c *Client) UpsertOptimizationTask(ctx context.Context, task *OptimizationTask) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(task).Error
}

// GetOptimizationTask fetches a single non-deleted task by id.
func (c *Client) GetOptimizationTask(ctx context.Context, id string) (*OptimizationTask, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	var task OptimizationTask
	result := db.WithContext(ctx).
		Where("id = ? AND is_deleted = ?", id, false).
		First(&task)
	if result.Error != nil {
		return nil, result.Error
	}
	return &task, nil
}

// ListOptimizationTasks returns tasks matching filter, newest first.
func (c *Client) ListOptimizationTasks(
	ctx context.Context,
	filter OptimizationTaskFilter,
) ([]*OptimizationTask, int64, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, 0, err
	}
	query := db.WithContext(ctx).Model(&OptimizationTask{}).Where("is_deleted = ?", false)
	if filter.Workspace != "" {
		query = query.Where("workspace = ?", filter.Workspace)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.ModelID != "" {
		query = query.Where("model_id = ?", filter.ModelID)
	}
	if filter.UserID != "" {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.Search != "" {
		query = query.Where("display_name ILIKE ?", "%"+filter.Search+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var tasks []*OptimizationTask
	if err := query.Order("created_at DESC").Find(&tasks).Error; err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

// UpdateOptimizationTaskStatus is a partial update that only touches status
// fields; it avoids clobbering other columns when the task controller is only
// reacting to Claw events.
func (c *Client) UpdateOptimizationTaskStatus(
	ctx context.Context,
	id string,
	status OptimizationTaskStatus,
	currentPhase int,
	message string,
) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	updates := map[string]interface{}{
		"status":        status,
		"current_phase": currentPhase,
		"message":       message,
	}
	// Promote timestamps based on lifecycle transitions.
	switch status {
	case OptimizationTaskStatusRunning:
		// Only set started_at once. Controllers may call Update multiple times.
		db = db.WithContext(ctx).Model(&OptimizationTask{}).Where("id = ?", id)
		if err := db.Update("started_at", time.Now()).Error; err != nil {
			return err
		}
	case OptimizationTaskStatusSucceeded,
		OptimizationTaskStatusFailed,
		OptimizationTaskStatusInterrupted:
		updates["finished_at"] = time.Now()
	}
	return db.WithContext(ctx).
		Model(&OptimizationTask{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// UpdateOptimizationTaskClawSession wires a fresh Claw session onto a task
// that was just created. Called right after POST /claw-api/v1/sessions.
func (c *Client) UpdateOptimizationTaskClawSession(
	ctx context.Context,
	id string,
	sessionID string,
) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).
		Model(&OptimizationTask{}).
		Where("id = ?", id).
		Update("claw_session_id", sessionID).Error
}

// UpdateOptimizationTaskResult stores the final artifacts JSON + report path.
func (c *Client) UpdateOptimizationTaskResult(
	ctx context.Context,
	id string,
	finalMetrics, reportPath string,
) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).
		Model(&OptimizationTask{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"final_metrics": finalMetrics,
			"report_path":   reportPath,
		}).Error
}

// DeleteOptimizationTask performs a soft delete. Callers should ensure the
// underlying Claw session is stopped before invoking this.
func (c *Client) DeleteOptimizationTask(ctx context.Context, id string) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).
		Model(&OptimizationTask{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"is_deleted":    true,
			"deletion_time": time.Now(),
		}).Error
}

// CountRunningOptimizationTasks is used by the workspace-level concurrency
// gate when creating a new task.
func (c *Client) CountRunningOptimizationTasks(
	ctx context.Context,
	workspace string,
) (int64, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return 0, err
	}
	var count int64
	q := db.WithContext(ctx).Model(&OptimizationTask{}).
		Where("is_deleted = ?", false).
		Where("status IN (?, ?)",
			OptimizationTaskStatusPending,
			OptimizationTaskStatusRunning,
		)
	if workspace != "" {
		q = q.Where("workspace = ?", workspace)
	}
	if err := q.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// AppendOptimizationEvent persists a structured event for the given task and
// returns the auto-generated row id. The caller is responsible for passing a
// monotonic Seq so history replay can preserve order.
func (c *Client) AppendOptimizationEvent(
	ctx context.Context,
	event *OptimizationEvent,
) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Create(event).Error
}

// ListOptimizationEvents returns events for a task starting after a given
// sequence number (use 0 for from-beginning), up to limit rows.
func (c *Client) ListOptimizationEvents(
	ctx context.Context,
	taskID string,
	afterSeq int64,
	limit int,
) ([]*OptimizationEvent, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	query := db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Where("seq > ?", afterSeq).
		Order("seq ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var events []*OptimizationEvent
	if err := query.Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

// LatestOptimizationEventSeq returns the highest persisted seq for the task,
// used to seed a reattached SSE stream on apiserver restart.
func (c *Client) LatestOptimizationEventSeq(
	ctx context.Context,
	taskID string,
) (int64, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return 0, err
	}
	var seq int64
	row := db.WithContext(ctx).Model(&OptimizationEvent{}).
		Select("COALESCE(MAX(seq), 0)").
		Where("task_id = ?", taskID).Row()
	if err := row.Scan(&seq); err != nil {
		return 0, err
	}
	return seq, nil
}
