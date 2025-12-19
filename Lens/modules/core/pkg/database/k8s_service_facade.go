package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// K8sServiceFacadeInterface defines the database operation interface for K8s Service
type K8sServiceFacadeInterface interface {
	// K8sService operations
	UpsertService(ctx context.Context, svc *model.K8sService) error
	GetServiceByUID(ctx context.Context, uid string) (*model.K8sService, error)
	GetServiceByName(ctx context.Context, namespace, name string) (*model.K8sService, error)
	ListServices(ctx context.Context, namespace string) ([]*model.K8sService, error)
	ListActiveServices(ctx context.Context) ([]*model.K8sService, error)
	MarkServiceDeleted(ctx context.Context, namespace, name string) error
	DeleteServiceByUID(ctx context.Context, uid string) error

	// ServicePodReference operations
	CreateServicePodRef(ctx context.Context, ref *model.ServicePodReference) error
	UpsertServicePodRef(ctx context.Context, ref *model.ServicePodReference) error
	DeleteServicePodRefs(ctx context.Context, serviceUID string) error
	GetPodsByServiceName(ctx context.Context, namespace, name string) ([]*model.ServicePodReference, error)
	GetPodsByServiceUID(ctx context.Context, serviceUID string) ([]*model.ServicePodReference, error)
	GetServicesByPodUID(ctx context.Context, podUID string) ([]*model.ServicePodReference, error)
	GetServicePodRefsByWorkloadID(ctx context.Context, workloadID string) ([]*model.ServicePodReference, error)

	// WithCluster method
	WithCluster(clusterName string) K8sServiceFacadeInterface
}

// K8sServiceFacade implements K8sServiceFacadeInterface
type K8sServiceFacade struct {
	BaseFacade
}

// NewK8sServiceFacade creates a new K8sServiceFacade instance
func NewK8sServiceFacade() K8sServiceFacadeInterface {
	return &K8sServiceFacade{}
}

func (f *K8sServiceFacade) WithCluster(clusterName string) K8sServiceFacadeInterface {
	return &K8sServiceFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// UpsertService creates or updates a service record
func (f *K8sServiceFacade) UpsertService(ctx context.Context, svc *model.K8sService) error {
	svc.UpdatedAt = time.Now()
	if svc.CreatedAt.IsZero() {
		svc.CreatedAt = time.Now()
	}

	return f.getDB().WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "uid"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "namespace", "cluster_ip", "service_type",
			"selector", "ports", "labels", "annotations", "deleted", "updated_at",
		}),
	}).Create(svc).Error
}

// GetServiceByUID retrieves a service by its UID
func (f *K8sServiceFacade) GetServiceByUID(ctx context.Context, uid string) (*model.K8sService, error) {
	var svc model.K8sService
	err := f.getDB().WithContext(ctx).
		Where("uid = ?", uid).
		First(&svc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &svc, nil
}

// GetServiceByName retrieves a service by namespace and name
func (f *K8sServiceFacade) GetServiceByName(ctx context.Context, namespace, name string) (*model.K8sService, error) {
	var svc model.K8sService
	err := f.getDB().WithContext(ctx).
		Where("namespace = ? AND name = ? AND deleted = false", namespace, name).
		First(&svc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &svc, nil
}

// ListServices lists all services in a namespace
func (f *K8sServiceFacade) ListServices(ctx context.Context, namespace string) ([]*model.K8sService, error) {
	var services []*model.K8sService
	query := f.getDB().WithContext(ctx)
	if namespace != "" {
		query = query.Where("namespace = ?", namespace)
	}
	err := query.Where("deleted = false").Find(&services).Error
	return services, err
}

// ListActiveServices lists all non-deleted services
func (f *K8sServiceFacade) ListActiveServices(ctx context.Context) ([]*model.K8sService, error) {
	var services []*model.K8sService
	err := f.getDB().WithContext(ctx).
		Where("deleted = false").
		Find(&services).Error
	return services, err
}

// MarkServiceDeleted marks a service as deleted
func (f *K8sServiceFacade) MarkServiceDeleted(ctx context.Context, namespace, name string) error {
	return f.getDB().WithContext(ctx).
		Model(&model.K8sService{}).
		Where("namespace = ? AND name = ?", namespace, name).
		Updates(map[string]interface{}{
			"deleted":    true,
			"updated_at": time.Now(),
		}).Error
}

// DeleteServiceByUID deletes a service by its UID
func (f *K8sServiceFacade) DeleteServiceByUID(ctx context.Context, uid string) error {
	return f.getDB().WithContext(ctx).
		Where("uid = ?", uid).
		Delete(&model.K8sService{}).Error
}

// CreateServicePodRef creates a service-pod reference
func (f *K8sServiceFacade) CreateServicePodRef(ctx context.Context, ref *model.ServicePodReference) error {
	ref.CreatedAt = time.Now()
	ref.UpdatedAt = time.Now()
	return f.getDB().WithContext(ctx).Create(ref).Error
}

// UpsertServicePodRef creates or updates a service-pod reference
func (f *K8sServiceFacade) UpsertServicePodRef(ctx context.Context, ref *model.ServicePodReference) error {
	ref.UpdatedAt = time.Now()
	if ref.CreatedAt.IsZero() {
		ref.CreatedAt = time.Now()
	}

	return f.getDB().WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "service_uid"}, {Name: "pod_uid"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"service_name", "service_namespace", "pod_name", "pod_ip",
			"pod_labels", "workload_id", "workload_owner", "workload_type",
			"node_name", "updated_at",
		}),
	}).Create(ref).Error
}

// DeleteServicePodRefs deletes all pod references for a service
func (f *K8sServiceFacade) DeleteServicePodRefs(ctx context.Context, serviceUID string) error {
	return f.getDB().WithContext(ctx).
		Where("service_uid = ?", serviceUID).
		Delete(&model.ServicePodReference{}).Error
}

// GetPodsByServiceName retrieves all pods for a service by namespace and name
func (f *K8sServiceFacade) GetPodsByServiceName(ctx context.Context, namespace, name string) ([]*model.ServicePodReference, error) {
	var refs []*model.ServicePodReference
	err := f.getDB().WithContext(ctx).
		Where("service_namespace = ? AND service_name = ?", namespace, name).
		Find(&refs).Error
	return refs, err
}

// GetPodsByServiceUID retrieves all pods for a service by service UID
func (f *K8sServiceFacade) GetPodsByServiceUID(ctx context.Context, serviceUID string) ([]*model.ServicePodReference, error) {
	var refs []*model.ServicePodReference
	err := f.getDB().WithContext(ctx).
		Where("service_uid = ?", serviceUID).
		Find(&refs).Error
	return refs, err
}

// GetServicesByPodUID retrieves all services that a pod belongs to
func (f *K8sServiceFacade) GetServicesByPodUID(ctx context.Context, podUID string) ([]*model.ServicePodReference, error) {
	var refs []*model.ServicePodReference
	err := f.getDB().WithContext(ctx).
		Where("pod_uid = ?", podUID).
		Find(&refs).Error
	return refs, err
}

// GetServicePodRefsByWorkloadID retrieves all service-pod refs for a workload
func (f *K8sServiceFacade) GetServicePodRefsByWorkloadID(ctx context.Context, workloadID string) ([]*model.ServicePodReference, error) {
	var refs []*model.ServicePodReference
	err := f.getDB().WithContext(ctx).
		Where("workload_id = ?", workloadID).
		Find(&refs).Error
	return refs, err
}

