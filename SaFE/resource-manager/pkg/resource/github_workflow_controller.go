/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	githubpkg "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/github"
)

type GitHubWorkflowReconciler struct {
	client.Client
	tracker *githubpkg.WorkflowTracker
}

func SetupGitHubWorkflowController(mgr manager.Manager) error {
	if !commonconfig.IsDBEnable() {
		klog.Info("[github-workflow] DB not enabled, controller disabled")
		return nil
	}
	if !commonconfig.IsCICDEnable() {
		klog.Info("[github-workflow] CI/CD not enabled, controller disabled")
		return nil
	}

	gormDB, err := dbclient.NewClient().GetGormDB()
	if err != nil {
		klog.Warningf("[github-workflow] DB init failed, controller disabled: %v", err)
		return nil
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		klog.Warningf("[github-workflow] sql.DB failed, controller disabled: %v", err)
		return nil
	}

	store := githubpkg.NewStore(sqlDB)
	tracker := githubpkg.NewWorkflowTracker(store)

	syncJob := githubpkg.NewSyncJob(store, 20, 30*time.Second)
	go syncJob.Start(context.Background())

	r := &GitHubWorkflowReconciler{
		Client:  mgr.GetClient(),
		tracker: tracker,
	}

	err = ctrlruntime.NewControllerManagedBy(mgr).
		Named("github-workflow").
		For(&v1.Workload{}, builder.WithPredicates(predicate.Funcs{
			CreateFunc:  func(e event.CreateEvent) bool { return isEphemeralRunnerWorkload(e.Object) },
			UpdateFunc:  func(e event.UpdateEvent) bool { return isEphemeralRunnerWorkload(e.ObjectNew) },
			DeleteFunc:  func(e event.DeleteEvent) bool { return isEphemeralRunnerWorkload(e.Object) },
			GenericFunc: func(e event.GenericEvent) bool { return false },
		})).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("[github-workflow] controller registered + sync job started (cicd=%v)", commonconfig.IsCICDEnable())
	return nil
}

func isEphemeralRunnerWorkload(obj client.Object) bool {
	wl, ok := obj.(*v1.Workload)
	if !ok {
		return false
	}
	return wl.Spec.GroupVersionKind.Kind == common.CICDEphemeralRunnerKind
}

func (r *GitHubWorkflowReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	defer func() {
		if rec := recover(); rec != nil {
			klog.Errorf("[github-workflow] panic recovered: %v", rec)
		}
	}()

	wl := &v1.Workload{}
	if err := r.Get(ctx, req.NamespacedName, wl); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}

	if wl.Spec.GroupVersionKind.Kind != common.CICDEphemeralRunnerKind {
		return ctrlruntime.Result{}, nil
	}

	annotations := wl.GetAnnotations()
	if annotations == nil {
		return ctrlruntime.Result{}, nil
	}

	// GitHub annotations are synced from EphemeralRunner by job-manager.
	// If run-id is not present yet, skip (will be called again when job-manager syncs it).
	runID := annotations["actions.github.com/run-id"]
	if runID == "" {
		return ctrlruntime.Result{}, nil
	}

	cluster := v1.GetClusterId(wl)
	isCompleted := wl.IsEnd()

	// Build an unstructured object with the Workload's annotations for the tracker
	obj := &unstructured.Unstructured{}
	obj.SetAnnotations(annotations)
	obj.SetLabels(wl.GetLabels())
	obj.SetName(wl.Name)
	obj.SetNamespace(wl.Namespace)

	klog.Infof("[github-workflow] processing workload=%s run-id=%s cluster=%s completed=%v",
		wl.Name, runID, cluster, isCompleted)

	r.tracker.OnEphemeralRunnerEvent(ctx, obj, wl.Name, cluster, isCompleted)
	return ctrlruntime.Result{}, nil
}
