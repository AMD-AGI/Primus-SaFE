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

// MetricBaselineFacadeInterface defines the interface for MetricBaseline operations
type MetricBaselineFacadeInterface interface {
	// Create creates a new metric baseline
	Create(ctx context.Context, baseline *model.MetricBaselines) error
	// Upsert creates or updates a metric baseline
	Upsert(ctx context.Context, baseline *model.MetricBaselines) error
	// GetByID gets a metric baseline by ID
	GetByID(ctx context.Context, id int64) (*model.MetricBaselines, error)
	// GetByConfigMetricAndDimension gets a baseline by config, metric name, dimension key, and type
	GetByConfigMetricAndDimension(ctx context.Context, configID int64, metricName, dimensionKey, baselineType string) (*model.MetricBaselines, error)
	// ListByConfig lists all baselines for a config
	ListByConfig(ctx context.Context, configID int64) ([]*model.MetricBaselines, error)
	// ListByConfigAndMetric lists baselines for a config and metric name
	ListByConfigAndMetric(ctx context.Context, configID int64, metricName string) ([]*model.MetricBaselines, error)
	// Update updates a metric baseline
	Update(ctx context.Context, baseline *model.MetricBaselines) error
	// Delete deletes a metric baseline
	Delete(ctx context.Context, id int64) error
	// DeleteByConfig deletes all baselines for a config
	DeleteByConfig(ctx context.Context, configID int64) (int64, error)
	// BulkUpsert bulk upserts multiple baselines
	BulkUpsert(ctx context.Context, baselines []*model.MetricBaselines) error
	// WithCluster returns a new facade instance using the specified cluster
	WithCluster(clusterName string) MetricBaselineFacadeInterface
}

// MetricBaselineFacade implements MetricBaselineFacadeInterface
type MetricBaselineFacade struct {
	BaseFacade
}

// NewMetricBaselineFacade creates a new MetricBaselineFacade
func NewMetricBaselineFacade() *MetricBaselineFacade {
	return &MetricBaselineFacade{}
}

// WithCluster returns a new facade instance using the specified cluster
func (f *MetricBaselineFacade) WithCluster(clusterName string) MetricBaselineFacadeInterface {
	return &MetricBaselineFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// Create creates a new metric baseline
func (f *MetricBaselineFacade) Create(ctx context.Context, baseline *model.MetricBaselines) error {
	return f.getDB().WithContext(ctx).Create(baseline).Error
}

// Upsert creates or updates a metric baseline
func (f *MetricBaselineFacade) Upsert(ctx context.Context, baseline *model.MetricBaselines) error {
	return f.getDB().WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "config_id"},
			{Name: "metric_name"},
			{Name: "dimension_key"},
			{Name: "baseline_type"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"dimensions", "baseline_value", "avg_value", "min_value", "max_value",
			"stddev_value", "sample_count", "start_date", "end_date",
			"last_updated_at", "updated_at",
		}),
	}).Create(baseline).Error
}

// GetByID gets a metric baseline by ID
func (f *MetricBaselineFacade) GetByID(ctx context.Context, id int64) (*model.MetricBaselines, error) {
	var baseline model.MetricBaselines
	err := f.getDB().WithContext(ctx).Where("id = ?", id).First(&baseline).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &baseline, nil
}

// GetByConfigMetricAndDimension gets a baseline by config, metric name, dimension key, and type
func (f *MetricBaselineFacade) GetByConfigMetricAndDimension(ctx context.Context, configID int64, metricName, dimensionKey, baselineType string) (*model.MetricBaselines, error) {
	var baseline model.MetricBaselines
	query := f.getDB().WithContext(ctx).
		Where("config_id = ? AND metric_name = ? AND baseline_type = ?", configID, metricName, baselineType)

	if dimensionKey == "" {
		query = query.Where("dimension_key IS NULL OR dimension_key = ''")
	} else {
		query = query.Where("dimension_key = ?", dimensionKey)
	}

	err := query.First(&baseline).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &baseline, nil
}

// ListByConfig lists all baselines for a config
func (f *MetricBaselineFacade) ListByConfig(ctx context.Context, configID int64) ([]*model.MetricBaselines, error) {
	var baselines []*model.MetricBaselines
	err := f.getDB().WithContext(ctx).
		Where("config_id = ?", configID).
		Order("metric_name, dimension_key").
		Find(&baselines).Error
	return baselines, err
}

// ListByConfigAndMetric lists baselines for a config and metric name
func (f *MetricBaselineFacade) ListByConfigAndMetric(ctx context.Context, configID int64, metricName string) ([]*model.MetricBaselines, error) {
	var baselines []*model.MetricBaselines
	err := f.getDB().WithContext(ctx).
		Where("config_id = ? AND metric_name = ?", configID, metricName).
		Order("dimension_key").
		Find(&baselines).Error
	return baselines, err
}

// Update updates a metric baseline
func (f *MetricBaselineFacade) Update(ctx context.Context, baseline *model.MetricBaselines) error {
	baseline.LastUpdated = time.Now()
	return f.getDB().WithContext(ctx).Save(baseline).Error
}

// Delete deletes a metric baseline
func (f *MetricBaselineFacade) Delete(ctx context.Context, id int64) error {
	return f.getDB().WithContext(ctx).Delete(&model.MetricBaselines{}, id).Error
}

// DeleteByConfig deletes all baselines for a config
func (f *MetricBaselineFacade) DeleteByConfig(ctx context.Context, configID int64) (int64, error) {
	result := f.getDB().WithContext(ctx).
		Where("config_id = ?", configID).
		Delete(&model.MetricBaselines{})
	return result.RowsAffected, result.Error
}

// BulkUpsert bulk upserts multiple baselines
func (f *MetricBaselineFacade) BulkUpsert(ctx context.Context, baselines []*model.MetricBaselines) error {
	if len(baselines) == 0 {
		return nil
	}

	return f.getDB().WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "config_id"},
			{Name: "metric_name"},
			{Name: "dimension_key"},
			{Name: "baseline_type"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"dimensions", "baseline_value", "avg_value", "min_value", "max_value",
			"stddev_value", "sample_count", "start_date", "end_date",
			"last_updated_at", "updated_at",
		}),
	}).CreateInBatches(baselines, 100).Error
}
