// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package reconciler

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/github-runners-exporter/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// AutoScalingRunnerSetReconciler watches AutoScalingRunnerSet resources and syncs to database
type AutoScalingRunnerSetReconciler struct {
	client        *clientsets.K8SClientSet
	dynamicClient dynamic.Interface
}

// NewAutoScalingRunnerSetReconciler creates a new reconciler
func NewAutoScalingRunnerSetReconciler() *AutoScalingRunnerSetReconciler {
	return &AutoScalingRunnerSetReconciler{}
}

// Init initializes the reconciler with required clients
func (r *AutoScalingRunnerSetReconciler) Init(ctx context.Context) error {
	clusterManager := clientsets.GetClusterManager()
	currentCluster := clusterManager.GetCurrentClusterClients()
	if currentCluster.K8SClientSet == nil {
		return fmt.Errorf("K8S client not initialized in ClusterManager")
	}
	r.client = currentCluster.K8SClientSet

	if r.client.Dynamic == nil {
		return fmt.Errorf("dynamic client not initialized in K8SClientSet")
	}
	r.dynamicClient = r.client.Dynamic

	log.Info("AutoScalingRunnerSetReconciler initialized")
	return nil
}

// SetupWithManager sets up the controller with the Manager
func (r *AutoScalingRunnerSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create an unstructured object for AutoScalingRunnerSet
	arsExample := &unstructured.Unstructured{}
	arsExample.SetGroupVersionKind(types.AutoScalingRunnerSetGVK)

	// Use Watches with unstructured object
	return ctrl.NewControllerManagedBy(mgr).
		Named("autoscaling-runner-set-controller").
		For(arsExample).
		Complete(r)
}

// Reconcile handles AutoScalingRunnerSet events
func (r *AutoScalingRunnerSetReconciler) Reconcile(ctx context.Context, req reconcile.Request) (result reconcile.Result, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic recovered: %v", rec)
			log.Errorf("Panic in AutoScalingRunnerSetReconciler for %s/%s: %v\nStack trace:\n%s",
				req.Namespace, req.Name, rec, string(debug.Stack()))
		}
	}()

	log.Debugf("AutoScalingRunnerSetReconciler: reconciling %s/%s", req.Namespace, req.Name)

	// Get the AutoScalingRunnerSet
	obj, err := r.dynamicClient.Resource(types.AutoScalingRunnerSetGVR).
		Namespace(req.Namespace).
		Get(ctx, req.Name, metav1.GetOptions{})
	if err != nil {
		// Check if not found - resource was deleted
		if client.IgnoreNotFound(err) == nil {
			log.Infof("AutoScalingRunnerSetReconciler: %s/%s deleted, marking as inactive", req.Namespace, req.Name)
			return r.handleDelete(ctx, req)
		}
		log.Errorf("AutoScalingRunnerSetReconciler: failed to get %s/%s: %v", req.Namespace, req.Name, err)
		return ctrl.Result{}, err
	}

	// Parse the object
	info := types.ParseAutoScalingRunnerSet(obj)

	// Sync to database
	if err := r.syncToDatabase(ctx, info, obj.GetDeletionTimestamp() != nil); err != nil {
		log.Errorf("AutoScalingRunnerSetReconciler: failed to sync %s/%s to database: %v",
			req.Namespace, req.Name, err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	log.Infof("AutoScalingRunnerSetReconciler: successfully synced %s/%s (owner: %s, repo: %s)",
		req.Namespace, req.Name, info.GithubOwner, info.GithubRepo)

	return ctrl.Result{}, nil
}

// handleDelete handles the deletion of an AutoScalingRunnerSet
func (r *AutoScalingRunnerSetReconciler) handleDelete(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	facade := database.GetFacade().GetGithubRunnerSet()

	// Find by namespace and name
	runnerSet, err := facade.GetByNamespaceName(ctx, req.Namespace, req.Name)
	if err != nil {
		log.Errorf("AutoScalingRunnerSetReconciler: failed to find runner set %s/%s: %v",
			req.Namespace, req.Name, err)
		return ctrl.Result{}, err
	}

	if runnerSet == nil {
		// Already deleted from database
		return ctrl.Result{}, nil
	}

	// Mark as deleted
	runnerSet.Status = model.RunnerSetStatusDeleted
	runnerSet.UpdatedAt = time.Now()

	if err := facade.Upsert(ctx, runnerSet); err != nil {
		log.Errorf("AutoScalingRunnerSetReconciler: failed to mark %s/%s as deleted: %v",
			req.Namespace, req.Name, err)
		return ctrl.Result{}, err
	}

	log.Infof("AutoScalingRunnerSetReconciler: marked %s/%s as deleted", req.Namespace, req.Name)
	return ctrl.Result{}, nil
}

// syncToDatabase syncs the AutoScalingRunnerSet info to database
func (r *AutoScalingRunnerSetReconciler) syncToDatabase(ctx context.Context, info *types.AutoScalingRunnerSetInfo, isDeleting bool) error {
	facade := database.GetFacade().GetGithubRunnerSet()

	status := model.RunnerSetStatusActive
	if isDeleting {
		status = model.RunnerSetStatusDeleted
	}

	runnerSet := &model.GithubRunnerSets{
		UID:                info.UID,
		Name:               info.Name,
		Namespace:          info.Namespace,
		GithubConfigURL:    info.GithubConfigURL,
		GithubConfigSecret: info.GithubConfigSecret,
		RunnerGroup:        info.RunnerGroup,
		GithubOwner:        info.GithubOwner,
		GithubRepo:         info.GithubRepo,
		MinRunners:         info.MinRunners,
		MaxRunners:         info.MaxRunners,
		CurrentRunners:     info.CurrentRunners,
		DesiredRunners:     info.DesiredRunners,
		Status:             status,
		LastSyncAt:         time.Now(),
	}

	return facade.Upsert(ctx, runnerSet)
}

