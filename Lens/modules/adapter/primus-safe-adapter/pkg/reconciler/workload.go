// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	primusSafeConstant "github.com/AMD-AGI/Primus-SaFE/Lens/primus-safe-adapter/pkg/constant"
	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type WorkloadReconciler struct {
	client             *clientsets.K8SClientSet
	frameworkDetection *FrameworkDetectionIntegration
}

func (r *WorkloadReconciler) Init(ctx context.Context) error {
	// Get K8S client from ClusterManager
	clusterManager := clientsets.GetClusterManager()
	currentCluster := clusterManager.GetCurrentClusterClients()
	if currentCluster.K8SClientSet == nil {
		return fmt.Errorf("K8S client not initialized in ClusterManager")
	}
	r.client = currentCluster.K8SClientSet
	log.Info("WorkloadReconciler initialized with K8S client")

	// Initialize framework detection integration
	facade := database.GetFacade()
	frameworkDetection, err := NewFrameworkDetectionIntegration(facade.GetAiWorkloadMetadata())
	if err != nil {
		log.Errorf("Failed to initialize framework detection: %v", err)
		// Don't fail, continue with degraded functionality
	} else {
		r.frameworkDetection = frameworkDetection
		log.Info("Framework detection integration initialized")
	}

	return nil
}

func (r *WorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&primusSafeV1.Workload{}).
		Complete(r)
}

func (r *WorkloadReconciler) Reconcile(ctx context.Context, req reconcile.Request) (result reconcile.Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic recovered: %v", r)
			log.Errorf("Panic in Reconcile for workload %s/%s: %v\nStack trace:\n%s",
				req.Namespace, req.Name, r, string(debug.Stack()))
		}
	}()

	workload := &primusSafeV1.Workload{}
	err = r.client.ControllerRuntimeClient.Get(ctx, req.NamespacedName, workload)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	err = r.saveWorkloadToDB(ctx, workload)
	if err != nil {
		return reconcile.Result{}, err
	}

	if workload.DeletionTimestamp != nil {
		if !controllerutil.RemoveFinalizer(workload, constant.PrimusLensGpuWorkloadExporterFinalizer) {
			return reconcile.Result{}, nil
		}
		finalizers := workload.GetFinalizers()
		patchObj := map[string]any{
			"metadata": map[string]any{
				"resourceVersion": workload.ResourceVersion,
				"finalizers":      finalizers,
			},
		}
		p, err := json.Marshal(patchObj)
		if err != nil {
			log.Errorf("Failed to marshal patch object for removing finalizer: %v", err)
			return reconcile.Result{}, err
		}
		if err = r.client.ControllerRuntimeClient.Patch(ctx, workload, client.RawPatch(types.MergePatchType, p)); err != nil {
			log.Errorf("Failed to patch workload for removing finalizer: %v", err)
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}
	if controllerutil.AddFinalizer(workload, constant.PrimusLensGpuWorkloadExporterFinalizer) {
		// Use raw patch with resource version to add finalizer
		finalizers := workload.GetFinalizers()
		patchObj := map[string]any{
			"metadata": map[string]any{
				"resourceVersion": workload.ResourceVersion,
				"finalizers":      finalizers,
			},
		}
		p, err := json.Marshal(patchObj)
		if err != nil {
			log.Errorf("Failed to marshal patch object for adding finalizer: %v", err)
			return reconcile.Result{}, err
		}
		if err = r.client.ControllerRuntimeClient.Patch(ctx, workload, client.RawPatch(types.MergePatchType, p)); err != nil {
			log.Errorf("Failed to patch workload for adding finalizer: %v", err)
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *WorkloadReconciler) calculateGpuRequest(workload *primusSafeV1.Workload) int32 {
	totalGpu := 0
	for _, res := range workload.Spec.Resources {
		n, err := strconv.Atoi(res.GPU)
		if err != nil {
			return 0
		}
		totalGpu += n * res.Replica
	}
	return int32(totalGpu)
}

func (r *WorkloadReconciler) saveWorkloadToDB(ctx context.Context, workload *primusSafeV1.Workload) error {
	log.Debugf("Saving workload to DB: namespace=%s, name=%s, uid=%s", workload.Namespace, workload.Name, workload.UID)

	// Get cluster ID from workload labels
	clusterID := primusSafeV1.GetClusterId(workload)

	// Get the appropriate facade based on cluster ID
	var facade database.FacadeInterface
	if clusterID != "" {
		facade = database.GetFacadeForCluster(clusterID)
		log.Debugf("Using facade for cluster: %s", clusterID)
	} else {
		facade = database.GetFacade()
		log.Debug("Using default facade")
	}

	existWorkload, err := facade.GetWorkload().GetGpuWorkloadByUid(ctx, string(workload.UID))
	if err != nil {
		log.Errorf("Failed to get existing workload by uid %s: %v", workload.UID, err)
		return err
	}
	dbWorkload := &model.GpuWorkload{
		GroupVersion: "amd.com/v1",
		Kind:         "Workload",
		Namespace:    workload.Spec.Workspace,
		Name:         workload.Name,
		UID:          string(workload.UID),
		GpuRequest:   r.calculateGpuRequest(workload),
		CreatedAt:    workload.CreationTimestamp.Time,
		UpdatedAt:    time.Now(),
		Labels:       map[string]interface{}{},
		Source:       constant.ContainerSourceK8S,
		Status:       metadata.WorkloadStatusRunning,
		Annotations:  map[string]interface{}{},
	}
	switch workload.Status.Phase {
	case primusSafeV1.WorkloadPending:
		dbWorkload.Status = metadata.WorkloadStatusPending
	case primusSafeV1.WorkloadRunning:
		dbWorkload.Status = metadata.WorkloadStatusRunning
	case primusSafeV1.WorkloadSucceeded:
		dbWorkload.Status = metadata.WorkloadStatusDone
	case primusSafeV1.WorkloadFailed:
		dbWorkload.Status = metadata.WorkloadStatusFailed
	}

	for key, value := range workload.Labels {
		if primusSafeConstant.WorkloadDispatchCountLabel == key {
			count, _ := strconv.Atoi(value)
			dbWorkload.Labels[key] = count
		} else {
			dbWorkload.Labels[key] = value
		}
	}
	for key, value := range workload.Annotations {
		dbWorkload.Annotations[key] = value
	}

	if workload.DeletionTimestamp != nil {
		dbWorkload.Status = metadata.WorkloadStatusDone
		dbWorkload.EndAt = workload.DeletionTimestamp.Time
		log.Debugf("Workload %s/%s is being deleted, status set to Done", workload.Namespace, workload.Name)
	}
	if existWorkload == nil {
		log.Debugf("Creating new gpu_workload record: name=%s, uid=%s", workload.Name, workload.UID)
		err = facade.GetWorkload().CreateGpuWorkload(ctx, dbWorkload)
		if err != nil {
			log.Errorf("Failed to create gpu_workload %s/%s: %v", workload.Namespace, workload.Name, err)
			return err
		}
		log.Infof("Successfully created gpu_workload: name=%s, uid=%s", workload.Name, workload.UID)
	} else {
		log.Debugf("Updating existing gpu_workload record: name=%s, uid=%s, id=%d", workload.Name, workload.UID, existWorkload.ID)
		dbWorkload.ID = existWorkload.ID
		err = facade.GetWorkload().UpdateGpuWorkload(ctx, dbWorkload)
		if err != nil {
			log.Errorf("Failed to update gpu_workload %s/%s: %v", workload.Namespace, workload.Name, err)
			return err
		}
		log.Debugf("Successfully updated gpu_workload: name=%s, uid=%s", workload.Name, workload.UID)
	}

	// Link this Workload as the parent workload for related gpu_workloads
	return r.linkChildrenWorkloads(ctx, workload, facade)
}

func (r *WorkloadReconciler) linkChildrenWorkloads(ctx context.Context, workload *primusSafeV1.Workload, facade database.FacadeInterface) error {

	// Find all gpu_workloads with label "primus-safe.workload.id" = workload.Name
	childWorkloads, err := facade.GetWorkload().ListWorkloadByLabelValue(ctx, primusSafeConstant.WorkloadIdLabel, workload.Name)
	if err != nil {
		log.Errorf("Failed to list child workloads for parent %s: %v", workload.Name, err)
		return err
	}

	// Return directly if no child workloads found
	if len(childWorkloads) == 0 {
		log.Debugf("No child workloads found for parent workload: %s", workload.Name)
		return nil
	}

	// Set the parent_uid of found child workloads to current Workload's UID
	for _, child := range childWorkloads {
		// Only update workloads that don't have parent_uid set yet
		if child.ParentUID == "" {
			log.Debugf("Linking child workload: name=%s, uid=%s to parent uid=%s", child.Name, child.UID, workload.UID)
			child.ParentUID = string(workload.UID)
			err = facade.GetWorkload().UpdateGpuWorkload(ctx, child)
			if err != nil {
				// Log error but continue processing other child workloads
				log.Errorf("Failed to update parent_uid for child workload %s/%s (uid=%s): %v",
					child.Namespace, child.Name, child.UID, err)
				continue
			}
		}
	}

	return nil
}
