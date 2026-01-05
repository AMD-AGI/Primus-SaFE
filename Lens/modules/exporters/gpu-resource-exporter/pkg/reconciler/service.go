package reconciler

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ServiceReconciler watches Kubernetes Services and syncs them to the database
type ServiceReconciler struct {
	clientSets *clientsets.K8SClientSet
}

// NewServiceReconciler creates a new ServiceReconciler
func NewServiceReconciler() *ServiceReconciler {
	return &ServiceReconciler{}
}

// SetupWithManager sets up the controller with the Manager
func (s *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Complete(s)
}

// Reconcile handles Service create/update/delete events
func (s *ServiceReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	if s.clientSets == nil {
		s.clientSets = clientsets.GetClusterManager().GetCurrentClusterClients().K8SClientSet
	}

	svc := &corev1.Service{}
	err := s.clientSets.ControllerRuntimeClient.Get(ctx, req.NamespacedName, svc)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Service deleted, mark as deleted in DB
			return s.handleServiceDelete(ctx, req.Namespace, req.Name)
		}
		log.Errorf("Error getting service %s/%s: %v", req.Namespace, req.Name, err)
		return reconcile.Result{}, err
	}

	// Save/update service info
	if err := s.saveServiceInfo(ctx, svc); err != nil {
		log.Errorf("Failed to save service info %s/%s: %v", svc.Namespace, svc.Name, err)
		return reconcile.Result{}, err
	}

	// Save service-pod relationships via Endpoints
	if err := s.saveServicePodRelationships(ctx, svc); err != nil {
		log.Errorf("Failed to save service-pod relationships %s/%s: %v", svc.Namespace, svc.Name, err)
		// Don't return error here as the service info was saved successfully
		// The endpoints might not exist yet
	}

	log.Debugf("Reconciled service %s/%s", svc.Namespace, svc.Name)
	return reconcile.Result{}, nil
}

// saveServiceInfo saves the service information to the database
func (s *ServiceReconciler) saveServiceInfo(ctx context.Context, svc *corev1.Service) error {
	// Convert selector to ExtType
	selector := make(model.ExtType)
	for k, v := range svc.Spec.Selector {
		selector[k] = v
	}

	// Convert ports to ExtJSON (as JSON array)
	ports := make([]model.ServicePort, 0, len(svc.Spec.Ports))
	for _, port := range svc.Spec.Ports {
		ports = append(ports, model.ServicePort{
			Name:       port.Name,
			Port:       int(port.Port),
			TargetPort: port.TargetPort.String(),
			Protocol:   string(port.Protocol),
			NodePort:   int(port.NodePort),
		})
	}
	var portsExtJSON model.ExtJSON
	_ = portsExtJSON.MarshalFrom(ports)

	// Convert labels to ExtType
	labels := make(model.ExtType)
	for k, v := range svc.Labels {
		labels[k] = v
	}

	// Convert annotations to ExtType
	annotations := make(model.ExtType)
	for k, v := range svc.Annotations {
		annotations[k] = v
	}

	k8sService := &model.K8sService{
		UID:         string(svc.UID),
		Name:        svc.Name,
		Namespace:   svc.Namespace,
		ClusterIP:   svc.Spec.ClusterIP,
		ServiceType: string(svc.Spec.Type),
		Selector:    selector,
		Ports:       portsExtJSON,
		Labels:      labels,
		Annotations: annotations,
		Deleted:     svc.DeletionTimestamp != nil,
		CreatedAt:   svc.CreationTimestamp.Time,
		UpdatedAt:   time.Now(),
	}

	return database.GetFacade().GetK8sService().UpsertService(ctx, k8sService)
}

// saveServicePodRelationships saves the relationship between service and pods via Endpoints
func (s *ServiceReconciler) saveServicePodRelationships(ctx context.Context, svc *corev1.Service) error {
	// Get Endpoints for this service
	endpoints := &corev1.Endpoints{}
	if err := s.clientSets.ControllerRuntimeClient.Get(ctx, types.NamespacedName{
		Name:      svc.Name,
		Namespace: svc.Namespace,
	}, endpoints); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil // No endpoints yet
		}
		return err
	}

	// Delete existing relationships for this service
	if err := database.GetFacade().GetK8sService().DeleteServicePodRefs(ctx, string(svc.UID)); err != nil {
		log.Warnf("Failed to delete existing service-pod refs for %s/%s: %v", svc.Namespace, svc.Name, err)
	}

	// Create new relationships
	for _, subset := range endpoints.Subsets {
		for _, addr := range subset.Addresses {
			if addr.TargetRef != nil && addr.TargetRef.Kind == "Pod" {
				ref := s.createServicePodRef(ctx, svc, addr)
				if err := database.GetFacade().GetK8sService().CreateServicePodRef(ctx, ref); err != nil {
					log.Warnf("Failed to create service-pod ref: %v", err)
				}
			}
		}
		// Also process NotReadyAddresses to capture pods that are not yet ready
		for _, addr := range subset.NotReadyAddresses {
			if addr.TargetRef != nil && addr.TargetRef.Kind == "Pod" {
				ref := s.createServicePodRef(ctx, svc, addr)
				if err := database.GetFacade().GetK8sService().CreateServicePodRef(ctx, ref); err != nil {
					log.Warnf("Failed to create service-pod ref: %v", err)
				}
			}
		}
	}

	return nil
}

// createServicePodRef creates a ServicePodReference from service and endpoint address
func (s *ServiceReconciler) createServicePodRef(ctx context.Context, svc *corev1.Service, addr corev1.EndpointAddress) *model.ServicePodReference {
	ref := &model.ServicePodReference{
		ServiceUID:       string(svc.UID),
		ServiceName:      svc.Name,
		ServiceNamespace: svc.Namespace,
		PodUID:           string(addr.TargetRef.UID),
		PodName:          addr.TargetRef.Name,
		PodIP:            addr.IP,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Set node name if available
	if addr.NodeName != nil {
		ref.NodeName = *addr.NodeName
	}

	// Skip pod lookup if clientSets is not available (e.g., in tests)
	if s.clientSets == nil || s.clientSets.ControllerRuntimeClient == nil {
		return ref
	}

	// Try to get pod details for additional labels
	pod := &corev1.Pod{}
	if err := s.clientSets.ControllerRuntimeClient.Get(ctx, types.NamespacedName{
		Name:      addr.TargetRef.Name,
		Namespace: addr.TargetRef.Namespace,
	}, pod); err == nil {
		// Convert pod labels to ExtType
		podLabels := make(model.ExtType)
		for k, v := range pod.Labels {
			podLabels[k] = v
		}
		ref.PodLabels = podLabels
		ref.NodeName = pod.Spec.NodeName

		// Extract workload info from pod labels
		if workloadID, ok := pod.Labels["primus-safe.workload.id"]; ok {
			ref.WorkloadID = workloadID
		}
		if owner, ok := pod.Labels["primus-safe.user.name"]; ok {
			ref.WorkloadOwner = owner
		}

		// Determine workload type from owner references
		for _, ownerRef := range pod.OwnerReferences {
			if ownerRef.Controller != nil && *ownerRef.Controller {
				ref.WorkloadType = ownerRef.Kind
				break
			}
		}
	}

	return ref
}

// handleServiceDelete marks a service as deleted in the database
func (s *ServiceReconciler) handleServiceDelete(ctx context.Context, namespace, name string) (reconcile.Result, error) {
	// Mark service as deleted
	if err := database.GetFacade().GetK8sService().MarkServiceDeleted(ctx, namespace, name); err != nil {
		log.Errorf("Failed to mark service %s/%s as deleted: %v", namespace, name, err)
		return reconcile.Result{}, err
	}
	log.Infof("Marked service %s/%s as deleted", namespace, name)
	return reconcile.Result{}, nil
}
