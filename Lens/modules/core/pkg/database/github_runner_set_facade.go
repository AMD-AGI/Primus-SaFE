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

// RunnerSetWithStats extends GithubRunnerSets with run statistics and config info
type RunnerSetWithStats struct {
	*model.GithubRunnerSets
	TotalRuns     int64  `json:"total_runs"`
	PendingRuns   int64  `json:"pending_runs"`
	CompletedRuns int64  `json:"completed_runs"`
	FailedRuns    int64  `json:"failed_runs"`
	HasConfig     bool   `json:"has_config"`
	ConfigID      int64  `json:"config_id,omitempty"`
	ConfigName    string `json:"config_name,omitempty"`
}

// GithubRunnerSetFacadeInterface defines the interface for GithubRunnerSet operations
type GithubRunnerSetFacadeInterface interface {
	// Upsert creates or updates a runner set
	Upsert(ctx context.Context, runnerSet *model.GithubRunnerSets) error
	// GetByID gets a runner set by ID
	GetByID(ctx context.Context, id int64) (*model.GithubRunnerSets, error)
	// GetByUID gets a runner set by UID
	GetByUID(ctx context.Context, uid string) (*model.GithubRunnerSets, error)
	// GetByNamespaceName gets a runner set by namespace and name
	GetByNamespaceName(ctx context.Context, namespace, name string) (*model.GithubRunnerSets, error)
	// List lists all active runner sets
	List(ctx context.Context) ([]*model.GithubRunnerSets, error)
	// ListByNamespace lists runner sets in a namespace
	ListByNamespace(ctx context.Context, namespace string) ([]*model.GithubRunnerSets, error)
	// ListWithRunStats lists runner sets with run statistics and config info
	ListWithRunStats(ctx context.Context) ([]*RunnerSetWithStats, error)
	// MarkDeleted marks a runner set as deleted
	MarkDeleted(ctx context.Context, uid string) error
	// UpdateStatus updates the runner counts
	UpdateStatus(ctx context.Context, uid string, current, desired int) error
	// CleanupStale removes runner sets not synced since the given time
	CleanupStale(ctx context.Context, before time.Time) (int64, error)
	// WithCluster returns a new facade instance using the specified cluster
	WithCluster(clusterName string) GithubRunnerSetFacadeInterface
}

// GithubRunnerSetFacade implements GithubRunnerSetFacadeInterface
type GithubRunnerSetFacade struct {
	BaseFacade
}

// NewGithubRunnerSetFacade creates a new GithubRunnerSetFacade
func NewGithubRunnerSetFacade() *GithubRunnerSetFacade {
	return &GithubRunnerSetFacade{}
}

// WithCluster returns a new facade instance using the specified cluster
func (f *GithubRunnerSetFacade) WithCluster(clusterName string) GithubRunnerSetFacadeInterface {
	return &GithubRunnerSetFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// Upsert creates or updates a runner set
func (f *GithubRunnerSetFacade) Upsert(ctx context.Context, runnerSet *model.GithubRunnerSets) error {
	runnerSet.LastSyncAt = time.Now()
	runnerSet.UpdatedAt = time.Now()

	return f.getDB().WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "uid"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "namespace", "github_config_url", "github_config_secret",
			"runner_group", "github_owner", "github_repo", "min_runners", "max_runners",
			"status", "current_runners", "desired_runners", "last_sync_at", "updated_at",
		}),
	}).Create(runnerSet).Error
}

// GetByID gets a runner set by ID
func (f *GithubRunnerSetFacade) GetByID(ctx context.Context, id int64) (*model.GithubRunnerSets, error) {
	var runnerSet model.GithubRunnerSets
	err := f.getDB().WithContext(ctx).Where("id = ?", id).First(&runnerSet).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &runnerSet, nil
}

// GetByUID gets a runner set by UID
func (f *GithubRunnerSetFacade) GetByUID(ctx context.Context, uid string) (*model.GithubRunnerSets, error) {
	var runnerSet model.GithubRunnerSets
	err := f.getDB().WithContext(ctx).Where("uid = ?", uid).First(&runnerSet).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &runnerSet, nil
}

// GetByNamespaceName gets a runner set by namespace and name
func (f *GithubRunnerSetFacade) GetByNamespaceName(ctx context.Context, namespace, name string) (*model.GithubRunnerSets, error) {
	var runnerSet model.GithubRunnerSets
	err := f.getDB().WithContext(ctx).
		Where("namespace = ? AND name = ? AND status = ?", namespace, name, model.RunnerSetStatusActive).
		First(&runnerSet).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &runnerSet, nil
}

// List lists all active runner sets
func (f *GithubRunnerSetFacade) List(ctx context.Context) ([]*model.GithubRunnerSets, error) {
	var runnerSets []*model.GithubRunnerSets
	err := f.getDB().WithContext(ctx).
		Where("status = ?", model.RunnerSetStatusActive).
		Order("namespace, name").
		Find(&runnerSets).Error
	return runnerSets, err
}

// ListByNamespace lists runner sets in a namespace
func (f *GithubRunnerSetFacade) ListByNamespace(ctx context.Context, namespace string) ([]*model.GithubRunnerSets, error) {
	var runnerSets []*model.GithubRunnerSets
	err := f.getDB().WithContext(ctx).
		Where("namespace = ? AND status = ?", namespace, model.RunnerSetStatusActive).
		Order("name").
		Find(&runnerSets).Error
	return runnerSets, err
}

// MarkDeleted marks a runner set as deleted
func (f *GithubRunnerSetFacade) MarkDeleted(ctx context.Context, uid string) error {
	return f.getDB().WithContext(ctx).
		Model(&model.GithubRunnerSets{}).
		Where("uid = ?", uid).
		Updates(map[string]interface{}{
			"status":     model.RunnerSetStatusDeleted,
			"updated_at": time.Now(),
		}).Error
}

// UpdateStatus updates the runner counts
func (f *GithubRunnerSetFacade) UpdateStatus(ctx context.Context, uid string, current, desired int) error {
	return f.getDB().WithContext(ctx).
		Model(&model.GithubRunnerSets{}).
		Where("uid = ?", uid).
		Updates(map[string]interface{}{
			"current_runners": current,
			"desired_runners": desired,
			"updated_at":      time.Now(),
		}).Error
}

// ListWithRunStats lists runner sets with run statistics and config info
func (f *GithubRunnerSetFacade) ListWithRunStats(ctx context.Context) ([]*RunnerSetWithStats, error) {
	var results []*RunnerSetWithStats

	// Query to get runner sets with aggregated run stats and config info
	err := f.getDB().WithContext(ctx).
		Table("github_runner_sets AS rs").
		Select(`
			rs.id, rs.uid, rs.name, rs.namespace, rs.github_config_url, rs.github_config_secret,
			rs.runner_group, rs.github_owner, rs.github_repo, rs.min_runners, rs.max_runners,
			rs.status, rs.current_runners, rs.desired_runners, rs.last_sync_at, rs.created_at, rs.updated_at,
			COALESCE(COUNT(DISTINCT r.id), 0) AS total_runs,
			COALESCE(COUNT(DISTINCT CASE WHEN r.status = 'pending' THEN r.id END), 0) AS pending_runs,
			COALESCE(COUNT(DISTINCT CASE WHEN r.status = 'completed' THEN r.id END), 0) AS completed_runs,
			COALESCE(COUNT(DISTINCT CASE WHEN r.status = 'failed' THEN r.id END), 0) AS failed_runs,
			CASE WHEN c.id IS NOT NULL THEN true ELSE false END AS has_config,
			COALESCE(c.id, 0) AS config_id,
			COALESCE(c.name, '') AS config_name
		`).
		Joins("LEFT JOIN github_workflow_runs r ON rs.id = r.runner_set_id").
		Joins(`LEFT JOIN github_workflow_configs c ON 
			rs.namespace = c.runner_set_namespace AND 
			rs.name = c.runner_set_name AND 
			c.enabled = true`).
		Where("rs.status = ?", model.RunnerSetStatusActive).
		Group("rs.id, rs.uid, rs.name, rs.namespace, rs.github_config_url, rs.github_config_secret, " +
			"rs.runner_group, rs.github_owner, rs.github_repo, rs.min_runners, rs.max_runners, " +
			"rs.status, rs.current_runners, rs.desired_runners, rs.last_sync_at, rs.created_at, rs.updated_at, " +
			"c.id, c.name").
		Order("rs.namespace, rs.name").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	// Populate the embedded GithubRunnerSets struct for each result
	for _, result := range results {
		if result.GithubRunnerSets == nil {
			result.GithubRunnerSets = &model.GithubRunnerSets{}
		}
	}

	return results, nil
}

// CleanupStale removes runner sets not synced since the given time
func (f *GithubRunnerSetFacade) CleanupStale(ctx context.Context, before time.Time) (int64, error) {
	result := f.getDB().WithContext(ctx).
		Model(&model.GithubRunnerSets{}).
		Where("last_sync_at < ? AND status = ?", before, model.RunnerSetStatusActive).
		Updates(map[string]interface{}{
			"status":     model.RunnerSetStatusDeleted,
			"updated_at": time.Now(),
		})
	return result.RowsAffected, result.Error
}

