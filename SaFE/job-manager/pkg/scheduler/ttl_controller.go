/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"time"

	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

type WorkloadTTLController struct {
	client.Client
}

// SetupWorkloadTTLController initializes and registers the WorkloadTTLController with the controller manager.
func SetupWorkloadTTLController(mgr manager.Manager) error {
	r := &WorkloadTTLController{
		Client: mgr.GetClient(),
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Workload{}, builder.WithPredicates(WorkloadTTLChangePredicate{})).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Workload TTL Controller successfully")
	return nil
}

type WorkloadTTLChangePredicate struct {
	predicate.Funcs
}

// Create determines if a CreateEvent should trigger workload TTL reconciliation.
func (WorkloadTTLChangePredicate) Create(e event.CreateEvent) bool {
	workload, ok := e.Object.(*v1.Workload)
	if !ok {
		return false
	}
	if workload.IsEnd() || workload.GetTimeout() > 0 {
		return true
	}
	return false
}

// Update determines if an UpdateEvent should trigger workload TTL reconciliation.
func (WorkloadTTLChangePredicate) Update(e event.UpdateEvent) bool {
	oldWorkload, ok1 := e.ObjectOld.(*v1.Workload)
	newWorkload, ok2 := e.ObjectNew.(*v1.Workload)
	if !ok1 || !ok2 {
		return false
	}
	if !oldWorkload.IsEnd() && newWorkload.IsEnd() {
		return true
	}
	if !oldWorkload.IsTimeout() && newWorkload.IsTimeout() {
		return true
	}
	if oldWorkload.GetTimeout() != newWorkload.GetTimeout() {
		return true
	}
	if newWorkload.GetTimeout() > 0 && oldWorkload.Status.StartTime == nil && newWorkload.Status.StartTime != nil {
		return true
	}
	return false
}

// Reconcile is the main control loop for Workload TTL management.
func (r *WorkloadTTLController) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	startTime := time.Now().UTC()
	defer func() {
		klog.V(4).Infof("Finished reconcile workload-ttl %s cost (%v)", req.Name, time.Since(startTime))
	}()

	workload := new(v1.Workload)
	if err := r.Get(ctx, req.NamespacedName, workload); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if !workload.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, nil
	}
	return r.handle(ctx, workload)
}

// handle processes the TTL logic for workloads based on their state and TTL settings.
func (r *WorkloadTTLController) handle(ctx context.Context, workload *v1.Workload) (ctrlruntime.Result, error) {
	nowTime := time.Now().UTC()
	var err error
	result := ctrlruntime.Result{}

	switch {
	case workload.IsEnd():
		ttlSeconds := workload.GetTTLSecond()
		elapsedSeconds := ttlSeconds
		if workload.Status.EndTime != nil {
			elapsedSeconds = int(nowTime.Sub(workload.Status.EndTime.Time).Seconds())
		}
		if elapsedSeconds >= ttlSeconds {
			err = r.deleteWorkload(ctx, workload)
		} else {
			result.RequeueAfter = time.Duration(ttlSeconds-elapsedSeconds) * time.Second
		}
	case workload.IsTimeout():
		if err = jobutils.SetWorkloadTimeout(ctx, r.Client, workload, "the workload has timed out"); err != nil {
			break
		}
		err = r.deleteWorkload(ctx, workload)
	case workload.Status.StartTime == nil:
		break
	case workload.GetTimeout() > 0:
		timeoutStamp := workload.Status.StartTime.Add(time.Duration(workload.GetTimeout()) * time.Second)
		result.RequeueAfter = timeoutStamp.Sub(nowTime)
		klog.Infof("the workload %s will time out in %d seconds", workload.Name, int(result.RequeueAfter.Seconds()))
	}
	return result, err
}

// deleteWorkload deletes a workload that has exceeded its TTL.
func (r *WorkloadTTLController) deleteWorkload(ctx context.Context, workload *v1.Workload) error {
	err := r.Delete(ctx, workload)
	if err != nil {
		klog.ErrorS(err, "failed to delete workload", "workload", workload.Name)
		return client.IgnoreNotFound(err)
	}
	return nil
}
