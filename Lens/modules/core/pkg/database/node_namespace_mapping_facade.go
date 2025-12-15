package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// NodeNamespaceMappingFacadeInterface defines the database operation interface for NodeNamespaceMapping
type NodeNamespaceMappingFacadeInterface interface {
	// NodeNamespaceMapping operations
	Create(ctx context.Context, mapping *model.NodeNamespaceMapping) error
	Update(ctx context.Context, mapping *model.NodeNamespaceMapping) error
	Delete(ctx context.Context, id int32) error
	GetByNodeAndNamespace(ctx context.Context, nodeID int32, namespaceID int64) (*model.NodeNamespaceMapping, error)
	GetByNodeName(ctx context.Context, nodeName string) ([]*model.NodeNamespaceMapping, error)
	GetByNamespaceName(ctx context.Context, namespaceName string) ([]*model.NodeNamespaceMapping, error)
	ListActiveByNamespaceID(ctx context.Context, namespaceID int64) ([]*model.NodeNamespaceMapping, error)
	ListActiveByNamespaceName(ctx context.Context, namespaceName string) ([]*model.NodeNamespaceMapping, error)
	SoftDelete(ctx context.Context, id int32) error

	// NodeNamespaceMappingHistory operations
	CreateHistory(ctx context.Context, history *model.NodeNamespaceMappingHistory) error
	GetLatestHistoryByNodeAndNamespace(ctx context.Context, nodeID int32, namespaceID int64) (*model.NodeNamespaceMappingHistory, error)
	UpdateHistoryRecordEnd(ctx context.Context, historyID int32, recordEnd time.Time) error
	ListHistoryByNamespaceAtTime(ctx context.Context, namespaceID int64, atTime time.Time) ([]*model.NodeNamespaceMappingHistory, error)
	ListHistoryByNamespaceNameAtTime(ctx context.Context, namespaceName string, atTime time.Time) ([]*model.NodeNamespaceMappingHistory, error)

	// WithCluster method
	WithCluster(clusterName string) NodeNamespaceMappingFacadeInterface
}

// NodeNamespaceMappingFacade implements NodeNamespaceMappingFacadeInterface
type NodeNamespaceMappingFacade struct {
	BaseFacade
}

// NewNodeNamespaceMappingFacade creates a new NodeNamespaceMappingFacade instance
func NewNodeNamespaceMappingFacade() NodeNamespaceMappingFacadeInterface {
	return &NodeNamespaceMappingFacade{}
}

func (f *NodeNamespaceMappingFacade) WithCluster(clusterName string) NodeNamespaceMappingFacadeInterface {
	return &NodeNamespaceMappingFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// Create creates a new node-namespace mapping
func (f *NodeNamespaceMappingFacade) Create(ctx context.Context, mapping *model.NodeNamespaceMapping) error {
	return f.getDAL().NodeNamespaceMapping.WithContext(ctx).Create(mapping)
}

// Update updates an existing node-namespace mapping
func (f *NodeNamespaceMappingFacade) Update(ctx context.Context, mapping *model.NodeNamespaceMapping) error {
	return f.getDAL().NodeNamespaceMapping.WithContext(ctx).Save(mapping)
}

// Delete hard deletes a node-namespace mapping
func (f *NodeNamespaceMappingFacade) Delete(ctx context.Context, id int32) error {
	q := f.getDAL().NodeNamespaceMapping
	_, err := q.WithContext(ctx).Where(q.ID.Eq(id)).Delete()
	return err
}

// GetByNodeAndNamespace gets a mapping by node ID and namespace ID
func (f *NodeNamespaceMappingFacade) GetByNodeAndNamespace(ctx context.Context, nodeID int32, namespaceID int64) (*model.NodeNamespaceMapping, error) {
	q := f.getDAL().NodeNamespaceMapping
	result, err := q.WithContext(ctx).
		Where(q.NodeID.Eq(nodeID)).
		Where(q.NamespaceID.Eq(namespaceID)).
		First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

// GetByNodeName gets all mappings for a node by name
func (f *NodeNamespaceMappingFacade) GetByNodeName(ctx context.Context, nodeName string) ([]*model.NodeNamespaceMapping, error) {
	q := f.getDAL().NodeNamespaceMapping
	results, err := q.WithContext(ctx).
		Where(q.NodeName.Eq(nodeName)).
		Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return results, nil
}

// GetByNamespaceName gets all mappings for a namespace by name
func (f *NodeNamespaceMappingFacade) GetByNamespaceName(ctx context.Context, namespaceName string) ([]*model.NodeNamespaceMapping, error) {
	q := f.getDAL().NodeNamespaceMapping
	results, err := q.WithContext(ctx).
		Where(q.NamespaceName.Eq(namespaceName)).
		Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return results, nil
}

// ListActiveByNamespaceID lists all active (not soft deleted) mappings for a namespace
func (f *NodeNamespaceMappingFacade) ListActiveByNamespaceID(ctx context.Context, namespaceID int64) ([]*model.NodeNamespaceMapping, error) {
	q := f.getDAL().NodeNamespaceMapping
	results, err := q.WithContext(ctx).
		Where(q.NamespaceID.Eq(namespaceID)).
		Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return results, nil
}

// ListActiveByNamespaceName lists all active (not soft deleted) mappings for a namespace by name
func (f *NodeNamespaceMappingFacade) ListActiveByNamespaceName(ctx context.Context, namespaceName string) ([]*model.NodeNamespaceMapping, error) {
	q := f.getDAL().NodeNamespaceMapping
	results, err := q.WithContext(ctx).
		Where(q.NamespaceName.Eq(namespaceName)).
		Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return results, nil
}

// SoftDelete soft deletes a node-namespace mapping
func (f *NodeNamespaceMappingFacade) SoftDelete(ctx context.Context, id int32) error {
	q := f.getDAL().NodeNamespaceMapping
	_, err := q.WithContext(ctx).Where(q.ID.Eq(id)).Delete()
	return err
}

// CreateHistory creates a new history record
func (f *NodeNamespaceMappingFacade) CreateHistory(ctx context.Context, history *model.NodeNamespaceMappingHistory) error {
	return f.getDAL().NodeNamespaceMappingHistory.WithContext(ctx).Create(history)
}

// GetLatestHistoryByNodeAndNamespace gets the latest history record for a node-namespace pair
func (f *NodeNamespaceMappingFacade) GetLatestHistoryByNodeAndNamespace(ctx context.Context, nodeID int32, namespaceID int64) (*model.NodeNamespaceMappingHistory, error) {
	q := f.getDAL().NodeNamespaceMappingHistory
	result, err := q.WithContext(ctx).
		Where(q.NodeID.Eq(nodeID)).
		Where(q.NamespaceID.Eq(namespaceID)).
		Order(q.RecordStart.Desc()).
		First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

// UpdateHistoryRecordEnd updates the record_end time of a history record
func (f *NodeNamespaceMappingFacade) UpdateHistoryRecordEnd(ctx context.Context, historyID int32, recordEnd time.Time) error {
	q := f.getDAL().NodeNamespaceMappingHistory
	_, err := q.WithContext(ctx).
		Where(q.ID.Eq(historyID)).
		Update(q.RecordEnd, recordEnd)
	return err
}

// ListHistoryByNamespaceAtTime lists all history records for a namespace at a specific time
func (f *NodeNamespaceMappingFacade) ListHistoryByNamespaceAtTime(ctx context.Context, namespaceID int64, atTime time.Time) ([]*model.NodeNamespaceMappingHistory, error) {
	q := f.getDAL().NodeNamespaceMappingHistory
	// Query: record_start <= atTime AND (record_end IS NULL OR record_end > atTime)
	results, err := q.WithContext(ctx).
		Where(q.NamespaceID.Eq(namespaceID)).
		Where(q.RecordStart.Lte(atTime)).
		Where(q.WithContext(ctx).Or(
			q.RecordEnd.IsNull(),
			q.RecordEnd.Gt(atTime),
		)).
		Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return results, nil
}

// ListHistoryByNamespaceNameAtTime lists all history records for a namespace by name at a specific time
func (f *NodeNamespaceMappingFacade) ListHistoryByNamespaceNameAtTime(ctx context.Context, namespaceName string, atTime time.Time) ([]*model.NodeNamespaceMappingHistory, error) {
	q := f.getDAL().NodeNamespaceMappingHistory
	// Query: record_start <= atTime AND (record_end IS NULL OR record_end > atTime)
	results, err := q.WithContext(ctx).
		Where(q.NamespaceName.Eq(namespaceName)).
		Where(q.RecordStart.Lte(atTime)).
		Where(q.WithContext(ctx).Or(
			q.RecordEnd.IsNull(),
			q.RecordEnd.Gt(atTime),
		)).
		Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return results, nil
}

