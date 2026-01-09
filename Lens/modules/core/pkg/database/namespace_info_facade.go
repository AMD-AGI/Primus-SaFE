// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// NamespaceInfoFacadeInterface defines the NamespaceInfo Facade interface
type NamespaceInfoFacadeInterface interface {
	// GetByName retrieves a namespace info by name
	GetByName(ctx context.Context, name string) (*model.NamespaceInfo, error)
	// GetByNameIncludingDeleted retrieves a namespace info by name including soft deleted records
	GetByNameIncludingDeleted(ctx context.Context, name string) (*model.NamespaceInfo, error)
	// Create creates a new namespace info
	Create(ctx context.Context, namespaceInfo *model.NamespaceInfo) error
	// Update updates an existing namespace info
	Update(ctx context.Context, namespaceInfo *model.NamespaceInfo) error
	// Delete deletes a namespace info by name (soft delete)
	DeleteByName(ctx context.Context, name string) error
	// List lists all namespace infos (excluding soft deleted)
	List(ctx context.Context) ([]*model.NamespaceInfo, error)
	// ListAllIncludingDeleted lists all namespace infos including soft deleted ones
	ListAllIncludingDeleted(ctx context.Context) ([]*model.NamespaceInfo, error)
	// Recover recovers a soft deleted namespace info record
	Recover(ctx context.Context, name string, gpuModel string, gpuResource int32) error
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

// GetByNameIncludingDeleted retrieves a namespace info by name including soft deleted records
func (f *NamespaceInfoFacade) GetByNameIncludingDeleted(ctx context.Context, name string) (*model.NamespaceInfo, error) {
	db := f.getDB()
	if db == nil {
		return nil, nil
	}

	var result model.NamespaceInfo
	err := db.WithContext(ctx).Unscoped().Where("name = ?", name).First(&result).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
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

// List lists all namespace infos (excluding soft deleted)
func (f *NamespaceInfoFacade) List(ctx context.Context) ([]*model.NamespaceInfo, error) {
	q := f.getDAL().NamespaceInfo
	return q.WithContext(ctx).Find()
}

// ListAllIncludingDeleted lists all namespace infos including soft deleted ones
func (f *NamespaceInfoFacade) ListAllIncludingDeleted(ctx context.Context) ([]*model.NamespaceInfo, error) {
	db := f.getDB()
	if db == nil {
		return nil, nil
	}

	var results []*model.NamespaceInfo
	// Unscoped() includes soft deleted records
	err := db.WithContext(ctx).Unscoped().Find(&results).Error
	if err != nil {
		return nil, err
	}
	return results, nil
}

// Recover recovers a soft deleted namespace info record
func (f *NamespaceInfoFacade) Recover(ctx context.Context, name string, gpuModel string, gpuResource int32) error {
	db := f.getDB()
	if db == nil {
		return errors.New("database connection not available")
	}

	now := time.Now()

	// Use Unscoped to update soft deleted record and clear deleted_at
	return db.WithContext(ctx).Unscoped().
		Model(&model.NamespaceInfo{}).
		Where("name = ?", name).
		Updates(map[string]interface{}{
			"gpu_model":    gpuModel,
			"gpu_resource": gpuResource,
			"deleted_at":   nil,
			"updated_at":   now,
		}).Error
}

// WithCluster returns a new facade instance for the specified cluster
func (f *NamespaceInfoFacade) WithCluster(clusterName string) NamespaceInfoFacadeInterface {
	return &NamespaceInfoFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}
