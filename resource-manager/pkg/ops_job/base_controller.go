/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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

const (
	JobProcessingType = "JobProcessing"
	JobCompletedType  = "JobCompleted"
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
	quit, err := component.observe(ctx, job)
	if err != nil {
		klog.ErrorS(err, "failed to observe job", "job", job.Name)
		if utils.IsNonRetryableError(err) {
			err = r.setJobCompleted(ctx, job, v1.OpsJobFailed, err.Error(), nil)
		}
		return ctrlruntime.Result{}, err
	}
	if quit {
		return ctrlruntime.Result{}, nil
	}
	if job.IsTimeout() {
		return ctrlruntime.Result{}, r.timeout(ctx, job)
	}
	result, err := component.handle(ctx, job)
	if err != nil {
		klog.ErrorS(err, "failed to handle job", "job", job.Name)
		if utils.IsNonRetryableError(err) {
			err = r.setJobCompleted(ctx, job, v1.OpsJobFailed, err.Error(), nil)
		}
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
		Type:    JobCompletedType,
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

func (r *OpsJobBaseReconciler) setJobPhase(ctx context.Context, job *v1.OpsJob, phase v1.OpsJobPhase) error {
	if job.Status.Phase == phase && job.Status.StartedAt != nil {
		return nil
	}
	patch := client.MergeFrom(job.DeepCopy())
	job.Status.Phase = phase
	if job.Status.StartedAt == nil {
		job.Status.StartedAt = &metav1.Time{Time: time.Now().UTC()}
	}
	return r.Status().Patch(ctx, job, patch)
}

func (r *OpsJobBaseReconciler) updateCondition(ctx context.Context, job *v1.OpsJob, cond *metav1.Condition) error {
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
	if !node.GetDeletionTimestamp().IsZero() {
		return nil, commonerrors.NewInternalError("the node is deleting")
	}
	if !node.IsReady() {
		return nil, fmt.Errorf("the node is not ready")
	}
	return node, nil
}

// Create a fault to block workload scheduling on the node for upgrade purposes
func (r *OpsJobBaseReconciler) createFault(ctx context.Context,
	job *v1.OpsJob, adminNode *v1.Node, monitorId, message string) error {
	_, err := r.getFault(ctx, adminNode.Name, monitorId)
	if err == nil || !apierrors.IsNotFound(err) {
		return err
	}
	config, err := r.getFaultConfig(ctx, monitorId)
	if err != nil {
		return err
	}
	fault := &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonfaults.GenerateFaultName(adminNode.Name, monitorId),
			Labels: map[string]string{
				v1.ClusterIdLabel: v1.GetClusterId(job),
				v1.NodeIdLabel:    adminNode.Name,
				v1.OpsJobIdLabel:  job.Name,
			},
		},
		Spec: v1.FaultSpec{
			MonitorId: monitorId,
			Message:   message,
			Action:    string(config.Action),
			Node: &v1.FaultNode{
				ClusterName: v1.GetClusterId(job),
				AdminName:   adminNode.Name,
				K8sName:     adminNode.GetK8sNodeName(),
			},
		},
	}
	if err = r.Create(ctx, fault); err != nil {
		return client.IgnoreAlreadyExists(err)
	}
	klog.Infof("create fault, id: %s", fault.Name)
	return nil
}

func (r *OpsJobBaseReconciler) getFaultConfig(ctx context.Context, monitorId string) (*resource.FaultConfig, error) {
	configs, err := resource.GetFaultConfigmap(ctx, r.Client)
	if err != nil {
		klog.ErrorS(err, "failed to get fault configmap")
		return nil, err
	}
	config, ok := configs[monitorId]
	if !ok {
		return nil, commonerrors.NewNotFoundWithMessage(
			fmt.Sprintf("fault config is not found: %s", monitorId))
	}
	if !config.IsEnable() {
		return nil, commonerrors.NewInternalError(fmt.Sprintf("fault config is disabled: %s", monitorId))
	}
	return config, nil
}

func (r *OpsJobBaseReconciler) getFault(ctx context.Context, adminNodeName, monitorId string) (*v1.Fault, error) {
	faultName := commonfaults.GenerateFaultName(adminNodeName, monitorId)
	fault := &v1.Fault{}
	err := r.Get(ctx, client.ObjectKey{Name: faultName}, fault)
	if err != nil {
		return nil, err
	}
	return fault, nil
}

func (r *OpsJobBaseReconciler) deleteFault(ctx context.Context, adminNodeName, monitorId string) error {
	if fault, _ := r.getFault(ctx, adminNodeName, monitorId); fault != nil {
		return r.Delete(ctx, fault)
	}
	return nil
}

func (r *OpsJobBaseReconciler) getInputNodes(ctx context.Context, job *v1.OpsJob) ([]*v1.Node, error) {
	var results []*v1.Node
	for _, p := range job.Spec.Inputs {
		if p.Name != v1.ParameterNode {
			continue
		}
		node, err := r.getAdminNode(ctx, p.Value)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, err
			}
		} else {
			results = append(results, node)
		}
	}
	if len(results) == 0 {
		return nil, commonerrors.NewBadRequest("no input nodes found")
	}
	return results, nil
}

func (r *OpsJobBaseReconciler) listJobs(ctx context.Context, clusterId, opsjobType string) ([]v1.OpsJob, error) {
	labelSelector := labels.SelectorFromSet(map[string]string{v1.ClusterIdLabel: clusterId, v1.OpsJobTypeLabel: opsjobType})
	jobList := &v1.OpsJobList{}
	if err := r.List(ctx, jobList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return nil, err
	}
	result := make([]v1.OpsJob, 0, len(jobList.Items))
	for i := range jobList.Items {
		if jobList.Items[i].IsEnd() {
			continue
		}
		result = append(result, jobList.Items[i])
	}
	return result, nil
}
func onFirstPhaseChanged() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldJob, ok1 := e.ObjectOld.(*v1.OpsJob)
			newJob, ok2 := e.ObjectNew.(*v1.OpsJob)
			if !ok1 || !ok2 {
				return false
			}
			if oldJob.Status.Phase == "" && newJob.Status.Phase != "" {
				return true
			}
			return false
		},
	}
}

func genRequeueAfterResult(job *v1.OpsJob) ctrlruntime.Result {
	result := ctrlruntime.Result{}
	if job.Spec.TimeoutSecond > 0 {
		result.RequeueAfter = time.Second * time.Duration(job.Spec.TimeoutSecond)
	}
	return result
}
