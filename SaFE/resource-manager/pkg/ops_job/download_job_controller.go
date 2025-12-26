/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"sync"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
)

type DownloadJobReconciler struct {
	*OpsJobBaseReconciler
	sync.RWMutex
}

// SetupDownloadJobController initializes and registers the DownloadJobReconciler with the controller manager.
func SetupDownloadJobController(mgr manager.Manager) error {
	r := &DownloadJobReconciler{
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
	klog.Infof("Setup Download Job Controller successfully")
	return nil
}

// handleWorkloadEvent creates an event handler that watches Workload resource events.
func (r *DownloadJobReconciler) handleWorkloadEvent() handler.EventHandler {
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			workload, ok := evt.Object.(*v1.Workload)
			if !ok || !isDownloadWorkload(workload) {
				return
			}
			r.handleWorkloadEventImpl(ctx, workload)
		},
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldWorkload, ok1 := evt.ObjectOld.(*v1.Workload)
			newWorkload, ok2 := evt.ObjectNew.(*v1.Workload)
			if !ok1 || !ok2 || !isDownloadWorkload(newWorkload) {
				return
			}
			if (!oldWorkload.IsEnd() && newWorkload.IsEnd()) ||
				(!oldWorkload.IsRunning() && newWorkload.IsRunning()) {
				r.handleWorkloadEventImpl(ctx, newWorkload)
			}
		},
	}
}

// isDownloadWorkload checks if a workload is a download job workload.
func isDownloadWorkload(workload *v1.Workload) bool {
	if v1.GetOpsJobId(workload) != "" &&
		v1.GetOpsJobType(workload) == string(v1.OpsJobDownloadType) {
		return true
	}
	return false
}

// Reconcile is the main control loop for DownloadJob resources.
func (r *DownloadJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	clearFuncs := []ClearFunc{r.cleanupJobRelatedInfo}
	return r.OpsJobBaseReconciler.Reconcile(ctx, req, r, clearFuncs...)
}

// cleanupJobRelatedInfo cleans up job-related resources.
func (r *DownloadJobReconciler) cleanupJobRelatedInfo(ctx context.Context, job *v1.OpsJob) error {
	return commonjob.CleanupJobRelatedResource(ctx, r.Client, job.Name)
}

// observe the job status. Returns true if the expected state is met (no handling required), false otherwise.
func (r *DownloadJobReconciler) observe(_ context.Context, job *v1.OpsJob) (bool, error) {
	return job.IsEnd(), nil
}

// filter determines if the job should be processed by this download job reconciler.
func (r *DownloadJobReconciler) filter(_ context.Context, job *v1.OpsJob) bool {
	return job.Spec.Type != v1.OpsJobDownloadType
}

// handle processes the download job by creating a corresponding workload.
func (r *DownloadJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
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
	workload, err = r.generateDownloadWorkload(ctx, job)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	if err = r.Create(ctx, workload); err != nil {
		return ctrlruntime.Result{}, client.IgnoreAlreadyExists(err)
	}
	klog.Infof("Processing download job %s for workload %s", job.Name, workload.Name)
	return ctrlruntime.Result{}, nil
}

// generateDownloadWorkload generates a download workload based on the job specification.
func (r *DownloadJobReconciler) generateDownloadWorkload(ctx context.Context, job *v1.OpsJob) (*v1.Workload, error) {
	workspaceId := v1.GetWorkspaceId(job)
	if workspaceId == "" {
		return nil, commonerrors.NewInternalError("workspaceId is empty")
	}
	workspace := &v1.Workspace{}
	if err := r.Get(ctx, client.ObjectKey{Name: workspaceId}, workspace); err != nil {
		return nil, err
	}
	secretParam, err := commonjob.GetRequiredParameter(job, v1.ParameterSecret)
	if err != nil {
		return nil, err
	}
	inputUrl, err := commonjob.GetRequiredParameter(job, v1.ParameterEndpoint)
	if err != nil {
		return nil, err
	}
	destPath, err := commonjob.GetRequiredParameter(job, v1.ParameterDestPath)
	if err != nil {
		return nil, err
	}
	nfsPath := getNfsPathFromWorkspace(workspace)
	if nfsPath == "" {
		return nil, commonerrors.NewInternalError("nfs path is empty")
	}

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: job.Name,
			Labels: map[string]string{
				v1.ClusterIdLabel:   v1.GetClusterId(job),
				v1.UserIdLabel:      v1.GetUserId(job),
				v1.OpsJobIdLabel:    job.Name,
				v1.OpsJobTypeLabel:  string(job.Spec.Type),
				v1.DisplayNameLabel: job.Name,
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation:          v1.GetUserName(job),
				v1.WorkloadScheduledAnnotation: timeutil.FormatRFC3339(time.Now().UTC()),
			},
		},
		Spec: v1.WorkloadSpec{
			Resource: v1.WorkloadResource{
				Replica:          1,
				CPU:              "6",
				Memory:           "8Gi",
				EphemeralStorage: "50Gi",
			},
			GroupVersionKind: v1.GroupVersionKind{
				Version: common.DefaultVersion,
				Kind:    common.JobKind,
			},
			Workspace: workspaceId,
			Image:     *job.Spec.Image,
			Env:       job.Spec.Env,
		},
	}
	if err = controllerutil.SetControllerReference(job, workload, r.Client.Scheme()); err != nil {
		return nil, err
	}
	if job.Spec.TimeoutSecond > 0 {
		workload.Spec.Timeout = pointer.Int(job.Spec.TimeoutSecond)
	}
	if job.Spec.TTLSecondsAfterFinished > 0 {
		workload.Spec.TTLSecondsAfterFinished = pointer.Int(job.Spec.TTLSecondsAfterFinished)
	}
	if len(workload.Spec.Env) == 0 {
		workload.Spec.Env = make(map[string]string)
	}
	workload.Spec.Env["INPUT_URL"] = inputUrl.Value
	workload.Spec.Env["DEST_PATH"] = nfsPath + "/" + destPath.Value
	workload.Spec.Env["SECRET_PATH"] = common.SecretPath + "/" + secretParam.Value
	workload.Spec.Secrets = []v1.SecretEntity{{
		Id:   secretParam.Value,
		Type: v1.SecretGeneral,
	}}
	return workload, nil
}

// getNfsPathFromWorkspace retrieves the NFS path from the workspace's volumes.
// It prioritizes PFS type volumes, otherwise falls back to the first available volume's mount path.
func getNfsPathFromWorkspace(workspace *v1.Workspace) string {
	result := ""
	for _, vol := range workspace.Spec.Volumes {
		if vol.Type == v1.PFS {
			result = vol.MountPath
			break
		}
	}
	if result == "" && len(workspace.Spec.Volumes) > 0 {
		result = workspace.Spec.Volumes[0].MountPath
	}
	return result
}
