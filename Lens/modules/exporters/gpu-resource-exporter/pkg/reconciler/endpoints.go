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

// EndpointsReconciler watches Kubernetes Endpoints and updates service-pod relationships
type EndpointsReconciler struct {
	clientSets *clientsets.K8SClientSet
}

// NewEndpointsReconciler creates a new EndpointsReconciler
func NewEndpointsReconciler() *EndpointsReconciler {
	return &EndpointsReconciler{}
}

// SetupWithManager sets up the controller with the Manager
func (e *EndpointsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Endpoints{}).
		Complete(e)
}

// Reconcile handles Endpoints create/update/delete events
func (e *EndpointsReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	if e.clientSets == nil {
		e.clientSets = clientsets.GetClusterManager().GetCurrentClusterClients().K8SClientSet
	}

	endpoints := &corev1.Endpoints{}
	err := e.clientSets.ControllerRuntimeClient.Get(ctx, req.NamespacedName, endpoints)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Endpoints deleted, the service reconciler will handle cleanup
			return reconcile.Result{}, nil
		}
		log.Errorf("Error getting endpoints %s/%s: %v", req.Namespace, req.Name, err)
		return reconcile.Result{}, err
	}

	// Get corresponding Service
	svc := &corev1.Service{}
	if err := e.clientSets.ControllerRuntimeClient.Get(ctx, req.NamespacedName, svc); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Service doesn't exist, skip
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Update service-pod relationships
	return e.updateServicePodRelationships(ctx, svc, endpoints)
}

// updateServicePodRelationships updates the relationship between service and pods
func (e *EndpointsReconciler) updateServicePodRelationships(ctx context.Context, svc *corev1.Service, endpoints *corev1.Endpoints) (reconcile.Result, error) {
	serviceUID := string(svc.UID)

	// Delete existing relationships for this service
	if err := database.GetFacade().GetK8sService().DeleteServicePodRefs(ctx, serviceUID); err != nil {
		log.Warnf("Failed to delete existing service-pod refs for %s/%s: %v", svc.Namespace, svc.Name, err)
	}

	// Create new relationships
	refsCreated := 0
	for _, subset := range endpoints.Subsets {
		// Process ready addresses
		for _, addr := range subset.Addresses {
			if addr.TargetRef != nil && addr.TargetRef.Kind == "Pod" {
				ref := e.createServicePodRef(ctx, svc, addr)
				if err := database.GetFacade().GetK8sService().CreateServicePodRef(ctx, ref); err != nil {
					log.Warnf("Failed to create service-pod ref: %v", err)
				} else {
					refsCreated++
				}
			}
		}
		// Also process NotReadyAddresses to capture pods that are not yet ready
		for _, addr := range subset.NotReadyAddresses {
			if addr.TargetRef != nil && addr.TargetRef.Kind == "Pod" {
				ref := e.createServicePodRef(ctx, svc, addr)
				if err := database.GetFacade().GetK8sService().CreateServicePodRef(ctx, ref); err != nil {
					log.Warnf("Failed to create service-pod ref: %v", err)
				} else {
					refsCreated++
				}
			}
		}
	}

	log.Debugf("Updated service-pod relationships for %s/%s, created %d refs", svc.Namespace, svc.Name, refsCreated)
	return reconcile.Result{}, nil
}

// createServicePodRef creates a ServicePodReference from service and endpoint address
func (e *EndpointsReconciler) createServicePodRef(ctx context.Context, svc *corev1.Service, addr corev1.EndpointAddress) *model.ServicePodReference {
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

	// Set node name if available from endpoint
	if addr.NodeName != nil {
		ref.NodeName = *addr.NodeName
	}

	// Skip pod lookup if clientSets is not available (e.g., in tests)
	if e.clientSets == nil || e.clientSets.ControllerRuntimeClient == nil {
		return ref
	}

	// Try to get pod details for additional labels and workload info
	pod := &corev1.Pod{}
	if err := e.clientSets.ControllerRuntimeClient.Get(ctx, types.NamespacedName{
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

		// Extract workload info from pod labels (Primus-SaFE specific labels)
		if workloadID, ok := pod.Labels["primus-safe.workload.id"]; ok {
			ref.WorkloadID = workloadID
		}
		if owner, ok := pod.Labels["primus-safe.user.name"]; ok {
			ref.WorkloadOwner = owner
		}

		// Try alternative label patterns for workload identification
		if ref.WorkloadID == "" {
			// Try app.kubernetes.io/name label
			if appName, ok := pod.Labels["app.kubernetes.io/name"]; ok {
				ref.WorkloadID = appName
			} else if appLabel, ok := pod.Labels["app"]; ok {
				ref.WorkloadID = appLabel
			}
		}

		// Determine workload type from owner references
		for _, ownerRef := range pod.OwnerReferences {
			if ownerRef.Controller != nil && *ownerRef.Controller {
				ref.WorkloadType = ownerRef.Kind
				break
			}
		}
	} else {
		log.Debugf("Could not get pod details for %s/%s: %v", addr.TargetRef.Namespace, addr.TargetRef.Name, err)
	}

	return ref
}

