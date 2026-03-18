// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
)

// ClusterDeploymentHistoryFacadeInterface defines the interface for deployment history operations
type ClusterDeploymentHistoryFacadeInterface interface {
	// Create creates a new deployment history record
	Create(ctx context.Context, history *model.ClusterDeploymentHistory) error
	// UpdateStatus updates the status of a deployment
	UpdateStatus(ctx context.Context, id int32, status, message string) error
	// AppendLogs appends logs to an existing deployment record
	AppendLogs(ctx context.Context, id int32, logs string) error
	// GetByID gets a deployment history by ID
	GetByID(ctx context.Context, id int32) (*model.ClusterDeploymentHistory, error)
	// ListByCluster lists deployment history for a cluster
	ListByCluster(ctx context.Context, clusterName string, limit int) ([]*model.ClusterDeploymentHistory, error)
	// GetLatestByCluster gets the latest deployment for a cluster
	GetLatestByCluster(ctx context.Context, clusterName string) (*model.ClusterDeploymentHistory, error)
	// FinishDeployment marks a deployment as finished
	FinishDeployment(ctx context.Context, id int32, status, message string) error
}

// ClusterDeploymentHistoryFacade implements ClusterDeploymentHistoryFacadeInterface
type ClusterDeploymentHistoryFacade struct {
	db *gorm.DB
}

// NewClusterDeploymentHistoryFacade creates a new ClusterDeploymentHistoryFacade
func NewClusterDeploymentHistoryFacade(db *gorm.DB) *ClusterDeploymentHistoryFacade {
	return &ClusterDeploymentHistoryFacade{db: db}
}

// Create creates a new deployment history record
func (f *ClusterDeploymentHistoryFacade) Create(ctx context.Context, history *model.ClusterDeploymentHistory) error {
	return f.db.WithContext(ctx).Create(history).Error
}

// UpdateStatus updates the status of a deployment
func (f *ClusterDeploymentHistoryFacade) UpdateStatus(ctx context.Context, id int32, status, message string) error {
	return f.db.WithContext(ctx).
		Model(&model.ClusterDeploymentHistory{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":  status,
			"message": message,
		}).Error
}

// AppendLogs appends logs to an existing deployment record
func (f *ClusterDeploymentHistoryFacade) AppendLogs(ctx context.Context, id int32, logs string) error {
	return f.db.WithContext(ctx).
		Model(&model.ClusterDeploymentHistory{}).
		Where("id = ?", id).
		Update("logs", gorm.Expr("COALESCE(logs, '') || ?", logs)).Error
}

// GetByID gets a deployment history by ID
func (f *ClusterDeploymentHistoryFacade) GetByID(ctx context.Context, id int32) (*model.ClusterDeploymentHistory, error) {
	var history model.ClusterDeploymentHistory
	err := f.db.WithContext(ctx).
		Where("id = ?", id).
		First(&history).Error
	if err != nil {
		return nil, err
	}
	return &history, nil
}

// ListByCluster lists deployment history for a cluster
func (f *ClusterDeploymentHistoryFacade) ListByCluster(ctx context.Context, clusterName string, limit int) ([]*model.ClusterDeploymentHistory, error) {
	var histories []*model.ClusterDeploymentHistory
	query := f.db.WithContext(ctx).
		Where("cluster_name = ?", clusterName).
		Order("started_at DESC")
	
	if limit > 0 {
		query = query.Limit(limit)
	}
	
	err := query.Find(&histories).Error
	if err != nil {
		return nil, err
	}
	return histories, nil
}

// GetLatestByCluster gets the latest deployment for a cluster
func (f *ClusterDeploymentHistoryFacade) GetLatestByCluster(ctx context.Context, clusterName string) (*model.ClusterDeploymentHistory, error) {
	var history model.ClusterDeploymentHistory
	err := f.db.WithContext(ctx).
		Where("cluster_name = ?", clusterName).
		Order("started_at DESC").
		First(&history).Error
	if err != nil {
		return nil, err
	}
	return &history, nil
}

// FinishDeployment marks a deployment as finished
func (f *ClusterDeploymentHistoryFacade) FinishDeployment(ctx context.Context, id int32, status, message string) error {
	now := time.Now()
	return f.db.WithContext(ctx).
		Model(&model.ClusterDeploymentHistory{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":      status,
			"message":     message,
			"finished_at": now,
		}).Error
}
