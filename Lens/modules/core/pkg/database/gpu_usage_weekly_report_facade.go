package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
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
	// GetLatestByCluster gets the most recent report for a cluster
	GetLatestByCluster(ctx context.Context, clusterName string) (*model.GpuUsageWeeklyReports, error)
	// DeleteOlderThan deletes reports older than the specified time
	DeleteOlderThan(ctx context.Context, before time.Time) (int64, error)
	// UpdateStatus updates only the status field of a report
	UpdateStatus(ctx context.Context, id string, status string) error
	// WithCluster returns a new facade instance with cluster context
	WithCluster(clusterName string) GpuUsageWeeklyReportFacadeInterface
}

// GpuUsageWeeklyReportFacade implements GpuUsageWeeklyReportFacadeInterface
type GpuUsageWeeklyReportFacade struct {
	BaseFacade
}

// NewGpuUsageWeeklyReportFacade creates a new GpuUsageWeeklyReportFacade instance
func NewGpuUsageWeeklyReportFacade() GpuUsageWeeklyReportFacadeInterface {
	return &GpuUsageWeeklyReportFacade{}
}

func (f *GpuUsageWeeklyReportFacade) WithCluster(clusterName string) GpuUsageWeeklyReportFacadeInterface {
	return &GpuUsageWeeklyReportFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// Create creates a new GPU usage weekly report
func (f *GpuUsageWeeklyReportFacade) Create(ctx context.Context, report *model.GpuUsageWeeklyReports) error {
	q := f.getDAL().GpuUsageWeeklyReports
	err := q.WithContext(ctx).Create(report)
	if err != nil {
		log.Errorf("GpuUsageWeeklyReportFacade Create: failed to create report: %v", err)
		return err
	}
	log.Infof("GpuUsageWeeklyReportFacade Create: created report with ID: %s", report.ID)
	return nil
}

// GetByID retrieves a report by its ID
func (f *GpuUsageWeeklyReportFacade) GetByID(ctx context.Context, id string) (*model.GpuUsageWeeklyReports, error) {
	q := f.getDAL().GpuUsageWeeklyReports
	report, err := q.WithContext(ctx).Where(q.ID.Eq(id)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Debugf("GpuUsageWeeklyReportFacade GetByID: no report found with ID: %s", id)
			return nil, nil
		}
		log.Errorf("GpuUsageWeeklyReportFacade GetByID: failed to get report: %v", err)
		return nil, err
	}
	return report, nil
}

// Update updates an existing report
func (f *GpuUsageWeeklyReportFacade) Update(ctx context.Context, report *model.GpuUsageWeeklyReports) error {
	report.UpdatedAt = time.Now()
	q := f.getDAL().GpuUsageWeeklyReports
	err := q.WithContext(ctx).Save(report)
	if err != nil {
		log.Errorf("GpuUsageWeeklyReportFacade Update: failed to update report: %v", err)
		return err
	}
	log.Infof("GpuUsageWeeklyReportFacade Update: updated report with ID: %s", report.ID)
	return nil
}

// List retrieves reports with pagination and filtering
func (f *GpuUsageWeeklyReportFacade) List(ctx context.Context, clusterName string, status string, pageNum, pageSize int) ([]*model.GpuUsageWeeklyReports, int64, error) {
	q := f.getDAL().GpuUsageWeeklyReports
	query := q.WithContext(ctx)

	// Apply filters
	if clusterName != "" {
		query = query.Where(q.ClusterName.Eq(clusterName))
	}
	if status != "" {
		query = query.Where(q.Status.Eq(status))
	}

	// Get total count
	count, err := query.Count()
	if err != nil {
		log.Errorf("GpuUsageWeeklyReportFacade List: failed to count reports: %v", err)
		return nil, 0, err
	}

	// Apply pagination and ordering
	gormDB := query.UnderlyingDB()
	gormDB = gormDB.Order("generated_at DESC")

	if pageSize > 0 {
		gormDB = gormDB.Limit(pageSize)
	}
	if pageNum > 0 && pageSize > 0 {
		gormDB = gormDB.Offset((pageNum - 1) * pageSize)
	}

	var reports []*model.GpuUsageWeeklyReports
	err = gormDB.Find(&reports).Error
	if err != nil {
		log.Errorf("GpuUsageWeeklyReportFacade List: failed to list reports: %v", err)
		return nil, 0, err
	}

	log.Debugf("GpuUsageWeeklyReportFacade List: found %d reports (total: %d)", len(reports), count)
	return reports, count, nil
}

// GetLatestByCluster gets the most recent report for a cluster
func (f *GpuUsageWeeklyReportFacade) GetLatestByCluster(ctx context.Context, clusterName string) (*model.GpuUsageWeeklyReports, error) {
	q := f.getDAL().GpuUsageWeeklyReports
	report, err := q.WithContext(ctx).
		Where(q.ClusterName.Eq(clusterName)).
		Order(q.GeneratedAt.Desc()).
		First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Debugf("GpuUsageWeeklyReportFacade GetLatestByCluster: no report found for cluster: %s", clusterName)
			return nil, nil
		}
		log.Errorf("GpuUsageWeeklyReportFacade GetLatestByCluster: failed to get latest report: %v", err)
		return nil, err
	}
	return report, nil
}

// DeleteOlderThan deletes reports older than the specified time
func (f *GpuUsageWeeklyReportFacade) DeleteOlderThan(ctx context.Context, before time.Time) (int64, error) {
	q := f.getDAL().GpuUsageWeeklyReports
	result, err := q.WithContext(ctx).Where(q.GeneratedAt.Lt(before)).Delete()
	if err != nil {
		log.Errorf("GpuUsageWeeklyReportFacade DeleteOlderThan: failed to delete old reports: %v", err)
		return 0, err
	}

	rowsAffected := result.RowsAffected
	if rowsAffected > 0 {
		log.Infof("GpuUsageWeeklyReportFacade DeleteOlderThan: deleted %d old reports", rowsAffected)
	}
	return rowsAffected, nil
}

// UpdateStatus updates only the status field of a report
func (f *GpuUsageWeeklyReportFacade) UpdateStatus(ctx context.Context, id string, status string) error {
	q := f.getDAL().GpuUsageWeeklyReports
	_, err := q.WithContext(ctx).
		Where(q.ID.Eq(id)).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now(),
		})
	if err != nil {
		log.Errorf("GpuUsageWeeklyReportFacade UpdateStatus: failed to update status: %v", err)
		return err
	}
	log.Infof("GpuUsageWeeklyReportFacade UpdateStatus: updated report %s status to %s", id, status)
	return nil
}
