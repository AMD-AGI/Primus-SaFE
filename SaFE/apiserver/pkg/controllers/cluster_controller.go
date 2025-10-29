/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package controllers

import (
	"context"
	"fmt"

	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commoncluster "github.com/AMD-AIG-AIMA/SAFE/common/pkg/cluster"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

type ClusterReconciler struct {
	ctx context.Context
	client.Client
}

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

func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	cluster := new(v1.Cluster)
	err := r.Get(ctx, req.NamespacedName, cluster)
	if err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if !cluster.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, r.deleteClientFactory(cluster)
	}
	if err = r.addClientFactory(r.ctx, cluster); err != nil {
		klog.Errorf("failed to add cluster clients, err: %v", err)
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, nil
}

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

func (r *ClusterReconciler) addClientFactory(ctx context.Context, cluster *v1.Cluster) error {
	if !cluster.IsReady() {
		return nil
	}
	clientManager := commonutils.NewObjectManagerSingleton()
	if clientManager == nil {
		return fmt.Errorf("failed to initialize cluster client manager for cluster %s", cluster.Name)
	}
	if clientManager.Has(cluster.Name) {
		return nil
	}
	endpoint, err := commoncluster.GetEndpoint(ctx, r.Client, cluster)
	if err != nil {
		return err
	}

	controlPlane := &cluster.Status.ControlPlaneStatus
	k8sClientFactory, err := commonclient.NewClientFactory(ctx, cluster.Name, endpoint,
		controlPlane.CertData, controlPlane.KeyData, controlPlane.CAData, commonclient.DisableInformer)
	if err != nil {
		return err
	}
	clientManager.AddOrReplace(cluster.Name, k8sClientFactory)
	klog.Infof("add cluster %s clients", cluster.Name)
	return nil
}
