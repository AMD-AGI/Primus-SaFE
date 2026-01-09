// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// DetectionCoverageFacadeInterface defines the database operation interface for detection coverage
type DetectionCoverageFacadeInterface interface {
	// CreateCoverage creates a new detection coverage record
	CreateCoverage(ctx context.Context, coverage *model.DetectionCoverage) error

	// UpsertCoverage creates or updates a coverage record by workload_uid and source
	UpsertCoverage(ctx context.Context, coverage *model.DetectionCoverage) error

	// GetCoverage retrieves a coverage record by workload_uid and source
	GetCoverage(ctx context.Context, workloadUID, source string) (*model.DetectionCoverage, error)

	// GetCoverageByID retrieves a coverage record by ID
	GetCoverageByID(ctx context.Context, id int64) (*model.DetectionCoverage, error)

	// ListCoverageByWorkload retrieves all coverage records for a workload
	ListCoverageByWorkload(ctx context.Context, workloadUID string) ([]*model.DetectionCoverage, error)

	// ListPendingCoverageByWorkload retrieves pending coverage records for a workload
	ListPendingCoverageByWorkload(ctx context.Context, workloadUID string) ([]*model.DetectionCoverage, error)

	// UpdateCoverageStatus updates the status of a coverage record
	UpdateCoverageStatus(ctx context.Context, workloadUID, source, status string, lastError string) error

	// MarkCollecting marks a coverage as collecting and increments attempt count
	MarkCollecting(ctx context.Context, workloadUID, source string) error

	// MarkCollected marks a coverage as collected with success timestamp
	MarkCollected(ctx context.Context, workloadUID, source string, evidenceCount int32) error

	// MarkFailed marks a coverage as failed with error message
	MarkFailed(ctx context.Context, workloadUID, source, errMsg string) error

	// UpdateLogAvailableTime updates log available time range (called by telemetry-processor)
	UpdateLogAvailableTime(ctx context.Context, workloadUID string, logTimestamp time.Time) error

	// UpdateCoveredTimeRange updates the covered time range for log source
	UpdateCoveredTimeRange(ctx context.Context, workloadUID string, from, to time.Time) error

	// IncrementEvidenceCount increments the evidence count for a coverage
	IncrementEvidenceCount(ctx context.Context, workloadUID, source string, count int32) error

	// InitializeCoverageForWorkload initializes coverage records for all sources
	InitializeCoverageForWorkload(ctx context.Context, workloadUID string) error

	// DeleteCoverageByWorkload deletes all coverage records for a workload
	DeleteCoverageByWorkload(ctx context.Context, workloadUID string) error

	// GetUncoveredLogWindow returns the uncovered log time window if any
	GetUncoveredLogWindow(ctx context.Context, workloadUID string) (*time.Time, *time.Time, error)

	// WithCluster returns a new facade instance for the specified cluster
	WithCluster(clusterName string) DetectionCoverageFacadeInterface
}

// DetectionCoverageFacade implements DetectionCoverageFacadeInterface
type DetectionCoverageFacade struct {
	BaseFacade
}

// NewDetectionCoverageFacade creates a new DetectionCoverageFacade instance
func NewDetectionCoverageFacade() DetectionCoverageFacadeInterface {
	return &DetectionCoverageFacade{}
}

func (f *DetectionCoverageFacade) WithCluster(clusterName string) DetectionCoverageFacadeInterface {
	return &DetectionCoverageFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// CreateCoverage creates a new detection coverage record
func (f *DetectionCoverageFacade) CreateCoverage(ctx context.Context, coverage *model.DetectionCoverage) error {
	now := time.Now()
	if coverage.CreatedAt.IsZero() {
		coverage.CreatedAt = now
	}
	if coverage.UpdatedAt.IsZero() {
		coverage.UpdatedAt = now
	}
	if coverage.Status == "" {
		coverage.Status = constant.DetectionStatusPending
	}
	return f.getDAL().DetectionCoverage.WithContext(ctx).Create(coverage)
}

// UpsertCoverage creates or updates a coverage record by workload_uid and source
func (f *DetectionCoverageFacade) UpsertCoverage(ctx context.Context, coverage *model.DetectionCoverage) error {
	now := time.Now()
	coverage.UpdatedAt = now

	db := f.getDB()
	return db.WithContext(ctx).
		Where("workload_uid = ? AND source = ?", coverage.WorkloadUID, coverage.Source).
		Assign(coverage).
		FirstOrCreate(coverage).Error
}

// GetCoverage retrieves a coverage record by workload_uid and source
func (f *DetectionCoverageFacade) GetCoverage(ctx context.Context, workloadUID, source string) (*model.DetectionCoverage, error) {
	q := f.getDAL().DetectionCoverage
	result, err := q.WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUID)).
		Where(q.Source.Eq(source)).
		First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

// GetCoverageByID retrieves a coverage record by ID
func (f *DetectionCoverageFacade) GetCoverageByID(ctx context.Context, id int64) (*model.DetectionCoverage, error) {
	q := f.getDAL().DetectionCoverage
	result, err := q.WithContext(ctx).Where(q.ID.Eq(id)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

// ListCoverageByWorkload retrieves all coverage records for a workload
func (f *DetectionCoverageFacade) ListCoverageByWorkload(ctx context.Context, workloadUID string) ([]*model.DetectionCoverage, error) {
	q := f.getDAL().DetectionCoverage
	results, err := q.WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUID)).
		Find()
	if err != nil {
		return nil, err
	}
	return results, nil
}

// ListPendingCoverageByWorkload retrieves pending coverage records for a workload
func (f *DetectionCoverageFacade) ListPendingCoverageByWorkload(ctx context.Context, workloadUID string) ([]*model.DetectionCoverage, error) {
	q := f.getDAL().DetectionCoverage
	results, err := q.WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUID)).
		Where(q.Status.Eq(constant.DetectionStatusPending)).
		Find()
	if err != nil {
		return nil, err
	}
	return results, nil
}

// UpdateCoverageStatus updates the status of a coverage record
func (f *DetectionCoverageFacade) UpdateCoverageStatus(ctx context.Context, workloadUID, source, status string, lastError string) error {
	q := f.getDAL().DetectionCoverage

	if lastError != "" {
		_, err := q.WithContext(ctx).
			Where(q.WorkloadUID.Eq(workloadUID)).
			Where(q.Source.Eq(source)).
			UpdateSimple(
				q.Status.Value(status),
				q.LastError.Value(lastError),
			)
		return err
	}

	_, err := q.WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUID)).
		Where(q.Source.Eq(source)).
		UpdateSimple(
			q.Status.Value(status),
		)
	return err
}

// MarkCollecting marks a coverage as collecting and increments attempt count
func (f *DetectionCoverageFacade) MarkCollecting(ctx context.Context, workloadUID, source string) error {
	q := f.getDAL().DetectionCoverage
	now := time.Now()

	_, err := q.WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUID)).
		Where(q.Source.Eq(source)).
		UpdateSimple(
			q.Status.Value(constant.DetectionStatusCollecting),
			q.AttemptCount.Add(1),
			q.LastAttemptAt.Value(now),
		)
	return err
}

// MarkCollected marks a coverage as collected with success timestamp
func (f *DetectionCoverageFacade) MarkCollected(ctx context.Context, workloadUID, source string, evidenceCount int32) error {
	q := f.getDAL().DetectionCoverage
	now := time.Now()

	// Note: Don't set UpdatedAt explicitly as GORM handles it automatically
	_, err := q.WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUID)).
		Where(q.Source.Eq(source)).
		UpdateSimple(
			q.Status.Value(constant.DetectionStatusCollected),
			q.LastSuccessAt.Value(now),
			q.EvidenceCount.Value(evidenceCount),
		)
	return err
}

// MarkFailed marks a coverage as failed with error message
func (f *DetectionCoverageFacade) MarkFailed(ctx context.Context, workloadUID, source, errMsg string) error {
	q := f.getDAL().DetectionCoverage

	_, err := q.WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUID)).
		Where(q.Source.Eq(source)).
		UpdateSimple(
			q.Status.Value(constant.DetectionStatusFailed),
			q.LastError.Value(errMsg),
		)
	return err
}

// UpdateLogAvailableTime updates log available time range (called by telemetry-processor)
func (f *DetectionCoverageFacade) UpdateLogAvailableTime(ctx context.Context, workloadUID string, logTimestamp time.Time) error {
	q := f.getDAL().DetectionCoverage

	// Get current coverage
	coverage, err := f.GetCoverage(ctx, workloadUID, constant.DetectionSourceLog)
	if err != nil {
		return err
	}

	if coverage == nil {
		// Create new coverage record for log source
		return f.CreateCoverage(ctx, &model.DetectionCoverage{
			WorkloadUID:      workloadUID,
			Source:           constant.DetectionSourceLog,
			Status:           constant.DetectionStatusPending,
			LogAvailableFrom: logTimestamp,
			LogAvailableTo:   logTimestamp,
		})
	}

	// Update log_available_from if this is earlier
	updateFrom := coverage.LogAvailableFrom.IsZero() || logTimestamp.Before(coverage.LogAvailableFrom)
	// Update log_available_to if this is later
	updateTo := coverage.LogAvailableTo.IsZero() || logTimestamp.After(coverage.LogAvailableTo)

	if !updateFrom && !updateTo {
		return nil // No update needed
	}

	// Use conditional updates based on which fields need updating
	if updateFrom && updateTo {
		_, err = q.WithContext(ctx).
			Where(q.WorkloadUID.Eq(workloadUID)).
			Where(q.Source.Eq(constant.DetectionSourceLog)).
			UpdateSimple(
				q.LogAvailableFrom.Value(logTimestamp),
				q.LogAvailableTo.Value(logTimestamp),
			)
	} else if updateFrom {
		_, err = q.WithContext(ctx).
			Where(q.WorkloadUID.Eq(workloadUID)).
			Where(q.Source.Eq(constant.DetectionSourceLog)).
			UpdateSimple(
				q.LogAvailableFrom.Value(logTimestamp),
			)
	} else {
		_, err = q.WithContext(ctx).
			Where(q.WorkloadUID.Eq(workloadUID)).
			Where(q.Source.Eq(constant.DetectionSourceLog)).
			UpdateSimple(
				q.LogAvailableTo.Value(logTimestamp),
			)
	}
	return err
}

// UpdateCoveredTimeRange updates the covered time range for log source
func (f *DetectionCoverageFacade) UpdateCoveredTimeRange(ctx context.Context, workloadUID string, from, to time.Time) error {
	q := f.getDAL().DetectionCoverage

	// Get current coverage
	coverage, err := f.GetCoverage(ctx, workloadUID, constant.DetectionSourceLog)
	if err != nil {
		return err
	}

	if coverage == nil {
		return errors.New("log coverage not found")
	}

	// Expand covered range
	updateFrom := coverage.CoveredFrom.IsZero() || from.Before(coverage.CoveredFrom)
	updateTo := coverage.CoveredTo.IsZero() || to.After(coverage.CoveredTo)

	if !updateFrom && !updateTo {
		// No update needed
		return nil
	}

	if updateFrom && updateTo {
		_, err = q.WithContext(ctx).
			Where(q.WorkloadUID.Eq(workloadUID)).
			Where(q.Source.Eq(constant.DetectionSourceLog)).
			UpdateSimple(
				q.CoveredFrom.Value(from),
				q.CoveredTo.Value(to),
			)
	} else if updateFrom {
		_, err = q.WithContext(ctx).
			Where(q.WorkloadUID.Eq(workloadUID)).
			Where(q.Source.Eq(constant.DetectionSourceLog)).
			UpdateSimple(
				q.CoveredFrom.Value(from),
			)
	} else {
		_, err = q.WithContext(ctx).
			Where(q.WorkloadUID.Eq(workloadUID)).
			Where(q.Source.Eq(constant.DetectionSourceLog)).
			UpdateSimple(
				q.CoveredTo.Value(to),
			)
	}
	return err
}

// IncrementEvidenceCount increments the evidence count for a coverage
func (f *DetectionCoverageFacade) IncrementEvidenceCount(ctx context.Context, workloadUID, source string, count int32) error {
	q := f.getDAL().DetectionCoverage

	_, err := q.WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUID)).
		Where(q.Source.Eq(source)).
		UpdateSimple(
			q.EvidenceCount.Add(count),
		)
	return err
}

// InitializeCoverageForWorkload initializes coverage records for all sources
func (f *DetectionCoverageFacade) InitializeCoverageForWorkload(ctx context.Context, workloadUID string) error {
	sources := []string{
		constant.DetectionSourceProcess,
		constant.DetectionSourceLog,
		constant.DetectionSourceImage,
		constant.DetectionSourceLabel,
	}

	now := time.Now()
	coverages := make([]*model.DetectionCoverage, 0, len(sources))

	for _, source := range sources {
		coverages = append(coverages, &model.DetectionCoverage{
			WorkloadUID:  workloadUID,
			Source:       source,
			Status:       constant.DetectionStatusPending,
			AttemptCount: 0,
			CreatedAt:    now,
			UpdatedAt:    now,
		})
	}

	return f.getDAL().DetectionCoverage.WithContext(ctx).CreateInBatches(coverages, len(coverages))
}

// DeleteCoverageByWorkload deletes all coverage records for a workload
func (f *DetectionCoverageFacade) DeleteCoverageByWorkload(ctx context.Context, workloadUID string) error {
	q := f.getDAL().DetectionCoverage
	_, err := q.WithContext(ctx).Where(q.WorkloadUID.Eq(workloadUID)).Delete()
	return err
}

// GetUncoveredLogWindow returns the uncovered log time window if any
func (f *DetectionCoverageFacade) GetUncoveredLogWindow(ctx context.Context, workloadUID string) (*time.Time, *time.Time, error) {
	coverage, err := f.GetCoverage(ctx, workloadUID, constant.DetectionSourceLog)
	if err != nil {
		return nil, nil, err
	}

	if coverage == nil {
		return nil, nil, nil
	}

	// No log available yet
	if coverage.LogAvailableTo.IsZero() {
		return nil, nil, nil
	}

	// Case 1: Never scanned
	if coverage.CoveredTo.IsZero() {
		return &coverage.LogAvailableFrom, &coverage.LogAvailableTo, nil
	}

	// Case 2: New logs available
	if coverage.LogAvailableTo.After(coverage.CoveredTo) {
		return &coverage.CoveredTo, &coverage.LogAvailableTo, nil
	}

	// Case 3: Early logs missed
	if !coverage.LogAvailableFrom.IsZero() && coverage.CoveredFrom.After(coverage.LogAvailableFrom) {
		return &coverage.LogAvailableFrom, &coverage.CoveredFrom, nil
	}

	return nil, nil, nil
}

