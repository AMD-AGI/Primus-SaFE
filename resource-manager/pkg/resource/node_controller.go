/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type NodeReconciler struct {
	client.Client
}

func SetupNodeController(mgr manager.Manager) error {
	r := &NodeReconciler{
		Client: mgr.GetClient(),
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Node{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Node Controller successfully")
	return nil
}

func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	startTime := time.Now().UTC()
	defer func() {
		klog.V(4).Infof("Finished node reconcile %s cost (%v)", req.Name, time.Since(startTime))
	}()
	return ctrlruntime.Result{}, nil
}
