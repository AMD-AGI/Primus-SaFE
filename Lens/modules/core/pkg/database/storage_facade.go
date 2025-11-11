package database

import (
	"context"
	"errors"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// StorageFacadeInterface defines the database operation interface for Storage
type StorageFacadeInterface interface {
	// Storage operations
	GetStorageByKindAndName(ctx context.Context, kind, name string) (*model.Storage, error)
	CreateStorage(ctx context.Context, storage *model.Storage) error
	UpdateStorage(ctx context.Context, storage *model.Storage) error
	ListStorage(ctx context.Context, pageNum, pageSize int) ([]*model.Storage, int, error)

	// WithCluster method
	WithCluster(clusterName string) StorageFacadeInterface
}

// StorageFacade implements StorageFacadeInterface
type StorageFacade struct {
	BaseFacade
}

// NewStorageFacade creates a new StorageFacade instance
func NewStorageFacade() StorageFacadeInterface {
	return &StorageFacade{}
}

func (f *StorageFacade) WithCluster(clusterName string) StorageFacadeInterface {
	return &StorageFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// Storage operation implementations
func (f *StorageFacade) GetStorageByKindAndName(ctx context.Context, kind, name string) (*model.Storage, error) {
	q := f.getDAL().Storage
	result, err := q.WithContext(ctx).Where(q.Kind.Eq(kind)).Where(q.Name.Eq(name)).First()
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

func (f *StorageFacade) CreateStorage(ctx context.Context, storage *model.Storage) error {
	return f.getDAL().Storage.WithContext(ctx).Create(storage)
}

func (f *StorageFacade) UpdateStorage(ctx context.Context, storage *model.Storage) error {
	return f.getDAL().Storage.WithContext(ctx).Save(storage)
}

func (f *StorageFacade) ListStorage(ctx context.Context, pageNum, pageSize int) ([]*model.Storage, int, error) {
	q := f.getDAL().Storage
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
	var storages []*model.Storage
	err = gormDB.Find(&storages).Error
	if err != nil {
		return nil, 0, err
	}
	return storages, int(count), nil
}
