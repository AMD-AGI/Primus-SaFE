/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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
	client.Client
}

func SetupClusterController(mgr manager.Manager) error {
	r := &ClusterReconciler{
		Client: mgr.GetClient(),
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Cluster{}, builder.WithPredicates(r.CaredPredicate())).
		Complete(r)
	if err != nil {
		return err
	}
	return nil
}

func (r *ClusterReconciler) CaredPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldCluster, ok1 := e.ObjectOld.(*v1.Cluster)
			newCluster, ok2 := e.ObjectNew.(*v1.Cluster)
			if !ok1 || !ok2 {
				return false
			}
			if !oldCluster.IsReady() && newCluster.IsReady() ||
				!oldCluster.IsControlPlaneCertEqual(newCluster.Status.ControlPlaneStatus) ||
				!oldCluster.IsControlPlaneEndpointEqual(newCluster.Status.ControlPlaneStatus.Endpoints) {
				if err := r.addClientFactory(context.Background(), newCluster); err != nil {
					klog.Errorf("failed to add cluster, err: %v", err)
				}
			} else if oldCluster.IsReady() &&
				(!newCluster.IsReady() || !newCluster.GetDeletionTimestamp().IsZero()) {
				if mgr := commonutils.NewObjectManagerSingleton(); mgr != nil {
					if err := mgr.Delete(newCluster.Name); err != nil {
						klog.Errorf("failed to delete cluster clients, err: %v", err)
					}
				}
			}
			return false
		},
	}
}

func (r *ClusterReconciler) Reconcile(_ context.Context, _ ctrlruntime.Request) (ctrlruntime.Result, error) {
	return ctrlruntime.Result{}, nil
}

func (r *ClusterReconciler) addClientFactory(ctx context.Context, c *v1.Cluster) error {
	if !c.IsReady() {
		return fmt.Errorf("cluster %s is not ready", c.Name)
	}
	clientManager := commonutils.NewObjectManagerSingleton()
	if clientManager == nil {
		return fmt.Errorf("failed to new clients manager")
	}
	controlPlane := &c.Status.ControlPlaneStatus
	endpoint, err := commoncluster.GetEndpoint(ctx, r.Client, c.Name, controlPlane.Endpoints)
	if err != nil {
		return err
	}

	k8sClientFactory, err := commonclient.NewClientFactory(ctx, c.Name, endpoint,
		controlPlane.CertData, controlPlane.KeyData, controlPlane.CAData, commonclient.DisableInformer)
	if err != nil {
		return err
	}
	clientManager.AddOrReplace(c.Name, k8sClientFactory)
	return nil
}
