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

	// modelSizeMarker prefixes the line in which the download job reports the size of the
	// files it left on disk. The job output carries the tail of its log.
	modelSizeMarker = "MODEL_SIZE_BYTES="

	// cleanupRetryInterval is how long a failed cleanup waits before it runs again.
	cleanupRetryInterval = 30 * time.Second
	// downloadSlotWaitInterval is how long a download waits for a free slot.
	downloadSlotWaitInterval = 15 * time.Second
)

// ModelReconciler reconciles a Model object
type ModelReconciler struct {
	*ClusterBaseReconciler
	// apiReader reads straight from the API server. The global download limit counts
	// jobs with it, so a job created a moment ago is never missed by a stale cache.
	apiReader client.Reader
}

// SetupModelController sets up the controller with the Manager.
func SetupModelController(mgr manager.Manager) error {
	r := &ModelReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: mgr.GetClient(),
		},
		apiReader: mgr.GetAPIReader(),
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
	case v1.ModelPhaseReady, v1.ModelPhaseFailed:
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
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

	// 1. A download still writing into the directory would refill it after the cleanup.
	stopped, err := r.stopDownloads(ctx, model)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !stopped {
		klog.InfoS("Waiting for model downloads to stop before cleanup", "model", model.Name)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
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
// all gone, including the workloads that ran them.
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
	for _, lp := range model.Status.LocalPaths {
		wl := &v1.Workload{}
		err = r.Get(ctx, client.ObjectKey{Name: downloadJobName(model, lp.Workspace)}, wl)
		if err == nil {
			if wl.GetDeletionTimestamp().IsZero() {
				if err = r.Delete(ctx, wl); err != nil && !errors.IsNotFound(err) {
					return false, err
				}
			}
			return false, nil
		}
		if !errors.IsNotFound(err) {
			return false, err
		}
	}
	return true, nil
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
			fmt.Sprintf("S3 cleanup failed, retrying: %s", reason))
	}
	return ctrl.Result{RequeueAfter: 5 * time.Second}, false, nil
}

// cleanupLocalPaths removes the local directory of every entry in status.localPaths with
// a cleanup job that runs in the entry's workspace, where the volume is mounted. An entry
// leaves status.localPaths once its directory is gone, so the remaining entries are
// exactly what is still on disk. A directory another model still points at is kept.
func (r *ModelReconciler) cleanupLocalPaths(ctx context.Context, model *v1.Model) (ctrl.Result, bool, error) {
	if len(model.Status.LocalPaths) == 0 {
		return ctrl.Result{}, true, nil
	}
	models := &v1.ModelList{}
	if err := r.List(ctx, models); err != nil {
		return ctrl.Result{}, false, err
	}

	var (
		remaining []v1.ModelLocalPath
		messages  []string
		requeue   = 5 * time.Second
	)
	for _, lp := range model.Status.LocalPaths {
		if lp.Path == "" {
			continue
		}
		if owner := liveModelOnPath(models.Items, model.Name, lp.Path); owner != "" {
			klog.InfoS("Keeping model directory, another model points at it",
				"model", model.Name, "path", lp.Path, "otherModel", owner)
			continue
		}
		workspace := &v1.Workspace{}
		if err := r.Get(ctx, client.ObjectKey{Name: lp.Workspace}, workspace); err != nil {
			if !errors.IsNotFound(err) {
				return ctrl.Result{}, false, err
			}
			// The directory cannot be reached without its workspace. Keep the finalizer so
			// the files are not silently orphaned; an administrator has to decide.
			remaining = append(remaining, lp)
			messages = append(messages, fmt.Sprintf("cannot clean %s: workspace %s not found", lp.Path, lp.Workspace))
			requeue = 5 * time.Minute
			continue
		}
		if err := validateCleanupPath(workspace, lp.Path); err != nil {
			// Never delete a path that does not look like a model directory of this
			// workspace, and never release the model with it either: an administrator
			// has to decide what happens to those files.
			klog.ErrorS(err, "Refusing to clean up model path", "model", model.Name, "path", lp.Path)
			remaining = append(remaining, lp)
			messages = append(messages, fmt.Sprintf("refusing to clean %s: %v", lp.Path, err))
			requeue = 5 * time.Minute
			continue
		}

		jobName := cleanupJobName(model, lp.Workspace, lp.Path)
		job := &v1.OpsJob{}
		err := r.Get(ctx, client.ObjectKey{Name: jobName}, job)
		switch {
		case errors.IsNotFound(err):
			job, err = r.constructModelCleanupOpsJob(model, workspace, lp.Path)
			if err == nil {
				err = r.Create(ctx, job)
			}
			if err != nil {
				klog.ErrorS(err, "Failed to create model cleanup job, will retry", "model", model.Name, "path", lp.Path)
				messages = append(messages, fmt.Sprintf("cleanup of %s cannot start: %v", lp.Path, err))
				requeue = cleanupRetryInterval
			} else {
				klog.InfoS("Model cleanup job created", "model", model.Name, "job", jobName, "path", lp.Path)
			}
			remaining = append(remaining, lp)
		case err != nil:
			return ctrl.Result{}, false, err
		case !job.GetDeletionTimestamp().IsZero():
			remaining = append(remaining, lp)
		case job.Status.Phase == v1.OpsJobSucceeded:
			klog.InfoS("Model directory removed", "model", model.Name, "path", lp.Path)
			if err = r.Delete(ctx, job); err != nil && !errors.IsNotFound(err) {
				klog.ErrorS(err, "Failed to delete finished cleanup job", "job", jobName)
			}
		case job.Status.Phase == v1.OpsJobFailed:
			reason := r.extractOpsJobFailureReason(job)
			klog.ErrorS(nil, "Model cleanup job failed, will retry", "model", model.Name, "path", lp.Path, "reason", reason)
			if err = r.Delete(ctx, job); err != nil && !errors.IsNotFound(err) {
				return ctrl.Result{}, false, err
			}
			remaining = append(remaining, lp)
			messages = append(messages, fmt.Sprintf("cleanup of %s failed, retrying: %s", lp.Path, reason))
			requeue = cleanupRetryInterval
		default:
			remaining = append(remaining, lp)
		}
	}

	message := strings.Join(messages, "; ")
	if len(remaining) != len(model.Status.LocalPaths) || (message != "" && message != model.Status.Message) {
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
	return ctrl.Result{RequeueAfter: requeue}, false, nil
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
// and records path in status.localPaths, or "" when there is none.
func liveModelOnPath(models []v1.Model, self, path string) string {
	for i := range models {
		m := &models[i]
		if m.Name == self || !m.GetDeletionTimestamp().IsZero() {
			continue
		}
		for _, lp := range m.Status.LocalPaths {
			if lp.Path == path {
				return m.Name
			}
		}
	}
	return ""
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
	fullS3Path := fmt.Sprintf("s3://%s/%s", s3Bucket, s3Path)

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
			Type:                    v1.OpsJobModelCleanupType,
			Image:                   &image,
			EntryPoint:              &entryPoint,
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

// modelCleanupScript removes $DEST_PATH and fails unless it is gone afterwards.
const modelCleanupScript = `set -eu
case "$DEST_PATH" in
  /*/models/?*) ;;
  *) echo "refusing to remove $DEST_PATH: not a model directory" >&2; exit 2 ;;
esac
echo "Removing model directory $DEST_PATH"
rm -rf -- "$DEST_PATH"
if [ -e "$DEST_PATH" ]; then
  echo "$DEST_PATH still exists after removal" >&2
  exit 1
fi
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

	// A directory that another model is still being deleted from, or that another
	// model owns, cannot be used: the download would race the cleanup, or two models
	// would share one set of files.
	if result, blocked, err := r.checkTargetPaths(ctx, model); err != nil || blocked {
		return result, err
	}

	// Without S3 the model is downloaded from HuggingFace straight into the workspace
	// storage by the per-workspace download job.
	if !isS3ImportModel(model) && !commonconfig.IsS3Enable() {
		model.Status.Phase = v1.ModelPhaseDownloading
		model.Status.Message = "Starting download into workspace storage"
		model.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
		model.Status.S3Path = ""
		model.Status.LocalPaths = r.initializeLocalPaths(ctx, model)
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
		model.Status.LocalPaths = r.initializeLocalPaths(ctx, model)
		klog.InfoS("S3 import model: skipped Uploading phase", "model", model.Name, "url", model.Spec.Source.URL)
		return ctrl.Result{}, r.Status().Update(ctx, model)
	}

	// For local models, start the upload job to S3
	jobName := stringutil.NormalizeForDNS(model.Name)
	job := &batchv1.Job{}
	err := r.Get(ctx, client.ObjectKey{Name: jobName, Namespace: common.PrimusSafeNamespace}, job)

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

// checkTargetPaths keeps a pending model from starting while one of the directories it
// would download into is held by another model. A model being deleted from that
// directory makes this one wait; a live model owning it fails this one.
func (r *ModelReconciler) checkTargetPaths(ctx context.Context, model *v1.Model) (ctrl.Result, bool, error) {
	models := &v1.ModelList{}
	if err := r.List(ctx, models); err != nil {
		return ctrl.Result{}, false, err
	}
	for _, lp := range r.initializeLocalPaths(ctx, model) {
		for i := range models.Items {
			other := &models.Items[i]
			if other.Name == model.Name || !other.IsLocal() {
				continue
			}
			for _, olp := range other.Status.LocalPaths {
				if olp.Path != lp.Path {
					continue
				}
				if !other.GetDeletionTimestamp().IsZero() {
					message := fmt.Sprintf("Waiting for model %s to finish deleting %s", other.Name, lp.Path)
					klog.InfoS(message, "model", model.Name)
					if model.Status.Message != message {
						model.Status.Message = message
						model.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
						if err := r.Status().Update(ctx, model); err != nil {
							return ctrl.Result{}, true, err
						}
					}
					return ctrl.Result{RequeueAfter: 10 * time.Second}, true, nil
				}
				model.Status.Phase = v1.ModelPhaseFailed
				model.Status.Message = fmt.Sprintf("Path %s is already used by model %s", lp.Path, other.Name)
				model.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
				return ctrl.Result{}, true, r.Status().Update(ctx, model)
			}
		}
	}
	return ctrl.Result{}, false, nil
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
		// S3 upload completed, now start downloading to local PFS
		model.Status.Phase = v1.ModelPhaseDownloading
		model.Status.Message = "S3 upload completed, starting local download"
		model.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}

		// Initialize local paths based on workspace configuration
		model.Status.LocalPaths = r.initializeLocalPaths(ctx, model)

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
// the model are fetched: weights in safetensors, configs, tokenizer and remote code.
// The HuggingFace cache lives inside $DEST_PATH, so a large model does not fill the
// container's ephemeral storage, and a retry resumes from what is already there.
// Nothing is removed on failure: the model deletion cleans the directory up.
const hfDownloadScript = `set -eu
mkdir -p "$DEST_PATH"
export HF_HOME="$DEST_PATH/.cache/hf-home"
export HF_HUB_DISABLE_TELEMETRY=1
if [ -n "${SECRET_PATH:-}" ] && [ -f "$SECRET_PATH/token" ]; then
  HF_TOKEN="$(cat "$SECRET_PATH/token")"
  export HF_TOKEN
fi
echo "Downloading $HF_REPO_ID into $DEST_PATH"
if command -v huggingface-cli >/dev/null 2>&1; then
  huggingface-cli download "$HF_REPO_ID" --local-dir "$DEST_PATH" \
    --include '*.safetensors' '*.json' 'tokenizer*' '*.model' '*.tiktoken' '*.txt' '*.py' '*.jinja' \
    --exclude 'original/*'
else
  hf download "$HF_REPO_ID" --local-dir "$DEST_PATH" \
    --include '*.safetensors' --include '*.json' --include 'tokenizer*' --include '*.model' \
    --include '*.tiktoken' --include '*.txt' --include '*.py' --include '*.jinja' \
    --exclude 'original/*'
fi
size="$(du -sb --exclude=.cache "$DEST_PATH" | cut -f1)"
echo "` + modelSizeMarker + `$size"
`

// extractOpsJobFailureReason extracts detailed failure information from OpsJob
func (r *ModelReconciler) extractOpsJobFailureReason(opsJob *v1.OpsJob) string {
	for _, condition := range opsJob.Status.Conditions {
		if condition.Type == "Failed" && condition.Status == metav1.ConditionTrue {
			if condition.Reason != "" {
				return fmt.Sprintf("%s: %s", condition.Reason, condition.Message)
			}
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
