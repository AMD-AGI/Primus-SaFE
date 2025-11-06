/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

type PreflightJobReconciler struct {
	*OpsJobBaseReconciler
	sync.RWMutex
}

// SetupPreflightJobController initializes and registers the PreflightJobReconciler with the controller manager.
func SetupPreflightJobController(mgr manager.Manager) error {
	r := &PreflightJobReconciler{
		OpsJobBaseReconciler: &OpsJobBaseReconciler{
			Client: mgr.GetClient(),
		},
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, onFirstPhaseChangedPredicate()))).
		Watches(&v1.Workload{}, r.handleWorkloadEvent()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Preflight Job Controller successfully")
	return nil
}

// handleWorkloadEvent creates an event handler that watches Workload resource events.
func (r *PreflightJobReconciler) handleWorkloadEvent() handler.EventHandler {
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			workload, ok := evt.Object.(*v1.Workload)
			if !ok || !isPreflightWorkload(workload) {
				return
			}
			r.handleWorkloadEventImpl(ctx, workload)
		},
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldWorkload, ok1 := evt.ObjectOld.(*v1.Workload)
			newWorkload, ok2 := evt.ObjectNew.(*v1.Workload)
			if !ok1 || !ok2 || !isPreflightWorkload(newWorkload) {
				return
			}
			if (!oldWorkload.IsEnd() && newWorkload.IsEnd()) ||
				(!oldWorkload.IsRunning() && newWorkload.IsRunning()) {
				r.handleWorkloadEventImpl(ctx, newWorkload)
			}
		},
	}
}

// isPreflightWorkload checks if a workload is a preflight job workload.
func isPreflightWorkload(workload *v1.Workload) bool {
	if v1.GetOpsJobId(workload) != "" &&
		v1.GetOpsJobType(workload) == string(v1.OpsJobPreflightType) {
		return true
	}
	return false
}

// handleWorkloadEventImpl handles workload events by updating the corresponding OpsJob status.
func (r *PreflightJobReconciler) handleWorkloadEventImpl(ctx context.Context, workload *v1.Workload) {
	var phase v1.OpsJobPhase
	completionMessage := ""
	switch {
	case workload.IsEnd():
		if workload.Status.Phase == v1.WorkloadSucceeded {
			phase = v1.OpsJobSucceeded
		} else {
			phase = v1.OpsJobFailed
		}
		completionMessage = getWorkloadCompletionMessage(workload)
		if completionMessage == "" {
			completionMessage = "unknown"
		}
	case workload.IsRunning():
		phase = v1.OpsJobRunning
	default:
		phase = v1.OpsJobPending
	}

	jobId := v1.GetOpsJobId(workload)
	err := backoff.Retry(func() error {
		job := &v1.OpsJob{}
		var err error
		if err = r.Get(ctx, client.ObjectKey{Name: jobId}, job); err != nil {
			return client.IgnoreNotFound(err)
		}
		switch phase {
		case v1.OpsJobPending, v1.OpsJobRunning:
			err = r.setJobPhase(ctx, job, phase)
		default:
			output := []v1.Parameter{{Name: "result", Value: completionMessage}}
			err = r.setJobCompleted(ctx, job, phase, completionMessage, output)
		}
		if err != nil {
			return err
		}
		return nil
	}, 2*time.Second, 200*time.Millisecond)
	if err != nil {
		klog.ErrorS(err, "failed to update job status", "jobId", jobId)
	}
}

// Reconcile is the main control loop for PreflightJob resources.
func (r *PreflightJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	clearFuncs := []ClearFunc{r.cleanupJobRelatedInfo}
	return r.OpsJobBaseReconciler.Reconcile(ctx, req, r, clearFuncs...)
}

// cleanupJobRelatedInfo cleans up job-related resources.
func (r *PreflightJobReconciler) cleanupJobRelatedInfo(ctx context.Context, job *v1.OpsJob) error {
	return commonjob.CleanupJobRelatedResource(ctx, r.Client, job.Name)
}

// observe the job status. Returns true if the expected state is met (no handling required), false otherwise.
func (r *PreflightJobReconciler) observe(_ context.Context, job *v1.OpsJob) (bool, error) {
	return job.IsEnd(), nil
}

// filter determines if the job should be processed by this preflight job reconciler.
func (r *PreflightJobReconciler) filter(_ context.Context, job *v1.OpsJob) bool {
	return job.Spec.Type != v1.OpsJobPreflightType
}

// handle processes the preflight job by creating a corresponding workload.
func (r *PreflightJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	if job.Status.Phase == "" {
		originalJob := client.MergeFrom(job.DeepCopy())
		job.Status.Phase = v1.OpsJobPending
		if err := r.Status().Patch(ctx, job, originalJob); err != nil {
			return ctrlruntime.Result{}, err
		}
		// ensure that job will be reconciled when it is timeout
		return newRequeueAfterResult(job), nil
	}

	workload := &v1.Workload{}
	if r.Get(ctx, client.ObjectKey{Name: job.Name}, workload) == nil {
		return ctrlruntime.Result{}, nil
	}
	var err error
	workload, err = r.generatePreflightWorkload(ctx, job)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	if err = r.Create(ctx, workload); err != nil {
		return ctrlruntime.Result{}, client.IgnoreAlreadyExists(err)
	}
	klog.Infof("Processing preflight job %s for workload %s", job.Name, workload.Name)
	return ctrlruntime.Result{}, nil
}

// generatePreflightWorkload generates a preflight workload based on the job specification.
func (r *PreflightJobReconciler) generatePreflightWorkload(ctx context.Context, job *v1.OpsJob) (*v1.Workload, error) {
	nodeParams := job.GetParameters(v1.ParameterNode)
	if len(nodeParams) == 0 {
		return nil, commonerrors.NewBadRequest("node parameter is empty")
	}
	nodeNames := ""
	for i, param := range nodeParams {
		if i > 0 {
			nodeNames += " "
		}
		nodeNames += param.Value
	}
	node := &v1.Node{}
	if err := r.Get(ctx, client.ObjectKey{Name: nodeParams[0].Value}, node); err != nil {
		return nil, err
	}

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: job.Name,
			Labels: map[string]string{
				v1.ClusterIdLabel:    v1.GetClusterId(job),
				v1.NodeFlavorIdLabel: v1.GetNodeFlavorId(node),
				v1.OpsJobIdLabel:     job.Name,
				v1.OpsJobTypeLabel:   string(job.Spec.Type),
				v1.DisplayNameLabel:  job.Name,
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation: common.UserSystem,
				// Dispatch the workload immediately, skipping the queue.
				v1.WorkloadScheduledAnnotation: timeutil.FormatRFC3339(time.Now().UTC()),
			},
		},
		Spec: v1.WorkloadSpec{
			Resource:   *job.Spec.Resource,
			EntryPoint: *job.Spec.EntryPoint,
			GroupVersionKind: v1.GroupVersionKind{
				Version: common.DefaultVersion,
				Kind:    common.PytorchJobKind,
			},
			IsTolerateAll: job.Spec.IsTolerateAll,
			Priority:      common.HighPriorityInt,
			CustomerLabels: map[string]string{
				v1.K8sHostName: nodeNames,
			},
			Workspace: corev1.NamespaceDefault,
			Image:     *job.Spec.Image,
			Env:       job.Spec.Env,
			Hostpath:  job.Spec.Hostpath,
		},
	}
	if job.Spec.TimeoutSecond > 0 {
		workload.Spec.Timeout = pointer.Int(job.Spec.TimeoutSecond)
	}
	if job.Spec.TTLSecondsAfterFinished > 0 {
		workload.Spec.TTLSecondsAfterFinished = pointer.Int(job.Spec.TTLSecondsAfterFinished)
	}
	return workload, nil
}

// getWorkloadCompletionMessage extracts the completion message from a workload.
func getWorkloadCompletionMessage(workload *v1.Workload) string {
	// Handle stopped or deleted workloads first
	if workload.Status.Phase == v1.WorkloadStopped || !workload.GetDeletionTimestamp().IsZero() {
		return "workload is stopped"
	}

	// Extract message from containers for completed workloads
	if workload.Status.Phase == v1.WorkloadFailed || workload.Status.Phase == v1.WorkloadSucceeded {
		for _, pod := range workload.Status.Pods {
			for _, container := range pod.Containers {
				if container.Name == v1.GetMainContainer(workload) && container.Message != "" {
					return container.Message
				}
			}
		}
	}

	// Default message for unknown cases
	return "unknown"
}
