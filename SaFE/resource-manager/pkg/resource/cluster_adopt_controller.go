/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"

	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	rmmetrics "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/metrics"
)

// ClusterAdoptReconciler records an existing data plane on the admin plane.
// It creates NodeFlavor, Workspace and Node objects and does not install Kubernetes.
type ClusterAdoptReconciler struct {
	*ClusterReconciler
}

// SetupClusterAdoptController registers the reconciler for adopted clusters.
func SetupClusterAdoptController(mgr manager.Manager) error {
	baseReconciler, err := newClusterBaseReconciler(mgr)
	if err != nil {
		return err
	}
	r := &ClusterAdoptReconciler{ClusterReconciler: &ClusterReconciler{
		ClusterBaseReconciler: baseReconciler,
		clientManager:         commonutils.NewObjectManagerSingleton(),
	}}
	if r.clientManager == nil {
		return err
	}
	err = ctrlruntime.NewControllerManagedBy(mgr).
		Named("cluster-adopt").
		For(&v1.Cluster{}, builder.WithPredicates(adoptedClusterPredicate())).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Cluster Adopt Controller successfully")
	return nil
}

// Reconcile registers flavors, workspaces and nodes from a connected adopted cluster.
func (r *ClusterAdoptReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	cluster := &v1.Cluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if err := r.adoptClusterContents(ctx, cluster); err != nil {
		klog.ErrorS(err, "failed to adopt cluster contents", "cluster", cluster.Name)
		rmmetrics.ClusterReconcileErrorsTotal.WithLabelValues("adopt_cluster").Inc()
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, nil
}

// adoptedClusterPredicate admits clusters marked for adoption.
func adoptedClusterPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			cluster, ok := e.Object.(*v1.Cluster)
			return ok && isAdoptedClusterPending(cluster)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			cluster, ok := e.ObjectNew.(*v1.Cluster)
			return ok && isAdoptedClusterPending(cluster)
		},
		DeleteFunc: func(event.DeleteEvent) bool { return false },
	}
}
