package database

import (
	"context"
	"errors"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"gorm.io/gorm"
)

// ContainerFacadeInterface defines the database operation interface for Container
type ContainerFacadeInterface interface {
	// NodeContainer operations
	CreateNodeContainer(ctx context.Context, nodeContainer *model.NodeContainer) error
	UpdateNodeContainer(ctx context.Context, nodeContainer *model.NodeContainer) error
	GetNodeContainerByContainerId(ctx context.Context, containerId string) (*model.NodeContainer, error)
	ListRunningContainersByPodUid(ctx context.Context, podUid string) ([]*model.NodeContainer, error)

	// NodeContainerDevices operations
	CreateNodeContainerDevice(ctx context.Context, nodeContainerDevice *model.NodeContainerDevices) error
	UpdateNodeContainerDevice(ctx context.Context, nodeContainerDevice *model.NodeContainerDevices) error
	GetNodeContainerDeviceByContainerIdAndDeviceUid(ctx context.Context, containerId, deviceUid string) (*model.NodeContainerDevices, error)
	ListContainerDevicesByContainerId(ctx context.Context, containerId string) ([]*model.NodeContainerDevices, error)

	// NodeContainerEvent operations
	CreateNodeContainerEvent(ctx context.Context, nodeContainerEvent *model.NodeContainerEvent) error

	// WithCluster method
	WithCluster(clusterName string) ContainerFacadeInterface
}

// ContainerFacade implements ContainerFacadeInterface
type ContainerFacade struct {
	BaseFacade
}

// NewContainerFacade creates a new ContainerFacade instance
func NewContainerFacade() ContainerFacadeInterface {
	return &ContainerFacade{}
}

func (f *ContainerFacade) WithCluster(clusterName string) ContainerFacadeInterface {
	return &ContainerFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// NodeContainer operation implementations
func (f *ContainerFacade) CreateNodeContainer(ctx context.Context, nodeContainer *model.NodeContainer) error {
	err := f.getDAL().NodeContainer.WithContext(ctx).Create(nodeContainer)
	if err != nil {
		log.Errorf("CreateNodeContainer failed: %v", err)
		return err
	}
	return nil
}

func (f *ContainerFacade) UpdateNodeContainer(ctx context.Context, nodeContainer *model.NodeContainer) error {
	return f.getDAL().NodeContainer.WithContext(ctx).Save(nodeContainer)
}

func (f *ContainerFacade) GetNodeContainerByContainerId(ctx context.Context, containerId string) (*model.NodeContainer, error) {
	q := f.getDAL().NodeContainer
	result, err := q.WithContext(ctx).Where(q.ContainerID.Eq(containerId)).First()
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

func (f *ContainerFacade) ListRunningContainersByPodUid(ctx context.Context, podUid string) ([]*model.NodeContainer, error) {
	q := f.getDAL().NodeContainer
	containers, err := q.WithContext(ctx).
		Where(q.PodUID.Eq(podUid)).
		Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return containers, nil
}

// NodeContainerDevices operation implementations
func (f *ContainerFacade) CreateNodeContainerDevice(ctx context.Context, nodeContainerDevice *model.NodeContainerDevices) error {
	return f.getDAL().NodeContainerDevices.WithContext(ctx).Create(nodeContainerDevice)
}

func (f *ContainerFacade) UpdateNodeContainerDevice(ctx context.Context, nodeContainerDevice *model.NodeContainerDevices) error {
	return f.getDAL().NodeContainerDevices.WithContext(ctx).Save(nodeContainerDevice)
}

func (f *ContainerFacade) GetNodeContainerDeviceByContainerIdAndDeviceUid(ctx context.Context, containerId, deviceUid string) (*model.NodeContainerDevices, error) {
	q := f.getDAL().NodeContainerDevices
	result, err := q.WithContext(ctx).
		Where(q.ContainerID.Eq(containerId)).
		Where(q.DeviceUUID.Eq(deviceUid)).
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

func (f *ContainerFacade) ListContainerDevicesByContainerId(ctx context.Context, containerId string) ([]*model.NodeContainerDevices, error) {
	q := f.getDAL().NodeContainerDevices
	devices, err := q.WithContext(ctx).Where(q.ContainerID.Eq(containerId)).Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return devices, nil
}

// NodeContainerEvent operation implementations
func (f *ContainerFacade) CreateNodeContainerEvent(ctx context.Context, nodeContainerEvent *model.NodeContainerEvent) error {
	return f.getDAL().NodeContainerEvent.WithContext(ctx).Create(nodeContainerEvent)
}
