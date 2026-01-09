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

// DetectionConflictLogFacadeInterface defines the database operation interface for Detection Conflict Log
type DetectionConflictLogFacadeInterface interface {
	// CreateDetectionConflictLog creates a new detection conflict log record
	CreateDetectionConflictLog(ctx context.Context, log *model.DetectionConflictLog) error

	// GetDetectionConflictLogByID retrieves a conflict log by ID
	GetDetectionConflictLogByID(ctx context.Context, id int64) (*model.DetectionConflictLog, error)

	// ListDetectionConflictLogsByWorkloadUID lists all conflict logs for a workload
	ListDetectionConflictLogsByWorkloadUID(ctx context.Context, workloadUID string, limit int, offset int) ([]*model.DetectionConflictLog, int64, error)

	// ListRecentConflicts lists recent conflicts across all workloads
	ListRecentConflicts(ctx context.Context, limit int, offset int) ([]*model.DetectionConflictLog, int64, error)

	// GetUnresolvedConflictsByWorkloadUID gets unresolved conflicts for a workload
	GetUnresolvedConflictsByWorkloadUID(ctx context.Context, workloadUID string) ([]*model.DetectionConflictLog, error)

	// UpdateDetectionConflictLog updates a conflict log record
	UpdateDetectionConflictLog(ctx context.Context, log *model.DetectionConflictLog) error

	// WithCluster method
	WithCluster(clusterName string) DetectionConflictLogFacadeInterface
}

// DetectionConflictLogFacade implements DetectionConflictLogFacadeInterface
type DetectionConflictLogFacade struct {
	BaseFacade
}

// NewDetectionConflictLogFacade creates a new DetectionConflictLogFacade instance
func NewDetectionConflictLogFacade() DetectionConflictLogFacadeInterface {
	return &DetectionConflictLogFacade{}
}

func (f *DetectionConflictLogFacade) WithCluster(clusterName string) DetectionConflictLogFacadeInterface {
	return &DetectionConflictLogFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// CreateDetectionConflictLog creates a new detection conflict log record
func (f *DetectionConflictLogFacade) CreateDetectionConflictLog(ctx context.Context, log *model.DetectionConflictLog) error {
	// Ensure created_at is set
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	return f.getDAL().DetectionConflictLog.WithContext(ctx).Create(log)
}

// GetDetectionConflictLogByID retrieves a conflict log by ID
func (f *DetectionConflictLogFacade) GetDetectionConflictLogByID(ctx context.Context, id int64) (*model.DetectionConflictLog, error) {
	q := f.getDAL().DetectionConflictLog
	result, err := q.WithContext(ctx).Where(q.ID.Eq(id)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

// ListDetectionConflictLogsByWorkloadUID lists all conflict logs for a workload with pagination
func (f *DetectionConflictLogFacade) ListDetectionConflictLogsByWorkloadUID(ctx context.Context, workloadUID string, limit int, offset int) ([]*model.DetectionConflictLog, int64, error) {
	q := f.getDAL().DetectionConflictLog
	
	query := q.WithContext(ctx).Where(q.WorkloadUID.Eq(workloadUID)).Order(q.CreatedAt.Desc())
	
	// Get total count
	count, err := query.Count()
	if err != nil {
		return nil, 0, err
	}
	
	// Get paginated results
	results, err := query.Limit(limit).Offset(offset).Find()
	if err != nil {
		return nil, 0, err
	}
	
	return results, count, nil
}

// ListRecentConflicts lists recent conflicts across all workloads with pagination
func (f *DetectionConflictLogFacade) ListRecentConflicts(ctx context.Context, limit int, offset int) ([]*model.DetectionConflictLog, int64, error) {
	q := f.getDAL().DetectionConflictLog
	
	query := q.WithContext(ctx).Order(q.CreatedAt.Desc())
	
	// Get total count
	count, err := query.Count()
	if err != nil {
		return nil, 0, err
	}
	
	// Get paginated results
	results, err := query.Limit(limit).Offset(offset).Find()
	if err != nil {
		return nil, 0, err
	}
	
	return results, count, nil
}

// GetUnresolvedConflictsByWorkloadUID gets unresolved conflicts (where resolution_strategy is empty or null)
func (f *DetectionConflictLogFacade) GetUnresolvedConflictsByWorkloadUID(ctx context.Context, workloadUID string) ([]*model.DetectionConflictLog, error) {
	q := f.getDAL().DetectionConflictLog
	
	results, err := q.WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUID)).
		Where(q.ResolutionStrategy.Eq("")).
		Order(q.CreatedAt.Desc()).
		Find()
		
	if err != nil {
		return nil, err
	}
	
	return results, nil
}

// UpdateDetectionConflictLog updates a conflict log record
func (f *DetectionConflictLogFacade) UpdateDetectionConflictLog(ctx context.Context, log *model.DetectionConflictLog) error {
	return f.getDAL().DetectionConflictLog.WithContext(ctx).Save(log)
}

