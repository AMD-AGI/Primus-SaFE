// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package reconciler

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/node"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/goroutineUtil"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/k8sUtil"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/mapUtil"
	"github.com/AMD-AGI/Primus-SaFE/Lens/gpu-resource-exporter/pkg/listener"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/strings/slices"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewGpuPodsReconciler() *GpuPodsReconciler {
	return &GpuPodsReconciler{}
}

type GpuPodsReconciler struct {
	clientSets *clientsets.K8SClientSet
}

func (g *GpuPodsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return k8sUtil.HasGPU(e.Object.(*corev1.Pod), metadata.GetResourceName(metadata.DefaultGpuVendor))
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return k8sUtil.HasGPU(e.ObjectNew.(*corev1.Pod), metadata.GetResourceName(metadata.DefaultGpuVendor))
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return k8sUtil.HasGPU(e.Object.(*corev1.Pod), metadata.GetResourceName(metadata.DefaultGpuVendor))
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return k8sUtil.HasGPU(e.Object.(*corev1.Pod), metadata.GetResourceName(metadata.DefaultGpuVendor))
			},
		}).
		Complete(g)
}

func (g *GpuPodsReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	defer func() {
		if r := recover(); r != nil {
			goroutineUtil.DefaultRecoveryFunc(r)
		}
	}()
	if g.clientSets == nil {
		g.clientSets = clientsets.GetClusterManager().GetCurrentClusterClients().K8SClientSet
	}
	pod := &corev1.Pod{}
	err := g.clientSets.ControllerRuntimeClient.Get(ctx, req.NamespacedName, pod)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Pod was deleted - close any open running periods for this pod
			// We need to find the pod UID from the database since we can't get it from the deleted pod
			if err := g.handleDeletedPod(ctx, req.Namespace, req.Name); err != nil {
				log.Warnf("Failed to handle deleted pod %s/%s: %v", req.Namespace, req.Name, err)
			}
			return reconcile.Result{}, nil
		}
		log.Error(err, "Error getting pod")
		return reconcile.Result{}, err
	}
	currentSnapshot, err := g.savePodSnapshot(ctx, pod)
	if err != nil {
		log.Error(err, "Error getting current snapshot")
		return reconcile.Result{}, err
	}
	err = g.saveGpuPodsStatus(ctx, pod)
	if err != nil {
		log.Error(err, "Error getting gpu pod status")
		return reconcile.Result{}, err
	}
	err = g.saveGpuPodResource(ctx, pod)
	if err != nil {
		log.Error(err, "Error getting gpu pod resource")
		return reconcile.Result{}, err
	}
	err = g.saveGpuPodEvent(ctx, pod, currentSnapshot)
	if err != nil {
		return reconcile.Result{}, err
	}
	err = g.tracePodOwners(ctx, pod)
	if err != nil {
		log.Error(err, "Error tracing pod owners")
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (g *GpuPodsReconciler) tracePodOwners(ctx context.Context, pod *corev1.Pod) error {

	var ownerReference *metav1.OwnerReference
	namespace := pod.Namespace
	if len(pod.OwnerReferences) == 0 {
		return nil
	}
	ownerReference = &pod.OwnerReferences[0]
	for {
		log.Infof("tracePodOwners: namespace: %s, ownerReference: %v", namespace, *ownerReference)
		ownerObj, err := k8sUtil.GetOwnerObject(ctx, g.clientSets.ControllerRuntimeClient, *ownerReference, namespace)
		if err != nil {
			if client.IgnoreNotFound(err) == nil {
				return nil
			}
			return err
		}
		if ownerObj == nil {
			log.Infof("tracePodOwners: namespace: %s, ownerReference: %v.Owner obj is null", namespace, *ownerReference)
			break
		}
		resourceVersion, err := strconv.Atoi(ownerObj.GetResourceVersion())
		if err != nil {
			resourceVersion = 0
		}
		_ = g.saveWorkloadPodReference(ctx, string(ownerObj.GetUID()), string(pod.UID))
		snapshot := &model.GpuWorkloadSnapshot{
			UID:             string(ownerObj.GetUID()),
			GroupVersion:    ownerObj.GetAPIVersion(),
			Kind:            ownerObj.GetKind(),
			Name:            ownerObj.GetName(),
			Namespace:       ownerObj.GetNamespace(),
			Metadata:        nil,
			Detail:          ownerObj.Object,
			ResourceVersion: int32(resourceVersion),
			CreatedAt:       time.Now(),
		}
		err = database.GetFacade().GetWorkload().CreateGpuWorkloadSnapshot(ctx, snapshot)
		if err != nil {
			log.Errorf("Failed to create gpu workload snapshot %v: %v", snapshot, err)
			continue
		}
		err = g.saveGpuWorkload(ctx, ownerObj)
		if err != nil {
			log.Errorf("Failed to save gpu workload %v: %v", snapshot, err)
		}
		if len(ownerObj.GetOwnerReferences()) == 0 {
			break
		}
		if ownerObj.GetNamespace() != "" {
			namespace = ownerObj.GetNamespace()
		}
		_ = g.addListener(ctx, ownerObj)
		ownerReference = &ownerObj.GetOwnerReferences()[0]
	}
	return nil
}

func (g *GpuPodsReconciler) saveWorkloadPodReference(ctx context.Context, workloadUid, podUid string) error {
	return database.GetFacade().GetWorkload().CreateWorkloadPodReference(ctx, workloadUid, podUid)
}

func (g *GpuPodsReconciler) saveGpuWorkload(ctx context.Context, obj *unstructured.Unstructured) error {
	parentUid := ""
	if len(obj.GetOwnerReferences()) > 0 {
		parentUid = string(obj.GetOwnerReferences()[0].UID)

	}
	gpuWorkload := &model.GpuWorkload{
		GroupVersion: obj.GetAPIVersion(),
		Kind:         obj.GetKind(),
		Namespace:    obj.GetNamespace(),
		Name:         obj.GetName(),
		UID:          string(obj.GetUID()),
		ParentUID:    parentUid,
		GpuRequest:   0,
		Status:       metadata.WorkloadStatusRunning,
		CreatedAt:    obj.GetCreationTimestamp().Time,
		UpdatedAt:    time.Now(),
		Labels:       map[string]interface{}{},
		Annotations:  map[string]interface{}{},
	}
	for key, value := range obj.GetLabels() {
		gpuWorkload.Labels[key] = value
	}
	for key, value := range obj.GetAnnotations() {
		gpuWorkload.Annotations[key] = value
	}
	if obj.GetDeletionTimestamp() != nil {
		gpuWorkload.EndAt = obj.GetDeletionTimestamp().Time
	}
	existGpuWorkload, err := database.GetFacade().GetWorkload().GetGpuWorkloadByUid(ctx, string(obj.GetUID()))
	if err != nil {
		return err
	}
	if existGpuWorkload == nil {
		existGpuWorkload = gpuWorkload
	} else {
		gpuWorkload.ID = existGpuWorkload.ID
		gpuWorkload.ParentUID = existGpuWorkload.ParentUID
	}
	if existGpuWorkload.ID == 0 {
		err = database.GetFacade().GetWorkload().CreateGpuWorkload(ctx, existGpuWorkload)

	} else {
		err = database.GetFacade().GetWorkload().UpdateGpuWorkload(ctx, existGpuWorkload)
	}
	if err != nil {
		return err
	}
	return nil
}

func (g *GpuPodsReconciler) saveGpuPodResource(ctx context.Context, pod *corev1.Pod) error {
	existResource, err := database.GetFacade().GetPod().GetPodResourceByUid(ctx, string(pod.GetUID()))
	if err != nil {
		return err
	}
	if existResource == nil {
		existResource = &model.PodResource{
			ID:           0,
			UID:          string(pod.GetUID()),
			GpuModel:     "",
			GpuAllocated: int32(gpu.GetAllocatedGpuResourceFromPod(pod, metadata.GetResourceName(metadata.GpuVendorAMD))),
			CreatedAt:    pod.CreationTimestamp.Time,
		}
		gpuModel, err := node.GetNodeGpuModel(ctx, pod.Spec.NodeName)
		if err != nil {

		}
		existResource.GpuModel = gpuModel
	}
	needUpdated := false
	if pod.DeletionTimestamp != nil {
		needUpdated = true
		existResource.EndAt = pod.DeletionTimestamp.Time
	} else if k8sUtil.IsPodDone(pod) {
		existResource.EndAt = k8sUtil.GetCompeletedAt(pod)
	}
	if existResource.ID == 0 {
		return database.GetFacade().GetPod().CreatePodResource(ctx, existResource)
	} else if needUpdated {
		return database.GetFacade().GetPod().UpdatePodResource(ctx, existResource)
	}
	return nil

}

func (g *GpuPodsReconciler) saveGpuPodEvent(ctx context.Context, pod *corev1.Pod, currentSnapshot *model.PodSnapshot) error {
	formerSnapshot, err := database.GetFacade().GetPod().GetLastPodSnapshot(ctx, currentSnapshot.PodUID, int(currentSnapshot.ResourceVersion))
	if err != nil {
		return err
	}
	events, err := g.compareSnapshotAndGetNewEvent(ctx, pod, formerSnapshot, currentSnapshot)
	if err != nil {
		return err
	}
	for i := range events {
		podsEvent := events[i]
		err := database.GetFacade().GetPod().CreateGpuPodsEvent(ctx, podsEvent)
		if err != nil {
			log.Errorf("Fail to CreateGpuPodsEvent.Error %+v", err)
		}
	}
	return nil
}

func (g *GpuPodsReconciler) compareSnapshotAndGetNewEvent(ctx context.Context, pod *corev1.Pod, formerSnapshot, newSnapshot *model.PodSnapshot) ([]*model.GpuPodsEvent, error) {
	formerConditions := getConditionFromSnapshot(formerSnapshot)
	currentConditions := getConditionFromSnapshot(newSnapshot)
	var newEvents []*model.GpuPodsEvent

	for _, currCond := range currentConditions {
		if currCond.Status != corev1.ConditionTrue {
			continue
		}
		found := false
		for _, oldCond := range formerConditions {
			if reflect.DeepEqual(currCond, oldCond) {
				found = true
				break
			}
		}
		if !found {
			restartCount := int32(0)
			if len(pod.Status.ContainerStatuses) > 0 {
				restartCount = pod.Status.ContainerStatuses[0].RestartCount
			}
			newEvents = append(newEvents, &model.GpuPodsEvent{
				PodUUID:      string(pod.GetUID()),
				PodPhase:     string(pod.Status.Phase),
				EventType:    string(currCond.Type),
				CreatedAt:    time.Time{},
				RestartCount: restartCount,
			})
		}
	}
	return newEvents, nil
}

func getConditionFromSnapshot(snapshot *model.PodSnapshot) []corev1.PodCondition {
	if snapshot == nil {
		return nil
	}
	podStatus := &corev1.PodStatus{}
	err := mapUtil.DecodeFromMap(snapshot.Status, podStatus)
	if err != nil {
		return nil
	}
	return podStatus.Conditions
}

func (g *GpuPodsReconciler) savePodSnapshot(ctx context.Context, pod *corev1.Pod) (*model.PodSnapshot, error) {
	specMap, err := mapUtil.ConvertInterfaceToExt(pod.Spec)
	if err != nil {
		return nil, err
	}
	statusMap, err := mapUtil.ConvertInterfaceToExt(pod.Status)
	if err != nil {
		return nil, err
	}
	metadataMap, err := mapUtil.ConvertInterfaceToExt(pod.ObjectMeta)
	if err != nil {
		return nil, err
	}
	resourceVersion, _ := strconv.Atoi(pod.ResourceVersion)
	currentSnapshot := &model.PodSnapshot{
		PodUID:          string(pod.GetUID()),
		PodName:         pod.GetName(),
		Namespace:       pod.GetNamespace(),
		Spec:            specMap,
		Metadata:        metadataMap,
		Status:          statusMap,
		CreatedAt:       time.Now(),
		ResourceVersion: int32(resourceVersion),
	}
	err = database.GetFacade().GetPod().CreatePodSnapshot(ctx, currentSnapshot)
	if err != nil {
		return nil, err
	}
	return currentSnapshot, nil
}

func (g *GpuPodsReconciler) saveGpuPodsStatus(ctx context.Context, pod *corev1.Pod) error {
	gpuPods := &model.GpuPods{
		Namespace:      pod.Namespace,
		Name:           pod.Name,
		NodeName:       pod.Spec.NodeName,
		UID:            string(pod.UID),
		IP:             pod.Status.PodIP, // Added: sync Pod IP to database
		GpuAllocated:   int32(k8sUtil.GetGpuAllocated(pod, metadata.GetResourceName(metadata.GpuVendorAMD))),
		Phase:          string(pod.Status.Phase),
		Deleted:        pod.DeletionTimestamp != nil,
		ContainerImage: extractPrimaryContainerImage(pod),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	isRunning := !gpuPods.Deleted && slices.Contains([]string{
		string(corev1.PodRunning),
	}, string(pod.Status.Phase))
	if isRunning {
		gpuPods.Running = true
	}

	existGpuPodsRecord, err := database.GetFacade().GetPod().GetGpuPodsByPodUid(ctx, string(pod.UID))
	if err != nil {
		return err
	}

	// Track running period transitions
	wasRunning := existGpuPodsRecord != nil && existGpuPodsRecord.Running

	if existGpuPodsRecord == nil {
		existGpuPodsRecord = gpuPods
	} else {
		gpuPods.ID = existGpuPodsRecord.ID
		gpuPods.CreatedAt = existGpuPodsRecord.CreatedAt
		existGpuPodsRecord = gpuPods
	}
	if existGpuPodsRecord.ID == 0 {
		err := database.GetFacade().GetPod().CreateGpuPods(ctx, existGpuPodsRecord)
		if err != nil {
			return err
		}
	} else {
		err := database.GetFacade().GetPod().UpdateGpuPods(ctx, existGpuPodsRecord)
		if err != nil {
			return err
		}
	}

	// Handle running period tracking
	if err := g.trackRunningPeriod(ctx, pod, wasRunning, isRunning); err != nil {
		log.Warnf("Failed to track running period for pod %s/%s: %v", pod.Namespace, pod.Name, err)
		// Don't fail the entire reconcile for running period tracking errors
	}

	return nil
}

// trackRunningPeriod tracks pod running state transitions
func (g *GpuPodsReconciler) trackRunningPeriod(ctx context.Context, pod *corev1.Pod, wasRunning, isRunning bool) error {
	podUID := string(pod.UID)
	now := time.Now()

	// Case 1: Pod just entered Running state
	if !wasRunning && isRunning {
		// Check if there's already an active running period (shouldn't happen, but be defensive)
		existingPeriod, err := database.GetFacade().GetPodRunningPeriods().GetCurrentRunningPeriod(ctx, podUID)
		if err != nil {
			return fmt.Errorf("failed to check existing running period: %w", err)
		}
		if existingPeriod != nil {
			log.Warnf("Pod %s/%s already has an active running period, skipping create", pod.Namespace, pod.Name)
			return nil
		}

		// Create new running period
		period := &model.PodRunningPeriods{
			PodUID:       podUID,
			Namespace:    pod.Namespace,
			PodName:      pod.Name,
			StartAt:      now,
			GpuAllocated: int32(k8sUtil.GetGpuAllocated(pod, metadata.GetResourceName(metadata.GpuVendorAMD))),
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := database.GetFacade().GetPodRunningPeriods().CreateRunningPeriod(ctx, period); err != nil {
			return fmt.Errorf("failed to create running period: %w", err)
		}
		log.Infof("Created running period for pod %s/%s", pod.Namespace, pod.Name)
		return nil
	}

	// Case 2: Pod just left Running state (or was deleted)
	if wasRunning && !isRunning {
		if err := database.GetFacade().GetPodRunningPeriods().EndRunningPeriod(ctx, podUID, now); err != nil {
			return fmt.Errorf("failed to end running period: %w", err)
		}
		log.Infof("Ended running period for pod %s/%s", pod.Namespace, pod.Name)
		return nil
	}

	// Case 3: No transition, nothing to do
	return nil
}

func (g *GpuPodsReconciler) addListener(
	ctx context.Context,
	obj *unstructured.Unstructured) error {
	uid := string(obj.GetUID())
	apiVersion := obj.GetAPIVersion()
	kind := obj.GetKind()
	name := obj.GetName()
	namespace := obj.GetNamespace()
	log.Infof("Adding listener for %s/%s (%s)", namespace, name, uid)
	err := listener.GetManager().RegisterListener(apiVersion, kind, namespace, name, uid)
	if err != nil {
		return fmt.Errorf("failed to register listener for %s %s %s %s: %v", apiVersion, kind, namespace, name, err)
	}
	log.Infof("Added listener for %s/%s (%s)", namespace, name, uid)
	return nil
}

// handleDeletedPod closes running periods for pods that have been deleted from the cluster
func (g *GpuPodsReconciler) handleDeletedPod(ctx context.Context, namespace, name string) error {
	// Find the pod by namespace and name in our database
	gpuPod, err := database.GetFacade().GetPod().GetGpuPodsByNamespaceName(ctx, namespace, name)
	if err != nil {
		return fmt.Errorf("failed to get pod from database: %w", err)
	}
	if gpuPod == nil {
		// Pod not found in database, nothing to do
		return nil
	}

	// Update the pod status in database
	gpuPod.Deleted = true
	gpuPod.Running = false
	gpuPod.UpdatedAt = time.Now()
	if err := database.GetFacade().GetPod().UpdateGpuPods(ctx, gpuPod); err != nil {
		log.Warnf("Failed to update deleted status for pod %s/%s: %v", namespace, name, err)
	}

	// End any open running periods for this pod
	if err := database.GetFacade().GetPodRunningPeriods().EndRunningPeriod(ctx, gpuPod.UID, time.Now()); err != nil {
		return fmt.Errorf("failed to end running period: %w", err)
	}
	log.Infof("Closed running period for deleted pod %s/%s (uid=%s)", namespace, name, gpuPod.UID)
	return nil
}

// extractPrimaryContainerImage returns the image of the primary (GPU-using) container.
// It prefers the container that requests GPU resources; falls back to the first container.
func extractPrimaryContainerImage(pod *corev1.Pod) string {
	if pod == nil || len(pod.Spec.Containers) == 0 {
		return ""
	}

	// Prefer a container that explicitly requests GPU resources
	gpuResourceName := metadata.GetResourceName(metadata.GpuVendorAMD)
	for _, c := range pod.Spec.Containers {
		if q, ok := c.Resources.Limits[corev1.ResourceName(gpuResourceName)]; ok && !q.IsZero() {
			return c.Image
		}
		if q, ok := c.Resources.Requests[corev1.ResourceName(gpuResourceName)]; ok && !q.IsZero() {
			return c.Image
		}
	}

	// Fallback: first container
	return pod.Spec.Containers[0].Image
}
