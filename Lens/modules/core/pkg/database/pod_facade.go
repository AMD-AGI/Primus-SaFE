// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
)

// PodFacadeInterface defines the database operation interface for Pod
type PodFacadeInterface interface {
	// GpuPods operations
	CreateGpuPods(ctx context.Context, gpuPods *model.GpuPods) error
	UpdateGpuPods(ctx context.Context, gpuPods *model.GpuPods) error
	GetGpuPodsByPodUid(ctx context.Context, podUid string) (*model.GpuPods, error)
	GetActiveGpuPodByNodeName(ctx context.Context, nodeName string) ([]*model.GpuPods, error)
	GetHistoryGpuPodByNodeName(ctx context.Context, nodeName string, pageNum, pageSize int) ([]*model.GpuPods, int, error)
	ListActivePodsByUids(ctx context.Context, uids []string) ([]*model.GpuPods, error)
	ListPodsByUids(ctx context.Context, uids []string) ([]*model.GpuPods, error)
	ListActiveGpuPods(ctx context.Context) ([]*model.GpuPods, error)
	// ListPodsActiveInTimeRange returns pods that were active during the specified time range
	// A pod is considered active in the time range if:
	// - created_at <= endTime AND
	// - (phase = 'Running' OR updated_at >= startTime)
	ListPodsActiveInTimeRange(ctx context.Context, startTime, endTime time.Time) ([]*model.GpuPods, error)

	// GpuPodsEvent operations
	CreateGpuPodsEvent(ctx context.Context, gpuPods *model.GpuPodsEvent) error
	UpdateGpuPodsEvent(ctx context.Context, gpuPods *model.GpuPods) error

	// PodSnapshot operations
	CreatePodSnapshot(ctx context.Context, podSnapshot *model.PodSnapshot) error
	UpdatePodSnapshot(ctx context.Context, podSnapshot *model.PodSnapshot) error
	GetLastPodSnapshot(ctx context.Context, podUid string, resourceVersion int) (*model.PodSnapshot, error)

	// PodResource operations
	GetPodResourceByUid(ctx context.Context, uid string) (*model.PodResource, error)
	CreatePodResource(ctx context.Context, podResource *model.PodResource) error
	UpdatePodResource(ctx context.Context, podResource *model.PodResource) error
	// ListPodResourcesByUids returns PodResource records for the given UIDs that have GPU allocation > 0
	ListPodResourcesByUids(ctx context.Context, uids []string) ([]*model.PodResource, error)

	// New methods for Pod REST API
	// QueryPodsWithFilters queries Pods with filtering, pagination and returns total count
	QueryPodsWithFilters(ctx context.Context, namespace, podName, startTime, endTime string, page, pageSize int) ([]*model.GpuPods, int64, error)
	// GetAverageGPUUtilizationByNode gets average GPU utilization for a node
	GetAverageGPUUtilizationByNode(ctx context.Context, nodeName string) (float64, error)
	// GetLatestGPUMetricsByNode gets the latest GPU metrics for a node
	GetLatestGPUMetricsByNode(ctx context.Context, nodeName string) (*model.GpuDevice, error)
	// QueryGPUHistoryByNode queries GPU history for a node in a time range
	QueryGPUHistoryByNode(ctx context.Context, nodeName string, startTime, endTime time.Time) ([]*model.GpuDevice, error)
	// ListPodEventsByUID lists all events for a pod
	ListPodEventsByUID(ctx context.Context, podUID string) ([]*model.GpuPodsEvent, error)

	// WithCluster method
	WithCluster(clusterName string) PodFacadeInterface
}

// PodFacade implements PodFacadeInterface
type PodFacade struct {
	BaseFacade
}

// NewPodFacade creates a new PodFacade instance
func NewPodFacade() PodFacadeInterface {
	return &PodFacade{}
}

func (f *PodFacade) WithCluster(clusterName string) PodFacadeInterface {
	return &PodFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// GpuPods operation implementations
func (f *PodFacade) CreateGpuPods(ctx context.Context, gpuPods *model.GpuPods) error {
	return f.getDAL().GpuPods.WithContext(ctx).Create(gpuPods)
}

func (f *PodFacade) UpdateGpuPods(ctx context.Context, gpuPods *model.GpuPods) error {
	return f.getDAL().GpuPods.WithContext(ctx).Save(gpuPods)
}

func (f *PodFacade) GetGpuPodsByPodUid(ctx context.Context, podUid string) (*model.GpuPods, error) {
	q := f.getDAL().GpuPods
	result, err := q.WithContext(ctx).Where(q.UID.Eq(podUid)).First()
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

func (f *PodFacade) GetActiveGpuPodByNodeName(ctx context.Context, nodeName string) ([]*model.GpuPods, error) {
	q := f.getDAL().GpuPods
	result, err := q.WithContext(ctx).Where(q.NodeName.Eq(nodeName)).Where(q.Running.Is(true)).Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
	}
	return result, nil
}

func (f *PodFacade) GetHistoryGpuPodByNodeName(ctx context.Context, nodeName string, pageNum, pageSize int) ([]*model.GpuPods, int, error) {
	q := f.getDAL().GpuPods
	query := q.
		WithContext(ctx).
		Where(q.NodeName.Eq(nodeName)).
		Where(q.Running.Is(false)).
		Where(q.Phase.Neq(string(corev1.PodPending)))
	count, err := query.Count()
	if err != nil {
		return nil, 0, err
	}
	result, err := query.Order(q.CreatedAt.Desc()).Offset((pageNum - 1) * pageSize).Limit(pageSize).Find()
	if err != nil {
		return nil, 0, err
	}
	return result, int(count), nil
}

func (f *PodFacade) ListActivePodsByUids(ctx context.Context, uids []string) ([]*model.GpuPods, error) {
	q := f.getDAL().GpuPods
	pods, err := q.WithContext(ctx).Where(q.UID.In(uids...)).Where(q.Running.Is(true)).Find()
	if err != nil {
		return nil, err
	}
	return pods, nil
}

func (f *PodFacade) ListPodsByUids(ctx context.Context, uids []string) ([]*model.GpuPods, error) {
	q := f.getDAL().GpuPods
	pods, err := q.WithContext(ctx).Where(q.UID.In(uids...)).Find()
	if err != nil {
		return nil, err
	}
	return pods, nil
}

func (f *PodFacade) ListActiveGpuPods(ctx context.Context) ([]*model.GpuPods, error) {
	q := f.getDAL().GpuPods
	result, err := q.WithContext(ctx).Where(q.Running.Is(true)).Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

// ListPodsActiveInTimeRange returns pods that were active during the specified time range
// A pod is considered active if:
// - created_at <= endTime AND
// - (phase = 'Running' OR updated_at >= startTime)
// This is used for time-based aggregation calculations
func (f *PodFacade) ListPodsActiveInTimeRange(ctx context.Context, startTime, endTime time.Time) ([]*model.GpuPods, error) {
	q := f.getDAL().GpuPods
	// Pod was created before or during the time range
	// AND (pod is still running OR pod ended during or after the time range)
	result, err := q.WithContext(ctx).
		Where(q.CreatedAt.Lte(endTime)).
		Where(q.WithContext(ctx).Or(
			q.Phase.Eq(string(corev1.PodRunning)),
			q.UpdatedAt.Gte(startTime),
		)).
		Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

// GpuPodsEvent operation implementations
func (f *PodFacade) CreateGpuPodsEvent(ctx context.Context, gpuPods *model.GpuPodsEvent) error {
	return f.getDAL().GpuPodsEvent.WithContext(ctx).Create(gpuPods)
}

func (f *PodFacade) UpdateGpuPodsEvent(ctx context.Context, gpuPods *model.GpuPods) error {
	return f.getDAL().GpuPods.WithContext(ctx).Save(gpuPods)
}

// PodSnapshot operation implementations
func (f *PodFacade) CreatePodSnapshot(ctx context.Context, podSnapshot *model.PodSnapshot) error {
	return f.getDAL().PodSnapshot.WithContext(ctx).Create(podSnapshot)
}

func (f *PodFacade) UpdatePodSnapshot(ctx context.Context, podSnapshot *model.PodSnapshot) error {
	return f.getDAL().PodSnapshot.WithContext(ctx).Save(podSnapshot)
}

func (f *PodFacade) GetLastPodSnapshot(ctx context.Context, podUid string, resourceVersion int) (*model.PodSnapshot, error) {
	q := f.getDAL().PodSnapshot
	result, err := f.getDAL().
		PodSnapshot.
		WithContext(ctx).
		Where(q.PodUID.Eq(podUid)).
		Where(q.ResourceVersion.Lt(int32(resourceVersion))).
		Order(q.ResourceVersion.Desc()).
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

// PodResource operation implementations
func (f *PodFacade) GetPodResourceByUid(ctx context.Context, uid string) (*model.PodResource, error) {
	q := f.getDAL().PodResource
	result, err := q.WithContext(ctx).Where(q.UID.Eq(uid)).First()
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

func (f *PodFacade) CreatePodResource(ctx context.Context, podResource *model.PodResource) error {
	return f.getDAL().PodResource.WithContext(ctx).Create(podResource)
}

func (f *PodFacade) UpdatePodResource(ctx context.Context, podResource *model.PodResource) error {
	return f.getDAL().PodResource.WithContext(ctx).Save(podResource)
}

// ListPodResourcesByUids returns PodResource records for the given UIDs that have GPU allocation > 0
func (f *PodFacade) ListPodResourcesByUids(ctx context.Context, uids []string) ([]*model.PodResource, error) {
	if len(uids) == 0 {
		return []*model.PodResource{}, nil
	}

	q := f.getDAL().PodResource
	results, err := q.WithContext(ctx).
		Where(q.UID.In(uids...)).
		Where(q.GpuAllocated.Gt(0)).
		Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*model.PodResource{}, nil
		}
		return nil, err
	}

	return results, nil
}

// QueryPodsWithFilters queries Pods with filtering, pagination and returns total count
func (f *PodFacade) QueryPodsWithFilters(ctx context.Context, namespace, podName, startTime, endTime string, page, pageSize int) ([]*model.GpuPods, int64, error) {
	q := f.getDAL().GpuPods.WithContext(ctx)
	
	// Apply filters
	if namespace != "" {
		q = q.Where(f.getDAL().GpuPods.Namespace.Eq(namespace))
	}
	
	if podName != "" {
		q = q.Where(f.getDAL().GpuPods.Name.Like("%" + podName + "%"))
	}
	
	// Time range filter
	if startTime != "" {
		parsedStartTime, err := time.Parse(time.RFC3339, startTime)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid start_time format: %w (expected RFC3339)", err)
		}
		q = q.Where(f.getDAL().GpuPods.CreatedAt.Gte(parsedStartTime))
	}
	
	if endTime != "" {
		parsedEndTime, err := time.Parse(time.RFC3339, endTime)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid end_time format: %w (expected RFC3339)", err)
		}
		q = q.Where(f.getDAL().GpuPods.CreatedAt.Lte(parsedEndTime))
	}

	// Get total count
	total, err := q.Count()
	if err != nil {
		return nil, 0, err
	}

	// Get paginated results
	offset := (page - 1) * pageSize
	pods, err := q.Order(f.getDAL().GpuPods.CreatedAt.Desc()).
		Offset(offset).
		Limit(pageSize).
		Find()
	if err != nil {
		return nil, 0, err
	}

	return pods, total, nil
}

// GetAverageGPUUtilizationByNode gets average GPU utilization for a node
func (f *PodFacade) GetAverageGPUUtilizationByNode(ctx context.Context, nodeName string) (float64, error) {
	// First get node ID
	nodeQ := f.getDAL().Node
	node, err := nodeQ.WithContext(ctx).Where(nodeQ.Name.Eq(nodeName)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0.0, nil
		}
		return 0.0, err
	}

	// Query average GPU utilization
	type Result struct {
		AvgUtilization float64
	}
	
	var result Result
	gpuDeviceQ := f.getDAL().GpuDevice
	err = gpuDeviceQ.WithContext(ctx).
		Select(gpuDeviceQ.Utilization.Avg().As("avg_utilization")).
		Where(gpuDeviceQ.NodeID.Eq(node.ID)).
		Scan(&result)
	
	if err != nil {
		return 0.0, err
	}
	
	return result.AvgUtilization, nil
}

// GetLatestGPUMetricsByNode gets the latest GPU metrics for a node
func (f *PodFacade) GetLatestGPUMetricsByNode(ctx context.Context, nodeName string) (*model.GpuDevice, error) {
	// First get node ID
	nodeQ := f.getDAL().Node
	node, err := nodeQ.WithContext(ctx).Where(nodeQ.Name.Eq(nodeName)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	// Query latest GPU device metrics
	gpuDeviceQ := f.getDAL().GpuDevice
	device, err := gpuDeviceQ.WithContext(ctx).
		Where(gpuDeviceQ.NodeID.Eq(node.ID)).
		Order(gpuDeviceQ.UpdatedAt.Desc()).
		First()
	
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return device, nil
}

// QueryGPUHistoryByNode queries GPU history for a node in a time range
func (f *PodFacade) QueryGPUHistoryByNode(ctx context.Context, nodeName string, startTime, endTime time.Time) ([]*model.GpuDevice, error) {
	// First get node ID
	nodeQ := f.getDAL().Node
	node, err := nodeQ.WithContext(ctx).Where(nodeQ.Name.Eq(nodeName)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*model.GpuDevice{}, nil
		}
		return nil, err
	}

	// Query GPU device history
	gpuDeviceQ := f.getDAL().GpuDevice
	devices, err := gpuDeviceQ.WithContext(ctx).
		Where(gpuDeviceQ.NodeID.Eq(node.ID)).
		Where(gpuDeviceQ.UpdatedAt.Gte(startTime)).
		Where(gpuDeviceQ.UpdatedAt.Lte(endTime)).
		Order(gpuDeviceQ.UpdatedAt).
		Find()
	
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*model.GpuDevice{}, nil
		}
		return nil, err
	}

	return devices, nil
}

// ListPodEventsByUID lists all events for a pod
func (f *PodFacade) ListPodEventsByUID(ctx context.Context, podUID string) ([]*model.GpuPodsEvent, error) {
	q := f.getDAL().GpuPodsEvent
	events, err := q.WithContext(ctx).
		Where(q.PodUUID.Eq(podUID)).
		Order(q.CreatedAt.Desc()).
		Find()
	
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*model.GpuPodsEvent{}, nil
		}
		return nil, err
	}

	return events, nil
}
