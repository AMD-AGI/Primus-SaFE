/*
 * Copyright (c) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonsecret "github.com/AMD-AIG-AIMA/SAFE/common/pkg/secret"
	commonworkspace "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workspace"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

const (
	// ModelFinalizer is the finalizer for Model resources
	ModelFinalizer = "model.amd.com/finalizer"
	// CleanupJobPrefix is the prefix for cleanup job names
	CleanupJobPrefix = "cleanup-"
	// DownloadJobPrefix is the prefix for download job names
	DownloadJobPrefix = "download-"

	// MaxFailoverAttempts is the maximum number of workspace failover attempts per path
	MaxFailoverAttempts = 3
	// FailoverTriedAnnotation stores tried workspaces per path for failover tracking
	// Value format: JSON map[string][]string where key is base PFS path and value is list of tried workspace names
	FailoverTriedAnnotation = "model.amd.com/failover-tried"

	// AbandonCleanupAnnotation, set to "true" by an administrator on a model that is being
	// deleted, releases the model without removing its files: the remaining directories
	// (and the S3 copy) are left where they are. It is the way out of a cleanup that keeps
	// failing.
	AbandonCleanupAnnotation = "model.amd.com/abandon-cleanup"

	// modelSizeMarker prefixes the value with which the download job reports the size of
	// the files it left on disk. The job prints it on a "[SUCCESS]" line, the only kind of
	// line besides "[ERROR]" that reaches the job outputs.
	modelSizeMarker = "MODEL_SIZE_BYTES="

	// cleanupRetryInterval is how long a failed cleanup waits before it runs again.
	cleanupRetryInterval = 30 * time.Second
	// cleanupMaxRetryInterval caps the backoff between failed cleanups of one directory.
	cleanupMaxRetryInterval = 10 * time.Minute
	// cleanupFailureAlertThreshold is the number of failed cleanups of one directory after
	// which the model asks for an administrator.
	cleanupFailureAlertThreshold = 5
	// opsJobCompletedCondition and opsJobFailedReason are the condition the OpsJob
	// controller completes a job with, and its reason when the job failed.
	opsJobCompletedCondition = "JobCompleted"
	opsJobFailedReason       = "JobFailed"
	// downloadSlotWaitInterval is how long a download waits for a free slot.
	downloadSlotWaitInterval = 15 * time.Second
	// deletingPathWaitTimeout is how long a pending model waits for another model's
	// deletion to clean up a directory it wants; after that the directory is given up.
	deletingPathWaitTimeout = 30 * time.Minute
)

// ModelReconciler reconciles a Model object
type ModelReconciler struct {
	*ClusterBaseReconciler
	// apiReader reads straight from the API server. The global download limit counts
	// jobs with it, so a job created a moment ago is never missed by a stale cache.
	apiReader client.Reader
	// recorder reports what happens to a model's files when they cannot be removed.
	recorder record.EventRecorder
}

// SetupModelController sets up the controller with the Manager.
func SetupModelController(mgr manager.Manager) error {
	r := &ModelReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: mgr.GetClient(),
		},
		apiReader: mgr.GetAPIReader(),
		recorder:  mgr.GetEventRecorderFor("model-controller"),
	}
	err := ctrl.NewControllerManagedBy(mgr).
		For(&v1.Model{}).
		Owns(&batchv1.Job{}). // Watch Jobs created by this controller (S3 upload and S3 cleanup)
		Owns(&v1.OpsJob{}).   // Watch OpsJobs created by this controller (local download and cleanup)
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Model Controller successfully")
	return nil
}

// Reconcile handles the reconciliation loop
func (r *ModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// 1. Fetch the Model instance
	model := &v1.Model{}
	if err := r.Get(ctx, req.NamespacedName, model); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 2. Handle deletion
	if !model.GetDeletionTimestamp().IsZero() {
		return r.handleDelete(ctx, model)
	}

	// 3. Add finalizer if needed (only for Local models that need cleanup)
	if r.needsCleanup(model) && !controllerutil.ContainsFinalizer(model, ModelFinalizer) {
		controllerutil.AddFinalizer(model, ModelFinalizer)
		if err := r.Update(ctx, model); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// 4. Skip local_path models — they are already Ready on disk, no download needed.
	// We also backfill status.LocalPaths from spec.source.localPath when missing because
	// the apiserver Create call cannot atomically populate the status subresource, so
	// the array would otherwise stay empty in K8s/DB and the UI would not see a path.
	if model.Spec.Source.AccessMode == v1.AccessModeLocalPath {
		needsUpdate := false
		if model.Status.Phase != v1.ModelPhaseReady {
			model.Status.Phase = v1.ModelPhaseReady
			model.Status.Message = "Model available from local path"
			needsUpdate = true
		}
		if len(model.Status.LocalPaths) == 0 && strings.TrimSpace(model.Spec.Source.LocalPath) != "" {
			model.Status.LocalPaths = []v1.ModelLocalPath{{
				Workspace: model.Spec.Workspace,
				Path:      model.Spec.Source.LocalPath,
				Status:    v1.LocalPathStatusReady,
				Message:   "Registered from local_path",
			}}
			needsUpdate = true
		}
		if needsUpdate {
			model.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
			if err := r.Status().Update(ctx, model); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// 5. Initialize Status if needed
	if model.Status.Phase == "" {
		if model.IsRemoteAPI() {
			// Remote API models are immediately ready
			model.Status.Phase = v1.ModelPhaseReady
			model.Status.Message = "Remote API model is ready"
		} else {
			// Local models start in Pending phase
			model.Status.Phase = v1.ModelPhasePending
			model.Status.Message = "Waiting for processing"
		}
		model.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
		if err := r.Status().Update(ctx, model); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// 5. Processing logic based on Phase
	switch model.Status.Phase {
	case v1.ModelPhasePending:
		return r.handlePending(ctx, model)
	case v1.ModelPhaseUploading:
		return r.handleUploading(ctx, model)
	case v1.ModelPhaseDownloading:
		return r.handleDownloading(ctx, model)
	case v1.ModelPhaseReady:
		// A model is Ready as soon as one directory is, while its other directories
		// may still be downloading or waiting for a download slot; they are driven on.
		if model.IsLocal() && hasUnfinishedLocalPath(model) {
			return r.handleDownloading(ctx, model)
		}
		return ctrl.Result{}, nil
	case v1.ModelPhaseFailed:
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// hasUnfinishedLocalPath reports whether a directory of the model is still waiting for
// or running its download.
func hasUnfinishedLocalPath(model *v1.Model) bool {
	for _, lp := range model.Status.LocalPaths {
		if lp.Status == v1.LocalPathStatusPending || lp.Status == v1.LocalPathStatusDownloading {
			return true
		}
	}
	return false
}

// needsCleanup checks if the model needs cleanup on deletion (only Local type needs cleanup)
func (r *ModelReconciler) needsCleanup(model *v1.Model) bool {
	return model.Spec.Source.AccessMode == v1.AccessModeLocal
}

// handleDelete removes what a local model left behind before the finalizer is released:
// it stops the downloads, deletes the S3 copy and removes every local directory recorded
// in status.localPaths. A step that fails keeps the finalizer and is retried; the model
// is never released with its files still on disk.
func (r *ModelReconciler) handleDelete(ctx context.Context, model *v1.Model) (ctrl.Result, error) {
	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(model, ModelFinalizer) {
		return ctrl.Result{}, nil
	}

	// Only cleanup for Local models
	if !r.needsCleanup(model) {
		return r.releaseModel(ctx, model)
	}

	// An administrator gave up on the cleanup: release the model, leave the files.
	if model.GetAnnotations()[AbandonCleanupAnnotation] == v1.TrueStr {
		left := make([]string, 0, len(model.Status.LocalPaths)+1)
		for _, lp := range model.Status.LocalPaths {
			left = append(left, lp.Path)
		}
		if model.Status.S3Path != "" && !isS3ImportModel(model) {
			left = append(left, "s3:"+model.Status.S3Path)
		}
		message := fmt.Sprintf("Cleanup abandoned by %s, files left in place: %s",
			AbandonCleanupAnnotation, strings.Join(left, ", "))
		klog.InfoS(message, "model", model.Name)
		r.event(model, corev1.EventTypeWarning, "CleanupAbandoned", message)
		return r.releaseModel(ctx, model)
	}

	// 1. A download still writing into the directory would refill it after the cleanup.
	stopped, err := r.stopDownloads(ctx, model)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !stopped {
		klog.InfoS("Waiting for model downloads to stop before cleanup", "model", model.Name)
		message := "Waiting for the model's downloads to stop before removing its files"
		if time.Since(model.GetDeletionTimestamp().Time) > deletingPathWaitTimeout {
			message += fmt.Sprintf("; this is taking long: check the model's download workloads, or set the annotation "+
				"%s=true on the model to release it and leave the files on disk", AbandonCleanupAnnotation)
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, r.setDeletingMessage(ctx, model, message)
	}

	// 2. The S3 copy (only when the model was staged through S3).
	if result, done, err := r.cleanupS3(ctx, model); err != nil || !done {
		return result, err
	}

	// 3. The local directories.
	if result, done, err := r.cleanupLocalPaths(ctx, model); err != nil || !done {
		return result, err
	}
	return r.releaseModel(ctx, model)
}

// releaseModel removes the finalizer so the Model can go away.
func (r *ModelReconciler) releaseModel(ctx context.Context, model *v1.Model) (ctrl.Result, error) {
	controllerutil.RemoveFinalizer(model, ModelFinalizer)
	if err := r.Update(ctx, model); err != nil {
		return ctrl.Result{}, err
	}
	klog.InfoS("Model cleanup completed, finalizer removed", "model", model.Name)
	return ctrl.Result{}, nil
}

// stopDownloads deletes every download job of the model and reports whether they are
// all gone, including the workloads that ran them. Workloads are looked up in every
// workspace, not only the ones status.localPaths names now: after a failover the
// workload of the earlier workspace may still be terminating and writing.
func (r *ModelReconciler) stopDownloads(ctx context.Context, model *v1.Model) (bool, error) {
	stopped := true

	// HuggingFace -> S3 upload job (S3 staging only).
	uploadJob := &batchv1.Job{}
	err := r.Get(ctx, client.ObjectKey{Name: stringutil.NormalizeForDNS(model.Name), Namespace: common.PrimusSafeNamespace}, uploadJob)
	if err == nil {
		stopped = false
		if uploadJob.GetDeletionTimestamp().IsZero() {
			if err = r.Delete(ctx, uploadJob, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil && !errors.IsNotFound(err) {
				return false, err
			}
		}
	} else if !errors.IsNotFound(err) {
		return false, err
	}

	opsJobs := &v1.OpsJobList{}
	if err = r.List(ctx, opsJobs, client.MatchingLabels{v1.ModelIdLabel: model.Name}); err != nil {
		return false, err
	}
	for i := range opsJobs.Items {
		job := &opsJobs.Items[i]
		if job.Spec.Type != v1.OpsJobDownloadType {
			continue
		}
		stopped = false
		if job.GetDeletionTimestamp().IsZero() {
			if err = r.Delete(ctx, job); err != nil && !errors.IsNotFound(err) {
				return false, err
			}
			klog.InfoS("Stopping model download before cleanup", "model", model.Name, "job", job.Name)
		}
	}
	if !stopped {
		return false, nil
	}

	// The OpsJob can be gone while its workload is still terminating.
	workloads, err := r.downloadWorkloads(ctx, model)
	if err != nil {
		return false, err
	}
	for i := range workloads {
		wl := &workloads[i]
		// Only a workload labelled with this model is certainly its own; one matched by
		// name alone may belong to another model whose name and workspace join to the
		// same job name, so it is waited for but never deleted.
		if wl.GetLabels()[v1.ModelIdLabel] == model.Name && wl.GetDeletionTimestamp().IsZero() {
			if err = r.Delete(ctx, wl); err != nil && !errors.IsNotFound(err) {
				return false, err
			}
		}
		klog.InfoS("Waiting for model download workload to terminate", "model", model.Name,
			"workload", wl.Name, "workspace", wl.Spec.Workspace)
	}
	return len(workloads) == 0, nil
}

// downloadWorkloads returns the download workloads of the model in any workspace: those
// labelled with the model id, and, for workloads created before that label existed,
// unlabelled ones named after the model's download job in some workspace.
func (r *ModelReconciler) downloadWorkloads(ctx context.Context, model *v1.Model) ([]v1.Workload, error) {
	names := map[string]bool{}
	for _, lp := range model.Status.LocalPaths {
		names[downloadJobName(model, lp.Workspace)] = true
	}
	workspaces := &v1.WorkspaceList{}
	if err := r.List(ctx, workspaces); err != nil {
		return nil, err
	}
	for i := range workspaces.Items {
		names[downloadJobName(model, workspaces.Items[i].Name)] = true
	}
	list := &v1.WorkloadList{}
	if err := r.List(ctx, list, client.MatchingLabels{v1.OpsJobTypeLabel: string(v1.OpsJobDownloadType)}); err != nil {
		return nil, err
	}
	var out []v1.Workload
	for i := range list.Items {
		wl := &list.Items[i]
		owner := wl.GetLabels()[v1.ModelIdLabel]
		if owner == model.Name || (owner == "" && names[wl.Name]) {
			out = append(out, *wl)
		}
	}
	return out, nil
}

// cleanupS3 removes the S3 copy of a model that was staged through S3. Success clears
// status.s3Path; failure is retried and keeps the finalizer.
func (r *ModelReconciler) cleanupS3(ctx context.Context, model *v1.Model) (ctrl.Result, bool, error) {
	if model.Status.S3Path == "" || isS3ImportModel(model) {
		return ctrl.Result{}, true, nil
	}
	if !commonconfig.IsS3Enable() {
		// The bucket cannot be reached any more; nothing in this cluster can clean it.
		klog.InfoS("S3 is disabled, skipping S3 cleanup of the model", "model", model.Name, "s3Path", model.Status.S3Path)
		return ctrl.Result{}, true, nil
	}

	jobName := stringutil.NormalizeForDNS(CleanupJobPrefix + model.Name)
	job := &batchv1.Job{}
	err := r.Get(ctx, client.ObjectKey{Name: jobName, Namespace: common.PrimusSafeNamespace}, job)
	if errors.IsNotFound(err) {
		// The S3 path is derived from the display name, so another model can share it.
		if owner, err := r.liveModelOnS3Path(ctx, model); err != nil {
			return ctrl.Result{}, false, err
		} else if owner != "" {
			message := fmt.Sprintf("Leaving the S3 copy %s in place, model %s uses it", model.Status.S3Path, owner)
			klog.InfoS(message, "model", model.Name)
			r.event(model, corev1.EventTypeNormal, "CleanupSkipped", message)
			model.Status.S3Path = ""
			model.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
			return ctrl.Result{Requeue: true}, false, r.Status().Update(ctx, model)
		}
		if job, err = r.constructCleanupJob(model); err != nil {
			klog.ErrorS(err, "Failed to construct S3 cleanup job, will retry", "model", model.Name)
			return ctrl.Result{RequeueAfter: cleanupRetryInterval}, false, r.setDeletingMessage(ctx, model,
				fmt.Sprintf("S3 cleanup cannot start: %v", err))
		}
		if err = r.Create(ctx, job); err != nil {
			return ctrl.Result{}, false, err
		}
		klog.InfoS("S3 cleanup job created", "model", model.Name, "job", jobName)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, false, nil
	} else if err != nil {
		return ctrl.Result{}, false, err
	}
	if !job.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, false, nil
	}

	if job.Status.Succeeded > 0 {
		if err = r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, false, err
		}
		model.Status.S3Path = ""
		model.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
		klog.InfoS("Model S3 cleanup completed", "model", model.Name)
		return ctrl.Result{Requeue: true}, false, r.Status().Update(ctx, model)
	}
	if job.Status.Failed > 0 && job.Status.Active == 0 {
		reason := r.extractJobFailureReason(job)
		klog.ErrorS(nil, "S3 cleanup job failed, will retry", "model", model.Name, "reason", reason)
		if err = r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, false, err
		}
		return ctrl.Result{RequeueAfter: cleanupRetryInterval}, false, r.setDeletingMessage(ctx, model,
			fmt.Sprintf("S3 cleanup failed, retrying: %s (to give up and leave the S3 copy, set the annotation %s=true on the model)",
				reason, AbandonCleanupAnnotation))
	}
	return ctrl.Result{RequeueAfter: 5 * time.Second}, false, nil
}

// liveModelOnS3Path returns a model other than model, not being deleted, that stages its
// files at the same platform S3 path, or "" when there is none. Only a path recorded in
// the other model's status counts: a model that never uploaded (S3 disabled when it was
// created, or failed before the upload) holds no S3 copy, even though GetS3Path would
// derive the same path from its display name.
func (r *ModelReconciler) liveModelOnS3Path(ctx context.Context, model *v1.Model) (string, error) {
	models := &v1.ModelList{}
	if err := r.List(ctx, models); err != nil {
		return "", err
	}
	self := s3Prefix(model.Status.S3Path)
	for i := range models.Items {
		m := &models.Items[i]
		if m.Name == model.Name || !m.GetDeletionTimestamp().IsZero() || !m.IsLocal() || isS3ImportModel(m) {
			continue
		}
		if m.Status.S3Path != "" && s3Prefix(m.Status.S3Path) == self {
			return m.Name, nil
		}
	}
	return "", nil
}

// s3Prefix is the key prefix of the objects below an S3 path: the path with exactly one
// trailing "/", so that "models/a" never matches the objects of "models/a-b".
func s3Prefix(p string) string {
	return strings.TrimRight(p, "/") + "/"
}

// cleanupLocalPaths removes the local directory of every entry in status.localPaths with
// a cleanup job that runs in a workspace where the directory's volume is mounted. An entry
// leaves status.localPaths once its directory is gone, so the remaining entries are
// exactly what is still on disk. A directory another model still points at is kept.
//
// A directory no workspace can reach any more (its workspace and every other workspace
// of the cluster lost the volume, or the workspace itself is gone) can neither be removed
// nor written to by this platform: it is left on disk with a warning event and the model
// is not held for it. A cleanup that fails is retried with a backoff; after
// cleanupFailureAlertThreshold failures the model says that it needs an administrator,
// who fixes the cause or sets AbandonCleanupAnnotation.
func (r *ModelReconciler) cleanupLocalPaths(ctx context.Context, model *v1.Model) (ctrl.Result, bool, error) {
	if len(model.Status.LocalPaths) == 0 {
		return ctrl.Result{}, true, nil
	}
	models := &v1.ModelList{}
	if err := r.List(ctx, models); err != nil {
		return ctrl.Result{}, false, err
	}
	clusterOf, err := r.workspaceClusters(ctx)
	if err != nil {
		return ctrl.Result{}, false, err
	}

	var (
		remaining []v1.ModelLocalPath
		messages  []string
		changed   bool
		requeue   time.Duration
	)
	// wait asks for the next pass after d at the latest.
	wait := func(d time.Duration) {
		if d > 0 && (requeue == 0 || d < requeue) {
			requeue = d
		}
	}
	now := time.Now().UTC()
	for _, lp := range model.Status.LocalPaths {
		if lp.Path == "" {
			changed = true
			continue
		}
		if owner := liveModelOnPath(models.Items, clusterOf, model.Name, lp); owner != "" {
			klog.InfoS("Keeping model directory, another model points at it",
				"model", model.Name, "path", lp.Path, "otherModel", owner)
			changed = true
			continue
		}
		workspace, unreachable, err := r.cleanupWorkspace(ctx, lp)
		if err != nil {
			return ctrl.Result{}, false, err
		}
		if workspace == nil {
			message := fmt.Sprintf("Leaving %s on disk: %s", lp.Path, unreachable)
			klog.InfoS(message, "model", model.Name)
			r.event(model, corev1.EventTypeWarning, "CleanupSkipped", message)
			changed = true
			continue
		}

		jobName := cleanupJobName(model, workspace.Name, lp.Path)
		job := &v1.OpsJob{}
		err = r.Get(ctx, client.ObjectKey{Name: jobName}, job)
		failure := ""
		switch {
		case errors.IsNotFound(err):
			if delay := cleanupBackoff(&lp, now); delay > 0 {
				messages = append(messages, cleanupFailureMessage(&lp, delay))
				wait(delay)
				break
			}
			job, err = r.constructModelCleanupOpsJob(model, workspace, lp.Path)
			if err == nil {
				err = r.Create(ctx, job)
			}
			if err != nil {
				klog.ErrorS(err, "Failed to create model cleanup job, will retry", "model", model.Name, "path", lp.Path)
				failure = fmt.Sprintf("cleanup of %s cannot start: %v", lp.Path, err)
			} else {
				klog.InfoS("Model cleanup job created", "model", model.Name, "job", jobName,
					"path", lp.Path, "workspace", workspace.Name)
				wait(5 * time.Second)
			}
		case err != nil:
			return ctrl.Result{}, false, err
		case !job.GetDeletionTimestamp().IsZero():
			wait(5 * time.Second)
		case job.Status.Phase == v1.OpsJobSucceeded:
			klog.InfoS("Model directory removed", "model", model.Name, "path", lp.Path)
			if err = r.Delete(ctx, job); err != nil && !errors.IsNotFound(err) {
				klog.ErrorS(err, "Failed to delete finished cleanup job", "job", jobName)
			}
			changed = true
			continue
		case job.Status.Phase == v1.OpsJobFailed:
			reason := r.extractOpsJobFailureReason(job)
			klog.ErrorS(nil, "Model cleanup job failed, will retry", "model", model.Name, "path", lp.Path, "reason", reason)
			if err = r.Delete(ctx, job); err != nil && !errors.IsNotFound(err) {
				return ctrl.Result{}, false, err
			}
			failure = fmt.Sprintf("cleanup of %s failed: %s", lp.Path, reason)
		default:
			wait(5 * time.Second)
		}
		if failure != "" {
			lp.CleanupFailures++
			lp.LastCleanupFailureTime = &metav1.Time{Time: now}
			lp.Message = failure
			changed = true
			delay := cleanupBackoff(&lp, now)
			messages = append(messages, cleanupFailureMessage(&lp, delay))
			wait(delay)
			if lp.CleanupFailures == cleanupFailureAlertThreshold {
				r.event(model, corev1.EventTypeWarning, "CleanupStuck", cleanupFailureMessage(&lp, delay))
			}
		}
		remaining = append(remaining, lp)
	}

	message := strings.Join(messages, "; ")
	if changed || (message != "" && message != model.Status.Message) {
		model.Status.LocalPaths = remaining
		if message != "" {
			model.Status.Message = message
		}
		model.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
		if err := r.Status().Update(ctx, model); err != nil {
			return ctrl.Result{}, false, err
		}
	}
	if len(remaining) == 0 {
		return ctrl.Result{}, true, nil
	}
	if requeue == 0 {
		requeue = 5 * time.Second
	}
	return ctrl.Result{RequeueAfter: requeue}, false, nil
}

// cleanupWorkspace returns the workspace a cleanup of lp runs in: the workspace recorded
// for it, or, when that workspace no longer mounts the directory, another workspace of
// the same cluster that does. When there is none it returns nil and the reason; the
// directory is then out of reach of every job this platform can run.
func (r *ModelReconciler) cleanupWorkspace(ctx context.Context, lp v1.ModelLocalPath) (*v1.Workspace, string, error) {
	workspace := &v1.Workspace{}
	if err := r.Get(ctx, client.ObjectKey{Name: lp.Workspace}, workspace); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Sprintf("workspace %s no longer exists", lp.Workspace), nil
		}
		return nil, "", err
	}
	pathErr := validateCleanupPath(workspace, lp.Path)
	if pathErr == nil {
		return workspace, "", nil
	}
	if workspace.Spec.Cluster != "" {
		workspaces := &v1.WorkspaceList{}
		if err := r.List(ctx, workspaces); err != nil {
			return nil, "", err
		}
		for i := range workspaces.Items {
			ws := &workspaces.Items[i]
			if ws.Name != workspace.Name && ws.Spec.Cluster == workspace.Spec.Cluster &&
				ws.GetDeletionTimestamp().IsZero() && validateCleanupPath(ws, lp.Path) == nil {
				return ws, "", nil
			}
		}
	}
	return nil, fmt.Sprintf("no workspace can remove it: %v", pathErr), nil
}

// cleanupBackoff returns how much longer the next cleanup of lp has to wait after its
// last failure: cleanupRetryInterval after the first, doubling with every further
// failure up to cleanupMaxRetryInterval.
func cleanupBackoff(lp *v1.ModelLocalPath, now time.Time) time.Duration {
	if lp.CleanupFailures <= 0 || lp.LastCleanupFailureTime == nil {
		return 0
	}
	delay := cleanupRetryInterval
	for i := int32(1); i < lp.CleanupFailures && delay < cleanupMaxRetryInterval; i++ {
		delay *= 2
	}
	if delay > cleanupMaxRetryInterval {
		delay = cleanupMaxRetryInterval
	}
	if left := lp.LastCleanupFailureTime.Add(delay).Sub(now); left > 0 {
		return left
	}
	return 0
}

// cleanupFailureMessage describes a failing cleanup of lp, and once it has failed
// cleanupFailureAlertThreshold times, what an administrator can do about it.
func cleanupFailureMessage(lp *v1.ModelLocalPath, delay time.Duration) string {
	retry := "retrying"
	if delay > 0 {
		retry = fmt.Sprintf("retrying in %s", delay.Round(time.Second))
	}
	message := fmt.Sprintf("%s (failure %d), %s", lp.Message, lp.CleanupFailures, retry)
	if lp.CleanupFailures >= cleanupFailureAlertThreshold {
		message += fmt.Sprintf("; needs an administrator: fix the cause, or set the annotation %s=true "+
			"on the model to release it and leave the files on disk", AbandonCleanupAnnotation)
	}
	return message
}

// event records an event on the model when the controller has a recorder.
func (r *ModelReconciler) event(model *v1.Model, eventType, reason, message string) {
	if r.recorder != nil {
		r.recorder.Event(model, eventType, reason, message)
	}
}

// setDeletingMessage records why a deletion is still in progress.
func (r *ModelReconciler) setDeletingMessage(ctx context.Context, model *v1.Model, message string) error {
	if model.Status.Message == message {
		return nil
	}
	model.Status.Message = message
	model.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
	return r.Status().Update(ctx, model)
}

// liveModelOnPath returns the name of a model other than self that is not being deleted
// and holds the directory of target, or "" when there is none. Only an entry that is not
// Failed holds a directory: a Failed entry was never downloaded there, or gave up on it
// (e.g. "already used by model X"), and must not keep that directory on disk for ever.
func liveModelOnPath(models []v1.Model, clusterOf map[string]string, self string, target v1.ModelLocalPath) string {
	for i := range models {
		m := &models[i]
		if m.Name == self || !m.GetDeletionTimestamp().IsZero() {
			continue
		}
		for _, lp := range m.Status.LocalPaths {
			if lp.Status != v1.LocalPathStatusFailed && sameDirectory(clusterOf, lp, target) {
				return m.Name
			}
		}
	}
	return ""
}

// sameDirectory reports whether two entries name one directory: the same path on the
// same cluster. An entry whose workspace is gone has no known cluster and is taken to
// be on any cluster.
func sameDirectory(clusterOf map[string]string, a, b v1.ModelLocalPath) bool {
	if a.Path != b.Path {
		return false
	}
	ca, okA := clusterOf[a.Workspace]
	cb, okB := clusterOf[b.Workspace]
	return !okA || !okB || ca == cb
}

// workspaceClusters maps every workspace to its cluster.
func (r *ModelReconciler) workspaceClusters(ctx context.Context) (map[string]string, error) {
	workspaces := &v1.WorkspaceList{}
	if err := r.List(ctx, workspaces); err != nil {
		return nil, err
	}
	clusterOf := make(map[string]string, len(workspaces.Items))
	for i := range workspaces.Items {
		clusterOf[workspaces.Items[i].Name] = workspaces.Items[i].Spec.Cluster
	}
	return clusterOf, nil
}

// validateCleanupPath accepts only an absolute, clean path below a "models" segment that
// lies inside one of the workspace volumes and is not the volume root itself.
func validateCleanupPath(workspace *v1.Workspace, p string) error {
	if !strings.HasPrefix(p, "/") || path.Clean(p) != p {
		return fmt.Errorf("path %q is not absolute and clean", p)
	}
	segs := strings.Split(strings.TrimPrefix(p, "/"), "/")
	belowModels := false
	for i, seg := range segs {
		if seg == "models" && i < len(segs)-1 {
			belowModels = true
			break
		}
	}
	if !belowModels {
		return fmt.Errorf("path %q is not below a models directory", p)
	}
	if root := volumeRootOf(workspace, p); root == "" || root == p {
		return fmt.Errorf("path %q is not inside a volume of workspace %s", p, workspace.Name)
	}
	return nil
}

// cleanupJobName is the name of the job that removes one local directory of a model.
func cleanupJobName(model *v1.Model, workspace, p string) string {
	sum := sha256.Sum256([]byte(workspace + "\x00" + p))
	return stringutil.NormalizeForDNS(fmt.Sprintf("%s%s-%s", CleanupJobPrefix, model.Name, hex.EncodeToString(sum[:])[:8]))
}

// downloadJobName is the name of the job that downloads a model into one workspace.
func downloadJobName(model *v1.Model, workspace string) string {
	return stringutil.NormalizeForDNS(fmt.Sprintf("%s-%s-%s", DownloadJobPrefix, model.Name, workspace))
}

// constructCleanupJob creates a Job that deletes the S3 copy of a model.
func (r *ModelReconciler) constructCleanupJob(model *v1.Model) (*batchv1.Job, error) {
	// Get system S3 configuration
	if !commonconfig.IsS3Enable() {
		return nil, fmt.Errorf("S3 storage is not enabled in system configuration")
	}
	s3Endpoint := commonconfig.GetS3Endpoint()
	s3AccessKey := commonconfig.GetS3AccessKey()
	s3SecretKey := commonconfig.GetS3SecretKey()
	s3Bucket := commonconfig.GetS3Bucket()
	if s3Endpoint == "" || s3AccessKey == "" || s3SecretKey == "" || s3Bucket == "" {
		return nil, fmt.Errorf("S3 configuration is incomplete")
	}

	s3Path := model.Status.S3Path
	if s3Path == "" {
		s3Path = model.GetS3Path()
	}
	// The trailing "/" keeps "aws s3 rm --recursive" to this model's objects: without it
	// the key prefix "models/a" also matches the objects of "models/a-b".
	fullS3Path := fmt.Sprintf("s3://%s/%s", s3Bucket, s3Prefix(s3Path))

	// Use the model downloader image from config
	image := commonconfig.GetModelDownloaderImage()

	backoffLimit := int32(1)
	ttlSeconds := int32(60)

	jobName := stringutil.NormalizeForDNS(CleanupJobPrefix + model.Name)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{
				"app":   "model-cleanup",
				"model": model.Name,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttlSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":   "model-cleanup",
						"model": model.Name,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:            "cleanup",
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							// A failed removal fails the job, so the model keeps its finalizer.
							Command: []string{
								"/bin/sh", "-c",
								`set -e; echo "Cleaning S3 path: $S3_PATH"; aws s3 rm "$S3_PATH" --recursive --endpoint-url "$S3_ENDPOINT"`,
							},
							Env: []corev1.EnvVar{
								{Name: "S3_PATH", Value: fullS3Path},
								{Name: "S3_ENDPOINT", Value: s3Endpoint},
								{Name: "AWS_ACCESS_KEY_ID", Value: s3AccessKey},
								{Name: "AWS_SECRET_ACCESS_KEY", Value: s3SecretKey},
								{Name: "AWS_DEFAULT_REGION", Value: "us-east-1"},
							},
						},
					},
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(model, job, r.Scheme()); err != nil {
		return nil, err
	}
	return job, nil
}

// constructModelCleanupOpsJob creates the job that removes one local model directory.
// It runs in the workspace that owns the directory, as the model owner, so it sees the
// same volume and has the same rights as the download that wrote the files.
func (r *ModelReconciler) constructModelCleanupOpsJob(model *v1.Model, workspace *v1.Workspace, p string) (*v1.OpsJob, error) {
	if workspace.Spec.Cluster == "" {
		return nil, fmt.Errorf("workspace %s has no cluster configured", workspace.Name)
	}
	if err := validateCleanupPath(workspace, p); err != nil {
		return nil, err
	}
	userId, userName := modelOwner(model)
	image := commonconfig.GetModelCleanupImage()
	entryPoint := base64.StdEncoding.EncodeToString([]byte(modelCleanupScript))
	jobName := cleanupJobName(model, workspace.Name, p)
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: jobName,
			Labels: map[string]string{
				v1.ClusterIdLabel:   workspace.Spec.Cluster,
				v1.WorkspaceIdLabel: workspace.Name,
				v1.ModelIdLabel:     model.Name,
				v1.DisplayNameLabel: jobDisplayName("cleanup-" + model.GetLocalDirName()),
				v1.UserIdLabel:      userId,
				v1.OpsJobTypeLabel:  string(v1.OpsJobModelCleanupType),
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation: userName,
			},
		},
		Spec: v1.OpsJobSpec{
			Type:       v1.OpsJobModelCleanupType,
			Image:      &image,
			EntryPoint: &entryPoint,
			// The volume root validateCleanupPath found, which the script checks against.
			Env:                     map[string]string{"VOLUME_ROOT": volumeRootOf(workspace, p)},
			TimeoutSecond:           3600,
			TTLSecondsAfterFinished: 600,
			Inputs: []v1.Parameter{
				{Name: v1.ParameterDestPath, Value: p},
				{Name: v1.ParameterWorkspace, Value: workspace.Name},
			},
		},
	}
	if err := controllerutil.SetControllerReference(model, job, r.Scheme()); err != nil {
		return nil, err
	}
	return job, nil
}

// modelCleanupScript removes $DEST_PATH and fails unless it is gone afterwards. Every
// failure is reported on an "[ERROR]" line: only those lines reach the job's failure
// message, which the model shows while the cleanup is retried.
//
// Before removing anything it repeats the rule validateCleanupPath applied, against the
// volume root ($VOLUME_ROOT) that check found: an absolute, clean path strictly inside
// that volume with something below a "models" segment. A volume mounted at "/" is a
// volume like any other.
const modelCleanupScript = `set -u
fail() { echo "[ERROR] $*"; exit 1; }
[ -n "${VOLUME_ROOT:-}" ] || fail "refusing to remove $DEST_PATH: no volume root given"
root="${VOLUME_ROOT%/}"
case "$DEST_PATH" in
  "$root"/?*) ;;
  *) fail "refusing to remove $DEST_PATH: not inside the volume $VOLUME_ROOT" ;;
esac
case "$DEST_PATH" in
  *//*|*/./*|*/../*|*/.|*/..|*/) fail "refusing to remove $DEST_PATH: not a clean path" ;;
esac
case "$DEST_PATH" in
  */models/?*) ;;
  *) fail "refusing to remove $DEST_PATH: not a model directory" ;;
esac
echo "Removing model directory $DEST_PATH"
if ! out=$(rm -rf -- "$DEST_PATH" 2>&1); then
  fail "removing $DEST_PATH failed: $(printf '%s\n' "$out" | tail -n 3 | tr '\n' ' ')"
fi
[ ! -e "$DEST_PATH" ] || fail "$DEST_PATH still exists after removal"
echo "Removed $DEST_PATH"
`

// modelOwner returns the user a model's jobs run as: the model owner, so files are
// written and removed with the owner's identity on storage that maps users to their
// own uid. Models without an owner fall back to the system user.
func modelOwner(model *v1.Model) (string, string) {
	userId := v1.GetUserId(model)
	if userId == "" {
		return common.UserSystem, common.UserSystem
	}
	userName := v1.GetUserName(model)
	if userName == "" {
		userName = userId
	}
	return userId, userName
}

// jobDisplayName turns s into a display name the OpsJob webhook accepts.
func jobDisplayName(s string) string {
	name := stringutil.NormalizeForDNS(s)
	if len(name) > 40 {
		name = name[:40]
	}
	name = strings.Trim(name, "-")
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		name = "m" + name
	}
	return name
}

func (r *ModelReconciler) handlePending(ctx context.Context, model *v1.Model) (ctrl.Result, error) {
	// Remote API models should already be Ready
	if model.IsRemoteAPI() {
		model.Status.Phase = v1.ModelPhaseReady
		model.Status.Message = "Remote API model is ready"
		return ctrl.Result{}, r.Status().Update(ctx, model)
	}

	// A model whose name gives no safe directory has nowhere to be downloaded to.
	if message := localDirNameFailure(model); message != "" {
		return ctrl.Result{}, r.failModel(ctx, model, message)
	}

	// A directory that another model is still being deleted from, or that another
	// model owns, cannot be used: the download would race the cleanup, or two models
	// would share one set of files. Such a directory is given up on its own; the
	// model's other directories are downloaded as usual.
	paths, waitFor, err := r.planTargetPaths(ctx, model)
	if err != nil || waitFor > 0 {
		return ctrl.Result{RequeueAfter: waitFor}, err
	}
	if message := allPathsFailed(paths); message != "" {
		model.Status.Phase = v1.ModelPhaseFailed
		model.Status.Message = message
		model.Status.LocalPaths = paths
		model.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
		return ctrl.Result{}, r.Status().Update(ctx, model)
	}

	// Without S3 the model is downloaded from HuggingFace straight into the workspace
	// storage by the per-workspace download job.
	if !isS3ImportModel(model) && !commonconfig.IsS3Enable() {
		model.Status.Phase = v1.ModelPhaseDownloading
		model.Status.Message = "Starting download into workspace storage"
		model.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
		model.Status.S3Path = ""
		model.Status.LocalPaths = paths
		klog.InfoS("Model download goes straight to workspace storage", "model", model.Name, "url", model.Spec.Source.URL)
		return ctrl.Result{}, r.Status().Update(ctx, model)
	}

	// s3_sync (S3 import) models skip the platform-bucket upload step entirely.
	// We point the per-workspace download OpsJob INPUT_URL directly at the user's s3 URI,
	// using the user-provided secret if present. This saves a full copy in size+time.
	if isS3ImportModel(model) {
		model.Status.Phase = v1.ModelPhaseDownloading
		model.Status.Message = "S3 import: starting per-workspace download"
		model.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
		// Don't set Status.S3Path; downloads target user S3 directly.
		model.Status.LocalPaths = paths
		klog.InfoS("S3 import model: skipped Uploading phase", "model", model.Name, "url", model.Spec.Source.URL)
		return ctrl.Result{}, r.Status().Update(ctx, model)
	}

	// For local models, start the upload job to S3
	jobName := stringutil.NormalizeForDNS(model.Name)
	job := &batchv1.Job{}
	err = r.Get(ctx, client.ObjectKey{Name: jobName, Namespace: common.PrimusSafeNamespace}, job)

	if errors.IsNotFound(err) {
		// Construct download/upload job
		job, err = r.constructDownloadJob(model)
		if err != nil {
			klog.ErrorS(err, "Failed to construct download job", "model", model.Name, "url", model.Spec.Source.URL)
			model.Status.Phase = v1.ModelPhaseFailed
			model.Status.Message = fmt.Sprintf("Failed to construct download job: %v", err)
			return ctrl.Result{}, r.Status().Update(ctx, model)
		}

		if err := r.Create(ctx, job); err != nil {
			klog.ErrorS(err, "Failed to create download job", "model", model.Name, "jobName", jobName)
			if errors.IsInvalid(err) || errors.IsForbidden(err) {
				model.Status.Phase = v1.ModelPhaseFailed
				model.Status.Message = fmt.Sprintf("Failed to create download job: %v", err)
				return ctrl.Result{}, r.Status().Update(ctx, model)
			}
			return ctrl.Result{}, err
		}

		// Update status to Uploading
		model.Status.Phase = v1.ModelPhaseUploading
		model.Status.Message = fmt.Sprintf("Download job created: %s", jobName)
		model.Status.S3Path = model.GetS3Path()
		model.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
		klog.InfoS("Download job created", "model", model.Name, "jobName", jobName, "url", model.Spec.Source.URL)

		return ctrl.Result{}, r.Status().Update(ctx, model)
	} else if err != nil {
		klog.ErrorS(err, "Failed to get download job", "model", model.Name, "jobName", jobName)
		return ctrl.Result{}, err
	}

	// Job already exists, transition to Uploading
	model.Status.Phase = v1.ModelPhaseUploading
	model.Status.Message = fmt.Sprintf("Download in progress (Job: %s)", jobName)
	model.Status.S3Path = model.GetS3Path()
	klog.InfoS("Download job already exists", "model", model.Name, "jobName", jobName)

	return ctrl.Result{}, r.Status().Update(ctx, model)
}

// planTargetPaths returns the directories a model downloads into, one per (cluster,
// volume root). A directory held by another model is decided on its own:
//   - a live model records it in an entry that is not Failed: the entry is Failed with
//     the owner named, the model never writes into another model's files;
//   - a model being deleted records it: the model waits (waitFor > 0, status message
//     set) for that cleanup to finish, at most deletingPathWaitTimeout from the start
//     of that deletion, after which the entry is Failed with the reason.
//
// The other entries are returned Pending. Two directories are the same only when they
// have the same path on the same cluster; the same mount path on another cluster is a
// different filesystem.
func (r *ModelReconciler) planTargetPaths(ctx context.Context, model *v1.Model) ([]v1.ModelLocalPath, time.Duration, error) {
	paths := r.initializeLocalPaths(ctx, model)
	if len(paths) == 0 {
		return paths, 0, nil
	}
	models := &v1.ModelList{}
	if err := r.List(ctx, models); err != nil {
		return nil, 0, err
	}
	clusterOf, err := r.workspaceClusters(ctx)
	if err != nil {
		return nil, 0, err
	}

	var (
		waiting  []string
		waitFor  time.Duration
		now      = time.Now().UTC()
		failures int
	)
	for i := range paths {
		lp := &paths[i]
	others:
		for j := range models.Items {
			other := &models.Items[j]
			if other.Name == model.Name || !other.IsLocal() {
				continue
			}
			for _, olp := range other.Status.LocalPaths {
				if !sameDirectory(clusterOf, *lp, olp) {
					continue
				}
				deleting := other.GetDeletionTimestamp()
				// A live model's Failed entry does not hold the directory (see
				// liveModelOnPath); a deleting model's does, its cleanup removes it.
				if deleting.IsZero() && olp.Status == v1.LocalPathStatusFailed {
					continue
				}
				if !deleting.IsZero() {
					deadline := deleting.Add(deletingPathWaitTimeout)
					if now.Before(deadline) {
						waiting = append(waiting, fmt.Sprintf("model %s to finish deleting %s", other.Name, lp.Path))
						waitFor = 10 * time.Second
						break others
					}
					lp.Status = v1.LocalPathStatusFailed
					lp.Message = fmt.Sprintf("%s is still being cleaned up by the deletion of model %s, started %s ago; "+
						"see that model's status, then retry this model",
						lp.Path, other.Name, now.Sub(deleting.Time).Round(time.Minute))
				} else {
					lp.Status = v1.LocalPathStatusFailed
					lp.Message = fmt.Sprintf("%s is already used by model %s", lp.Path, other.Name)
				}
				failures++
				klog.InfoS("Model directory is held by another model", "model", model.Name,
					"path", lp.Path, "workspace", lp.Workspace, "reason", lp.Message)
				break others
			}
		}
	}

	if len(waiting) > 0 {
		message := "Waiting for " + strings.Join(waiting, ", ")
		klog.InfoS(message, "model", model.Name)
		if model.Status.Message != message {
			model.Status.Message = message
			model.Status.UpdateTime = &metav1.Time{Time: now}
			if err := r.Status().Update(ctx, model); err != nil {
				return nil, 0, err
			}
		}
		return nil, waitFor, nil
	}
	if failures > 0 && failures < len(paths) {
		r.event(model, corev1.EventTypeWarning, "PathSkipped",
			fmt.Sprintf("%d of %d target directories are held by other models and are skipped", failures, len(paths)))
	}
	return paths, 0, nil
}

// localDirNameFailure returns why the model has no local directory name, or "" when it
// has one. The apiserver refuses such names; this catches a model created or renamed
// another way, which would otherwise download outside its models directory.
func localDirNameFailure(model *v1.Model) string {
	if model.GetLocalDirName() != "" {
		return ""
	}
	return fmt.Sprintf("display name %q cannot be used as the model directory name: "+
		"use only letters, digits, '.', '-', '_', ' ', '/' or ':', and not \".\" or \"..\"", model.Spec.DisplayName)
}

// failModel moves the model to Failed with message.
func (r *ModelReconciler) failModel(ctx context.Context, model *v1.Model, message string) error {
	klog.InfoS("Model failed", "model", model.Name, "reason", message)
	model.Status.Phase = v1.ModelPhaseFailed
	model.Status.Message = message
	model.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
	return r.Status().Update(ctx, model)
}

// allPathsFailed returns why a model has nothing left to download when every one of
// paths has failed, or "" when at least one can still be downloaded.
func allPathsFailed(paths []v1.ModelLocalPath) string {
	if len(paths) == 0 {
		return ""
	}
	reasons := make([]string, 0, len(paths))
	for _, lp := range paths {
		if lp.Status != v1.LocalPathStatusFailed {
			return ""
		}
		reasons = append(reasons, lp.Message)
	}
	return "No target directory can be used: " + strings.Join(reasons, "; ")
}

// handleUploading handles the Uploading phase (downloading from HuggingFace to S3)
func (r *ModelReconciler) handleUploading(ctx context.Context, model *v1.Model) (ctrl.Result, error) {
	jobName := stringutil.NormalizeForDNS(model.Name)
	job := &batchv1.Job{}
	if err := r.Get(ctx, client.ObjectKey{Name: jobName, Namespace: common.PrimusSafeNamespace}, job); err != nil {
		if errors.IsNotFound(err) {
			model.Status.Phase = v1.ModelPhaseFailed
			model.Status.Message = "Download job lost or deleted unexpectedly"
			klog.InfoS("Download job lost or deleted unexpectedly", "model", model.Name)
			return ctrl.Result{}, r.Status().Update(ctx, model)
		}
		return ctrl.Result{}, err
	}

	// Success case
	if job.Status.Succeeded > 0 {
		// The display name may have changed since the model left Pending.
		if message := localDirNameFailure(model); message != "" {
			return ctrl.Result{}, r.failModel(ctx, model, message)
		}
		// Initialize local paths based on workspace configuration; a directory held by
		// another model is given up on its own, or waited for while it is being deleted.
		paths, waitFor, err := r.planTargetPaths(ctx, model)
		if err != nil || waitFor > 0 {
			return ctrl.Result{RequeueAfter: waitFor}, err
		}

		// S3 upload completed, now start downloading to local PFS
		model.Status.Phase = v1.ModelPhaseDownloading
		model.Status.Message = "S3 upload completed, starting local download"
		model.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
		model.Status.LocalPaths = paths
		if message := allPathsFailed(paths); message != "" {
			model.Status.Phase = v1.ModelPhaseFailed
			model.Status.Message = message
		}

		klog.InfoS("Model S3 upload completed, starting local download", "model", model.Name, "s3Path", model.Status.S3Path)

		// Delete the completed upload job
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !errors.IsNotFound(err) {
			klog.ErrorS(err, "Failed to delete completed job", "job", jobName)
		}

		return ctrl.Result{}, r.Status().Update(ctx, model)
	}

	// Failure case
	if job.Status.Failed > 0 && job.Status.Active == 0 {
		failureReason := r.extractJobFailureReason(job)
		model.Status.Phase = v1.ModelPhaseFailed
		model.Status.Message = fmt.Sprintf("Download failed after %d attempts: %s", job.Status.Failed, failureReason)
		klog.ErrorS(nil, "Model download failed", "model", model.Name, "url", model.Spec.Source.URL, "attempts", job.Status.Failed, "reason", failureReason)

		// Delete the failed job
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !errors.IsNotFound(err) {
			klog.ErrorS(err, "Failed to delete failed job", "job", jobName)
		}

		return ctrl.Result{}, r.Status().Update(ctx, model)
	}

	// Still in progress
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// handleDownloading handles the Downloading phase (downloading from S3 to local PFS)
func (r *ModelReconciler) handleDownloading(ctx context.Context, model *v1.Model) (ctrl.Result, error) {
	// activeDownloads is counted once, on the first download this pass wants to start.
	activeDownloads := -1
	waitingForSlot := false
	for i := range model.Status.LocalPaths {
		lp := &model.Status.LocalPaths[i]
		if lp.Status == v1.LocalPathStatusReady {
			continue
		}
		if lp.Status == v1.LocalPathStatusFailed {
			continue
		}

		// Check/create download OpsJob for this workspace
		jobName := downloadJobName(model, lp.Workspace)
		opsJob := &v1.OpsJob{}
		err := r.Get(ctx, client.ObjectKey{Name: jobName}, opsJob)

		if errors.IsNotFound(err) {
			// Downloads are capped across all models and workspaces.
			if limit := commonconfig.GetModelMaxConcurrentDownloads(); limit > 0 {
				if activeDownloads < 0 {
					if activeDownloads, err = r.countActiveDownloads(ctx); err != nil {
						return ctrl.Result{}, err
					}
				}
				if activeDownloads >= limit {
					lp.Status = v1.LocalPathStatusPending
					lp.Message = fmt.Sprintf("Waiting for a download slot (%d/%d in use)", activeDownloads, limit)
					waitingForSlot = true
					continue
				}
			}
			// Create local download OpsJob
			opsJob, err = r.constructLocalDownloadOpsJob(ctx, model, lp)
			if err != nil {
				klog.ErrorS(err, "Failed to construct local download OpsJob", "model", model.Name, "workspace", lp.Workspace)
				lp.Status = v1.LocalPathStatusFailed
				lp.Message = fmt.Sprintf("Failed to construct OpsJob: %v", err)
				continue
			}

			if err := r.Create(ctx, opsJob); err != nil {
				klog.ErrorS(err, "Failed to create local download OpsJob", "model", model.Name, "workspace", lp.Workspace)
				lp.Status = v1.LocalPathStatusFailed
				lp.Message = fmt.Sprintf("Failed to create OpsJob: %v", err)
				continue
			}

			if activeDownloads >= 0 {
				activeDownloads++
			}
			lp.Status = v1.LocalPathStatusDownloading
			lp.Message = "Download OpsJob created"
			klog.InfoS("Local download OpsJob created", "model", model.Name, "workspace", lp.Workspace, "path", lp.Path)
		} else if err != nil {
			klog.ErrorS(err, "Failed to get local download OpsJob", "model", model.Name, "workspace", lp.Workspace)
			continue
		} else {
			// Check OpsJob status
			if opsJob.Status.Phase == v1.OpsJobSucceeded {
				lp.Status = v1.LocalPathStatusReady
				lp.Message = "Download completed"
				lp.SizeBytes = reportedModelSize(opsJob)
				klog.InfoS("Local download completed", "model", model.Name, "workspace", lp.Workspace,
					"path", lp.Path, "sizeBytes", lp.SizeBytes)

				// Delete completed OpsJob
				if err := r.Delete(ctx, opsJob); err != nil && !errors.IsNotFound(err) {
					klog.ErrorS(err, "Failed to delete completed OpsJob", "job", jobName)
				}
			} else if opsJob.Status.Phase == v1.OpsJobFailed {
				failureReason := r.extractOpsJobFailureReason(opsJob)
				klog.ErrorS(nil, "Local download failed", "model", model.Name, "workspace", lp.Workspace, "reason", failureReason)

				// Delete failed OpsJob first
				if err := r.Delete(ctx, opsJob); err != nil && !errors.IsNotFound(err) {
					klog.ErrorS(err, "Failed to delete failed OpsJob", "job", jobName)
				}

				// Attempt failover to another workspace sharing the same storage path
				if r.tryFailover(ctx, model, lp) {
					klog.InfoS("Failover initiated for local download",
						"model", model.Name,
						"failedWorkspace", lp.Workspace,
						"path", lp.Path)
					// lp.Workspace and lp.Status are updated by tryFailover
				} else {
					// No failover possible, mark as final failure
					lp.Status = v1.LocalPathStatusFailed
					lp.Message = failureReason
				}
			}
		}
	}

	// Update status
	model.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}

	// Count status of all local paths
	readyCount := 0
	failedCount := 0
	downloadingCount := 0
	readyWorkspaces := []string{}

	for _, lp := range model.Status.LocalPaths {
		switch lp.Status {
		case v1.LocalPathStatusReady:
			readyCount++
			readyWorkspaces = append(readyWorkspaces, lp.Workspace)
		case v1.LocalPathStatusFailed:
			failedCount++
		case v1.LocalPathStatusDownloading, v1.LocalPathStatusPending:
			downloadingCount++
		}
	}

	totalCount := len(model.Status.LocalPaths)

	// As long as any workspace is ready, model is ready
	if readyCount > 0 {
		model.Status.Phase = v1.ModelPhaseReady
		if readyCount == totalCount {
			// All workspaces ready
			if totalCount == 1 {
				model.Status.Message = fmt.Sprintf("Model is ready in %s workspace", readyWorkspaces[0])
			} else {
				model.Status.Message = fmt.Sprintf("Model is ready in %d workspaces", readyCount)
			}
		} else {
			// Partial ready - show progress
			if downloadingCount > 0 {
				model.Status.Message = fmt.Sprintf("Model is ready in %d/%d workspaces (%d downloading)",
					readyCount, totalCount, downloadingCount)
			} else {
				model.Status.Message = fmt.Sprintf("Model is ready in %d/%d workspaces (%d failed)",
					readyCount, totalCount, failedCount)
			}
		}
		klog.InfoS("Model is ready", "model", model.Name, "readyWorkspaces", readyCount, "total", totalCount)
	} else if failedCount == totalCount {
		// All failed
		model.Status.Phase = v1.ModelPhaseFailed
		model.Status.Message = "All local downloads failed"
		if reasons := failedPathReasons(model.Status.LocalPaths); reasons != "" {
			model.Status.Message += ": " + reasons
		}
	}
	// else: still downloading, keep phase as Downloading

	if err := r.Status().Update(ctx, model); err != nil {
		return ctrl.Result{}, err
	}

	// Continue monitoring if there are still downloads in progress
	if waitingForSlot {
		return ctrl.Result{RequeueAfter: downloadSlotWaitInterval}, nil
	}
	if downloadingCount > 0 {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// failedPathReasons joins the distinct messages of the failed entries of paths.
func failedPathReasons(paths []v1.ModelLocalPath) string {
	var reasons []string
	seen := map[string]bool{}
	for _, lp := range paths {
		if lp.Status == v1.LocalPathStatusFailed && lp.Message != "" && !seen[lp.Message] {
			seen[lp.Message] = true
			reasons = append(reasons, lp.Message)
		}
	}
	return strings.Join(reasons, "; ")
}

// countActiveDownloads counts the model download jobs that have not finished, across all
// models and workspaces. It reads from the API server so that a job created by the
// previous reconcile is always counted.
func (r *ModelReconciler) countActiveDownloads(ctx context.Context) (int, error) {
	reader := r.apiReader
	if reader == nil {
		reader = r.Client
	}
	jobs := &v1.OpsJobList{}
	if err := reader.List(ctx, jobs, client.HasLabels{v1.ModelIdLabel},
		client.MatchingLabels{v1.OpsJobTypeLabel: string(v1.OpsJobDownloadType)}); err != nil {
		return 0, err
	}
	count := 0
	for i := range jobs.Items {
		job := &jobs.Items[i]
		if job.IsEnd() || job.Status.Phase == v1.OpsJobSucceeded || job.Status.Phase == v1.OpsJobFailed {
			continue
		}
		count++
	}
	return count, nil
}

var modelSizePattern = regexp.MustCompile(modelSizeMarker + `([0-9]+)`)

// reportedModelSize returns the size the download job reported in its output, or 0.
func reportedModelSize(job *v1.OpsJob) int64 {
	var size int64
	for _, out := range job.Status.Outputs {
		for _, m := range modelSizePattern.FindAllStringSubmatch(out.Value, -1) {
			if v, err := strconv.ParseInt(m[1], 10, 64); err == nil {
				size = v
			}
		}
	}
	return size
}

// initializeLocalPaths initializes the local paths based on workspace configuration
// It deduplicates paths - if multiple workspaces share the same PFS path, only one download is needed
func (r *ModelReconciler) initializeLocalPaths(ctx context.Context, model *v1.Model) []v1.ModelLocalPath {
	var paths []v1.ModelLocalPath
	modelDir := model.GetLocalDirName()
	if modelDir == "" {
		klog.InfoS("Model has no safe directory name, no local path", "model", model.Name)
		return paths
	}
	subpath := strings.TrimSpace(model.Spec.TargetSubpath)

	// Track unique paths to avoid duplicate downloads
	// Key: PFS path, Value: list of workspace IDs sharing this path
	seenPaths := make(map[string][]string)

	prefer := strings.TrimSpace(model.Spec.TargetVolume)

	if model.IsPublic() {
		// Public model: download to all workspaces (but deduplicate same paths)
		// Note: TargetVolume only takes effect for workspaces that actually expose that volume.
		workspaces, err := r.listWorkspaces(ctx, prefer)
		if err != nil {
			klog.ErrorS(err, "Failed to list workspaces for public model", "model", model.Name)
			return paths
		}

		for _, ws := range workspaces {
			// Skip workspaces without storage volumes
			if ws.PFSPath == "" {
				klog.InfoS("Skipping workspace without storage volume", "model", model.Name, "workspace", ws.ID)
				continue
			}
			// The same mount path on another cluster is a different filesystem, so
			// workspaces only share a download within one cluster.
			pfsPath := buildLocalModelPath(ws.PFSPath, subpath, modelDir)
			key := ws.Cluster + "\x00" + pfsPath
			seenPaths[key] = append(seenPaths[key], ws.ID)
		}

		// Create one LocalPath entry per unique (cluster, path)
		// Use the first workspace ID as the "primary" for this path
		for key, wsIDs := range seenPaths {
			pfsPath := key[strings.Index(key, "\x00")+1:]
			paths = append(paths, v1.ModelLocalPath{
				Workspace: wsIDs[0], // Use first workspace as primary
				Path:      pfsPath,
				Status:    v1.LocalPathStatusPending,
			})
			if len(wsIDs) > 1 {
				klog.InfoS("Multiple workspaces share the same PFS path, will only download once",
					"model", model.Name, "path", pfsPath, "workspaces", wsIDs)
			}
		}
	} else {
		// Private model: download only to specified workspace
		ws, err := r.getWorkspace(ctx, model.Spec.Workspace, prefer)
		if err != nil {
			klog.ErrorS(err, "Failed to get workspace for model", "model", model.Name, "workspace", model.Spec.Workspace)
			return paths
		}
		if ws.PFSPath == "" {
			// Without a volume there is no workspace storage to download into.
			klog.InfoS("Workspace has no storage volume", "model", model.Name, "workspace", ws.ID)
			return paths
		}

		pfsPath := buildLocalModelPath(ws.PFSPath, subpath, modelDir)
		paths = append(paths, v1.ModelLocalPath{
			Workspace: ws.ID,
			Path:      pfsPath,
			Status:    v1.LocalPathStatusPending,
		})
	}

	return paths
}

// buildLocalModelPath assembles the model directory under a volume root.
func buildLocalModelPath(root, subpath, modelDir string) string {
	return v1.BuildModelLocalPath(root, subpath, modelDir)
}

// WorkspaceInfo represents basic workspace information
type WorkspaceInfo struct {
	ID      string
	Cluster string
	PFSPath string
}

// listWorkspaces returns all available workspaces
// `preferVolume` is the optional model.spec.targetVolume that selects a non-default volume (mount path).
func (r *ModelReconciler) listWorkspaces(ctx context.Context, preferVolume string) ([]WorkspaceInfo, error) {
	// List Workspace CRs
	workspaceList := &v1.WorkspaceList{}
	if err := r.List(ctx, workspaceList); err != nil {
		return nil, err
	}

	var workspaces []WorkspaceInfo
	for _, ws := range workspaceList.Items {
		pfsPath := commonworkspace.ResolveDownloadRoot(&ws, preferVolume)
		workspaces = append(workspaces, WorkspaceInfo{
			ID:      ws.Name,
			Cluster: ws.Spec.Cluster,
			PFSPath: pfsPath,
		})
	}

	return workspaces, nil
}

// getWorkspace returns workspace info by ID
func (r *ModelReconciler) getWorkspace(ctx context.Context, workspaceID, preferVolume string) (*WorkspaceInfo, error) {
	ws := &v1.Workspace{}
	if err := r.Get(ctx, client.ObjectKey{Name: workspaceID}, ws); err != nil {
		return nil, err
	}

	pfsPath := commonworkspace.ResolveDownloadRoot(ws, preferVolume)

	return &WorkspaceInfo{
		ID:      ws.Name,
		Cluster: ws.Spec.Cluster,
		PFSPath: pfsPath,
	}, nil
}

// constructLocalDownloadOpsJob creates the OpsJob that downloads a model into one
// workspace. Without S3 staging it fetches from HuggingFace directly; otherwise it copies
// from S3 (the platform bucket, or the user's bucket for s3_sync imports).
// The job runs as the model owner and always writes to the absolute lp.Path, the path
// recorded in status, so download and cleanup can never disagree about the directory.
func (r *ModelReconciler) constructLocalDownloadOpsJob(ctx context.Context, model *v1.Model, lp *v1.ModelLocalPath) (*v1.OpsJob, error) {
	// Get Workspace to retrieve Cluster ID
	workspace := &v1.Workspace{}
	if err := r.Get(ctx, client.ObjectKey{Name: lp.Workspace}, workspace); err != nil {
		return nil, fmt.Errorf("failed to get workspace %s: %w", lp.Workspace, err)
	}

	if workspace.Spec.Cluster == "" {
		return nil, fmt.Errorf("workspace %s has no cluster configured", lp.Workspace)
	}
	if !strings.HasPrefix(lp.Path, "/") {
		return nil, fmt.Errorf("local path %q is not absolute", lp.Path)
	}

	var (
		image, inputURL, secretName string
		entryPoint                  *string
		env                         map[string]string
	)
	if model.Status.S3Path == "" && !isS3ImportModel(model) {
		repoID := model.GetHFRepoID()
		if repoID == "" {
			return nil, fmt.Errorf("source %q is not a HuggingFace repository", model.Spec.Source.URL)
		}
		if model.Spec.Source.Token != nil && model.Spec.Source.Token.Name != "" {
			secretName = model.Spec.Source.Token.Name
			if err := r.shareSecretWithWorkspace(ctx, secretName, lp.Workspace); err != nil {
				return nil, err
			}
		}
		image = commonconfig.GetModelDownloaderImage()
		inputURL = model.Spec.Source.URL
		ep := base64.StdEncoding.EncodeToString([]byte(hfDownloadScript))
		entryPoint = &ep
		env = map[string]string{"HF_REPO_ID": repoID}
	} else {
		var err error
		if inputURL, secretName, err = s3DownloadSource(model); err != nil {
			return nil, err
		}
		image = commonconfig.GetDownloadJoImage()
	}

	jobName := downloadJobName(model, lp.Workspace)
	userId, userName := modelOwner(model)
	inputs := []v1.Parameter{
		{Name: v1.ParameterEndpoint, Value: inputURL},
		// DEST_PATH: the absolute directory recorded in status.localPaths
		{Name: v1.ParameterDestPath, Value: lp.Path},
		// WORKSPACE: workspace ID for validation and path resolution
		{Name: v1.ParameterWorkspace, Value: lp.Workspace},
	}
	if secretName != "" {
		// SECRET: mounted to /etc/secrets/<secret-name>/
		inputs = append(inputs, v1.Parameter{Name: v1.ParameterSecret, Value: secretName})
	}
	klog.InfoS("Constructing OpsJob for model download", "model", model.Name, "workspace", lp.Workspace,
		"source", inputURL, "destPath", lp.Path, "image", image)

	opsJob := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: jobName,
			Labels: map[string]string{
				v1.ClusterIdLabel:   workspace.Spec.Cluster,
				v1.WorkspaceIdLabel: lp.Workspace,
				v1.ModelIdLabel:     model.Name,
				// Sanitized to a DNS-style label: the OpsJob webhook rejects names with '_'.
				v1.DisplayNameLabel: jobDisplayName(model.GetLocalDirName()),
				v1.UserIdLabel:      userId,
				v1.OpsJobTypeLabel:  string(v1.OpsJobDownloadType),
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation: userName,
			},
		},
		Spec: v1.OpsJobSpec{
			Type:                    v1.OpsJobDownloadType,
			Image:                   &image,
			EntryPoint:              entryPoint,
			Env:                     env,
			TimeoutSecond:           commonconfig.GetModelDownloadTimeoutSecond(),
			TTLSecondsAfterFinished: 600,
			Inputs:                  inputs,
		},
	}

	if err := controllerutil.SetControllerReference(model, opsJob, r.Scheme()); err != nil {
		return nil, err
	}

	return opsJob, nil
}

// s3DownloadSource returns the http(s) source URL and credential secret of a download
// that copies from S3: the platform bucket for staged models, the user's bucket for
// s3_sync imports.
func s3DownloadSource(model *v1.Model) (string, string, error) {
	if isS3ImportModel(model) {
		userURL, err := buildHTTPURLFromS3URI(model)
		if err != nil {
			return "", "", fmt.Errorf("s3 import: %w", err)
		}
		secretName := ""
		if model.Annotations != nil {
			secretName = strings.TrimSpace(model.Annotations[v1.ModelS3SourceSecretAnn])
		}
		if secretName == "" {
			// Public bucket / IAM-permitted access: fall back to platform secret.
			secretName = "primus-safe-s3"
		}
		return userURL, secretName, nil
	}

	if !commonconfig.IsS3Enable() {
		return "", "", fmt.Errorf("S3 storage is not enabled")
	}
	// The s3-downloader only supports HTTP/HTTPS endpoints, not the s3:// scheme.
	s3Endpoint := strings.TrimSuffix(commonconfig.GetS3Endpoint(), "/")
	if s3Endpoint == "" {
		return "", "", fmt.Errorf("S3 endpoint is not configured")
	}
	if !strings.HasPrefix(s3Endpoint, "http://") && !strings.HasPrefix(s3Endpoint, "https://") {
		if strings.Contains(s3Endpoint, ".") || strings.Contains(s3Endpoint, ":") {
			s3Endpoint = "https://" + s3Endpoint
		} else {
			return "", "", fmt.Errorf("S3 endpoint must be a valid HTTP/HTTPS URL, got: %s", s3Endpoint)
		}
	}
	return fmt.Sprintf("%s/%s/%s/", s3Endpoint, commonconfig.GetS3Bucket(), model.Status.S3Path), "primus-safe-s3", nil
}

// shareSecretWithWorkspace makes the secret available in the workspace, where the secret
// controller mirrors it to, so the download workload can mount it.
func (r *ModelReconciler) shareSecretWithWorkspace(ctx context.Context, name, workspace string) error {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: common.PrimusSafeNamespace}, secret); err != nil {
		return fmt.Errorf("failed to get token secret %s: %w", name, err)
	}
	workspaces := commonsecret.GetSecretWorkspaces(secret)
	for _, ws := range workspaces {
		if ws == workspace {
			return nil
		}
	}
	data, err := json.Marshal(append(workspaces, workspace))
	if err != nil {
		return err
	}
	v1.SetAnnotation(secret, v1.WorkspaceIdsAnnotation, string(data))
	return r.Update(ctx, secret)
}

// hfDownloadScript downloads $HF_REPO_ID into $DEST_PATH. Only the files needed to load
// the model are fetched: configs, tokenizer, remote code and one format of weights. The
// weights are safetensors when the repository has them, otherwise PyTorch checkpoints
// (*.bin, *.pth, *.pt); GGUF and TensorFlow/Flax weights are never fetched, since what
// serves these models loads neither, and a GGUF repository holds many quantizations of
// which all would be fetched. A download that leaves no weights fails with the reason.
// The HuggingFace cache lives inside $DEST_PATH, so a large model does not fill the
// container's ephemeral storage, and a retry resumes from what is already there.
// Nothing is removed on failure: the model deletion cleans the directory up.
// Only "[ERROR]" and "[SUCCESS]" lines reach the job's outputs and failure message.
const hfDownloadScript = `set -eu
set -f
mkdir -p "$DEST_PATH"
export HF_HOME="$DEST_PATH/.cache/hf-home"
export HF_HUB_DISABLE_TELEMETRY=1
if [ -n "${SECRET_PATH:-}" ] && [ -f "$SECRET_PATH/token" ]; then
  HF_TOKEN="$(cat "$SECRET_PATH/token")"
  export HF_TOKEN
fi
SUPPORT='*.json tokenizer* *.model *.tiktoken *.txt *.py *.jinja'

# fetch downloads the support files and the weights matching the given patterns.
fetch() {
  if command -v huggingface-cli >/dev/null 2>&1; then
    huggingface-cli download "$HF_REPO_ID" --local-dir "$DEST_PATH" \
      --include "$@" $SUPPORT --exclude 'original/*'
  else
    n=$#
    for p in "$@" $SUPPORT; do set -- "$@" --include "$p"; done
    shift "$n"
    hf download "$HF_REPO_ID" --local-dir "$DEST_PATH" "$@" --exclude 'original/*'
  fi
}

# has_files reports whether $DEST_PATH holds a file matching one of the name patterns.
has_files() {
  for p in "$@"; do
    if [ -n "$(find "$DEST_PATH" -path "$DEST_PATH/.cache" -prune -o -type f -name "$p" -print | head -n 1)" ]; then
      return 0
    fi
  done
  return 1
}

echo "Downloading $HF_REPO_ID into $DEST_PATH"
if ! fetch '*.safetensors'; then
  echo "[ERROR] downloading $HF_REPO_ID failed"
  exit 1
fi
if ! has_files '*.safetensors'; then
  echo "No safetensors weights in $HF_REPO_ID, fetching PyTorch weights"
  if ! fetch '*.bin' '*.pth' '*.pt'; then
    echo "[ERROR] downloading $HF_REPO_ID failed"
    exit 1
  fi
fi
if ! has_files '*.safetensors' '*.bin' '*.pth' '*.pt'; then
  echo "[ERROR] $HF_REPO_ID has no safetensors or PyTorch (.bin, .pth, .pt) weights; GGUF and TensorFlow/Flax weights are not downloaded"
  exit 1
fi
size="$(du -sb --exclude=.cache "$DEST_PATH" | cut -f1)"
echo "[SUCCESS] ` + modelSizeMarker + `$size"
`

// extractOpsJobFailureReason extracts detailed failure information from OpsJob. The
// OpsJob controller completes a failed job with a "JobCompleted" condition whose reason
// is "JobFailed" and whose message carries the workload's "[ERROR]" log lines; a
// "Failed" condition is accepted as well.
func (r *ModelReconciler) extractOpsJobFailureReason(opsJob *v1.OpsJob) string {
	for _, condition := range opsJob.Status.Conditions {
		switch {
		case condition.Type == "Failed" && condition.Status == metav1.ConditionTrue && condition.Reason != "":
			return fmt.Sprintf("%s: %s", condition.Reason, condition.Message)
		case condition.Type == opsJobCompletedCondition && condition.Reason == opsJobFailedReason && condition.Message != "":
			return condition.Message
		}
	}
	return "Unknown error during download"
}

// extractJobFailureReason extracts detailed failure information from batchv1.Job
// Used for HuggingFace download jobs (uploading to S3)
func (r *ModelReconciler) extractJobFailureReason(job *batchv1.Job) string {
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			if condition.Reason != "" {
				return fmt.Sprintf("%s: %s", condition.Reason, condition.Message)
			}
		}
	}

	if job.Spec.BackoffLimit != nil && job.Status.Failed >= *job.Spec.BackoffLimit {
		return "Maximum retry attempts exceeded"
	}

	return "Unknown error during download"
}

// isS3ImportModel returns true if the Model was created via API accessMode s3_sync.
// Such models carry the primus-safe.model.s3-import label and a s3:// source URL; we
// route their per-workspace download OpsJob directly at the source URI.
func isS3ImportModel(model *v1.Model) bool {
	return model != nil && model.Labels != nil && model.Labels[v1.ModelS3ImportLabel] == v1.TrueStr
}

// buildHTTPURLFromS3URI converts a "s3://bucket/prefix" URI into the http(s) form
// that the s3-downloader image consumes. If the model carries a user-provided endpoint
// annotation, we use it; otherwise we fall back to the platform endpoint (i.e. the user's
// bucket is hosted on the same MinIO/S3 the platform is using).
// The returned URL ends with "/" so the downloader treats it as a directory tree.
func buildHTTPURLFromS3URI(model *v1.Model) (string, error) {
	uri := strings.TrimSpace(model.Spec.Source.URL)
	if !strings.HasPrefix(uri, "s3://") {
		return "", fmt.Errorf("not an s3 URI: %s", uri)
	}
	rest := strings.TrimPrefix(uri, "s3://")
	if rest == "" {
		return "", fmt.Errorf("s3 URI missing bucket")
	}
	endpoint := ""
	if model.Annotations != nil {
		endpoint = strings.TrimSpace(model.Annotations[v1.ModelS3SourceEndpointAnn])
	}
	if endpoint == "" {
		endpoint = strings.TrimSpace(commonconfig.GetS3Endpoint())
	}
	if endpoint == "" {
		return "", fmt.Errorf("source endpoint is not configured")
	}
	endpoint = strings.TrimSuffix(endpoint, "/")
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	url := fmt.Sprintf("%s/%s", endpoint, rest)
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	return url, nil
}

func (r *ModelReconciler) constructDownloadJob(model *v1.Model) (*batchv1.Job, error) {
	// s3 imports skip this Uploading step — handled directly in handlePending.
	var envs []corev1.EnvVar

	if model.Spec.Source.URL == "" {
		return nil, fmt.Errorf("model source URL is empty")
	}

	// Get S3 configuration
	if !commonconfig.IsS3Enable() {
		return nil, fmt.Errorf("S3 storage is not enabled in system configuration")
	}
	s3Endpoint := commonconfig.GetS3Endpoint()
	s3AccessKey := commonconfig.GetS3AccessKey()
	s3SecretKey := commonconfig.GetS3SecretKey()
	s3Bucket := commonconfig.GetS3Bucket()
	if s3Endpoint == "" || s3AccessKey == "" || s3SecretKey == "" || s3Bucket == "" {
		return nil, fmt.Errorf("S3 configuration is incomplete")
	}

	image := commonconfig.GetModelDownloaderImage()

	// Mount HF_TOKEN from Secret if provided
	if model.Spec.Source.Token != nil {
		envs = append(envs, corev1.EnvVar{
			Name: "HF_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: *model.Spec.Source.Token,
					Key:                  "token",
				},
			},
		})
	}

	// Add S3 credentials
	envs = append(envs,
		corev1.EnvVar{Name: "AWS_ACCESS_KEY_ID", Value: s3AccessKey},
		corev1.EnvVar{Name: "AWS_SECRET_ACCESS_KEY", Value: s3SecretKey},
		corev1.EnvVar{Name: "AWS_DEFAULT_REGION", Value: "us-east-1"},
		corev1.EnvVar{Name: "S3_ENDPOINT", Value: s3Endpoint},
		corev1.EnvVar{Name: "S3_BUCKET", Value: s3Bucket},
	)

	repoId := extractHFRepoId(model.Spec.Source.URL)
	s3Path := fmt.Sprintf("s3://%s/%s", s3Bucket, model.GetS3Path())
	cmd := []string{
		"/bin/sh", "-c",
		fmt.Sprintf(`
			set -e
			echo "Downloading model from HuggingFace: %s"
			mkdir -p /tmp/model
			huggingface-cli download %s --local-dir /tmp/model || exit 1
			echo "Uploading model to S3: %s"
			aws s3 sync /tmp/model %s --endpoint-url %s || exit 1
			echo "Model download completed successfully"
		`, repoId, repoId, s3Path, s3Path, s3Endpoint),
	}

	backoffLimit := int32(3)
	ttlSeconds := int32(60)
	jobName := stringutil.NormalizeForDNS(model.Name)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{
				"app":   "model-downloader",
				"model": model.Name,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttlSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":   "model-downloader",
						"model": model.Name,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:            "downloader",
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         cmd,
							Env:             envs,
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(model, job, r.Scheme()); err != nil {
		return nil, err
	}

	return job, nil
}

// tryFailover attempts to switch the localPath to another workspace sharing the same storage path.
// Returns true if failover was initiated (lp.Workspace and lp.Status are updated).
// Returns false if no failover is possible (caller should mark as final failure).
//
// A candidate must be on the same cluster as the failed workspace and mount the same
// volume root that holds the path: an equal mount path on another cluster is a different
// filesystem, and a download there would report Ready for files this path does not hold.
func (r *ModelReconciler) tryFailover(ctx context.Context, model *v1.Model, lp *v1.ModelLocalPath) bool {
	failedWorkspace := lp.Workspace
	failedWS := &v1.Workspace{}
	if err := r.Get(ctx, client.ObjectKey{Name: failedWorkspace}, failedWS); err != nil {
		klog.ErrorS(err, "Failed to get the failed workspace for failover", "model", model.Name, "workspace", failedWorkspace)
		return false
	}
	basePath := volumeRootOf(failedWS, lp.Path)
	if basePath == "" || failedWS.Spec.Cluster == "" {
		klog.InfoS("Cannot determine storage root for failover", "model", model.Name, "path", lp.Path)
		return false
	}

	// Get and update tried workspaces from annotation
	triedWorkspaces := r.getTriedWorkspaces(model, basePath)
	triedWorkspaces = appendUnique(triedWorkspaces, failedWorkspace)

	// Check max failover attempts
	if len(triedWorkspaces) > MaxFailoverAttempts {
		klog.InfoS("Exceeded max failover attempts",
			"model", model.Name, "basePath", basePath,
			"attempts", len(triedWorkspaces), "max", MaxFailoverAttempts)
		r.setTriedWorkspaces(model, basePath, triedWorkspaces)
		return false
	}

	workspaces := &v1.WorkspaceList{}
	if err := r.List(ctx, workspaces); err != nil {
		klog.ErrorS(err, "Failed to list workspaces for failover", "model", model.Name)
		r.setTriedWorkspaces(model, basePath, triedWorkspaces)
		return false
	}
	var candidates []string
	for i := range workspaces.Items {
		ws := &workspaces.Items[i]
		if containsString(triedWorkspaces, ws.Name) || ws.Spec.Cluster != failedWS.Spec.Cluster ||
			volumeRootOf(ws, lp.Path) != basePath {
			continue
		}
		candidates = append(candidates, ws.Name)
	}

	if len(candidates) == 0 {
		klog.InfoS("No more workspaces available for failover",
			"model", model.Name, "basePath", basePath,
			"triedWorkspaces", triedWorkspaces)
		r.setTriedWorkspaces(model, basePath, triedWorkspaces)
		return false
	}

	// Pick the next candidate
	nextWorkspace := candidates[0]
	klog.InfoS("Failover to another workspace",
		"model", model.Name,
		"failedWorkspace", failedWorkspace,
		"nextWorkspace", nextWorkspace,
		"triedWorkspaces", triedWorkspaces,
		"remainingCandidates", candidates)

	// Update the localPath to use the new workspace, keep the same path
	lp.Workspace = nextWorkspace
	lp.Status = v1.LocalPathStatusPending
	lp.Message = fmt.Sprintf("Failover: %s → %s (attempt %d/%d)",
		failedWorkspace, nextWorkspace, len(triedWorkspaces), MaxFailoverAttempts)

	// Save tried workspaces in annotation
	r.setTriedWorkspaces(model, basePath, triedWorkspaces)

	return true
}

// volumeRootOf returns the mount path of the workspace volume that contains p, or "".
func volumeRootOf(workspace *v1.Workspace, p string) string {
	best := ""
	for _, vol := range workspace.Spec.Volumes {
		root := strings.TrimSpace(vol.MountPath)
		if root == "" {
			root = strings.TrimSpace(vol.HostPath)
		}
		if root == "" {
			continue
		}
		root = path.Clean(root)
		if (p == root || strings.HasPrefix(p, strings.TrimSuffix(root, "/")+"/")) && len(root) > len(best) {
			best = root
		}
	}
	return best
}

// getTriedWorkspaces retrieves the list of tried workspaces for a specific base path from model annotations.
func (r *ModelReconciler) getTriedWorkspaces(model *v1.Model, basePath string) []string {
	annotations := model.GetAnnotations()
	if annotations == nil {
		return nil
	}

	data, ok := annotations[FailoverTriedAnnotation]
	if !ok || data == "" {
		return nil
	}

	var triedMap map[string][]string
	if err := json.Unmarshal([]byte(data), &triedMap); err != nil {
		klog.ErrorS(err, "Failed to parse failover tried annotation", "model", model.Name)
		return nil
	}

	return triedMap[basePath]
}

// setTriedWorkspaces stores the list of tried workspaces for a specific base path in model annotations.
func (r *ModelReconciler) setTriedWorkspaces(model *v1.Model, basePath string, workspaces []string) {
	annotations := model.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// Read existing map
	var triedMap map[string][]string
	if data, ok := annotations[FailoverTriedAnnotation]; ok && data != "" {
		if err := json.Unmarshal([]byte(data), &triedMap); err != nil {
			triedMap = make(map[string][]string)
		}
	} else {
		triedMap = make(map[string][]string)
	}

	triedMap[basePath] = workspaces

	jsonBytes, err := json.Marshal(triedMap)
	if err != nil {
		klog.ErrorS(err, "Failed to marshal failover tried annotation", "model", model.Name)
		return
	}

	annotations[FailoverTriedAnnotation] = string(jsonBytes)
	model.SetAnnotations(annotations)
}

// appendUnique appends an item to a string slice if it's not already present.
func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

// containsString checks if a string slice contains a specific item.
func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// extractHFRepoId extracts the repository ID from a HuggingFace URL.
func extractHFRepoId(url string) string {
	url = strings.TrimSuffix(url, "/")
	if strings.Contains(url, "huggingface.co/") {
		parts := strings.Split(url, "huggingface.co/")
		if len(parts) > 1 {
			return parts[1]
		}
	}
	return url
}
