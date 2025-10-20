/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

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
)

type JobTTLController struct {
	client.Client
}

// SetupJobTTLController initializes and registers the JobTTLController with the controller manager
func SetupJobTTLController(mgr manager.Manager) error {
	r := &JobTTLController{
		Client: mgr.GetClient(),
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, r.relevantChangePredicate()))).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup OpsJob TTL Controller successfully")
	return nil
}

// relevantChangePredicate defines which OpsJob changes should trigger TTL reconciliation
func (r *JobTTLController) relevantChangePredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldJob, ok1 := e.ObjectOld.(*v1.OpsJob)
			newJob, ok2 := e.ObjectNew.(*v1.OpsJob)
			if !ok1 || !ok2 {
				return false
			}
			if !oldJob.IsEnd() && newJob.IsEnd() && newJob.Spec.TTLSecondsAfterFinished != 0 {
				return true
			}
			return false
		},
	}
}

// Reconcile is the main control loop for OpsJob TTL management
func (r *JobTTLController) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	startTime := time.Now().UTC()
	defer func() {
		klog.V(4).Infof("Finished reconcile job-ttl %s cost (%v)", req.Name, time.Since(startTime))
	}()

	job := new(v1.OpsJob)
	if err := r.Get(ctx, req.NamespacedName, job); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if !job.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, nil
	}
	if !job.IsEnd() || job.Spec.TTLSecondsAfterFinished == 0 {
		return ctrlruntime.Result{}, nil
	}
	return r.deleteExpiredJob(ctx, job)
}

// deleteExpiredJob: deletes jobs that have exceeded their TTL seconds after completion
func (r *JobTTLController) deleteExpiredJob(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	nowTime := time.Now().Unix()
	elapsedSeconds := nowTime - job.Status.FinishedAt.Unix()
	var err error
	if elapsedSeconds >= int64(job.Spec.TTLSecondsAfterFinished) {
		if err = r.Delete(ctx, job); err != nil {
			klog.ErrorS(err, "failed to delete job")
			return ctrlruntime.Result{}, client.IgnoreNotFound(err)
		} else {
			klog.Infof("delete job by ttl controller, name: %s", job.Name)
		}
	} else {
		leftTime := int64(job.Spec.TTLSecondsAfterFinished) - elapsedSeconds
		return ctrlruntime.Result{RequeueAfter: time.Duration(leftTime) * time.Second}, nil
	}
	return ctrlruntime.Result{}, nil
}
