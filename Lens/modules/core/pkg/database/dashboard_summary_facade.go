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

// DashboardSummaryFacadeInterface defines the interface for DashboardSummary operations
type DashboardSummaryFacadeInterface interface {
	// Create creates a new dashboard summary
	Create(ctx context.Context, summary *model.DashboardSummaries) error
	// Upsert creates or updates a dashboard summary
	Upsert(ctx context.Context, summary *model.DashboardSummaries) error
	// GetByConfigAndDate gets a dashboard summary by config ID and date
	GetByConfigAndDate(ctx context.Context, configID int64, date time.Time) (*model.DashboardSummaries, error)
	// GetLatestByConfig gets the latest dashboard summary for a config
	GetLatestByConfig(ctx context.Context, configID int64) (*model.DashboardSummaries, error)
	// ListByConfig lists dashboard summaries for a config with date range
	ListByConfig(ctx context.Context, configID int64, startDate, endDate *time.Time, limit int) ([]*model.DashboardSummaries, error)
	// MarkStale marks a summary as stale
	MarkStale(ctx context.Context, id int64) error
	// MarkAllStaleForConfig marks all summaries for a config as stale
	MarkAllStaleForConfig(ctx context.Context, configID int64) error
	// Delete deletes a dashboard summary
	Delete(ctx context.Context, id int64) error
	// DeleteOlderThan deletes summaries older than a given date
	DeleteOlderThan(ctx context.Context, configID int64, before time.Time) (int64, error)
	// WithCluster returns a new facade instance using the specified cluster
	WithCluster(clusterName string) DashboardSummaryFacadeInterface
}

// DashboardSummaryFacade implements DashboardSummaryFacadeInterface
type DashboardSummaryFacade struct {
	BaseFacade
}

// NewDashboardSummaryFacade creates a new DashboardSummaryFacade
func NewDashboardSummaryFacade() *DashboardSummaryFacade {
	return &DashboardSummaryFacade{}
}

// WithCluster returns a new facade instance using the specified cluster
func (f *DashboardSummaryFacade) WithCluster(clusterName string) DashboardSummaryFacadeInterface {
	return &DashboardSummaryFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// Create creates a new dashboard summary
func (f *DashboardSummaryFacade) Create(ctx context.Context, summary *model.DashboardSummaries) error {
	return f.getDB().WithContext(ctx).Create(summary).Error
}

// Upsert creates or updates a dashboard summary
func (f *DashboardSummaryFacade) Upsert(ctx context.Context, summary *model.DashboardSummaries) error {
	return f.getDB().WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "config_id"}, {Name: "summary_date"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"current_run_id", "previous_run_id", "build_status", "build_duration_seconds",
			"commit_count", "pr_count", "contributor_count", "total_additions", "total_deletions",
			"overall_perf_change_percent", "regression_count", "improvement_count", "new_metric_count",
			"top_improvements", "top_regressions", "top_contributors", "active_alerts",
			"generated_at", "expires_at", "is_stale", "updated_at",
		}),
	}).Create(summary).Error
}

// GetByConfigAndDate gets a dashboard summary by config ID and date
func (f *DashboardSummaryFacade) GetByConfigAndDate(ctx context.Context, configID int64, date time.Time) (*model.DashboardSummaries, error) {
	var summary model.DashboardSummaries
	// Normalize date to start of day
	dateOnly := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	err := f.getDB().WithContext(ctx).
		Where("config_id = ? AND summary_date = ?", configID, dateOnly).
		First(&summary).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

// GetLatestByConfig gets the latest dashboard summary for a config
func (f *DashboardSummaryFacade) GetLatestByConfig(ctx context.Context, configID int64) (*model.DashboardSummaries, error) {
	var summary model.DashboardSummaries
	err := f.getDB().WithContext(ctx).
		Where("config_id = ?", configID).
		Order("summary_date DESC").
		First(&summary).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

// ListByConfig lists dashboard summaries for a config with date range
func (f *DashboardSummaryFacade) ListByConfig(ctx context.Context, configID int64, startDate, endDate *time.Time, limit int) ([]*model.DashboardSummaries, error) {
	query := f.getDB().WithContext(ctx).Where("config_id = ?", configID)

	if startDate != nil {
		query = query.Where("summary_date >= ?", startDate)
	}
	if endDate != nil {
		query = query.Where("summary_date <= ?", endDate)
	}

	query = query.Order("summary_date DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	var summaries []*model.DashboardSummaries
	err := query.Find(&summaries).Error
	return summaries, err
}

// MarkStale marks a summary as stale
func (f *DashboardSummaryFacade) MarkStale(ctx context.Context, id int64) error {
	return f.getDB().WithContext(ctx).
		Model(&model.DashboardSummaries{}).
		Where("id = ?", id).
		Update("is_stale", true).Error
}

// MarkAllStaleForConfig marks all summaries for a config as stale
func (f *DashboardSummaryFacade) MarkAllStaleForConfig(ctx context.Context, configID int64) error {
	return f.getDB().WithContext(ctx).
		Model(&model.DashboardSummaries{}).
		Where("config_id = ?", configID).
		Update("is_stale", true).Error
}

// Delete deletes a dashboard summary
func (f *DashboardSummaryFacade) Delete(ctx context.Context, id int64) error {
	return f.getDB().WithContext(ctx).Delete(&model.DashboardSummaries{}, id).Error
}

// DeleteOlderThan deletes summaries older than a given date
func (f *DashboardSummaryFacade) DeleteOlderThan(ctx context.Context, configID int64, before time.Time) (int64, error) {
	result := f.getDB().WithContext(ctx).
		Where("config_id = ? AND summary_date < ?", configID, before).
		Delete(&model.DashboardSummaries{})
	return result.RowsAffected, result.Error
}
