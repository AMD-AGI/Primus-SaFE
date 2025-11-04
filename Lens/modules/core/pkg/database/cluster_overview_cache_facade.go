package database

import (
	"context"
	"errors"

	"github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// ClusterOverviewCacheFacadeInterface defines the database operation interface for ClusterOverviewCache
type ClusterOverviewCacheFacadeInterface interface {
	// ClusterOverviewCache operations
	GetClusterOverviewCache(ctx context.Context) (*model.ClusterOverviewCache, error)
	CreateClusterOverviewCache(ctx context.Context, cache *model.ClusterOverviewCache) error
	UpdateClusterOverviewCache(ctx context.Context, cache *model.ClusterOverviewCache) error
	ListClusterOverviewCache(ctx context.Context, pageNum, pageSize int) ([]*model.ClusterOverviewCache, int, error)

	// WithCluster method
	WithCluster(clusterName string) ClusterOverviewCacheFacadeInterface
}

// ClusterOverviewCacheFacade implements ClusterOverviewCacheFacadeInterface
type ClusterOverviewCacheFacade struct {
	BaseFacade
}

// NewClusterOverviewCacheFacade creates a new ClusterOverviewCacheFacade instance
func NewClusterOverviewCacheFacade() ClusterOverviewCacheFacadeInterface {
	return &ClusterOverviewCacheFacade{}
}

func (f *ClusterOverviewCacheFacade) WithCluster(clusterName string) ClusterOverviewCacheFacadeInterface {
	return &ClusterOverviewCacheFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// ClusterOverviewCache operation implementations
func (f *ClusterOverviewCacheFacade) GetClusterOverviewCache(ctx context.Context) (*model.ClusterOverviewCache, error) {
	q := f.getDAL().ClusterOverviewCache
	result, err := q.WithContext(ctx).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

func (f *ClusterOverviewCacheFacade) CreateClusterOverviewCache(ctx context.Context, cache *model.ClusterOverviewCache) error {
	return f.getDAL().ClusterOverviewCache.WithContext(ctx).Create(cache)
}

func (f *ClusterOverviewCacheFacade) UpdateClusterOverviewCache(ctx context.Context, cache *model.ClusterOverviewCache) error {
	return f.getDAL().ClusterOverviewCache.WithContext(ctx).Save(cache)
}

func (f *ClusterOverviewCacheFacade) ListClusterOverviewCache(ctx context.Context, pageNum, pageSize int) ([]*model.ClusterOverviewCache, int, error) {
	q := f.getDAL().ClusterOverviewCache
	query := q.WithContext(ctx)
	count, err := query.Count()
	if err != nil {
		return nil, 0, err
	}
	gormDB := query.UnderlyingDB()
	gormDB = gormDB.Order("created_at desc")

	if pageSize > 0 {
		gormDB = gormDB.Limit(pageSize)
	}
	if pageNum > 0 {
		gormDB = gormDB.Offset((pageNum - 1) * pageSize)
	}
	var caches []*model.ClusterOverviewCache
	err = gormDB.Find(&caches).Error
	if err != nil {
		return nil, 0, err
	}
	return caches, int(count), nil
}
