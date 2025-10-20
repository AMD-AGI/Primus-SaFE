/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"fmt"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type WorkloadFlowController struct {
	client.Client
}

func SetupWorkloadFlowController(ctx context.Context, mgr manager.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(ctx, &v1.Workload{}, "spec.dependencies", func(object client.Object) []string {
		workload := object.(*v1.Workload)
		if len(workload.Spec.Dependencies) == 0 {
			return nil
		}
		return workload.Spec.Dependencies
	}); err != nil {
		return fmt.Errorf("failed to setup field indexer for workload dependencies: %v", err)
	}

	r := &WorkloadFlowController{
		Client: mgr.GetClient(),
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Workload{}, builder.WithPredicates(CaredFlowChangePredicate{})).
		Watches(&v1.Workload{}, r.enqueueDependents(), builder.WithPredicates(CaredFlowChangePredicate{})).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Workload Flow Controller successfully")
	return nil
}

type CaredFlowChangePredicate struct {
	predicate.Funcs
}

func (CaredFlowChangePredicate) Create(e event.CreateEvent) bool {
	workload, ok := e.Object.(*v1.Workload)
	if !ok {
		return false
	}
	if workload.IsEnd() {
		return true
	}
	return false
}

func (CaredFlowChangePredicate) Update(e event.UpdateEvent) bool {
	oldWorkload, ok1 := e.ObjectOld.(*v1.Workload)
	newWorkload, ok2 := e.ObjectNew.(*v1.Workload)
	if !ok1 || !ok2 {
		return false
	}
	if !oldWorkload.IsEnd() && newWorkload.IsEnd() {
		return true
	}
	return false
}

func (r *WorkloadFlowController) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	return ctrlruntime.Result{}, nil
}

func (r *WorkloadFlowController) enqueueDependents() handler.EventHandler {
	return handler.Funcs{
		UpdateFunc: func(ctx context.Context, e event.TypedUpdateEvent[client.Object], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			oldWorkload, ok1 := e.ObjectOld.(*v1.Workload)
			newWorkload, ok2 := e.ObjectNew.(*v1.Workload)
			if !ok1 || !ok2 {
				return
			}
			if !oldWorkload.IsEnd() && newWorkload.IsEnd() {
				var dependents v1.WorkloadList
				if err := r.List(ctx, &dependents, client.MatchingFields{"spec.dependencies": newWorkload.Name}); err != nil {
					klog.Errorf("failed to list dependencies for workload %s: %v", newWorkload.Name, err)
					return
				}
				for _, dep := range dependents.Items {
					isAddQueue, err := r.setPhase(ctx, newWorkload, dep.Name)
					if err != nil {
						klog.Errorf("failed to set dependency phase for workload %s in dependent workload %s: %v", newWorkload.Name, dep.Name, err)
						continue
					}
					if isAddQueue {
						depName := reconcile.Request{NamespacedName: types.NamespacedName{Name: dep.Name}}
						klog.Infof("enqueue dependent workload %s of workload %s", depName, newWorkload.Name)
						w.Add(depName)
					}
				}
				return
			}
		},
	}
}

func (r *WorkloadFlowController) setPhase(ctx context.Context, workload *v1.Workload, depWorkloadId string) (bool, error) {
	depWorkload := new(v1.Workload)
	if err := r.Get(context.Background(), types.NamespacedName{Name: depWorkloadId}, depWorkload); err != nil {
		return true, err
	}
	depWorkload.Status.DependenciesPhase[workload.Name] = workload.Status.Phase
	if workload.Status.Phase != v1.WorkloadSucceeded {
		if err := utils.SetWorkloadFailed(ctx, r.Client, depWorkload, fmt.Sprintf("dependency workload %s failed", workload.Name)); err != nil {
			return true, err
		}
		return false, nil
	}

	if err := r.Status().Update(context.Background(), depWorkload); err != nil {
		return true, err
	}

	return true, nil
}
