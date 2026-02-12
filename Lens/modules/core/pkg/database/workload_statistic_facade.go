// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/dal"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// WorkloadStatisticFacadeInterface defines the WorkloadStatistic Facade interface
type WorkloadStatisticFacadeInterface interface {
	// GetDB returns the underlying GORM database connection
	GetDB() *gorm.DB
	// GetOrCreate gets or creates a statistic record for the workload
	GetOrCreate(ctx context.Context, clusterName string, workload *model.GpuWorkload) (*model.WorkloadStatistic, bool, error)
	// Update updates or creates a statistic record
	Update(ctx context.Context, record *model.WorkloadStatistic) error
	// UpdateInstantOnly updates only the instant utilization and timestamp fields.
	// This avoids rewriting the entire row (including large JSONB fields like histogram)
	// when no new sample data is available, significantly reducing dead tuple generation.
	UpdateInstantOnly(ctx context.Context, id int32, instantUtilization float64, lastQueryTime, statEndTime time.Time) error
	// UpdateStatistics updates only the statistics-related columns.
	// This avoids rewriting immutable columns (uid, cluster_name, namespace, labels, annotations)
	// to reduce row size and dead tuple generation.
	UpdateStatistics(ctx context.Context, record *model.WorkloadStatistic) error
	// GetByUID gets a workload statistic by UID
	GetByUID(ctx context.Context, uid string) (*model.WorkloadStatistic, error)
	// List lists workload statistics with optional filters
	List(ctx context.Context, clusterName string, namespace string, workloadName string, statuses []string) ([]*model.WorkloadStatistic, error)
	// Delete deletes a workload statistic record
	Delete(ctx context.Context, id int32) error
	// BatchUpdate updates multiple records in a batch
	BatchUpdate(ctx context.Context, records []*model.WorkloadStatistic) error
	// WithCluster returns a new facade instance for the specified cluster
	WithCluster(clusterName string) WorkloadStatisticFacadeInterface
}

// WorkloadStatisticFacade implements WorkloadStatisticFacadeInterface
type WorkloadStatisticFacade struct {
	BaseFacade
}

// NewWorkloadStatisticFacade creates a new WorkloadStatistic Facade
func NewWorkloadStatisticFacade() *WorkloadStatisticFacade {
	return &WorkloadStatisticFacade{}
}

// GetDB returns the underlying GORM database connection
func (f *WorkloadStatisticFacade) GetDB() *gorm.DB {
	return f.getDB()
}

// GetOrCreate gets or creates a statistic record for the workload
func (f *WorkloadStatisticFacade) GetOrCreate(ctx context.Context, clusterName string, workload *model.GpuWorkload) (*model.WorkloadStatistic, bool, error) {
	db := f.getDB()
	q := dal.Use(db).WorkloadStatistic

	// Use workload's own UID for statistics tracking
	// Each workload (including child workloads) has its own independent statistic record
	workloadUID := workload.UID

	// Try to query existing record
	// Use Take() instead of First() to avoid ORDER BY id which prevents using uid index
	record, err := q.WithContext(ctx).Where(
		q.ClusterName.Eq(clusterName),
		q.Namespace.Eq(workload.Namespace),
		q.WorkloadName.Eq(workload.Name),
		q.UID.Eq(workloadUID),
		q.WorkloadStatus.In("Running", "Pending"),
	).Take()

	// If found existing record, return it
	if err == nil && record.ID > 0 {
		return record, false, nil
	}

	// If error is not "record not found", return error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, false, fmt.Errorf("failed to query existing record: %w", err)
	}

	// Create new record with default values
	newRecord := &model.WorkloadStatistic{
		UID:                   workloadUID,
		ClusterName:           clusterName,
		Namespace:             workload.Namespace,
		WorkloadName:          workload.Name,
		WorkloadType:          workload.Kind,
		WorkloadStatus:        string(workload.Status),
		StatStartTime:         workload.CreatedAt,
		AllocatedGpuCount:     float64(workload.GpuRequest),
		Labels:                workload.Labels,
		Annotations:           workload.Annotations,
		SampleCount:           0,
		TotalSum:              0,
		InstantGpuUtilization: 0,
		AvgGpuUtilization:     0,
		MaxGpuUtilization:     0,
		MinGpuUtilization:     0,
		P50GpuUtilization:     0,
		P90GpuUtilization:     0,
		P95GpuUtilization:     0,
		Histogram:             model.ExtJSON(`{"buckets": []}`), // Initialize empty histogram
	}

	return newRecord, true, nil
}

// UpdateInstantOnly updates only the instant utilization and timestamp fields.
// This is used when no new sample data is available from Prometheus,
// avoiding the need to rewrite large JSONB columns (histogram, labels, annotations).
func (f *WorkloadStatisticFacade) UpdateInstantOnly(ctx context.Context, id int32, instantUtilization float64, lastQueryTime, statEndTime time.Time) error {
	db := f.getDB()
	return db.WithContext(ctx).
		Model(&model.WorkloadStatistic{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"instant_gpu_utilization": instantUtilization,
			"last_query_time":         lastQueryTime,
			"stat_end_time":           statEndTime,
			"updated_at":              time.Now(),
		}).Error
}

// UpdateStatistics updates only the statistics-related columns, avoiding immutable
// columns like uid, cluster_name, namespace, labels, and annotations.
// This significantly reduces the amount of data written per UPDATE and helps
// control dead tuple growth.
func (f *WorkloadStatisticFacade) UpdateStatistics(ctx context.Context, record *model.WorkloadStatistic) error {
	db := f.getDB()
	return db.WithContext(ctx).
		Model(&model.WorkloadStatistic{}).
		Where("id = ?", record.ID).
		Updates(map[string]interface{}{
			"instant_gpu_utilization": record.InstantGpuUtilization,
			"avg_gpu_utilization":     record.AvgGpuUtilization,
			"p50_gpu_utilization":     record.P50GpuUtilization,
			"p90_gpu_utilization":     record.P90GpuUtilization,
			"p95_gpu_utilization":     record.P95GpuUtilization,
			"max_gpu_utilization":     record.MaxGpuUtilization,
			"min_gpu_utilization":     record.MinGpuUtilization,
			"sample_count":            record.SampleCount,
			"total_sum":               record.TotalSum,
			"histogram":               record.Histogram,
			"stat_start_time":         record.StatStartTime,
			"stat_end_time":           record.StatEndTime,
			"last_query_time":         record.LastQueryTime,
			"workload_status":         record.WorkloadStatus,
			"allocated_gpu_count":     record.AllocatedGpuCount,
			"updated_at":              time.Now(),
		}).Error
}

// Update updates or creates a statistic record
func (f *WorkloadStatisticFacade) Update(ctx context.Context, record *model.WorkloadStatistic) error {
	db := f.getDB()
	q := dal.Use(db).WorkloadStatistic

	// If record exists (has ID), update it
	if record.ID > 0 {
		_, err := q.WithContext(ctx).Where(q.ID.Eq(record.ID)).Updates(record)
		return err
	}

	// Try to create new record
	err := q.WithContext(ctx).Create(record)
	if err == nil {
		return nil
	}

	// If create failed due to unique constraint violation (concurrent insert),
	// try to find the existing record and update it
	if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
		// Use Take() instead of First() to avoid ORDER BY id which prevents using uid index
		existingRecord, findErr := q.WithContext(ctx).Where(
			q.ClusterName.Eq(record.ClusterName),
			q.Namespace.Eq(record.Namespace),
			q.WorkloadName.Eq(record.WorkloadName),
			q.UID.Eq(record.UID),
			q.WorkloadStatus.In("Running", "Pending"),
		).Take()

		if findErr == nil && existingRecord.ID > 0 {
			// Found the record created by another goroutine, update it
			record.ID = existingRecord.ID
			_, updateErr := q.WithContext(ctx).Where(q.ID.Eq(record.ID)).Updates(record)
			return updateErr
		}
	}

	return err
}

// GetByUID gets a workload statistic by UID
func (f *WorkloadStatisticFacade) GetByUID(ctx context.Context, uid string) (*model.WorkloadStatistic, error) {
	db := f.getDB()
	q := dal.Use(db).WorkloadStatistic

	// Use Take() instead of First() to avoid ORDER BY id which prevents using uid index
	record, err := q.WithContext(ctx).Where(
		q.UID.Eq(uid),
	).Take()

	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	if err == gorm.ErrRecordNotFound || record.ID == 0 {
		return nil, nil
	}

	return record, nil
}

// List lists workload statistics with optional filters
func (f *WorkloadStatisticFacade) List(ctx context.Context, clusterName string, namespace string, workloadName string, statuses []string) ([]*model.WorkloadStatistic, error) {
	db := f.getDB()
	q := dal.Use(db).WorkloadStatistic

	query := q.WithContext(ctx).Where(q.ClusterName.Eq(clusterName))

	if namespace != "" {
		query = query.Where(q.Namespace.Eq(namespace))
	}

	if workloadName != "" {
		query = query.Where(q.WorkloadName.Eq(workloadName))
	}

	if len(statuses) > 0 {
		query = query.Where(q.WorkloadStatus.In(statuses...))
	}

	records, err := query.Find()
	if err != nil {
		return nil, err
	}

	return records, nil
}

// Delete deletes a workload statistic record
func (f *WorkloadStatisticFacade) Delete(ctx context.Context, id int32) error {
	db := f.getDB()
	q := dal.Use(db).WorkloadStatistic

	_, err := q.WithContext(ctx).Where(q.ID.Eq(id)).Delete()
	return err
}

// BatchUpdate updates multiple records in a batch
func (f *WorkloadStatisticFacade) BatchUpdate(ctx context.Context, records []*model.WorkloadStatistic) error {
	if len(records) == 0 {
		return nil
	}

	db := f.getDB()
	q := dal.Use(db).WorkloadStatistic

	// Use transaction for batch update
	return db.Transaction(func(tx *gorm.DB) error {
		txq := dal.Use(tx).WorkloadStatistic
		for _, record := range records {
			if record.ID > 0 {
				_, err := txq.WithContext(ctx).Where(q.ID.Eq(record.ID)).Updates(record)
				if err != nil {
					return err
				}
			} else {
				err := txq.WithContext(ctx).Create(record)
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// WithCluster returns a new facade instance for the specified cluster
func (f *WorkloadStatisticFacade) WithCluster(clusterName string) WorkloadStatisticFacadeInterface {
	newFacade := &WorkloadStatisticFacade{
		BaseFacade: f.withCluster(clusterName),
	}
	return newFacade
}
