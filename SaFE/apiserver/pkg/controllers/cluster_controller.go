/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package controllers

import (
	"context"
	"fmt"
	"time"

	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commoncluster "github.com/AMD-AIG-AIMA/SAFE/common/pkg/cluster"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

const (
	// clusterClientFactoryRefreshInterval requeues ready clusters to rebuild invalid data-plane clients.
	clusterClientFactoryRefreshInterval = 15 * time.Second
)

type ClusterReconciler struct {
	ctx context.Context
	client.Client
}

// SetupClusterController sets up the cluster controller with the manager.
func SetupClusterController(ctx context.Context, mgr manager.Manager) error {
	r := &ClusterReconciler{
		Client: mgr.GetClient(),
		ctx:    ctx,
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Cluster{}, builder.WithPredicates(r.relevantChangePredicate())).
		Complete(r)
	if err != nil {
		return err
	}
	return nil
}

// relevantChangePredicate returns a predicate that filters cluster events for reconciliation.
// It triggers reconciliation when a cluster becomes ready, is being deleted, or transitions to ready state.
func (r *ClusterReconciler) relevantChangePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			cluster, ok := e.Object.(*v1.Cluster)
			if !ok || !cluster.IsReady() {
				return false
			}
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldCluster, ok1 := e.ObjectOld.(*v1.Cluster)
			newCluster, ok2 := e.ObjectNew.(*v1.Cluster)
			if !ok1 || !ok2 {
				return false
			}
			if (!oldCluster.IsReady() && newCluster.IsReady()) ||
				(oldCluster.GetDeletionTimestamp().IsZero() && !newCluster.GetDeletionTimestamp().IsZero()) {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			_, ok := e.Object.(*v1.Cluster)
			if !ok {
				return false
			}
			return true
		},
	}
}

// Reconcile implements the reconciliation loop.
func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	cluster := new(v1.Cluster)
	err := r.Get(ctx, req.NamespacedName, cluster)
	if err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if !cluster.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, r.deleteClientFactory(cluster)
	}
	if err = r.addClientFactory(ctx, cluster); err != nil {
		klog.Errorf("failed to add cluster clients, err: %v", err)
		return ctrlruntime.Result{}, err
	}
	if shouldPeriodicRefreshClientFactory(cluster) {
		return ctrlruntime.Result{RequeueAfter: clusterClientFactoryRefreshInterval}, nil
	}
	return ctrlruntime.Result{}, nil
}

// shouldPeriodicRefreshClientFactory reports whether reconcile should requeue to refresh clients.
func shouldPeriodicRefreshClientFactory(cluster *v1.Cluster) bool {
	return cluster != nil && cluster.GetDeletionTimestamp().IsZero() && cluster.IsReady()
}

// deleteClientFactory removes the Kubernetes client factory for a cluster being deleted.
// It cleans up the client manager and releases resources associated with the cluster.
func (r *ClusterReconciler) deleteClientFactory(cluster *v1.Cluster) error {
	mgr := commonutils.NewObjectManagerSingleton()
	if mgr == nil {
		return nil
	}
	if err := mgr.Delete(cluster.Name); err != nil {
		klog.Errorf("failed to delete cluster clients, err: %v", err)
		return err
	}
	klog.Infof("delete cluster %s clients", cluster.Name)
	return nil
}

// addClientFactory creates and registers a new Kubernetes client factory for a ready cluster.
func (r *ClusterReconciler) addClientFactory(ctx context.Context, cluster *v1.Cluster) error {
	if !cluster.IsReady() {
		return nil
	}
	clientManager := commonutils.NewObjectManagerSingleton()
	if clientManager == nil {
		return fmt.Errorf("failed to initialize cluster client manager for cluster %s", cluster.Name)
	}
	if obj, ok := clientManager.Get(cluster.Name); ok {
		if factory, ok := obj.(*commonclient.ClientFactory); ok &&
			!commoncluster.ClientFactoryNeedsRefresh(ctx, r.Client, cluster, factory) {
			return nil
		}
	}
	endpoint, err := commoncluster.GetEndpoint(ctx, r.Client, cluster)
	if err != nil {
		return err
	}
	k8sClientFactory, err := commoncluster.NewClientFactoryForCluster(ctx, r.Client, cluster,
		commonclient.DisableInformer)
	if err != nil {
		return err
	}
	clientManager.AddOrReplace(cluster.Name, k8sClientFactory)
	klog.Infof("add cluster %s clients, endpoint: %s, selected: %s",
		cluster.Name, endpoint, k8sClientFactory.Endpoint())
	return nil
}
