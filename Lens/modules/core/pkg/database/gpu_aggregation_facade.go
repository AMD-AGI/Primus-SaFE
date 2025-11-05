package database

import (
	"context"
	"errors"
	"time"

	dbmodel "github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// GpuAggregationFacadeInterface 定义GPU聚合数据库操作接口
type GpuAggregationFacadeInterface interface {
	// ClusterGpuHourlyStats 操作
	SaveClusterHourlyStats(ctx context.Context, stats *dbmodel.ClusterGpuHourlyStats) error
	BatchSaveClusterHourlyStats(ctx context.Context, stats []*dbmodel.ClusterGpuHourlyStats) error
	GetClusterHourlyStats(ctx context.Context, clusterName string, startTime, endTime time.Time) ([]*dbmodel.ClusterGpuHourlyStats, error)

	// NamespaceGpuHourlyStats 操作
	SaveNamespaceHourlyStats(ctx context.Context, stats *dbmodel.NamespaceGpuHourlyStats) error
	BatchSaveNamespaceHourlyStats(ctx context.Context, stats []*dbmodel.NamespaceGpuHourlyStats) error
	GetNamespaceHourlyStats(ctx context.Context, clusterName string, namespace string, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error)
	ListNamespaceHourlyStats(ctx context.Context, clusterName string, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error)

	// LabelGpuHourlyStats 操作
	SaveLabelHourlyStats(ctx context.Context, stats *dbmodel.LabelGpuHourlyStats) error
	BatchSaveLabelHourlyStats(ctx context.Context, stats []*dbmodel.LabelGpuHourlyStats) error
	GetLabelHourlyStats(ctx context.Context, clusterName string, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error)
	ListLabelHourlyStatsByKey(ctx context.Context, clusterName string, dimensionType, dimensionKey string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error)

	// GpuAllocationSnapshot 操作
	SaveSnapshot(ctx context.Context, snapshot *dbmodel.GpuAllocationSnapshots) error
	GetLatestSnapshot(ctx context.Context, clusterName string) (*dbmodel.GpuAllocationSnapshots, error)
	ListSnapshots(ctx context.Context, clusterName string, startTime, endTime time.Time) ([]*dbmodel.GpuAllocationSnapshots, error)

	// 数据清理
	CleanupOldSnapshots(ctx context.Context, beforeTime time.Time) (int64, error)
	CleanupOldHourlyStats(ctx context.Context, beforeTime time.Time) (int64, error)

	// WithCluster 方法
	WithCluster(clusterName string) GpuAggregationFacadeInterface
}

// GpuAggregationFacade 实现 GpuAggregationFacadeInterface
type GpuAggregationFacade struct {
	BaseFacade
}

// NewGpuAggregationFacade 创建新的 GpuAggregationFacade 实例
func NewGpuAggregationFacade() GpuAggregationFacadeInterface {
	return &GpuAggregationFacade{}
}

func (f *GpuAggregationFacade) WithCluster(clusterName string) GpuAggregationFacadeInterface {
	return &GpuAggregationFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// ==================== ClusterGpuHourlyStats 操作实现 ====================

// SaveClusterHourlyStats 保存集群小时统计（使用 ON CONFLICT 更新）
func (f *GpuAggregationFacade) SaveClusterHourlyStats(ctx context.Context, stats *dbmodel.ClusterGpuHourlyStats) error {
	q := f.getDAL().ClusterGpuHourlyStats
	
	// 检查是否已存在
	existing, err := q.WithContext(ctx).
		Where(q.ClusterName.Eq(stats.ClusterName)).
		Where(q.StatHour.Eq(stats.StatHour)).
		First()
	
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	
	if existing != nil {
		// 更新现有记录
		stats.ID = existing.ID
		return q.WithContext(ctx).Save(stats)
	}
	
	// 创建新记录
	return q.WithContext(ctx).Create(stats)
}

// BatchSaveClusterHourlyStats 批量保存集群小时统计
func (f *GpuAggregationFacade) BatchSaveClusterHourlyStats(ctx context.Context, stats []*dbmodel.ClusterGpuHourlyStats) error {
	if len(stats) == 0 {
		return nil
	}
	
	// 使用事务批量插入
	return f.getDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, stat := range stats {
			if err := f.SaveClusterHourlyStats(ctx, stat); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetClusterHourlyStats 查询集群小时统计
func (f *GpuAggregationFacade) GetClusterHourlyStats(ctx context.Context, clusterName string, startTime, endTime time.Time) ([]*dbmodel.ClusterGpuHourlyStats, error) {
	q := f.getDAL().ClusterGpuHourlyStats
	
	result, err := q.WithContext(ctx).
		Where(q.ClusterName.Eq(clusterName)).
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

// ==================== NamespaceGpuHourlyStats 操作实现 ====================

// SaveNamespaceHourlyStats 保存 namespace 小时统计
func (f *GpuAggregationFacade) SaveNamespaceHourlyStats(ctx context.Context, stats *dbmodel.NamespaceGpuHourlyStats) error {
	q := f.getDAL().NamespaceGpuHourlyStats
	
	// 检查是否已存在
	existing, err := q.WithContext(ctx).
		Where(q.ClusterName.Eq(stats.ClusterName)).
		Where(q.Namespace.Eq(stats.Namespace)).
		Where(q.StatHour.Eq(stats.StatHour)).
		First()
	
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	
	if existing != nil {
		// 更新现有记录
		stats.ID = existing.ID
		return q.WithContext(ctx).Save(stats)
	}
	
	// 创建新记录
	return q.WithContext(ctx).Create(stats)
}

// BatchSaveNamespaceHourlyStats 批量保存 namespace 小时统计
func (f *GpuAggregationFacade) BatchSaveNamespaceHourlyStats(ctx context.Context, stats []*dbmodel.NamespaceGpuHourlyStats) error {
	if len(stats) == 0 {
		return nil
	}
	
	// 使用事务批量插入
	return f.getDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, stat := range stats {
			if err := f.SaveNamespaceHourlyStats(ctx, stat); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetNamespaceHourlyStats 查询特定 namespace 的小时统计
func (f *GpuAggregationFacade) GetNamespaceHourlyStats(ctx context.Context, clusterName string, namespace string, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error) {
	q := f.getDAL().NamespaceGpuHourlyStats
	
	result, err := q.WithContext(ctx).
		Where(q.ClusterName.Eq(clusterName)).
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

// ListNamespaceHourlyStats 查询所有 namespace 的小时统计
func (f *GpuAggregationFacade) ListNamespaceHourlyStats(ctx context.Context, clusterName string, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error) {
	q := f.getDAL().NamespaceGpuHourlyStats
	
	result, err := q.WithContext(ctx).
		Where(q.ClusterName.Eq(clusterName)).
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

// ==================== LabelGpuHourlyStats 操作实现 ====================

// SaveLabelHourlyStats 保存 label/annotation 小时统计
func (f *GpuAggregationFacade) SaveLabelHourlyStats(ctx context.Context, stats *dbmodel.LabelGpuHourlyStats) error {
	q := f.getDAL().LabelGpuHourlyStats
	
	// 检查是否已存在
	existing, err := q.WithContext(ctx).
		Where(q.ClusterName.Eq(stats.ClusterName)).
		Where(q.DimensionType.Eq(stats.DimensionType)).
		Where(q.DimensionKey.Eq(stats.DimensionKey)).
		Where(q.DimensionValue.Eq(stats.DimensionValue)).
		Where(q.StatHour.Eq(stats.StatHour)).
		First()
	
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	
	if existing != nil {
		// 更新现有记录
		stats.ID = existing.ID
		return q.WithContext(ctx).Save(stats)
	}
	
	// 创建新记录
	return q.WithContext(ctx).Create(stats)
}

// BatchSaveLabelHourlyStats 批量保存 label/annotation 小时统计
func (f *GpuAggregationFacade) BatchSaveLabelHourlyStats(ctx context.Context, stats []*dbmodel.LabelGpuHourlyStats) error {
	if len(stats) == 0 {
		return nil
	}
	
	// 使用事务批量插入
	return f.getDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, stat := range stats {
			if err := f.SaveLabelHourlyStats(ctx, stat); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetLabelHourlyStats 查询特定维度的小时统计
func (f *GpuAggregationFacade) GetLabelHourlyStats(ctx context.Context, clusterName string, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error) {
	q := f.getDAL().LabelGpuHourlyStats
	
	result, err := q.WithContext(ctx).
		Where(q.ClusterName.Eq(clusterName)).
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

// ListLabelHourlyStatsByKey 查询特定 key 的所有 value 的小时统计
func (f *GpuAggregationFacade) ListLabelHourlyStatsByKey(ctx context.Context, clusterName string, dimensionType, dimensionKey string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error) {
	q := f.getDAL().LabelGpuHourlyStats
	
	result, err := q.WithContext(ctx).
		Where(q.ClusterName.Eq(clusterName)).
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

// ==================== GpuAllocationSnapshot 操作实现 ====================

// SaveSnapshot 保存 GPU 分配快照
func (f *GpuAggregationFacade) SaveSnapshot(ctx context.Context, snapshot *dbmodel.GpuAllocationSnapshots) error {
	q := f.getDAL().GpuAllocationSnapshots
	return q.WithContext(ctx).Create(snapshot)
}

// GetLatestSnapshot 获取最新的快照
func (f *GpuAggregationFacade) GetLatestSnapshot(ctx context.Context, clusterName string) (*dbmodel.GpuAllocationSnapshots, error) {
	q := f.getDAL().GpuAllocationSnapshots
	
	result, err := q.WithContext(ctx).
		Where(q.ClusterName.Eq(clusterName)).
		Order(q.SnapshotTime.Desc()).
		First()
	
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	
	return result, nil
}

// ListSnapshots 查询指定时间范围的快照
func (f *GpuAggregationFacade) ListSnapshots(ctx context.Context, clusterName string, startTime, endTime time.Time) ([]*dbmodel.GpuAllocationSnapshots, error) {
	q := f.getDAL().GpuAllocationSnapshots
	
	result, err := q.WithContext(ctx).
		Where(q.ClusterName.Eq(clusterName)).
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

// ==================== 数据清理操作实现 ====================

// CleanupOldSnapshots 清理指定时间之前的快照
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

// CleanupOldHourlyStats 清理指定时间之前的小时统计
func (f *GpuAggregationFacade) CleanupOldHourlyStats(ctx context.Context, beforeTime time.Time) (int64, error) {
	totalDeleted := int64(0)
	
	// 清理集群统计
	clusterQ := f.getDAL().ClusterGpuHourlyStats
	clusterResult, err := clusterQ.WithContext(ctx).
		Where(clusterQ.StatHour.Lt(beforeTime)).
		Delete()
	if err != nil {
		return 0, err
	}
	totalDeleted += clusterResult.RowsAffected
	
	// 清理 namespace 统计
	namespaceQ := f.getDAL().NamespaceGpuHourlyStats
	namespaceResult, err := namespaceQ.WithContext(ctx).
		Where(namespaceQ.StatHour.Lt(beforeTime)).
		Delete()
	if err != nil {
		return totalDeleted, err
	}
	totalDeleted += namespaceResult.RowsAffected
	
	// 清理 label 统计
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

