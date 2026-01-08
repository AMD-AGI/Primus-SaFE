// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// AMD GPU resource name
	AMDGPUResourceName = "amd.com/gpu"
)

type WorkspaceReconciler struct {
	client *clientsets.K8SClientSet
}

func (r *WorkspaceReconciler) Init(ctx context.Context) error {
	// Get K8S client from ClusterManager
	clusterManager := clientsets.GetClusterManager()
	currentCluster := clusterManager.GetCurrentClusterClients()
	if currentCluster.K8SClientSet == nil {
		return fmt.Errorf("K8S client not initialized in ClusterManager")
	}
	r.client = currentCluster.K8SClientSet
	log.Info("WorkspaceReconciler initialized with K8S client")
	return nil
}

func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&primusSafeV1.Workspace{}).
		Complete(r)
}

func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req reconcile.Request) (result reconcile.Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic recovered: %v", r)
			log.Errorf("Panic in Reconcile for workspace %s: %v\nStack trace:\n%s",
				req.Name, r, string(debug.Stack()))
		}
	}()

	workspace := &primusSafeV1.Workspace{}
	err = r.client.ControllerRuntimeClient.Get(ctx, req.NamespacedName, workspace)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if workspace.DeletionTimestamp != nil {
		log.Infof("Workspace %s is being deleted, removing namespace info", workspace.Name)
		if !controllerutil.RemoveFinalizer(workspace, constant.PrimusLensGpuWorkloadExporterFinalizer) {
			return reconcile.Result{}, nil
		}
		finalizers := workspace.GetFinalizers()
		patchObj := map[string]any{
			"metadata": map[string]any{
				"resourceVersion": workspace.ResourceVersion,
				"finalizers":      finalizers,
			},
		}
		p, err := json.Marshal(patchObj)
		if err != nil {
			log.Errorf("Failed to marshal patch object for removing finalizer: %v", err)
			return reconcile.Result{}, err
		}
		if err = r.client.ControllerRuntimeClient.Patch(ctx, workspace, client.RawPatch(types.MergePatchType, p)); err != nil {
			log.Errorf("Failed to patch workspace for removing finalizer: %v", err)
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	// Add finalizer if not present
	if controllerutil.AddFinalizer(workspace, constant.PrimusLensGpuWorkloadExporterFinalizer) {
		// Use raw patch with resource version to add finalizer
		finalizers := workspace.GetFinalizers()
		patchObj := map[string]any{
			"metadata": map[string]any{
				"resourceVersion": workspace.ResourceVersion,
				"finalizers":      finalizers,
			},
		}
		p, err := json.Marshal(patchObj)
		if err != nil {
			log.Errorf("Failed to marshal patch object for adding finalizer: %v", err)
			return reconcile.Result{}, err
		}
		if err = r.client.ControllerRuntimeClient.Patch(ctx, workspace, client.RawPatch(types.MergePatchType, p)); err != nil {
			log.Errorf("Failed to patch workspace for adding finalizer: %v", err)
			return reconcile.Result{}, err
		}
	}

	// Save or update workspace to database
	err = r.saveWorkspaceToDB(ctx, workspace)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *WorkspaceReconciler) saveWorkspaceToDB(ctx context.Context, workspace *primusSafeV1.Workspace) error {
	log.Debugf("Saving workspace namespace info to DB: name=%s", workspace.Name)

	// Get cluster ID from workspace labels
	clusterID := primusSafeV1.GetClusterId(workspace)

	// Get the appropriate facade based on cluster ID
	var facade database.FacadeInterface
	if clusterID != "" {
		facade = database.GetFacadeForCluster(clusterID)
		log.Debugf("Using facade for cluster: %s", clusterID)
	} else {
		facade = database.GetFacade()
		log.Debug("Using default facade")
	}

	// Extract GPU resource count from workspace status
	gpuResource := r.extractGpuResource(workspace)
	if gpuResource == 0 {
		log.Debugf("Workspace %s has no GPU resource, skipping namespace info update", workspace.Name)
		return nil
	}

	// Get GPU model from the workspace
	gpuModel := r.getGpuModel(ctx, workspace, facade)

	// Check if namespace info already exists
	existingNamespaceInfo, err := facade.GetNamespaceInfo().GetByName(ctx, workspace.Name)
	if err != nil {
		log.Errorf("Failed to get existing namespace info for %s: %v", workspace.Name, err)
		return err
	}

	dbNamespaceInfo := &model.NamespaceInfo{
		Name:        workspace.Name,
		GpuModel:    gpuModel,
		GpuResource: gpuResource,
		UpdatedAt:   time.Now(),
	}

	if existingNamespaceInfo == nil {
		// Create new namespace info
		dbNamespaceInfo.CreatedAt = time.Now()
		log.Debugf("Creating new namespace_info record: name=%s, gpu_model=%s, gpu_resource=%d",
			workspace.Name, gpuModel, gpuResource)
		err = facade.GetNamespaceInfo().Create(ctx, dbNamespaceInfo)
		if err != nil {
			log.Errorf("Failed to create namespace_info for %s: %v", workspace.Name, err)
			return err
		}
		log.Infof("Successfully created namespace_info: name=%s, gpu_model=%s, gpu_resource=%d",
			workspace.Name, gpuModel, gpuResource)
	} else {
		// Update existing namespace info
		log.Debugf("Updating existing namespace_info record: name=%s, gpu_model=%s, gpu_resource=%d",
			workspace.Name, gpuModel, gpuResource)
		dbNamespaceInfo.ID = existingNamespaceInfo.ID
		dbNamespaceInfo.CreatedAt = existingNamespaceInfo.CreatedAt
		err = facade.GetNamespaceInfo().Update(ctx, dbNamespaceInfo)
		if err != nil {
			log.Errorf("Failed to update namespace_info for %s: %v", workspace.Name, err)
			return err
		}
		log.Debugf("Successfully updated namespace_info: name=%s, gpu_model=%s, gpu_resource=%d",
			workspace.Name, gpuModel, gpuResource)
	}

	return nil
}

func (r *WorkspaceReconciler) deleteNamespaceInfo(ctx context.Context, workspace *primusSafeV1.Workspace) error {
	// Get cluster ID from workspace labels
	clusterID := primusSafeV1.GetClusterId(workspace)

	// Get the appropriate facade based on cluster ID
	var facade database.FacadeInterface
	if clusterID != "" {
		facade = database.GetFacadeForCluster(clusterID)
		log.Debugf("Using facade for cluster: %s", clusterID)
	} else {
		facade = database.GetFacade()
		log.Debug("Using default facade")
	}

	log.Infof("Deleting namespace_info for workspace: %s", workspace.Name)
	err := facade.GetNamespaceInfo().DeleteByName(ctx, workspace.Name)
	if err != nil {
		log.Errorf("Failed to delete namespace_info for %s: %v", workspace.Name, err)
		return err
	}
	log.Infof("Successfully deleted namespace_info: name=%s", workspace.Name)
	return nil
}

// extractGpuResource extracts the GPU resource count from workspace status
func (r *WorkspaceReconciler) extractGpuResource(workspace *primusSafeV1.Workspace) int32 {
	if workspace.Status.TotalResources == nil {
		return 0
	}

	// Try to get AMD GPU resource
	if gpuQuantity, ok := workspace.Status.TotalResources[corev1.ResourceName(AMDGPUResourceName)]; ok {
		gpuCount := gpuQuantity.Value()
		log.Debugf("Extracted GPU resource for workspace %s: %d", workspace.Name, gpuCount)
		return int32(gpuCount)
	}

	// Try to get NVIDIA GPU resource as fallback
	if gpuQuantity, ok := workspace.Status.TotalResources["nvidia.com/gpu"]; ok {
		gpuCount := gpuQuantity.Value()
		log.Debugf("Extracted NVIDIA GPU resource for workspace %s: %d", workspace.Name, gpuCount)
		return int32(gpuCount)
	}

	return 0
}

// getGpuModel retrieves the GPU model for the workspace
// It attempts to get it from the workspace's node flavor by querying nodes in the workspace
func (r *WorkspaceReconciler) getGpuModel(ctx context.Context, workspace *primusSafeV1.Workspace, facade database.FacadeInterface) string {
	// If workspace has a node flavor specified, we can try to look up a node with that flavor
	// For now, we'll return a default or try to get from the first node in the workspace

	// Try to get GPU model from workspace node flavor label or annotation
	// This is a simplified approach - in a real scenario you might need to:
	// 1. Query nodes that belong to this workspace
	// 2. Get GPU model from node labels or from database

	// For now, return empty string and let it be populated when we have node information
	// Or we can use a default based on the cluster configuration
	gpuModel := ""

	// Try to infer from node flavor if available
	if workspace.Spec.NodeFlavor != "" {
		// In a real implementation, you would map node flavor to GPU model
		// For now, we'll use a placeholder
		gpuModel = workspace.Spec.NodeFlavor
		log.Debugf("Using node flavor as GPU model for workspace %s: %s", workspace.Name, gpuModel)
	}

	return gpuModel
}

// calculateGpuResource calculates total GPU resource from workspace spec and status
func (r *WorkspaceReconciler) calculateGpuResource(workspace *primusSafeV1.Workspace) int32 {
	// First try to get from status.TotalResources
	if workspace.Status.TotalResources != nil {
		if gpuQuantity, ok := workspace.Status.TotalResources[corev1.ResourceName(AMDGPUResourceName)]; ok {
			return int32(gpuQuantity.Value())
		}
		if gpuQuantity, ok := workspace.Status.TotalResources["nvidia.com/gpu"]; ok {
			return int32(gpuQuantity.Value())
		}
	}

	// If not in status, try to calculate from spec if possible
	// This would depend on your specific workspace implementation
	// For example, if you have replica count and GPUs per node in the spec

	return 0
}

// Helper function to parse GPU resource quantity
func parseGpuQuantity(q resource.Quantity) int32 {
	return int32(q.Value())
}
