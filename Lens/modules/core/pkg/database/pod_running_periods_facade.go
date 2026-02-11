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
	// A period overlaps if: start_at < endTime AND (end_at IS NULL OR end_at > startTime OR end_at = '0001-01-01' zero value)
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
	db := f.getDB()
	// Use raw SQL to handle both NULL and zero time (0001-01-01) as "not ended"
	// Go's zero time value is stored as '0001-01-01 00:00:00+00' in PostgreSQL
	result := db.WithContext(ctx).
		Model(&model.PodRunningPeriods{}).
		Where("pod_uid = ?", podUID).
		Where("end_at IS NULL OR end_at = '0001-01-01 00:00:00+00'").
		Updates(map[string]interface{}{
			"end_at":     endAt,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}

	// RowsAffected will be 0 if no matching record was found, which is expected
	return nil
}

// GetCurrentRunningPeriod returns the current running period (end_at is NULL or zero time)
func (f *PodRunningPeriodsFacade) GetCurrentRunningPeriod(ctx context.Context, podUID string) (*model.PodRunningPeriods, error) {
	db := f.getDB()
	var result model.PodRunningPeriods
	// Handle both NULL and zero time (0001-01-01) as "not ended"
	err := db.WithContext(ctx).
		Model(&model.PodRunningPeriods{}).
		Where("pod_uid = ?", podUID).
		Where("end_at IS NULL OR end_at = '0001-01-01 00:00:00+00'").
		First(&result).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	// Check if record was actually found
	if result.ID == 0 {
		return nil, nil
	}
	return &result, nil
}

// ListRunningPeriodsInTimeRange returns all running periods that overlap with the time range
func (f *PodRunningPeriodsFacade) ListRunningPeriodsInTimeRange(ctx context.Context, startTime, endTime time.Time) ([]*model.PodRunningPeriods, error) {
	db := f.getDB()
	var results []*model.PodRunningPeriods
	// Overlap condition: start_at < endTime AND (end_at IS NULL OR end_at > startTime OR end_at is zero value)
	// Note: Go's zero time (0001-01-01) is used to indicate "not ended yet"
	err := db.WithContext(ctx).
		Model(&model.PodRunningPeriods{}).
		Where("start_at < ?", endTime).
		Where("end_at IS NULL OR end_at > ? OR end_at = '0001-01-01 00:00:00+00'", startTime).
		Find(&results).Error
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
	db := f.getDB()
	var results []*model.PodRunningPeriods
	// Overlap condition: namespace = ? AND start_at < endTime AND (end_at IS NULL OR end_at > startTime OR end_at is zero value)
	// Note: Go's zero time (0001-01-01) is used to indicate "not ended yet"
	err := db.WithContext(ctx).
		Model(&model.PodRunningPeriods{}).
		Where("namespace = ?", namespace).
		Where("start_at < ?", endTime).
		Where("end_at IS NULL OR end_at > ? OR end_at = '0001-01-01 00:00:00+00'", startTime).
		Find(&results).Error
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

	db := f.getDB()
	var results []*model.PodRunningPeriods
	// Overlap condition: pod_uid IN (?) AND start_at < endTime AND (end_at IS NULL OR end_at > startTime OR end_at is zero value)
	// Note: Go's zero time (0001-01-01) is used to indicate "not ended yet"
	err := db.WithContext(ctx).
		Model(&model.PodRunningPeriods{}).
		Where("pod_uid IN ?", podUIDs).
		Where("start_at < ?", endTime).
		Where("end_at IS NULL OR end_at > ? OR end_at = '0001-01-01 00:00:00+00'", startTime).
		Find(&results).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*model.PodRunningPeriods{}, nil
		}
		return nil, err
	}
	return results, nil
}
