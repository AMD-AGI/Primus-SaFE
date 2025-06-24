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

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/resource"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

type OpsJobReason string

const (
	JobSucceed       OpsJobReason = "JobSucceed"
	JobFailed        OpsJobReason = "JobFailed"
	JobInternalError OpsJobReason = "InternalError"
	JobTimeout       OpsJobReason = "Timeout"
)

type FilterFunc func(ctx context.Context, job *v1.OpsJob) bool
type ObserveFunc func(ctx context.Context, job *v1.OpsJob) (bool, error)
type TimeoutFunc func(ctx context.Context, job *v1.OpsJob) error
type HandleFunc func(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error)
type ClearFunc func(ctx context.Context, job *v1.OpsJob) error

func doReconcile(ctx context.Context, cli client.Client, req ctrlruntime.Request,
	filter FilterFunc, observe ObserveFunc, timeout TimeoutFunc, handle HandleFunc, clears ...ClearFunc) (ctrlruntime.Result, error) {

	startTime := time.Now().UTC()
	defer func() {
		klog.V(4).Infof("Finished reconcile job %s cost (%v)", req.Name, time.Since(startTime))
	}()

	job := new(v1.OpsJob)
	if err := cli.Get(ctx, req.NamespacedName, job); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if filter(ctx, job) {
		return ctrlruntime.Result{}, nil
	}
	if !job.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, doDelete(ctx, cli, job, clears...)
	}
	if job.IsEnd() {
		return ctrlruntime.Result{}, nil
	}
	isTimeout := job.IsTimeout()
	if observe != nil {
		quit, err := observe(ctx, job)
		if err != nil || (quit && !isTimeout) {
			return ctrlruntime.Result{}, err
		}
	}
	if isTimeout {
		return ctrlruntime.Result{}, doTimeout(ctx, cli, job, timeout)
	}
	return handle(ctx, job)
}

func doTimeout(ctx context.Context, cli client.Client, job *v1.OpsJob, callBack TimeoutFunc) error {
	if callBack != nil {
		if err := callBack(ctx, job); err != nil {
			return err
		}
	}
	message := fmt.Sprintf("The job is timeout, timeoutSecond: %d", job.Spec.TimeoutSecond)
	return setJobCompleted(ctx, cli, job, v1.OpsJobFailed, JobTimeout, message)
}

func doDelete(ctx context.Context, cli client.Client, job *v1.OpsJob, clearFuncs ...ClearFunc) error {
	if !job.IsFinished() {
		if err := setJobCompleted(ctx, cli, job, v1.OpsJobFailed, "JobStopped", "The job is stopped"); err != nil {
			return err
		}
	}
	for _, f := range clearFuncs {
		if err := f(ctx, job); err != nil {
			klog.ErrorS(err, "failed to do clear function")
			return err
		}
	}
	return utils.RemoveFinalizer(ctx, cli, job, v1.OpsJobFinalizer)
}

func setJobCompleted(ctx context.Context, cli client.Client, job *v1.OpsJob,
	phase v1.OpsJobPhase, reason OpsJobReason, message string) error {
	if job.IsEnd() {
		return nil
	}
	job.Status.FinishedAt = &metav1.Time{Time: time.Now().UTC()}
	if job.Status.StartedAt == nil {
		job.Status.StartedAt = job.Status.FinishedAt
	}
	job.Status.Phase = phase
	cond := metav1.Condition{
		Type:    "JobCompleted",
		Status:  metav1.ConditionTrue,
		Reason:  string(reason),
		Message: message,
	}
	if phase == v1.OpsJobFailed {
		job.Status.Message = message
		cond.Status = metav1.ConditionFalse
	}
	meta.SetStatusCondition(&job.Status.Conditions, cond)
	if err := cli.Status().Update(ctx, job); err != nil {
		klog.ErrorS(err, "failed to patch job status", "name", job.Name)
		return err
	}
	klog.Infof("The job is completed. name: %s, phase: %s, message: %s", job.Name, phase, message)
	return nil
}

func updateJobCondition(ctx context.Context, cli client.Client, job *v1.OpsJob, cond *metav1.Condition) error {
	changed := meta.SetStatusCondition(&job.Status.Conditions, *cond)
	if !changed {
		return nil
	}
	if err := cli.Status().Update(ctx, job); err != nil {
		klog.ErrorS(err, "failed to update job condition", "name", job.Name)
		return err
	}
	return nil
}

func getAdminNode(ctx context.Context, cli client.Client, name string) (*v1.Node, error) {
	node := &v1.Node{}
	err := cli.Get(ctx, client.ObjectKey{Name: name}, node)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func getFault(ctx context.Context, cli client.Client, adminNodeName, faultId string) (*v1.Fault, error) {
	faultName := commonfaults.GenerateFaultName(adminNodeName, faultId)
	fault := &v1.Fault{}
	err := cli.Get(ctx, client.ObjectKey{Name: faultName}, fault)
	if err != nil {
		return nil, err
	}
	return fault, nil
}

func getFaultConfig(ctx context.Context, cli client.Client, faultId string) (*resource.FaultConfig, error) {
	configs, err := resource.GetFaultConfigmap(ctx, cli)
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

func findCondition(conditions []corev1.NodeCondition, condType corev1.NodeConditionType, reason string) *corev1.NodeCondition {
	for i, cond := range conditions {
		if cond.Type == condType && cond.Reason == reason {
			return &conditions[i]
		}
	}
	return nil
}
