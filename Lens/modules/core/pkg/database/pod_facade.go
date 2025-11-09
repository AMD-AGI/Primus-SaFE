package database

import (
	"context"
	"errors"

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
	return result, nil
}

func (f *PodFacade) CreatePodResource(ctx context.Context, podResource *model.PodResource) error {
	return f.getDAL().PodResource.WithContext(ctx).Create(podResource)
}

func (f *PodFacade) UpdatePodResource(ctx context.Context, podResource *model.PodResource) error {
	return f.getDAL().PodResource.WithContext(ctx).Save(podResource)
}
