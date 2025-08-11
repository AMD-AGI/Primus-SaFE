/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
)

type DiagnoseJobReconciler struct {
	*OpsJobBaseReconciler
	sync.RWMutex
}

func SetupDiagnoseJobController(mgr manager.Manager) error {
	if commonconfig.GetDiagnoseImage() == "" {
		return nil
	}
	r := &DiagnoseJobReconciler{
		OpsJobBaseReconciler: &OpsJobBaseReconciler{
			Client: mgr.GetClient(),
		},
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, onJobRunning()))).
		Watches(&v1.Workload{}, r.handleWorkloadEvent()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Diagnose Job Controller successfully")
	return nil
}

func (r *DiagnoseJobReconciler) handleWorkloadEvent() handler.EventHandler {
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			workload, ok := evt.Object.(*v1.Workload)
			if !ok || !isDiagnoseWorkload(workload) {
				return
			}
			if workload.IsEnd() {
				r.handleWorkloadEventImpl(ctx, workload)
			}
		},
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldWorkload, ok1 := evt.ObjectOld.(*v1.Workload)
			newWorkload, ok2 := evt.ObjectNew.(*v1.Workload)
			if !ok1 || !ok2 || !isDiagnoseWorkload(newWorkload) {
				return
			}
			if !oldWorkload.IsEnd() && newWorkload.IsEnd() {
				r.handleWorkloadEventImpl(ctx, newWorkload)
			}
		},
	}
}

func isDiagnoseWorkload(workload *v1.Workload) bool {
	opsJobId := v1.GetOpsJobId(workload)
	if opsJobId != "" && v1.GetOpsJobType(workload) == string(v1.OpsJobDiagnoseType) {
		return true
	}
	return false
}

func (r *DiagnoseJobReconciler) handleWorkloadEventImpl(ctx context.Context, workload *v1.Workload) {
	var phase v1.OpsJobPhase
	var message string
	if workload.Status.Phase == v1.WorkloadSucceeded {
		phase = v1.OpsJobSucceeded
	} else {
		phase = v1.OpsJobFailed
		message = commonworkload.GetFailedMessage(workload)
		if message == "" {
			message = "unknown reason"
		}
	}

	jobId := v1.GetOpsJobId(workload)
	err := backoff.Retry(func() error {
		job := &v1.OpsJob{}
		if err := r.Get(ctx, client.ObjectKey{Name: jobId}, job); err != nil {
			return client.IgnoreNotFound(err)
		}
		if err := r.setJobCompleted(ctx, job, phase, message, nil); err != nil {
			return err
		}
		return nil
	}, 2*time.Second, 200*time.Millisecond)
	if err != nil {
		klog.ErrorS(err, "failed to update job status", "jobId", jobId)
	}
}

func (r *DiagnoseJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	clearFuncs := []ClearFunc{r.cleanupJobRelatedInfo}
	return r.OpsJobBaseReconciler.Reconcile(ctx, req, r, clearFuncs...)
}

func (r *DiagnoseJobReconciler) cleanupJobRelatedInfo(ctx context.Context, job *v1.OpsJob) error {
	return commonjob.CleanupJobRelatedInfo(ctx, r.Client, job.Name)
}

// Observe the job status. Returns true if the expected state is met (no handling required), false otherwise.
func (r *DiagnoseJobReconciler) observe(_ context.Context, job *v1.OpsJob) (bool, error) {
	return job.IsEnd(), nil
}

func (r *DiagnoseJobReconciler) filter(_ context.Context, job *v1.OpsJob) bool {
	return job.Spec.Type != v1.OpsJobDiagnoseType
}

func (r *DiagnoseJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	if job.IsPending() {
		return r.setJobRunning(ctx, job)
	}
	workload := &v1.Workload{}
	if r.Get(ctx, client.ObjectKey{Name: job.Name}, workload) == nil {
		return ctrlruntime.Result{}, nil
	}

	var err error
	nodeParams := job.GetParameters(v1.ParameterNode)
	for _, param := range nodeParams {
		adminNode, err := r.getAdminNode(ctx, param.Value)
		if err != nil {
			return ctrlruntime.Result{}, err
		}
		if err = r.createFault(ctx, job, adminNode, common.DiagnoseMonitorId, "diagnose node"); err != nil {
			return ctrlruntime.Result{}, err
		}
	}

	workload, err = r.genDiagnoseWorkload(ctx, job)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	if err = r.Create(ctx, workload); err != nil {
		return ctrlruntime.Result{}, client.IgnoreAlreadyExists(err)
	}
	return ctrlruntime.Result{}, nil
}

func (r *DiagnoseJobReconciler) genDiagnoseWorkload(ctx context.Context, job *v1.OpsJob) (*v1.Workload, error) {
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
	res, err := r.genMaxResource(ctx, node)
	if err != nil {
		return nil, err
	}

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: job.Name,
			Labels: map[string]string{
				v1.ClusterIdLabel:    job.Spec.Cluster,
				v1.NodeFlavorIdLabel: v1.GetNodeFlavorId(node),
				v1.OpsJobIdLabel:     job.Name,
				v1.OpsJobTypeLabel:   string(job.Spec.Type),
				v1.DisplayNameLabel:  job.Name,
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation: v1.SystemUser,
				// Dispatch the workload immediately, skipping the queue.
				v1.WorkloadScheduledAnnotation: time.Now().UTC().Format(time.RFC3339),
			},
		},
		Spec: v1.WorkloadSpec{
			EntryPoint: fmt.Sprintf("bash run.sh"),
			GroupVersionKind: v1.GroupVersionKind{
				Version: common.DefaultVersion,
				Kind:    common.PytorchJobKind,
			},
			IsTolerateAll: true,
			Priority:      common.HighPriorityInt,
			CustomerLabels: map[string]string{
				common.K8sHostNameLabel: nodeNames,
			},
			Workspace: v1.GetWorkspaceId(node),
			Image:     commonconfig.GetDiagnoseImage(),
		},
	}
	workload.Spec.Resource = *res
	workload.Spec.Resource.Replica = len(nodeParams)
	if job.Spec.TimeoutSecond > 0 {
		workload.Spec.Timeout = pointer.Int(job.Spec.TimeoutSecond)
	}
	return workload, nil
}
