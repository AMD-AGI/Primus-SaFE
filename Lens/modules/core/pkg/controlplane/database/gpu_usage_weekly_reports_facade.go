// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"gorm.io/gorm"
)

// GpuUsageWeeklyReportFacadeInterface defines the database operation interface for GpuUsageWeeklyReports
type GpuUsageWeeklyReportFacadeInterface interface {
	// Create creates a new GPU usage weekly report
	Create(ctx context.Context, report *model.GpuUsageWeeklyReports) error
	// GetByID retrieves a report by its ID
	GetByID(ctx context.Context, id string) (*model.GpuUsageWeeklyReports, error)
	// Update updates an existing report
	Update(ctx context.Context, report *model.GpuUsageWeeklyReports) error
	// List retrieves reports with pagination and filtering
	List(ctx context.Context, clusterName string, status string, pageNum, pageSize int) ([]*model.GpuUsageWeeklyReports, int64, error)
	// ListAllClusters lists all clusters that have reports
	ListAllClusters(ctx context.Context) ([]string, error)
	// GetLatestByCluster gets the most recent report for a cluster
	GetLatestByCluster(ctx context.Context, clusterName string) (*model.GpuUsageWeeklyReports, error)
	// GetLatestForAllClusters gets the most recent report for each cluster
	GetLatestForAllClusters(ctx context.Context) ([]*model.GpuUsageWeeklyReports, error)
	// DeleteOlderThan deletes reports older than the specified time
	DeleteOlderThan(ctx context.Context, before time.Time) (int64, error)
	// UpdateStatus updates only the status field of a report
	UpdateStatus(ctx context.Context, id string, status string) error
	// CheckDuplicate checks if a report already exists for the given cluster and period
	CheckDuplicate(ctx context.Context, clusterName string, periodStart, periodEnd time.Time) (bool, error)
}

// GpuUsageWeeklyReportFacade implements GpuUsageWeeklyReportFacadeInterface
type GpuUsageWeeklyReportFacade struct {
	db *gorm.DB
}

// NewGpuUsageWeeklyReportFacade creates a new GpuUsageWeeklyReportFacade instance
func NewGpuUsageWeeklyReportFacade(db *gorm.DB) GpuUsageWeeklyReportFacadeInterface {
	return &GpuUsageWeeklyReportFacade{db: db}
}

// Create creates a new GPU usage weekly report
func (f *GpuUsageWeeklyReportFacade) Create(ctx context.Context, report *model.GpuUsageWeeklyReports) error {
	if report.CreatedAt.IsZero() {
		report.CreatedAt = time.Now()
	}
	if report.UpdatedAt.IsZero() {
		report.UpdatedAt = time.Now()
	}

	err := f.db.WithContext(ctx).Create(report).Error
	if err != nil {
		log.Errorf("GpuUsageWeeklyReportFacade Create: failed to create report: %v", err)
		return err
	}
	log.Infof("GpuUsageWeeklyReportFacade Create: created report %s for cluster %s", report.ID, report.ClusterName)
	return nil
}

// GetByID retrieves a report by its ID
func (f *GpuUsageWeeklyReportFacade) GetByID(ctx context.Context, id string) (*model.GpuUsageWeeklyReports, error) {
	var report model.GpuUsageWeeklyReports
	err := f.db.WithContext(ctx).Where("id = ?", id).First(&report).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		log.Errorf("GpuUsageWeeklyReportFacade GetByID: failed to get report: %v", err)
		return nil, err
	}
	return &report, nil
}

// Update updates an existing report
func (f *GpuUsageWeeklyReportFacade) Update(ctx context.Context, report *model.GpuUsageWeeklyReports) error {
	report.UpdatedAt = time.Now()
	err := f.db.WithContext(ctx).Save(report).Error
	if err != nil {
		log.Errorf("GpuUsageWeeklyReportFacade Update: failed to update report: %v", err)
		return err
	}
	log.Infof("GpuUsageWeeklyReportFacade Update: updated report %s", report.ID)
	return nil
}

// List retrieves reports with pagination and filtering
func (f *GpuUsageWeeklyReportFacade) List(ctx context.Context, clusterName string, status string, pageNum, pageSize int) ([]*model.GpuUsageWeeklyReports, int64, error) {
	query := f.db.WithContext(ctx).Model(&model.GpuUsageWeeklyReports{})

	// Apply filters
	if clusterName != "" {
		query = query.Where("cluster_name = ?", clusterName)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// Get total count
	var count int64
	if err := query.Count(&count).Error; err != nil {
		log.Errorf("GpuUsageWeeklyReportFacade List: failed to count reports: %v", err)
		return nil, 0, err
	}

	// Apply pagination and ordering
	query = query.Order("generated_at DESC")
	if pageSize > 0 {
		query = query.Limit(pageSize)
	}
	if pageNum > 0 && pageSize > 0 {
		query = query.Offset((pageNum - 1) * pageSize)
	}

	var reports []*model.GpuUsageWeeklyReports
	if err := query.Find(&reports).Error; err != nil {
		log.Errorf("GpuUsageWeeklyReportFacade List: failed to list reports: %v", err)
		return nil, 0, err
	}

	return reports, count, nil
}

// ListAllClusters lists all unique cluster names that have reports
func (f *GpuUsageWeeklyReportFacade) ListAllClusters(ctx context.Context) ([]string, error) {
	var clusters []string
	err := f.db.WithContext(ctx).
		Model(&model.GpuUsageWeeklyReports{}).
		Distinct("cluster_name").
		Pluck("cluster_name", &clusters).Error
	if err != nil {
		log.Errorf("GpuUsageWeeklyReportFacade ListAllClusters: failed to list clusters: %v", err)
		return nil, err
	}
	return clusters, nil
}

// GetLatestByCluster gets the most recent report for a cluster
func (f *GpuUsageWeeklyReportFacade) GetLatestByCluster(ctx context.Context, clusterName string) (*model.GpuUsageWeeklyReports, error) {
	var report model.GpuUsageWeeklyReports
	err := f.db.WithContext(ctx).
		Where("cluster_name = ?", clusterName).
		Order("generated_at DESC").
		First(&report).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		log.Errorf("GpuUsageWeeklyReportFacade GetLatestByCluster: failed to get latest report: %v", err)
		return nil, err
	}
	return &report, nil
}

// GetLatestForAllClusters gets the most recent report for each cluster
func (f *GpuUsageWeeklyReportFacade) GetLatestForAllClusters(ctx context.Context) ([]*model.GpuUsageWeeklyReports, error) {
	// Subquery to get the latest generated_at for each cluster
	subQuery := f.db.WithContext(ctx).
		Model(&model.GpuUsageWeeklyReports{}).
		Select("cluster_name, MAX(generated_at) as max_generated_at").
		Group("cluster_name")

	var reports []*model.GpuUsageWeeklyReports
	err := f.db.WithContext(ctx).
		Model(&model.GpuUsageWeeklyReports{}).
		Joins("JOIN (?) AS latest ON gpu_usage_weekly_reports.cluster_name = latest.cluster_name AND gpu_usage_weekly_reports.generated_at = latest.max_generated_at", subQuery).
		Find(&reports).Error
	if err != nil {
		log.Errorf("GpuUsageWeeklyReportFacade GetLatestForAllClusters: failed to get latest reports: %v", err)
		return nil, err
	}
	return reports, nil
}

// DeleteOlderThan deletes reports older than the specified time
func (f *GpuUsageWeeklyReportFacade) DeleteOlderThan(ctx context.Context, before time.Time) (int64, error) {
	result := f.db.WithContext(ctx).
		Where("generated_at < ?", before).
		Delete(&model.GpuUsageWeeklyReports{})
	if result.Error != nil {
		log.Errorf("GpuUsageWeeklyReportFacade DeleteOlderThan: failed to delete old reports: %v", result.Error)
		return 0, result.Error
	}

	if result.RowsAffected > 0 {
		log.Infof("GpuUsageWeeklyReportFacade DeleteOlderThan: deleted %d old reports", result.RowsAffected)
	}
	return result.RowsAffected, nil
}

// UpdateStatus updates only the status field of a report
func (f *GpuUsageWeeklyReportFacade) UpdateStatus(ctx context.Context, id string, status string) error {
	result := f.db.WithContext(ctx).
		Model(&model.GpuUsageWeeklyReports{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		log.Errorf("GpuUsageWeeklyReportFacade UpdateStatus: failed to update status: %v", result.Error)
		return result.Error
	}
	log.Infof("GpuUsageWeeklyReportFacade UpdateStatus: updated report %s status to %s", id, status)
	return nil
}

// CheckDuplicate checks if a report already exists for the given cluster and period
func (f *GpuUsageWeeklyReportFacade) CheckDuplicate(ctx context.Context, clusterName string, periodStart, periodEnd time.Time) (bool, error) {
	var count int64
	err := f.db.WithContext(ctx).
		Model(&model.GpuUsageWeeklyReports{}).
		Where("cluster_name = ? AND period_start = ? AND period_end = ?", clusterName, periodStart, periodEnd).
		Count(&count).Error
	if err != nil {
		log.Errorf("GpuUsageWeeklyReportFacade CheckDuplicate: failed to check duplicate: %v", err)
		return false, err
	}
	return count > 0, nil
}
