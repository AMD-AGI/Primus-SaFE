/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package controllers

import (
	"context"
	"fmt"

	"k8s.io/client-go/dynamic"
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
				if err := r.addClusterInfo(context.Background(), newCluster); err != nil {
					klog.Errorf("failed to add cluster, err: %v", err)
				}
			} else if oldCluster.IsReady() &&
				(!newCluster.IsReady() || !newCluster.GetDeletionTimestamp().IsZero()) {
				if cm := NewClusterManager(); cm != nil {
					cm.Delete(newCluster.Name)
				}
			}
			return false
		},
	}
}

func (r *ClusterReconciler) Reconcile(_ context.Context, _ ctrlruntime.Request) (ctrlruntime.Result, error) {
	return ctrlruntime.Result{}, nil
}

func (r *ClusterReconciler) addClusterInfo(ctx context.Context, c *v1.Cluster) error {
	if !c.IsReady() {
		return fmt.Errorf("cluster %s is not ready", c.Name)
	}
	cm := NewClusterManager()
	if cm == nil {
		return fmt.Errorf("failed to new cluster manager ")
	}
	controlPlane := &c.Status.ControlPlaneStatus

	endpoint, err := commoncluster.GetClusterEndpoint(ctx, r.Client, c.Name, controlPlane.Endpoints)
	if err != nil {
		return err
	}
	clientSet, restCfg, err := commonclient.NewClientSet(endpoint,
		controlPlane.CertData, controlPlane.KeyData, controlPlane.CAData, true)
	if err != nil {
		klog.ErrorS(err, "fail to new clientSet", "cluster", c.Name)
		return err
	}
	dynamicClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		klog.ErrorS(err, "fail to new dynamic client", "cluster", c.Name)
		return err
	}
	cm.Add(c.Name, &ClusterInfo{
		ControlPlane:  *controlPlane,
		ClientSet:     clientSet,
		DynamicClient: dynamicClient,
	})
	return nil
}
