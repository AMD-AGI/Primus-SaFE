package database

import (
	"context"
	"errors"
	"math"
	"time"

	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// PaginationOptions pagination options
type PaginationOptions struct {
	Page           int    // Page number, starting from 1
	PageSize       int    // Number of items per page
	OrderBy        string // Sort field: time, utilization
	OrderDirection string // Sort direction: asc, desc
}

// PaginatedResult pagination result
type PaginatedResult struct {
	Total      int64       // Total number of records
	Page       int         // Current page number
	PageSize   int         // Number of items per page
	TotalPages int         // Total number of pages
	Data       interface{} // Data list
}

// GpuAggregationFacadeInterface defines GPU aggregation database operations interface
type GpuAggregationFacadeInterface interface {
	// ClusterGpuHourlyStats operations
	SaveClusterHourlyStats(ctx context.Context, stats *dbmodel.ClusterGpuHourlyStats) error
	BatchSaveClusterHourlyStats(ctx context.Context, stats []*dbmodel.ClusterGpuHourlyStats) error
	GetClusterHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.ClusterGpuHourlyStats, error)
	GetClusterHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error)

	// NamespaceGpuHourlyStats operations
	SaveNamespaceHourlyStats(ctx context.Context, stats *dbmodel.NamespaceGpuHourlyStats) error
	BatchSaveNamespaceHourlyStats(ctx context.Context, stats []*dbmodel.NamespaceGpuHourlyStats) error
	GetNamespaceHourlyStats(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error)
	ListNamespaceHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error)
	GetNamespaceHourlyStatsPaginated(ctx context.Context, namespace string, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error)
	ListNamespaceHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error)

	// LabelGpuHourlyStats operations
	SaveLabelHourlyStats(ctx context.Context, stats *dbmodel.LabelGpuHourlyStats) error
	BatchSaveLabelHourlyStats(ctx context.Context, stats []*dbmodel.LabelGpuHourlyStats) error
	GetLabelHourlyStats(ctx context.Context, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error)
	ListLabelHourlyStatsByKey(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error)
	GetLabelHourlyStatsPaginated(ctx context.Context, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error)
	ListLabelHourlyStatsByKeyPaginated(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error)

	// WorkloadGpuHourlyStats operations
	SaveWorkloadHourlyStats(ctx context.Context, stats *dbmodel.WorkloadGpuHourlyStats) error
	BatchSaveWorkloadHourlyStats(ctx context.Context, stats []*dbmodel.WorkloadGpuHourlyStats) error
	GetWorkloadHourlyStats(ctx context.Context, namespace, workloadName string, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error)
	ListWorkloadHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error)
	ListWorkloadHourlyStatsByNamespace(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error)
	GetWorkloadHourlyStatsPaginated(ctx context.Context, namespace, workloadName, workloadType string, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error)
	ListWorkloadHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error)
	ListWorkloadHourlyStatsByNamespacePaginated(ctx context.Context, namespace string, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error)

	// GpuAllocationSnapshot operations
	SaveSnapshot(ctx context.Context, snapshot *dbmodel.GpuAllocationSnapshots) error
	GetLatestSnapshot(ctx context.Context) (*dbmodel.GpuAllocationSnapshots, error)
	ListSnapshots(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.GpuAllocationSnapshots, error)

	// Data cleanup
	CleanupOldSnapshots(ctx context.Context, beforeTime time.Time) (int64, error)
	CleanupOldHourlyStats(ctx context.Context, beforeTime time.Time) (int64, error)

	// Metadata queries
	GetDistinctNamespaces(ctx context.Context, startTime, endTime time.Time) ([]string, error)
	GetDistinctDimensionKeys(ctx context.Context, dimensionType string, startTime, endTime time.Time) ([]string, error)
	GetDistinctDimensionValues(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time) ([]string, error)

	// WithCluster method
	WithCluster(clusterName string) GpuAggregationFacadeInterface
}

// GpuAggregationFacade implements GpuAggregationFacadeInterface
type GpuAggregationFacade struct {
	BaseFacade
}

// NewGpuAggregationFacade creates a new GpuAggregationFacade instance
func NewGpuAggregationFacade() GpuAggregationFacadeInterface {
	return &GpuAggregationFacade{}
}

func (f *GpuAggregationFacade) WithCluster(clusterName string) GpuAggregationFacadeInterface {
	return &GpuAggregationFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// ==================== ClusterGpuHourlyStats operations implementation ====================

// SaveClusterHourlyStats saves cluster hourly statistics (using ON CONFLICT update)
func (f *GpuAggregationFacade) SaveClusterHourlyStats(ctx context.Context, stats *dbmodel.ClusterGpuHourlyStats) error {
	q := f.getDAL().ClusterGpuHourlyStats

	// Check if already exists
	existing, err := q.WithContext(ctx).
		Where(q.StatHour.Eq(stats.StatHour)).
		First()

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if existing != nil {
		// Update existing record
		stats.ID = existing.ID
		return q.WithContext(ctx).Save(stats)
	}

	// Create new record
	return q.WithContext(ctx).Create(stats)
}

// BatchSaveClusterHourlyStats batch saves cluster hourly statistics
func (f *GpuAggregationFacade) BatchSaveClusterHourlyStats(ctx context.Context, stats []*dbmodel.ClusterGpuHourlyStats) error {
	if len(stats) == 0 {
		return nil
	}

	// Use transaction for batch insert
	return f.getDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, stat := range stats {
			if err := f.SaveClusterHourlyStats(ctx, stat); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetClusterHourlyStats queries cluster hourly statistics
func (f *GpuAggregationFacade) GetClusterHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.ClusterGpuHourlyStats, error) {
	q := f.getDAL().ClusterGpuHourlyStats

	result, err := q.WithContext(ctx).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Order(q.StatHour.Asc()).
		Find()

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*dbmodel.ClusterGpuHourlyStats{}, nil
		}
		return nil, err
	}

	return result, nil
}

// ==================== NamespaceGpuHourlyStats operations implementation ====================

// SaveNamespaceHourlyStats saves namespace hourly statistics
func (f *GpuAggregationFacade) SaveNamespaceHourlyStats(ctx context.Context, stats *dbmodel.NamespaceGpuHourlyStats) error {
	q := f.getDAL().NamespaceGpuHourlyStats

	// Check if already exists
	existing, err := q.WithContext(ctx).
		Where(q.Namespace.Eq(stats.Namespace)).
		Where(q.StatHour.Eq(stats.StatHour)).
		First()

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if existing != nil {
		// Update existing record
		stats.ID = existing.ID
		return q.WithContext(ctx).Save(stats)
	}

	// Create new record
	return q.WithContext(ctx).Create(stats)
}

// BatchSaveNamespaceHourlyStats batch saves namespace hourly statistics
func (f *GpuAggregationFacade) BatchSaveNamespaceHourlyStats(ctx context.Context, stats []*dbmodel.NamespaceGpuHourlyStats) error {
	if len(stats) == 0 {
		return nil
	}

	// Use transaction for batch insert
	return f.getDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, stat := range stats {
			if err := f.SaveNamespaceHourlyStats(ctx, stat); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetNamespaceHourlyStats queries hourly statistics for specific namespace
func (f *GpuAggregationFacade) GetNamespaceHourlyStats(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error) {
	q := f.getDAL().NamespaceGpuHourlyStats

	result, err := q.WithContext(ctx).
		Where(q.Namespace.Eq(namespace)).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Order(q.StatHour.Asc()).
		Find()

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*dbmodel.NamespaceGpuHourlyStats{}, nil
		}
		return nil, err
	}

	return result, nil
}

// ListNamespaceHourlyStats queries hourly statistics for all namespaces
func (f *GpuAggregationFacade) ListNamespaceHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error) {
	q := f.getDAL().NamespaceGpuHourlyStats

	result, err := q.WithContext(ctx).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Order(q.Namespace.Asc()).
		Order(q.StatHour.Asc()).
		Find()

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*dbmodel.NamespaceGpuHourlyStats{}, nil
		}
		return nil, err
	}

	return result, nil
}

// ==================== LabelGpuHourlyStats operations implementation ====================

// SaveLabelHourlyStats saves label/annotation hourly statistics
func (f *GpuAggregationFacade) SaveLabelHourlyStats(ctx context.Context, stats *dbmodel.LabelGpuHourlyStats) error {
	q := f.getDAL().LabelGpuHourlyStats

	// Check if already exists
	existing, err := q.WithContext(ctx).
		Where(q.DimensionType.Eq(stats.DimensionType)).
		Where(q.DimensionKey.Eq(stats.DimensionKey)).
		Where(q.DimensionValue.Eq(stats.DimensionValue)).
		Where(q.StatHour.Eq(stats.StatHour)).
		First()

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if existing != nil {
		// Update existing record
		stats.ID = existing.ID
		return q.WithContext(ctx).Save(stats)
	}

	// Create new record
	return q.WithContext(ctx).Create(stats)
}

// BatchSaveLabelHourlyStats batch saves label/annotation hourly statistics
func (f *GpuAggregationFacade) BatchSaveLabelHourlyStats(ctx context.Context, stats []*dbmodel.LabelGpuHourlyStats) error {
	if len(stats) == 0 {
		return nil
	}

	// Use transaction for batch insert
	return f.getDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, stat := range stats {
			if err := f.SaveLabelHourlyStats(ctx, stat); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetLabelHourlyStats queries hourly statistics for specific dimension
func (f *GpuAggregationFacade) GetLabelHourlyStats(ctx context.Context, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error) {
	q := f.getDAL().LabelGpuHourlyStats

	result, err := q.WithContext(ctx).
		Where(q.DimensionType.Eq(dimensionType)).
		Where(q.DimensionKey.Eq(dimensionKey)).
		Where(q.DimensionValue.Eq(dimensionValue)).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Order(q.StatHour.Asc()).
		Find()

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*dbmodel.LabelGpuHourlyStats{}, nil
		}
		return nil, err
	}

	return result, nil
}

// ListLabelHourlyStatsByKey queries hourly statistics for all values of specific key
func (f *GpuAggregationFacade) ListLabelHourlyStatsByKey(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error) {
	q := f.getDAL().LabelGpuHourlyStats

	result, err := q.WithContext(ctx).
		Where(q.DimensionType.Eq(dimensionType)).
		Where(q.DimensionKey.Eq(dimensionKey)).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Order(q.DimensionValue.Asc()).
		Order(q.StatHour.Asc()).
		Find()

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*dbmodel.LabelGpuHourlyStats{}, nil
		}
		return nil, err
	}

	return result, nil
}

// ==================== GpuAllocationSnapshot operations implementation ====================

// SaveSnapshot saves GPU allocation snapshot
func (f *GpuAggregationFacade) SaveSnapshot(ctx context.Context, snapshot *dbmodel.GpuAllocationSnapshots) error {
	q := f.getDAL().GpuAllocationSnapshots
	return q.WithContext(ctx).Create(snapshot)
}

// GetLatestSnapshot gets the latest snapshot
func (f *GpuAggregationFacade) GetLatestSnapshot(ctx context.Context) (*dbmodel.GpuAllocationSnapshots, error) {
	q := f.getDAL().GpuAllocationSnapshots

	result, err := q.WithContext(ctx).
		Order(q.SnapshotTime.Desc()).
		First()

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if result.ID == 0 {
		return nil, nil
	}

	return result, nil
}

// ListSnapshots queries snapshots within specified time range
func (f *GpuAggregationFacade) ListSnapshots(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.GpuAllocationSnapshots, error) {
	q := f.getDAL().GpuAllocationSnapshots

	result, err := q.WithContext(ctx).
		Where(q.SnapshotTime.Gte(startTime)).
		Where(q.SnapshotTime.Lte(endTime)).
		Order(q.SnapshotTime.Asc()).
		Find()

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*dbmodel.GpuAllocationSnapshots{}, nil
		}
		return nil, err
	}

	return result, nil
}

// ==================== Data cleanup operations implementation ====================

// CleanupOldSnapshots cleans up snapshots before specified time
func (f *GpuAggregationFacade) CleanupOldSnapshots(ctx context.Context, beforeTime time.Time) (int64, error) {
	q := f.getDAL().GpuAllocationSnapshots

	result, err := q.WithContext(ctx).
		Where(q.SnapshotTime.Lt(beforeTime)).
		Delete()

	if err != nil {
		return 0, err
	}

	return result.RowsAffected, nil
}

// CleanupOldHourlyStats cleans up hourly statistics before specified time
func (f *GpuAggregationFacade) CleanupOldHourlyStats(ctx context.Context, beforeTime time.Time) (int64, error) {
	totalDeleted := int64(0)

	// Clean up cluster statistics
	clusterQ := f.getDAL().ClusterGpuHourlyStats
	clusterResult, err := clusterQ.WithContext(ctx).
		Where(clusterQ.StatHour.Lt(beforeTime)).
		Delete()
	if err != nil {
		return 0, err
	}
	totalDeleted += clusterResult.RowsAffected

	// Clean up namespace statistics
	namespaceQ := f.getDAL().NamespaceGpuHourlyStats
	namespaceResult, err := namespaceQ.WithContext(ctx).
		Where(namespaceQ.StatHour.Lt(beforeTime)).
		Delete()
	if err != nil {
		return totalDeleted, err
	}
	totalDeleted += namespaceResult.RowsAffected

	// Clean up label statistics
	labelQ := f.getDAL().LabelGpuHourlyStats
	labelResult, err := labelQ.WithContext(ctx).
		Where(labelQ.StatHour.Lt(beforeTime)).
		Delete()
	if err != nil {
		return totalDeleted, err
	}
	totalDeleted += labelResult.RowsAffected

	return totalDeleted, nil
}

// ==================== Metadata query operations implementation ====================

// GetDistinctNamespaces gets all distinct namespaces within specified time range
func (f *GpuAggregationFacade) GetDistinctNamespaces(ctx context.Context, startTime, endTime time.Time) ([]string, error) {
	q := f.getDAL().NamespaceGpuHourlyStats

	var namespaces []string
	err := q.WithContext(ctx).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Distinct(q.Namespace).
		Pluck(q.Namespace, &namespaces)

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []string{}, nil
		}
		return nil, err
	}

	return namespaces, nil
}

// GetDistinctDimensionKeys gets all distinct dimension keys within specified time range
func (f *GpuAggregationFacade) GetDistinctDimensionKeys(ctx context.Context, dimensionType string, startTime, endTime time.Time) ([]string, error) {
	q := f.getDAL().LabelGpuHourlyStats

	var keys []string
	err := q.WithContext(ctx).
		Where(q.DimensionType.Eq(dimensionType)).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Distinct(q.DimensionKey).
		Pluck(q.DimensionKey, &keys)

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []string{}, nil
		}
		return nil, err
	}

	return keys, nil
}

// GetDistinctDimensionValues gets all distinct values for a dimension key within specified time range
func (f *GpuAggregationFacade) GetDistinctDimensionValues(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time) ([]string, error) {
	q := f.getDAL().LabelGpuHourlyStats

	var values []string
	err := q.WithContext(ctx).
		Where(q.DimensionType.Eq(dimensionType)).
		Where(q.DimensionKey.Eq(dimensionKey)).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Distinct(q.DimensionValue).
		Pluck(q.DimensionValue, &values)

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []string{}, nil
		}
		return nil, err
	}

	return values, nil
}

// ==================== Pagination query operations implementation ====================

// calculatePagination calculates pagination parameters
func calculatePagination(page, pageSize int, total int64) (offset int, limit int, totalPages int) {
	// Set default values
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20 // Default 20 items per page
	}
	if pageSize > 1000 {
		pageSize = 1000 // Maximum 1000 items
	}

	// Calculate offset and total pages
	offset = (page - 1) * pageSize
	limit = pageSize
	totalPages = int(math.Ceil(float64(total) / float64(pageSize)))

	return offset, limit, totalPages
}

// GetClusterHourlyStatsPaginated queries cluster hourly statistics with pagination
func (f *GpuAggregationFacade) GetClusterHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error) {
	q := f.getDAL().ClusterGpuHourlyStats

	// Query total count
	total, err := q.WithContext(ctx).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Count()
	if err != nil {
		return nil, err
	}

	// Calculate pagination parameters
	offset, limit, totalPages := calculatePagination(opts.Page, opts.PageSize, total)

	// Build query with pagination
	baseQuery := q.WithContext(ctx).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Offset(offset).
		Limit(limit)

	// Apply sorting
	var result []*dbmodel.ClusterGpuHourlyStats
	if opts.OrderBy == "utilization" {
		if opts.OrderDirection == "desc" {
			result, err = baseQuery.Order(q.AvgUtilization.Desc()).Find()
		} else {
			result, err = baseQuery.Order(q.AvgUtilization).Find()
		}
	} else {
		// Default sort by time
		if opts.OrderDirection == "desc" {
			result, err = baseQuery.Order(q.StatHour.Desc()).Find()
		} else {
			result, err = baseQuery.Order(q.StatHour).Find()
		}
	}

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			result = []*dbmodel.ClusterGpuHourlyStats{}
		} else {
			return nil, err
		}
	}

	return &PaginatedResult{
		Total:      total,
		Page:       opts.Page,
		PageSize:   opts.PageSize,
		TotalPages: totalPages,
		Data:       result,
	}, nil
}

// GetNamespaceHourlyStatsPaginated queries hourly statistics for specific namespace with pagination
func (f *GpuAggregationFacade) GetNamespaceHourlyStatsPaginated(ctx context.Context, namespace string, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error) {
	q := f.getDAL().NamespaceGpuHourlyStats

	// Query total count
	total, err := q.WithContext(ctx).
		Where(q.Namespace.Eq(namespace)).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Count()
	if err != nil {
		return nil, err
	}

	// Calculate pagination parameters
	offset, limit, totalPages := calculatePagination(opts.Page, opts.PageSize, total)

	// Build base query with pagination
	baseQuery := q.WithContext(ctx).
		Where(q.Namespace.Eq(namespace)).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Offset(offset).
		Limit(limit)

	// Apply sorting
	var result []*dbmodel.NamespaceGpuHourlyStats
	if opts.OrderBy == "utilization" {
		if opts.OrderDirection == "desc" {
			result, err = baseQuery.Order(q.AvgUtilization.Desc()).Find()
		} else {
			result, err = baseQuery.Order(q.AvgUtilization).Find()
		}
	} else {
		// Default sort by time
		if opts.OrderDirection == "desc" {
			result, err = baseQuery.Order(q.StatHour.Desc()).Find()
		} else {
			result, err = baseQuery.Order(q.StatHour).Find()
		}
	}

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			result = []*dbmodel.NamespaceGpuHourlyStats{}
		} else {
			return nil, err
		}
	}

	return &PaginatedResult{
		Total:      total,
		Page:       opts.Page,
		PageSize:   opts.PageSize,
		TotalPages: totalPages,
		Data:       result,
	}, nil
}

// ListNamespaceHourlyStatsPaginated queries hourly statistics for all namespaces with pagination
func (f *GpuAggregationFacade) ListNamespaceHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error) {
	q := f.getDAL().NamespaceGpuHourlyStats

	// Query total count
	total, err := q.WithContext(ctx).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Count()
	if err != nil {
		return nil, err
	}

	// Calculate pagination parameters
	offset, limit, totalPages := calculatePagination(opts.Page, opts.PageSize, total)

	// Build base query with pagination
	baseQuery := q.WithContext(ctx).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Offset(offset).
		Limit(limit)

	// Apply sorting
	var result []*dbmodel.NamespaceGpuHourlyStats
	if opts.OrderBy == "utilization" {
		if opts.OrderDirection == "desc" {
			result, err = baseQuery.Order(q.AvgUtilization.Desc()).Find()
		} else {
			result, err = baseQuery.Order(q.AvgUtilization).Find()
		}
	} else {
		// Default sort by time
		if opts.OrderDirection == "desc" {
			result, err = baseQuery.Order(q.StatHour.Desc()).Find()
		} else {
			result, err = baseQuery.Order(q.StatHour).Find()
		}
	}

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			result = []*dbmodel.NamespaceGpuHourlyStats{}
		} else {
			return nil, err
		}
	}

	return &PaginatedResult{
		Total:      total,
		Page:       opts.Page,
		PageSize:   opts.PageSize,
		TotalPages: totalPages,
		Data:       result,
	}, nil
}

// GetLabelHourlyStatsPaginated queries hourly statistics for specific dimension with pagination
func (f *GpuAggregationFacade) GetLabelHourlyStatsPaginated(ctx context.Context, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error) {
	q := f.getDAL().LabelGpuHourlyStats

	// Query total count
	total, err := q.WithContext(ctx).
		Where(q.DimensionType.Eq(dimensionType)).
		Where(q.DimensionKey.Eq(dimensionKey)).
		Where(q.DimensionValue.Eq(dimensionValue)).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Count()
	if err != nil {
		return nil, err
	}

	// Calculate pagination parameters
	offset, limit, totalPages := calculatePagination(opts.Page, opts.PageSize, total)

	// Build base query with pagination
	baseQuery := q.WithContext(ctx).
		Where(q.DimensionType.Eq(dimensionType)).
		Where(q.DimensionKey.Eq(dimensionKey)).
		Where(q.DimensionValue.Eq(dimensionValue)).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Offset(offset).
		Limit(limit)

	// Apply sorting
	var result []*dbmodel.LabelGpuHourlyStats
	if opts.OrderBy == "utilization" {
		if opts.OrderDirection == "desc" {
			result, err = baseQuery.Order(q.AvgUtilization.Desc()).Find()
		} else {
			result, err = baseQuery.Order(q.AvgUtilization).Find()
		}
	} else {
		// Default sort by time
		if opts.OrderDirection == "desc" {
			result, err = baseQuery.Order(q.StatHour.Desc()).Find()
		} else {
			result, err = baseQuery.Order(q.StatHour).Find()
		}
	}

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			result = []*dbmodel.LabelGpuHourlyStats{}
		} else {
			return nil, err
		}
	}

	return &PaginatedResult{
		Total:      total,
		Page:       opts.Page,
		PageSize:   opts.PageSize,
		TotalPages: totalPages,
		Data:       result,
	}, nil
}

// ListLabelHourlyStatsByKeyPaginated queries hourly statistics for all values of specific key with pagination
func (f *GpuAggregationFacade) ListLabelHourlyStatsByKeyPaginated(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error) {
	q := f.getDAL().LabelGpuHourlyStats

	// Query total count
	total, err := q.WithContext(ctx).
		Where(q.DimensionType.Eq(dimensionType)).
		Where(q.DimensionKey.Eq(dimensionKey)).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Count()
	if err != nil {
		return nil, err
	}

	// Calculate pagination parameters
	offset, limit, totalPages := calculatePagination(opts.Page, opts.PageSize, total)

	// Build base query with pagination
	baseQuery := q.WithContext(ctx).
		Where(q.DimensionType.Eq(dimensionType)).
		Where(q.DimensionKey.Eq(dimensionKey)).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Offset(offset).
		Limit(limit)

	// Apply sorting
	var result []*dbmodel.LabelGpuHourlyStats
	if opts.OrderBy == "utilization" {
		if opts.OrderDirection == "desc" {
			result, err = baseQuery.Order(q.AvgUtilization.Desc()).Find()
		} else {
			result, err = baseQuery.Order(q.AvgUtilization).Find()
		}
	} else {
		// Default sort by time
		if opts.OrderDirection == "desc" {
			result, err = baseQuery.Order(q.StatHour.Desc()).Find()
		} else {
			result, err = baseQuery.Order(q.StatHour).Find()
		}
	}

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			result = []*dbmodel.LabelGpuHourlyStats{}
		} else {
			return nil, err
		}
	}

	return &PaginatedResult{
		Total:      total,
		Page:       opts.Page,
		PageSize:   opts.PageSize,
		TotalPages: totalPages,
		Data:       result,
	}, nil
}

// ==================== WorkloadGpuHourlyStats operations implementation ====================

// SaveWorkloadHourlyStats saves workload hourly statistics
func (f *GpuAggregationFacade) SaveWorkloadHourlyStats(ctx context.Context, stats *dbmodel.WorkloadGpuHourlyStats) error {
	q := f.getDAL().WorkloadGpuHourlyStats

	// Check if already exists
	existing, err := q.WithContext(ctx).
		Where(q.ClusterName.Eq(stats.ClusterName)).
		Where(q.Namespace.Eq(stats.Namespace)).
		Where(q.WorkloadName.Eq(stats.WorkloadName)).
		Where(q.StatHour.Eq(stats.StatHour)).
		First()

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if existing != nil {
		// Update existing record
		stats.ID = existing.ID
		return q.WithContext(ctx).Save(stats)
	}

	// Create new record
	return q.WithContext(ctx).Create(stats)
}

// BatchSaveWorkloadHourlyStats batch saves workload hourly statistics
func (f *GpuAggregationFacade) BatchSaveWorkloadHourlyStats(ctx context.Context, stats []*dbmodel.WorkloadGpuHourlyStats) error {
	if len(stats) == 0 {
		return nil
	}

	// Use transaction for batch insert
	return f.getDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, stat := range stats {
			if err := f.SaveWorkloadHourlyStats(ctx, stat); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetWorkloadHourlyStats queries hourly statistics for specific workload
func (f *GpuAggregationFacade) GetWorkloadHourlyStats(ctx context.Context, namespace, workloadName string, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) {
	q := f.getDAL().WorkloadGpuHourlyStats

	result, err := q.WithContext(ctx).
		Where(q.Namespace.Eq(namespace)).
		Where(q.WorkloadName.Eq(workloadName)).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Order(q.StatHour.Asc()).
		Find()

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*dbmodel.WorkloadGpuHourlyStats{}, nil
		}
		return nil, err
	}

	return result, nil
}

// ListWorkloadHourlyStats queries all workload hourly statistics
func (f *GpuAggregationFacade) ListWorkloadHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) {
	q := f.getDAL().WorkloadGpuHourlyStats

	result, err := q.WithContext(ctx).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Order(q.StatHour.Asc()).
		Find()

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*dbmodel.WorkloadGpuHourlyStats{}, nil
		}
		return nil, err
	}

	return result, nil
}

// ListWorkloadHourlyStatsByNamespace queries workload hourly statistics by namespace
func (f *GpuAggregationFacade) ListWorkloadHourlyStatsByNamespace(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) {
	q := f.getDAL().WorkloadGpuHourlyStats

	result, err := q.WithContext(ctx).
		Where(q.Namespace.Eq(namespace)).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime)).
		Order(q.StatHour.Asc()).
		Find()

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*dbmodel.WorkloadGpuHourlyStats{}, nil
		}
		return nil, err
	}

	return result, nil
}

// GetWorkloadHourlyStatsPaginated queries hourly statistics for workload with filters and pagination
func (f *GpuAggregationFacade) GetWorkloadHourlyStatsPaginated(ctx context.Context, namespace, workloadName, workloadType string, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error) {
	q := f.getDAL().WorkloadGpuHourlyStats

	// Build base query with filters
	query := q.WithContext(ctx).
		Where(q.StatHour.Gte(startTime)).
		Where(q.StatHour.Lte(endTime))

	if namespace != "" {
		query = query.Where(q.Namespace.Eq(namespace))
	}
	if workloadName != "" {
		query = query.Where(q.WorkloadName.Eq(workloadName))
	}
	if workloadType != "" {
		query = query.Where(q.WorkloadType.Eq(workloadType))
	}

	// Query total count
	total, err := query.Count()
	if err != nil {
		return nil, err
	}

	// Calculate pagination parameters
	offset, limit, totalPages := calculatePagination(opts.Page, opts.PageSize, total)

	// Add pagination
	query = query.Offset(offset).Limit(limit)

	// Apply sorting
	var result []*dbmodel.WorkloadGpuHourlyStats
	if opts.OrderBy == "utilization" {
		if opts.OrderDirection == "desc" {
			result, err = query.Order(q.AvgUtilization.Desc()).Find()
		} else {
			result, err = query.Order(q.AvgUtilization).Find()
		}
	} else {
		// Default sort by time
		if opts.OrderDirection == "desc" {
			result, err = query.Order(q.StatHour.Desc()).Find()
		} else {
			result, err = query.Order(q.StatHour).Find()
		}
	}

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			result = []*dbmodel.WorkloadGpuHourlyStats{}
		} else {
			return nil, err
		}
	}

	return &PaginatedResult{
		Total:      total,
		Page:       opts.Page,
		PageSize:   opts.PageSize,
		TotalPages: totalPages,
		Data:       result,
	}, nil
}

// ListWorkloadHourlyStatsPaginated queries all workload hourly statistics with pagination
func (f *GpuAggregationFacade) ListWorkloadHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error) {
	return f.GetWorkloadHourlyStatsPaginated(ctx, "", "", "", startTime, endTime, opts)
}

// ListWorkloadHourlyStatsByNamespacePaginated queries workload hourly statistics by namespace with pagination
func (f *GpuAggregationFacade) ListWorkloadHourlyStatsByNamespacePaginated(ctx context.Context, namespace string, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error) {
	return f.GetWorkloadHourlyStatsPaginated(ctx, namespace, "", "", startTime, endTime, opts)
}
