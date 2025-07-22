/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
)

type PreflightJob struct {
	// store the processing status for each workload. key is the workload name
	allWorkloads map[string]v1.WorkloadPhase
	// the maximum number of node failures that the system can tolerate during job execution.
}

type PreflightJobReconciler struct {
	*OpsJobBaseReconciler
	sync.RWMutex
	// key is job id
	allJobs map[string]*PreflightJob
}

func SetupPreflightJobController(mgr manager.Manager) error {
	r := &PreflightJobReconciler{
		OpsJobBaseReconciler: &OpsJobBaseReconciler{
			Client: mgr.GetClient(),
		},
		allJobs: make(map[string]*PreflightJob),
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, jobPhaseChangedPredicate()))).
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
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldWorkload, ok1 := evt.ObjectOld.(*v1.Workload)
			newWorkload, ok2 := evt.ObjectNew.(*v1.Workload)
			if !ok1 || !ok2 {
				return
			}
			opsJobId := v1.GetOpsJobId(newWorkload)
			if opsJobId == "" || v1.GetOpsJobType(newWorkload) != string(v1.OpsJobPreflightType) {
				return
			}
			if !oldWorkload.IsEnd() && newWorkload.IsEnd() {
				phase := v1.WorkloadSucceeded
				if newWorkload.Status.Phase != v1.WorkloadSucceeded {
					phase = v1.WorkloadFailed
				}
				if !r.setWorkloadPhase(opsJobId, newWorkload.Name, phase) {
					return
				}
				if phase == v1.WorkloadFailed {
					r.addFailedWorkload(ctx, opsJobId, newWorkload)
				}
				q.Add(reconcile.Request{NamespacedName: apitypes.NamespacedName{Name: opsJobId}})
			}
		},
	}
}

func (r *PreflightJobReconciler) addFailedWorkload(ctx context.Context, jobId string, workload *v1.Workload) {
	nodeName, _ := workload.Spec.Env[common.K8sHostNameLabel]
	if nodeName == "" {
		return
	}
	message := commonworkload.GetFailedMessage(workload)
	if message == "" {
		message = "unknown reason"
	}
	cond := &metav1.Condition{
		Type:               nodeName,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             "PreflightFailed",
		Message:            message,
	}
	err := backoff.Retry(func() error {
		job := &v1.OpsJob{}
		if err := r.Get(ctx, client.ObjectKey{Name: jobId}, job); err != nil {
			return client.IgnoreNotFound(err)
		}
		if err := r.updateJobCondition(ctx, job, cond); err != nil {
			return err
		}
		return nil
	}, 2*time.Second, 200*time.Millisecond)
	if err != nil {
		klog.ErrorS(err, "failed to update job condition", "jobId", jobId)
	}
}

func (r *PreflightJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	clearFuncs := []ClearFunc{r.cleanupJobRelatedInfo, r.removeJob}
	return r.OpsJobBaseReconciler.Reconcile(ctx, req, r, clearFuncs...)
}

func (r *PreflightJobReconciler) cleanupJobRelatedInfo(ctx context.Context, job *v1.OpsJob) error {
	return commonjob.CleanupJobRelatedInfo(ctx, r.Client, job.Name)
}

// Observe the job status. Returns true if the expected state is met (no handling required), false otherwise.
func (r *PreflightJobReconciler) observe(ctx context.Context, job *v1.OpsJob) (bool, error) {
	phase, message := r.getJobPhase(job.Name)
	switch phase {
	case v1.OpsJobPending, "":
		return false, nil
	case v1.OpsJobFailed, v1.OpsJobSucceeded:
		if err := r.setJobCompleted(ctx, job, phase, message, nil); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (r *PreflightJobReconciler) filter(_ context.Context, job *v1.OpsJob) bool {
	return job.Spec.Type != v1.OpsJobPreflightType
}

func (r *PreflightJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	if !job.IsPending() {
		return ctrlruntime.Result{}, nil
	}
	if commonconfig.GetPreflightImage() == "" {
		err := r.setJobCompleted(ctx, job, v1.OpsJobFailed, "the image for preflight is not set", nil)
		return ctrlruntime.Result{}, err
	}
	if err := r.handleImpl(ctx, job); err != nil {
		return ctrlruntime.Result{}, err
	}
	return r.setJobRunning(ctx, job)
}

func (r *PreflightJobReconciler) handleImpl(ctx context.Context, job *v1.OpsJob) error {
	inputNodes, err := r.getInputNodes(ctx, job)
	if err != nil {
		return err
	}
	workloads := make([]*v1.Workload, len(inputNodes))
	for i, n := range inputNodes {
		workloads[i] = genPreflightWorkload(job, n)
	}

	r.Lock()
	defer r.Unlock()
	preflightJob, ok := r.allJobs[job.Name]
	if !ok {
		preflightJob = &PreflightJob{allWorkloads: make(map[string]v1.WorkloadPhase)}
		r.allJobs[job.Name] = preflightJob
	}
	for i, w := range workloads {
		if _, ok = preflightJob.allWorkloads[w.Name]; ok {
			continue
		}
		err = r.createFault(ctx, job, inputNodes[i], common.PreflightMonitorId, "preflight check")
		if err != nil {
			return err
		}
		err = r.Create(ctx, w)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
		preflightJob.allWorkloads[w.Name] = v1.WorkloadRunning
	}
	return nil
}

func (r *PreflightJobReconciler) removeJob(_ context.Context, job *v1.OpsJob) error {
	r.Lock()
	defer r.Unlock()
	delete(r.allJobs, job.Name)
	return nil
}

func (r *PreflightJobReconciler) setWorkloadPhase(jobId, workloadId string, phase v1.WorkloadPhase) bool {
	r.Lock()
	defer r.Unlock()
	preflightJob, ok := r.allJobs[jobId]
	if !ok {
		return false
	}
	preflightJob.allWorkloads[workloadId] = phase
	return true
}

func (r *PreflightJobReconciler) getJobPhase(jobId string) (v1.OpsJobPhase, string) {
	r.RLock()
	defer r.RUnlock()
	job, ok := r.allJobs[jobId]
	if !ok {
		return v1.OpsJobPending, ""
	}
	totalFailCount := 0
	totalSuccessCount := 0
	for _, p := range job.allWorkloads {
		if p == v1.WorkloadSucceeded {
			totalSuccessCount++
		} else if p == v1.WorkloadFailed {
			totalFailCount++
		}
	}
	if totalFailCount+totalSuccessCount >= len(job.allWorkloads) {
		phase := v1.OpsJobSucceeded
		if totalFailCount > 0 {
			phase = v1.OpsJobFailed
		}
		return phase, fmt.Sprintf("success: %d, fail: %d", totalSuccessCount, totalFailCount)
	}
	return v1.OpsJobRunning, ""
}

func genPreflightWorkload(job *v1.OpsJob, adminNode *v1.Node) *v1.Workload {
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: job.Name + "-" + adminNode.Name,
			Labels: map[string]string{
				v1.ClusterIdLabel:  job.Spec.Cluster,
				v1.OpsJobIdLabel:   job.Name,
				v1.OpsJobTypeLabel: string(job.Spec.Type),
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation: v1.SystemUser,
				// Dispatch the workload immediately, skipping the queue.
				v1.WorkloadScheduledAnnotation: time.Now().UTC().Format(time.RFC3339),
			},
		},
		Spec: v1.WorkloadSpec{
			EntryPoint: fmt.Sprintf("GPU_PRODUCT=%s bash run.sh", v1.GetGpuProductName(job)),
			GroupVersionKind: v1.GroupVersionKind{
				Version: common.DefaultVersion,
				Kind:    common.JobKind,
			},
			IsTolerateAll: true,
			Priority:      common.HighPriorityInt,
			CustomerLabels: map[string]string{
				common.K8sHostNameLabel: adminNode.Name,
			},
			Resource: v1.WorkloadResource{
				Replica:          1,
				CPU:              "32",
				Memory:           "64Gi",
				EphemeralStorage: "50Gi",
				GPU:              strconv.Itoa(v1.GetNodeGpuCount(adminNode)),
			},
			Workspace: v1.GetWorkspaceId(adminNode),
			Image:     commonconfig.GetPreflightImage(),
		},
	}
	return workload
}
