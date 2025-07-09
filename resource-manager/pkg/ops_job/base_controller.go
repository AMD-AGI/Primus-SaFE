/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/resource"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

type ReconcilerComponent interface {
	observe(ctx context.Context, job *v1.OpsJob) (bool, error)
	filter(ctx context.Context, job *v1.OpsJob) bool
	handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error)
}

type ClearFunc func(ctx context.Context, job *v1.OpsJob) error

type OpsJobBaseReconciler struct {
	client.Client
}

func (r *OpsJobBaseReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request,
	component ReconcilerComponent, clears ...ClearFunc) (ctrlruntime.Result, error) {
	startTime := time.Now().UTC()
	defer func() {
		klog.V(4).Infof("Finished reconcile job %s cost (%v)", req.Name, time.Since(startTime))
	}()

	job := new(v1.OpsJob)
	if err := r.Get(ctx, req.NamespacedName, job); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if component.filter(ctx, job) {
		return ctrlruntime.Result{}, nil
	}
	if !job.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, r.delete(ctx, job, clears...)
	}
	if job.IsEnd() {
		return ctrlruntime.Result{}, nil
	}
	isTimeout := job.IsTimeout()
	quit, err := component.observe(ctx, job)
	if err != nil || (quit && !isTimeout) {
		return ctrlruntime.Result{}, err
	}
	if isTimeout {
		return ctrlruntime.Result{}, r.timeout(ctx, job)
	}
	result, err := component.handle(ctx, job)
	if err != nil {
		klog.ErrorS(err, "failed to handle job", "job", job.Name)
	}
	return result, err
}

func (r *OpsJobBaseReconciler) timeout(ctx context.Context, job *v1.OpsJob) error {
	message := fmt.Sprintf("The job is timeout, timeoutSecond: %d", job.Spec.TimeoutSecond)
	return r.setJobCompleted(ctx, job, v1.OpsJobFailed, message, nil)
}

func (r *OpsJobBaseReconciler) delete(ctx context.Context, job *v1.OpsJob, clearFuncs ...ClearFunc) error {
	if !job.IsFinished() {
		if err := r.setJobCompleted(ctx, job, v1.OpsJobFailed, "The job is stopped", nil); err != nil {
			return err
		}
	}
	for _, f := range clearFuncs {
		if err := f(ctx, job); err != nil {
			klog.ErrorS(err, "failed to do clear function")
			return err
		}
	}
	return utils.RemoveFinalizer(ctx, r.Client, job, v1.OpsJobFinalizer)
}

func (r *OpsJobBaseReconciler) setJobCompleted(ctx context.Context,
	job *v1.OpsJob, phase v1.OpsJobPhase, message string, outputs []v1.Parameter) error {
	if job.Status.Phase == phase {
		return nil
	}
	job.Status.FinishedAt = &metav1.Time{Time: time.Now().UTC()}
	if job.Status.StartedAt == nil {
		job.Status.StartedAt = job.Status.FinishedAt
	}
	job.Status.Phase = phase
	job.Status.Outputs = outputs

	cond := metav1.Condition{
		Type:    "JobCompleted",
		Message: message,
	}
	if phase == v1.OpsJobFailed {
		cond.Reason = "JobFailed"
		cond.Status = metav1.ConditionFalse
	} else {
		cond.Reason = "JobSucceeded"
		cond.Status = metav1.ConditionTrue
	}
	meta.SetStatusCondition(&job.Status.Conditions, cond)

	if err := r.Status().Update(ctx, job); err != nil {
		klog.ErrorS(err, "failed to patch job status", "name", job.Name)
		return err
	}
	klog.Infof("The job is completed. name: %s, phase: %s, message: %s", job.Name, phase, message)
	return nil
}

// this function changes the job state from Pending to Running and start the timeout timer.
func (r *OpsJobBaseReconciler) setJobRunning(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	patch := client.MergeFrom(job.DeepCopy())
	job.Status.Phase = v1.OpsJobRunning
	result := ctrlruntime.Result{}
	if err := r.Status().Patch(ctx, job, patch); err != nil {
		return result, err
	}
	// ensure that job will be reconciled when it is timeout
	if job.Spec.TimeoutSecond > 0 {
		result.RequeueAfter = time.Second * time.Duration(job.Spec.TimeoutSecond)
	}
	return result, nil
}

func (r *OpsJobBaseReconciler) updateJobCondition(ctx context.Context, job *v1.OpsJob, cond *metav1.Condition) error {
	changed := meta.SetStatusCondition(&job.Status.Conditions, *cond)
	if !changed {
		return nil
	}
	if err := r.Status().Update(ctx, job); err != nil {
		klog.ErrorS(err, "failed to update job condition", "name", job.Name)
		return err
	}
	return nil
}

func (r *OpsJobBaseReconciler) getAdminNode(ctx context.Context, name string) (*v1.Node, error) {
	node := &v1.Node{}
	err := r.Get(ctx, client.ObjectKey{Name: name}, node)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (r *OpsJobBaseReconciler) getFault(ctx context.Context, adminNodeName, faultId string) (*v1.Fault, error) {
	faultName := commonfaults.GenerateFaultName(adminNodeName, faultId)
	fault := &v1.Fault{}
	err := r.Get(ctx, client.ObjectKey{Name: faultName}, fault)
	if err != nil {
		return nil, err
	}
	return fault, nil
}

func (r *OpsJobBaseReconciler) getFaultConfig(ctx context.Context, faultId string) (*resource.FaultConfig, error) {
	configs, err := resource.GetFaultConfigmap(ctx, r.Client)
	if err != nil {
		klog.ErrorS(err, "failed to get fault configmap")
		return nil, err
	}
	config, ok := configs[faultId]
	if !ok {
		return nil, commonerrors.NewNotFoundWithMessage(
			fmt.Sprintf("fault config is not found: %s", faultId))
	}
	if !config.IsEnable() {
		return nil, commonerrors.NewInternalError(fmt.Sprintf("fault config is disabled: %s", faultId))
	}
	return config, nil
}

func jobPhaseChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldJob, ok1 := e.ObjectOld.(*v1.OpsJob)
			newJob, ok2 := e.ObjectNew.(*v1.OpsJob)
			if !ok1 || !ok2 {
				return false
			}
			if oldJob.IsPending() && !newJob.IsPending() {
				return true
			}
			return false
		},
	}
}

func findCondition(conditions []corev1.NodeCondition, condType corev1.NodeConditionType, reason string) *corev1.NodeCondition {
	for i, cond := range conditions {
		if cond.Type == condType && cond.Reason == reason {
			return &conditions[i]
		}
	}
	return nil
}
