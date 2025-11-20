package database

import (
	"context"
	"errors"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// NamespaceInfoFacadeInterface defines the NamespaceInfo Facade interface
type NamespaceInfoFacadeInterface interface {
	// GetByName retrieves a namespace info by name
	GetByName(ctx context.Context, name string) (*model.NamespaceInfo, error)
	// Create creates a new namespace info
	Create(ctx context.Context, namespaceInfo *model.NamespaceInfo) error
	// Update updates an existing namespace info
	Update(ctx context.Context, namespaceInfo *model.NamespaceInfo) error
	// Delete deletes a namespace info by name
	DeleteByName(ctx context.Context, name string) error
	// List lists all namespace infos
	List(ctx context.Context) ([]*model.NamespaceInfo, error)
	// WithCluster returns a new facade instance for the specified cluster
	WithCluster(clusterName string) NamespaceInfoFacadeInterface
}

// NamespaceInfoFacade implements NamespaceInfoFacadeInterface
type NamespaceInfoFacade struct {
	BaseFacade
}

// NewNamespaceInfoFacade creates a new NamespaceInfo Facade
func NewNamespaceInfoFacade() NamespaceInfoFacadeInterface {
	return &NamespaceInfoFacade{}
}

// GetByName retrieves a namespace info by name
func (f *NamespaceInfoFacade) GetByName(ctx context.Context, name string) (*model.NamespaceInfo, error) {
	q := f.getDAL().NamespaceInfo
	item, err := q.WithContext(ctx).Where(q.Name.Eq(name)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return item, nil
}

// Create creates a new namespace info
func (f *NamespaceInfoFacade) Create(ctx context.Context, namespaceInfo *model.NamespaceInfo) error {
	q := f.getDAL().NamespaceInfo
	return q.WithContext(ctx).Create(namespaceInfo)
}

// Update updates an existing namespace info
func (f *NamespaceInfoFacade) Update(ctx context.Context, namespaceInfo *model.NamespaceInfo) error {
	q := f.getDAL().NamespaceInfo
	return q.WithContext(ctx).Save(namespaceInfo)
}

// DeleteByName deletes a namespace info by name
func (f *NamespaceInfoFacade) DeleteByName(ctx context.Context, name string) error {
	q := f.getDAL().NamespaceInfo
	_, err := q.WithContext(ctx).Where(q.Name.Eq(name)).Delete()
	return err
}

// List lists all namespace infos
func (f *NamespaceInfoFacade) List(ctx context.Context) ([]*model.NamespaceInfo, error) {
	q := f.getDAL().NamespaceInfo
	return q.WithContext(ctx).Find()
}

// WithCluster returns a new facade instance for the specified cluster
func (f *NamespaceInfoFacade) WithCluster(clusterName string) NamespaceInfoFacadeInterface {
	return &NamespaceInfoFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

