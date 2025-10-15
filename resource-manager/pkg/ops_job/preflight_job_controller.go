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
)

type PreflightJobReconciler struct {
	*OpsJobBaseReconciler
	sync.RWMutex
}

func SetupPreflightJobController(mgr manager.Manager) error {
	r := &PreflightJobReconciler{
		OpsJobBaseReconciler: &OpsJobBaseReconciler{
			Client: mgr.GetClient(),
		},
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, onFirstPhaseChanged()))).
		Watches(&v1.Workload{}, r.handleWorkloadEvent()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Preflight Job Controller successfully")
	return nil
}

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

func isPreflightWorkload(workload *v1.Workload) bool {
	opsJobId := v1.GetOpsJobId(workload)
	if opsJobId != "" && v1.GetOpsJobType(workload) == string(v1.OpsJobPreflightType) {
		return true
	}
	return false
}

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

func (r *PreflightJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	clearFuncs := []ClearFunc{r.cleanupJobRelatedInfo}
	return r.OpsJobBaseReconciler.Reconcile(ctx, req, r, clearFuncs...)
}

func (r *PreflightJobReconciler) cleanupJobRelatedInfo(ctx context.Context, job *v1.OpsJob) error {
	return commonjob.CleanupJobRelatedInfo(ctx, r.Client, job.Name)
}

// Observe the job status. Returns true if the expected state is met (no handling required), false otherwise.
func (r *PreflightJobReconciler) observe(_ context.Context, job *v1.OpsJob) (bool, error) {
	return job.IsEnd(), nil
}

func (r *PreflightJobReconciler) filter(_ context.Context, job *v1.OpsJob) bool {
	return job.Spec.Type != v1.OpsJobPreflightType
}

func (r *PreflightJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	if job.Status.Phase == "" {
		patch := client.MergeFrom(job.DeepCopy())
		job.Status.Phase = v1.OpsJobPending
		if err := r.Status().Patch(ctx, job, patch); err != nil {
			return ctrlruntime.Result{}, err
		}
		// ensure that job will be reconciled when it is timeout
		return genRequeueAfterResult(job), nil
	}

	workload := &v1.Workload{}
	if r.Get(ctx, client.ObjectKey{Name: job.Name}, workload) == nil {
		return ctrlruntime.Result{}, nil
	}
	var err error
	workload, err = r.genPreflightWorkload(ctx, job)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	if err = r.Create(ctx, workload); err != nil {
		return ctrlruntime.Result{}, client.IgnoreAlreadyExists(err)
	}
	return ctrlruntime.Result{}, nil
}

func (r *PreflightJobReconciler) genPreflightWorkload(ctx context.Context, job *v1.OpsJob) (*v1.Workload, error) {
	nodeParams := job.GetParameters(v1.ParameterNode)
	if len(nodeParams) == 0 {
		return nil, commonerrors.NewBadRequest("node parameter is empty")
	}
	nodeNames := ""
	for i, p := range nodeParams {
		if i > 0 {
			nodeNames += " "
		}
		nodeNames += p.Value
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
				v1.WorkloadScheduledAnnotation: time.Now().UTC().Format(time.RFC3339),
			},
		},
		Spec: v1.WorkloadSpec{
			Resource:   *job.Spec.Resource,
			EntryPoint: *job.Spec.EntryPoint,
			GroupVersionKind: v1.GroupVersionKind{
				Version: v1.SchemeGroupVersion.Version,
				Kind:    common.PytorchJobKind,
			},
			IsTolerateAll: job.Spec.IsTolerateAll,
			Priority:      common.HighPriorityInt,
			CustomerLabels: map[string]string{
				common.K8sHostName: nodeNames,
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

func getWorkloadCompletionMessage(workload *v1.Workload) string {
	switch workload.Status.Phase {
	case v1.WorkloadFailed, v1.WorkloadSucceeded:
		for _, pod := range workload.Status.Pods {
			for _, c := range pod.Containers {
				if c.Name != v1.GetMainContainer(workload) {
					continue
				}
				if c.Message != "" {
					return c.Message
				}
			}
		}
	case v1.WorkloadStopped:
		return "workload is stopped"
	default:
		if !workload.GetDeletionTimestamp().IsZero() {
			return "workload is stopped"
		}
	}
	return ""
}
