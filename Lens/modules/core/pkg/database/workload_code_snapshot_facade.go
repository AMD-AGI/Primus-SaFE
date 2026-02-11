// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// WorkloadCodeSnapshotFacadeInterface defines the database operation interface for code snapshots
type WorkloadCodeSnapshotFacadeInterface interface {
	// Create creates a new code snapshot record
	Create(ctx context.Context, snapshot *model.WorkloadCodeSnapshot) error

	// GetByWorkloadUID retrieves a snapshot by workload UID
	GetByWorkloadUID(ctx context.Context, workloadUID string) (*model.WorkloadCodeSnapshot, error)

	// GetByFingerprint retrieves snapshots matching a fingerprint (for reuse/dedup)
	GetByFingerprint(ctx context.Context, fingerprint string) ([]*model.WorkloadCodeSnapshot, error)

	// Delete deletes a snapshot by ID
	Delete(ctx context.Context, id int64) error

	// DeleteByWorkloadUID deletes a snapshot by workload UID
	DeleteByWorkloadUID(ctx context.Context, workloadUID string) error

	// DeleteExpired deletes snapshots for workloads terminated before the given time
	// workloadUIDs should be pre-filtered by the caller (terminated workloads older than retention)
	DeleteExpired(ctx context.Context, workloadUIDs []string) (int64, error)

	// Exists checks if a snapshot exists for the given workload UID
	Exists(ctx context.Context, workloadUID string) (bool, error)

	// WithCluster returns a new facade instance for the specified cluster
	WithCluster(clusterName string) WorkloadCodeSnapshotFacadeInterface
}

// WorkloadCodeSnapshotFacade implements WorkloadCodeSnapshotFacadeInterface
type WorkloadCodeSnapshotFacade struct {
	BaseFacade
}

// NewWorkloadCodeSnapshotFacade creates a new WorkloadCodeSnapshotFacade instance
func NewWorkloadCodeSnapshotFacade() WorkloadCodeSnapshotFacadeInterface {
	return &WorkloadCodeSnapshotFacade{}
}

func (f *WorkloadCodeSnapshotFacade) WithCluster(clusterName string) WorkloadCodeSnapshotFacadeInterface {
	return &WorkloadCodeSnapshotFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

func (f *WorkloadCodeSnapshotFacade) Create(ctx context.Context, snapshot *model.WorkloadCodeSnapshot) error {
	now := time.Now()
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = now
	}
	return f.getDB().WithContext(ctx).Create(snapshot).Error
}

func (f *WorkloadCodeSnapshotFacade) GetByWorkloadUID(ctx context.Context, workloadUID string) (*model.WorkloadCodeSnapshot, error) {
	var result model.WorkloadCodeSnapshot
	err := f.getDB().WithContext(ctx).
		Where("workload_uid = ?", workloadUID).
		First(&result).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}

func (f *WorkloadCodeSnapshotFacade) GetByFingerprint(ctx context.Context, fingerprint string) ([]*model.WorkloadCodeSnapshot, error) {
	var results []*model.WorkloadCodeSnapshot
	err := f.getDB().WithContext(ctx).
		Where("fingerprint = ?", fingerprint).
		Order("created_at DESC").
		Find(&results).Error
	return results, err
}

func (f *WorkloadCodeSnapshotFacade) Delete(ctx context.Context, id int64) error {
	return f.getDB().WithContext(ctx).
		Where("id = ?", id).
		Delete(&model.WorkloadCodeSnapshot{}).Error
}

func (f *WorkloadCodeSnapshotFacade) DeleteByWorkloadUID(ctx context.Context, workloadUID string) error {
	return f.getDB().WithContext(ctx).
		Where("workload_uid = ?", workloadUID).
		Delete(&model.WorkloadCodeSnapshot{}).Error
}

func (f *WorkloadCodeSnapshotFacade) DeleteExpired(ctx context.Context, workloadUIDs []string) (int64, error) {
	if len(workloadUIDs) == 0 {
		return 0, nil
	}
	result := f.getDB().WithContext(ctx).
		Where("workload_uid IN ?", workloadUIDs).
		Delete(&model.WorkloadCodeSnapshot{})
	return result.RowsAffected, result.Error
}

func (f *WorkloadCodeSnapshotFacade) Exists(ctx context.Context, workloadUID string) (bool, error) {
	var count int64
	err := f.getDB().WithContext(ctx).
		Table(model.TableNameWorkloadCodeSnapshot).
		Where("workload_uid = ?", workloadUID).
		Count(&count).Error
	return count > 0, err
}
