// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/filter"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// NodeFacadeInterface defines the database operation interface for Node
type NodeFacadeInterface interface {
	// Node operations
	CreateNode(ctx context.Context, node *model.Node) error
	UpdateNode(ctx context.Context, node *model.Node) error
	UpdateNodeSelectedFields(ctx context.Context, id int32, fields map[string]interface{}) error
	GetNodeByName(ctx context.Context, name string) (*model.Node, error)
	SearchNode(ctx context.Context, f filter.NodeFilter) ([]*model.Node, int, error)
	ListGpuNodes(ctx context.Context) ([]*model.Node, error)

	// GpuDevice operations
	GetGpuDeviceByNodeAndGpuId(ctx context.Context, nodeId int32, gpuId int) (*model.GpuDevice, error)
	CreateGpuDevice(ctx context.Context, device *model.GpuDevice) error
	UpdateGpuDevice(ctx context.Context, device *model.GpuDevice) error
	UpsertGpuDevice(ctx context.Context, device *model.GpuDevice) error
	UpdateGpuDeviceMetrics(ctx context.Context, id int32, utilization, temperature, power float64) error
	ListGpuDeviceByNodeId(ctx context.Context, nodeId int32) ([]*model.GpuDevice, error)
	DeleteGpuDeviceById(ctx context.Context, id int32) error

	// RdmaDevice operations
	GetRdmaDeviceByNodeIdAndPort(ctx context.Context, nodeGuid string, port int) (*model.RdmaDevice, error)
	CreateRdmaDevice(ctx context.Context, rdmaDevice *model.RdmaDevice) error
	ListRdmaDeviceByNodeId(ctx context.Context, nodeId int32) ([]*model.RdmaDevice, error)
	DeleteRdmaDeviceById(ctx context.Context, id int32) error

	// NodeDeviceChangelog operations
	CreateNodeDeviceChangelog(ctx context.Context, changelog *model.NodeDeviceChangelog) error

	// WithCluster method
	WithCluster(clusterName string) NodeFacadeInterface
}

// NodeFacade implements NodeFacadeInterface
type NodeFacade struct {
	BaseFacade
}

// NewNodeFacade creates a new NodeFacade instance
func NewNodeFacade() NodeFacadeInterface {
	return &NodeFacade{}
}

func (f *NodeFacade) WithCluster(clusterName string) NodeFacadeInterface {
	return &NodeFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// Node operation implementations
func (f *NodeFacade) CreateNode(ctx context.Context, node *model.Node) error {
	return f.getDAL().Node.WithContext(ctx).Create(node)
}

func (f *NodeFacade) UpdateNode(ctx context.Context, node *model.Node) error {
	return f.getDAL().Node.WithContext(ctx).Save(node)
}

func (f *NodeFacade) UpdateNodeSelectedFields(ctx context.Context, id int32, fields map[string]interface{}) error {
	fields["updated_at"] = time.Now()
	return f.getDB().WithContext(ctx).Model(&model.Node{}).Where("id = ?", id).Updates(fields).Error
}

func (f *NodeFacade) GetNodeByName(ctx context.Context, name string) (*model.Node, error) {
	query := f.getDAL().Node
	node, err := query.WithContext(ctx).Where(query.Name.Eq(name)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if node.ID == 0 {
		return nil, nil
	}
	return node, nil
}

func (f *NodeFacade) SearchNode(ctx context.Context, filter filter.NodeFilter) ([]*model.Node, int, error) {
	q := f.getDAL().Node
	query := q.WithContext(ctx)

	if filter.Name != nil {
		query = query.Where(q.Name.Like(fmt.Sprintf("%%%s%%", *filter.Name)))
	}
	if filter.Address != nil {
		query = query.Where(q.Address.Eq(*filter.Address))
	}
	if filter.GPUName != nil {
		query = query.Where(q.GpuName.Like(fmt.Sprintf("%%%s%%", *filter.GPUName)))
	}
	if filter.GPUAllocation != nil {
		query = query.Where(q.GpuAllocation.Eq(int32(*filter.GPUAllocation)))
	}
	if filter.GPUCount != nil {
		query = query.Where(q.GpuCount.Eq(int32(*filter.GPUCount)))
	}
	if filter.GPUUtilMin != nil {
		query = query.Where(q.GpuUtilization.Gte(*filter.GPUUtilMin))
	}
	if filter.GPUUtilMax != nil {
		query = query.Where(q.GpuUtilization.Lte(*filter.GPUUtilMax))
	}
	if len(filter.Status) > 0 {
		query = query.Where(q.Status.In(filter.Status...))
	}
	if filter.CPU != nil {
		query = query.Where(q.CPU.Eq(*filter.CPU))
	}
	if filter.CPUCount != nil {
		query = query.Where(q.CPUCount.Eq(int32(*filter.CPUCount)))
	}
	if filter.Memory != nil {
		query = query.Where(q.Memory.Eq(*filter.Memory))
	}
	if filter.K8sVersion != nil {
		query = query.Where(q.K8sVersion.Eq(*filter.K8sVersion))
	}
	if filter.K8sStatus != nil {
		query = query.Where(q.K8sStatus.Eq(*filter.K8sStatus))
	}

	count, err := query.Count()
	if err != nil {
		return nil, 0, err
	}

	gormDB := query.UnderlyingDB()
	if filter.OrderBy != "" {
		order := filter.Order
		if order == "" {
			order = "DESC"
		}
		gormDB = gormDB.Order(fmt.Sprintf("%s %s", filter.OrderBy, order))
	}

	if filter.Limit > 0 {
		gormDB = gormDB.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		gormDB = gormDB.Offset(filter.Offset)
	}

	var nodes []*model.Node
	err = gormDB.Find(&nodes).Error
	if err != nil {
		return nil, 0, err
	}
	return nodes, int(count), nil
}

func (f *NodeFacade) ListGpuNodes(ctx context.Context) ([]*model.Node, error) {
	q := f.getDAL().Node
	nodes, err := q.WithContext(ctx).Where(q.GpuCount.Gt(0)).Where(q.Status.Eq("Ready")).Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*model.Node{}, nil
		}
		return nil, err
	}
	return nodes, nil
}

// GpuDevice operation implementations
func (f *NodeFacade) GetGpuDeviceByNodeAndGpuId(ctx context.Context, nodeId int32, gpuId int) (*model.GpuDevice, error) {
	q := f.getDAL().GpuDevice
	device, err := q.WithContext(ctx).Where(q.NodeID.Eq(nodeId)).Where(q.GpuID.Eq(int32(gpuId))).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if device.ID == 0 {
		return nil, nil
	}
	return device, nil
}

func (f *NodeFacade) CreateGpuDevice(ctx context.Context, device *model.GpuDevice) error {
	return f.getDAL().GpuDevice.WithContext(ctx).Create(device)
}

func (f *NodeFacade) UpdateGpuDevice(ctx context.Context, device *model.GpuDevice) error {
	return f.getDAL().GpuDevice.WithContext(ctx).Save(device)
}

func (f *NodeFacade) UpsertGpuDevice(ctx context.Context, device *model.GpuDevice) error {
	return f.getDB().WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "node_id"}, {Name: "gpu_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"gpu_model", "memory", "utilization", "temperature", "power",
				"serial", "rdma_device_name", "rdma_guid", "rdma_lid",
				"numa_node", "numa_affinity", "updated_at",
			}),
		}).Create(device).Error
}

func (f *NodeFacade) UpdateGpuDeviceMetrics(ctx context.Context, id int32, utilization, temperature, power float64) error {
	return f.getDB().WithContext(ctx).
		Model(&model.GpuDevice{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"utilization": utilization,
			"temperature": temperature,
			"power":       power,
			"updated_at":  time.Now(),
		}).Error
}

func (f *NodeFacade) ListGpuDeviceByNodeId(ctx context.Context, nodeId int32) ([]*model.GpuDevice, error) {
	q := f.getDAL().GpuDevice
	return q.WithContext(ctx).Where(q.NodeID.Eq(nodeId)).Order(q.GpuID.Asc()).Find()
}

func (f *NodeFacade) DeleteGpuDeviceById(ctx context.Context, id int32) error {
	q := f.getDAL().GpuDevice
	_, err := q.WithContext(ctx).Where(q.ID.Eq(id)).Delete()
	return err
}

// RdmaDevice operation implementations
func (f *NodeFacade) GetRdmaDeviceByNodeIdAndPort(ctx context.Context, nodeGuid string, port int) (*model.RdmaDevice, error) {
	q := f.getDAL().RdmaDevice
	result, err := q.WithContext(ctx).Where(q.NodeGUID.Eq(nodeGuid)).Where(q.IfIndex.Eq(int32(port))).First()
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

func (f *NodeFacade) CreateRdmaDevice(ctx context.Context, rdmaDevice *model.RdmaDevice) error {
	return f.getDAL().RdmaDevice.WithContext(ctx).Create(rdmaDevice)
}

func (f *NodeFacade) ListRdmaDeviceByNodeId(ctx context.Context, nodeId int32) ([]*model.RdmaDevice, error) {
	q := f.getDAL().RdmaDevice
	results, err := q.WithContext(ctx).Where(q.NodeID.Eq(nodeId)).Order(q.IfIndex.Asc()).Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return results, nil
}

func (f *NodeFacade) DeleteRdmaDeviceById(ctx context.Context, id int32) error {
	q := f.getDAL().RdmaDevice
	_, err := q.WithContext(ctx).Where(q.ID.Eq(id)).Delete()
	return err
}

// NodeDeviceChangelog operation implementations
func (f *NodeFacade) CreateNodeDeviceChangelog(ctx context.Context, changelog *model.NodeDeviceChangelog) error {
	return f.getDAL().NodeDeviceChangelog.WithContext(ctx).Create(changelog)
}
