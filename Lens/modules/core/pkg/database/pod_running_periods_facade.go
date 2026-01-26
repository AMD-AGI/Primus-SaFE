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

// PodRunningPeriodsFacadeInterface defines database operations for pod running periods
type PodRunningPeriodsFacadeInterface interface {
	// CreateRunningPeriod creates a new running period record when pod enters Running phase
	CreateRunningPeriod(ctx context.Context, period *model.PodRunningPeriods) error
	// EndRunningPeriod sets end_at for the current running period when pod leaves Running phase
	EndRunningPeriod(ctx context.Context, podUID string, endAt time.Time) error
	// GetCurrentRunningPeriod returns the current running period (end_at is NULL) for a pod
	GetCurrentRunningPeriod(ctx context.Context, podUID string) (*model.PodRunningPeriods, error)
	// ListRunningPeriodsInTimeRange returns all running periods that overlap with the given time range
	// A period overlaps if: start_at < endTime AND (end_at IS NULL OR end_at > startTime)
	ListRunningPeriodsInTimeRange(ctx context.Context, startTime, endTime time.Time) ([]*model.PodRunningPeriods, error)
	// ListRunningPeriodsInTimeRangeByNamespace returns running periods for a specific namespace
	ListRunningPeriodsInTimeRangeByNamespace(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*model.PodRunningPeriods, error)
	// ListRunningPeriodsInTimeRangeByPodUIDs returns running periods for specific pod UIDs
	ListRunningPeriodsInTimeRangeByPodUIDs(ctx context.Context, podUIDs []string, startTime, endTime time.Time) ([]*model.PodRunningPeriods, error)
	// WithCluster returns a new facade instance for the specified cluster
	WithCluster(clusterName string) PodRunningPeriodsFacadeInterface
}

// PodRunningPeriodsFacade implements PodRunningPeriodsFacadeInterface
type PodRunningPeriodsFacade struct {
	BaseFacade
}

// NewPodRunningPeriodsFacade creates a new PodRunningPeriodsFacade instance
func NewPodRunningPeriodsFacade() PodRunningPeriodsFacadeInterface {
	return &PodRunningPeriodsFacade{}
}

func (f *PodRunningPeriodsFacade) WithCluster(clusterName string) PodRunningPeriodsFacadeInterface {
	return &PodRunningPeriodsFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// CreateRunningPeriod creates a new running period record
func (f *PodRunningPeriodsFacade) CreateRunningPeriod(ctx context.Context, period *model.PodRunningPeriods) error {
	return f.getDAL().PodRunningPeriods.WithContext(ctx).Create(period)
}

// EndRunningPeriod sets end_at for the current running period
func (f *PodRunningPeriodsFacade) EndRunningPeriod(ctx context.Context, podUID string, endAt time.Time) error {
	q := f.getDAL().PodRunningPeriods
	// Find the running period with NULL end_at
	period, err := q.WithContext(ctx).
		Where(q.PodUID.Eq(podUID)).
		Where(q.EndAt.IsNull()).
		First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// No active running period found, ignore
			return nil
		}
		return err
	}

	// Check if record was actually found (GORM gen may return empty object instead of error)
	if period == nil || period.ID == 0 {
		// No active running period found, ignore
		return nil
	}

	// Update end_at
	period.EndAt = endAt
	period.UpdatedAt = time.Now()
	return f.getDAL().PodRunningPeriods.WithContext(ctx).Save(period)
}

// GetCurrentRunningPeriod returns the current running period (end_at is NULL)
func (f *PodRunningPeriodsFacade) GetCurrentRunningPeriod(ctx context.Context, podUID string) (*model.PodRunningPeriods, error) {
	q := f.getDAL().PodRunningPeriods
	result, err := q.WithContext(ctx).
		Where(q.PodUID.Eq(podUID)).
		Where(q.EndAt.IsNull()).
		First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	// Check if record was actually found (GORM gen may return empty object instead of error)
	if result == nil || result.ID == 0 {
		return nil, nil
	}
	return result, nil
}

// ListRunningPeriodsInTimeRange returns all running periods that overlap with the time range
func (f *PodRunningPeriodsFacade) ListRunningPeriodsInTimeRange(ctx context.Context, startTime, endTime time.Time) ([]*model.PodRunningPeriods, error) {
	q := f.getDAL().PodRunningPeriods
	// Overlap condition: start_at < endTime AND (end_at IS NULL OR end_at > startTime)
	results, err := q.WithContext(ctx).
		Where(q.StartAt.Lt(endTime)).
		Where(q.WithContext(ctx).Or(
			q.EndAt.IsNull(),
			q.EndAt.Gt(startTime),
		)).
		Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*model.PodRunningPeriods{}, nil
		}
		return nil, err
	}
	return results, nil
}

// ListRunningPeriodsInTimeRangeByNamespace returns running periods for a specific namespace
func (f *PodRunningPeriodsFacade) ListRunningPeriodsInTimeRangeByNamespace(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*model.PodRunningPeriods, error) {
	q := f.getDAL().PodRunningPeriods
	results, err := q.WithContext(ctx).
		Where(q.Namespace.Eq(namespace)).
		Where(q.StartAt.Lt(endTime)).
		Where(q.WithContext(ctx).Or(
			q.EndAt.IsNull(),
			q.EndAt.Gt(startTime),
		)).
		Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*model.PodRunningPeriods{}, nil
		}
		return nil, err
	}
	return results, nil
}

// ListRunningPeriodsInTimeRangeByPodUIDs returns running periods for specific pod UIDs
func (f *PodRunningPeriodsFacade) ListRunningPeriodsInTimeRangeByPodUIDs(ctx context.Context, podUIDs []string, startTime, endTime time.Time) ([]*model.PodRunningPeriods, error) {
	if len(podUIDs) == 0 {
		return []*model.PodRunningPeriods{}, nil
	}

	q := f.getDAL().PodRunningPeriods
	results, err := q.WithContext(ctx).
		Where(q.PodUID.In(podUIDs...)).
		Where(q.StartAt.Lt(endTime)).
		Where(q.WithContext(ctx).Or(
			q.EndAt.IsNull(),
			q.EndAt.Gt(startTime),
		)).
		Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*model.PodRunningPeriods{}, nil
		}
		return nil, err
	}
	return results, nil
}
