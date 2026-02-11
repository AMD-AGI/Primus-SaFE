// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
)

// ReleaseHistoryFacadeInterface defines the interface for release history operations
type ReleaseHistoryFacadeInterface interface {
	Create(ctx context.Context, history *model.ReleaseHistory) error
	GetByID(ctx context.Context, id int32) (*model.ReleaseHistory, error)
	ListByCluster(ctx context.Context, clusterName string, limit int) ([]*model.ReleaseHistory, error)
	ListByClusterWithVersions(ctx context.Context, clusterName string, limit int) ([]*model.ReleaseHistory, error)
	ListPending(ctx context.Context, limit int) ([]*model.ReleaseHistory, error)
	GetLatestByCluster(ctx context.Context, clusterName string) (*model.ReleaseHistory, error)
	GetLatestSuccessfulByCluster(ctx context.Context, clusterName string) (*model.ReleaseHistory, error)
	MarkRunning(ctx context.Context, id int32) error
	MarkCompleted(ctx context.Context, id int32, stagesCompleted []string) error
	MarkFailed(ctx context.Context, id int32, errorMsg string, stagesCompleted []string) error
	UpdateStages(ctx context.Context, id int32, stagesCompleted []string) error
	SetTaskID(ctx context.Context, id int32, taskID int32) error
}

// ReleaseHistoryFacade implements ReleaseHistoryFacadeInterface
type ReleaseHistoryFacade struct {
	db *gorm.DB
}

// NewReleaseHistoryFacade creates a new ReleaseHistoryFacade
func NewReleaseHistoryFacade(db *gorm.DB) *ReleaseHistoryFacade {
	return &ReleaseHistoryFacade{db: db}
}

// Create creates a new release history record
func (f *ReleaseHistoryFacade) Create(ctx context.Context, history *model.ReleaseHistory) error {
	return f.db.WithContext(ctx).Create(history).Error
}

// GetByID gets a release history by ID
func (f *ReleaseHistoryFacade) GetByID(ctx context.Context, id int32) (*model.ReleaseHistory, error) {
	var history model.ReleaseHistory
	err := f.db.WithContext(ctx).
		Preload("ReleaseVersion").
		Preload("PreviousVersion").
		Where("id = ?", id).
		First(&history).Error
	if err != nil {
		return nil, err
	}
	return &history, nil
}

// ListByCluster lists release history for a cluster
func (f *ReleaseHistoryFacade) ListByCluster(ctx context.Context, clusterName string, limit int) ([]*model.ReleaseHistory, error) {
	var histories []*model.ReleaseHistory
	query := f.db.WithContext(ctx).
		Where("cluster_name = ?", clusterName).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&histories).Error
	if err != nil {
		return nil, err
	}
	return histories, nil
}

// ListByClusterWithVersions lists release history with version info
func (f *ReleaseHistoryFacade) ListByClusterWithVersions(ctx context.Context, clusterName string, limit int) ([]*model.ReleaseHistory, error) {
	var histories []*model.ReleaseHistory
	query := f.db.WithContext(ctx).
		Preload("ReleaseVersion").
		Preload("PreviousVersion").
		Where("cluster_name = ?", clusterName).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&histories).Error
	if err != nil {
		return nil, err
	}
	return histories, nil
}

// ListPending lists pending release history records
func (f *ReleaseHistoryFacade) ListPending(ctx context.Context, limit int) ([]*model.ReleaseHistory, error) {
	var histories []*model.ReleaseHistory
	query := f.db.WithContext(ctx).
		Preload("ReleaseVersion").
		Where("status IN ?", []string{model.ReleaseHistoryStatusPending, model.ReleaseHistoryStatusRunning}).
		Order("created_at ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&histories).Error
	if err != nil {
		return nil, err
	}
	return histories, nil
}

// GetLatestByCluster gets the latest release history for a cluster
func (f *ReleaseHistoryFacade) GetLatestByCluster(ctx context.Context, clusterName string) (*model.ReleaseHistory, error) {
	var history model.ReleaseHistory
	err := f.db.WithContext(ctx).
		Preload("ReleaseVersion").
		Where("cluster_name = ?", clusterName).
		Order("created_at DESC").
		First(&history).Error
	if err != nil {
		return nil, err
	}
	return &history, nil
}

// GetLatestSuccessfulByCluster gets the latest successful deployment for a cluster
func (f *ReleaseHistoryFacade) GetLatestSuccessfulByCluster(ctx context.Context, clusterName string) (*model.ReleaseHistory, error) {
	var history model.ReleaseHistory
	err := f.db.WithContext(ctx).
		Preload("ReleaseVersion").
		Where("cluster_name = ? AND status = ?", clusterName, model.ReleaseHistoryStatusCompleted).
		Order("created_at DESC").
		First(&history).Error
	if err != nil {
		return nil, err
	}
	return &history, nil
}

// MarkRunning marks a release history as running
func (f *ReleaseHistoryFacade) MarkRunning(ctx context.Context, id int32) error {
	now := time.Now()
	return f.db.WithContext(ctx).
		Model(&model.ReleaseHistory{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     model.ReleaseHistoryStatusRunning,
			"started_at": now,
		}).Error
}

// MarkCompleted marks a release history as completed
func (f *ReleaseHistoryFacade) MarkCompleted(ctx context.Context, id int32, stagesCompleted []string) error {
	now := time.Now()
	return f.db.WithContext(ctx).
		Model(&model.ReleaseHistory{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":           model.ReleaseHistoryStatusCompleted,
			"stages_completed": model.StringArray(stagesCompleted),
			"completed_at":     now,
		}).Error
}

// MarkFailed marks a release history as failed
func (f *ReleaseHistoryFacade) MarkFailed(ctx context.Context, id int32, errorMsg string, stagesCompleted []string) error {
	now := time.Now()
	return f.db.WithContext(ctx).
		Model(&model.ReleaseHistory{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":           model.ReleaseHistoryStatusFailed,
			"error_message":    errorMsg,
			"stages_completed": model.StringArray(stagesCompleted),
			"completed_at":     now,
		}).Error
}

// UpdateStages updates the completed stages
func (f *ReleaseHistoryFacade) UpdateStages(ctx context.Context, id int32, stagesCompleted []string) error {
	return f.db.WithContext(ctx).
		Model(&model.ReleaseHistory{}).
		Where("id = ?", id).
		Update("stages_completed", model.StringArray(stagesCompleted)).Error
}

// SetTaskID sets the linked task ID
func (f *ReleaseHistoryFacade) SetTaskID(ctx context.Context, id int32, taskID int32) error {
	return f.db.WithContext(ctx).
		Model(&model.ReleaseHistory{}).
		Where("id = ?", id).
		Update("task_id", taskID).Error
}
