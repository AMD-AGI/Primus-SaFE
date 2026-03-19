/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	githubpkg "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/github"
	rmutils "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

var ephemeralRunnerGVR = schema.GroupVersionResource{
	Group:    "actions.github.com",
	Version:  "v1alpha1",
	Resource: "ephemeralrunners",
}

type GitHubWorkflowReconciler struct {
	client.Client
	tracker       *githubpkg.WorkflowTracker
	clientManager *commonutils.ObjectManager
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
		Client:        mgr.GetClient(),
		tracker:       tracker,
		clientManager: commonutils.NewObjectManagerSingleton(),
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
	klog.Info("[github-workflow] controller registered + sync job started")
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

	cluster := v1.GetClusterId(wl)
	isCompleted := wl.IsEnd()
	klog.Infof("[github-workflow] reconcile: workload=%s cluster=%s workspace=%s completed=%v",
		wl.Name, cluster, wl.Spec.Workspace, isCompleted)

	obj, retryable := r.fetchEphemeralRunner(ctx, cluster, wl)
	if obj == nil {
		if retryable {
			return ctrlruntime.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return ctrlruntime.Result{}, nil
	}

	klog.Infof("[github-workflow] tracked: workload=%s annotations=%v", wl.Name, obj.GetAnnotations())
	r.tracker.OnEphemeralRunnerEvent(ctx, obj, wl.Name, cluster, isCompleted)
	return ctrlruntime.Result{}, nil
}

func (r *GitHubWorkflowReconciler) fetchEphemeralRunner(ctx context.Context, cluster string, wl *v1.Workload) (*unstructured.Unstructured, bool) {
	k8sClients, err := rmutils.GetK8sClientFactory(r.clientManager, cluster)
	if err != nil {
		klog.Infof("[github-workflow] no client for cluster %s (will retry): %v", cluster, err)
		return nil, true
	}

	dynClient := k8sClients.DynamicClient()
	if dynClient == nil {
		klog.Infof("[github-workflow] no dynamic client for cluster %s (will retry)", cluster)
		return nil, true
	}

	obj, err := dynClient.Resource(ephemeralRunnerGVR).
		Namespace(wl.Spec.Workspace).
		Get(ctx, wl.Name, metav1.GetOptions{})
	if err != nil {
		klog.Infof("[github-workflow] cannot get EphemeralRunner %s/%s in cluster %s: %v",
			wl.Spec.Workspace, wl.Name, cluster, err)
		return nil, false
	}

	return obj, false
}
