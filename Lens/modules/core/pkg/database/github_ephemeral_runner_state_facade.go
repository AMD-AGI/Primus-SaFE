// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GithubEphemeralRunnerStateFacade provides database operations for ephemeral runner states.
// This facade uses raw GORM (not generated DAL) following the same pattern as WorkloadTaskFacade.
type GithubEphemeralRunnerStateFacade struct {
	db *gorm.DB
}

// NewGithubEphemeralRunnerStateFacade creates a new facade with lazy initialization
func NewGithubEphemeralRunnerStateFacade() *GithubEphemeralRunnerStateFacade {
	return &GithubEphemeralRunnerStateFacade{}
}

// getDB returns the database connection, initializing it lazily if needed
func (f *GithubEphemeralRunnerStateFacade) getDB() *gorm.DB {
	if f.db == nil {
		f.db = GetFacade().GetSystemConfig().GetDB()
	}
	return f.db
}

// EnsureTable creates the table if it does not exist (for development/testing).
// In production, the table is created by SQL migration files.
func (f *GithubEphemeralRunnerStateFacade) EnsureTable(ctx context.Context) error {
	return f.getDB().WithContext(ctx).AutoMigrate(&model.GithubEphemeralRunnerStates{})
}

// Upsert creates or updates an ephemeral runner state by (namespace, name).
// On conflict, updates all K8s state fields and bumps updated_at.
func (f *GithubEphemeralRunnerStateFacade) Upsert(ctx context.Context, state *model.GithubEphemeralRunnerStates) error {
	now := time.Now()
	if state.CreatedAt.IsZero() {
		state.CreatedAt = now
	}
	state.UpdatedAt = now

	return f.getDB().WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "namespace"}, {Name: "name"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"uid":                gorm.Expr("EXCLUDED.uid"),
				"runner_set_name":    gorm.Expr("EXCLUDED.runner_set_name"),
				"runner_type":        gorm.Expr("EXCLUDED.runner_type"),
				"phase":              gorm.Expr("EXCLUDED.phase"),
				"github_run_id":      gorm.Expr("EXCLUDED.github_run_id"),
				"github_job_id":      gorm.Expr("EXCLUDED.github_job_id"),
				"github_run_number":  gorm.Expr("EXCLUDED.github_run_number"),
				"workflow_name":      gorm.Expr("EXCLUDED.workflow_name"),
				"head_sha":           gorm.Expr("EXCLUDED.head_sha"),
				"head_branch":        gorm.Expr("EXCLUDED.head_branch"),
				"repository":         gorm.Expr("EXCLUDED.repository"),
				"pod_phase":          gorm.Expr("EXCLUDED.pod_phase"),
				"pod_condition":      gorm.Expr("EXCLUDED.pod_condition"),
				"pod_message":        gorm.Expr("EXCLUDED.pod_message"),
				"safe_workload_id":   gorm.Expr("CASE WHEN EXCLUDED.safe_workload_id != '' THEN EXCLUDED.safe_workload_id ELSE github_ephemeral_runner_states.safe_workload_id END"),
				"is_completed":       gorm.Expr("EXCLUDED.is_completed"),
				"creation_timestamp": gorm.Expr("EXCLUDED.creation_timestamp"),
				"completion_time":    gorm.Expr("EXCLUDED.completion_time"),
				"updated_at":         now,
			}),
		}).
		Create(state).Error
}

// MarkDeleted marks a runner state as deleted
func (f *GithubEphemeralRunnerStateFacade) MarkDeleted(ctx context.Context, namespace, name string) error {
	now := time.Now()
	return f.getDB().WithContext(ctx).
		Model(&model.GithubEphemeralRunnerStates{}).
		Where("namespace = ? AND name = ?", namespace, name).
		Updates(map[string]interface{}{
			"is_deleted":    true,
			"is_completed":  true,
			"deletion_time": now,
			"updated_at":    now,
		}).Error
}

// ListUnprocessed returns runner states that have changes since last processing.
// This is the main query used by RunnerStateProcessor to find work.
func (f *GithubEphemeralRunnerStateFacade) ListUnprocessed(ctx context.Context, limit int) ([]*model.GithubEphemeralRunnerStates, error) {
	var states []*model.GithubEphemeralRunnerStates
	err := f.getDB().WithContext(ctx).
		Where("last_processed_at IS NULL OR updated_at > last_processed_at").
		Order("updated_at ASC").
		Limit(limit).
		Find(&states).Error
	return states, err
}

// MarkProcessed updates the processing state after a runner state has been processed
func (f *GithubEphemeralRunnerStateFacade) MarkProcessed(ctx context.Context, id int64, workflowRunID, runSummaryID int64, lastStatus string) error {
	now := time.Now()
	return f.getDB().WithContext(ctx).
		Model(&model.GithubEphemeralRunnerStates{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"workflow_run_id":   workflowRunID,
			"run_summary_id":   runSummaryID,
			"last_status":      lastStatus,
			"last_processed_at": now,
		}).Error
}

// GetByNamespaceName retrieves a runner state by namespace and name
func (f *GithubEphemeralRunnerStateFacade) GetByNamespaceName(ctx context.Context, namespace, name string) (*model.GithubEphemeralRunnerStates, error) {
	var state model.GithubEphemeralRunnerStates
	err := f.getDB().WithContext(ctx).
		Where("namespace = ? AND name = ?", namespace, name).
		First(&state).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &state, err
}

// CleanupOldDeleted removes old deleted runner states that are already processed
func (f *GithubEphemeralRunnerStateFacade) CleanupOldDeleted(ctx context.Context, retentionDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	result := f.getDB().WithContext(ctx).
		Where("is_deleted = TRUE AND last_processed_at IS NOT NULL AND updated_at < ?", cutoff).
		Delete(&model.GithubEphemeralRunnerStates{})
	return result.RowsAffected, result.Error
}
