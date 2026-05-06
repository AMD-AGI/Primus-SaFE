/*
 * Copyright (c) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"encoding/json"
	"fmt"
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
)

// ModelReconciler reconciles a Model object
type ModelReconciler struct {
	*ClusterBaseReconciler
}

// SetupModelController sets up the controller with the Manager.
func SetupModelController(mgr manager.Manager) error {
	r := &ModelReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: mgr.GetClient(),
		},
	}
	err := ctrl.NewControllerManagedBy(mgr).
		For(&v1.Model{}).
		Owns(&batchv1.Job{}). // Watch Jobs created by this controller (cleanup, HF download)
		Owns(&v1.OpsJob{}).   // Watch OpsJobs created by this controller (local download)
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

	// 4. Skip local_path models — they are already Ready on disk, no download needed
	if model.Spec.Source.AccessMode == v1.AccessModeLocalPath {
		if model.Status.Phase != v1.ModelPhaseReady {
			model.Status.Phase = v1.ModelPhaseReady
			model.Status.Message = "Model available from local path"
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

// handleDelete handles the deletion of a Model resource
func (r *ModelReconciler) handleDelete(ctx context.Context, model *v1.Model) (ctrl.Result, error) {
	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(model, ModelFinalizer) {
		return ctrl.Result{}, nil
	}

	// Only cleanup for Local models
	if !r.needsCleanup(model) {
		controllerutil.RemoveFinalizer(model, ModelFinalizer)
		if err := r.Update(ctx, model); err != nil {
			return ctrl.Result{}, err
		}
		klog.InfoS("Model deleted (no cleanup needed)", "model", model.Name)
		return ctrl.Result{}, nil
	}

	// Check if cleanup job already exists
	cleanupJobName := stringutil.NormalizeForDNS(CleanupJobPrefix + model.Name)
	cleanupJob := &batchv1.Job{}
	err := r.Get(ctx, client.ObjectKey{Name: cleanupJobName, Namespace: common.PrimusSafeNamespace}, cleanupJob)

	if errors.IsNotFound(err) {
		// Create cleanup job for S3
		job, err := r.constructCleanupJob(model)
		if err != nil {
			klog.ErrorS(err, "Failed to construct cleanup job", "model", model.Name)
			// If we can't construct cleanup job, still remove finalizer to allow deletion
			controllerutil.RemoveFinalizer(model, ModelFinalizer)
			if updateErr := r.Update(ctx, model); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, nil
		}

		if err := r.Create(ctx, job); err != nil {
			klog.ErrorS(err, "Failed to create cleanup job", "model", model.Name)
			// If we can't create cleanup job, still remove finalizer to allow deletion
			controllerutil.RemoveFinalizer(model, ModelFinalizer)
			if updateErr := r.Update(ctx, model); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, nil
		}

		klog.InfoS("Cleanup job created", "model", model.Name, "job", cleanupJobName)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Check cleanup job status
	if cleanupJob.Status.Succeeded > 0 {
		// Cleanup completed, delete the job
		if err := r.Delete(ctx, cleanupJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !errors.IsNotFound(err) {
			klog.ErrorS(err, "Failed to delete cleanup job", "job", cleanupJobName)
		}

		controllerutil.RemoveFinalizer(model, ModelFinalizer)
		if err := r.Update(ctx, model); err != nil {
			return ctrl.Result{}, err
		}

		klog.InfoS("Model S3 cleanup completed and deleted", "model", model.Name, "s3Path", model.Status.S3Path)
		return ctrl.Result{}, nil
	}

	if cleanupJob.Status.Failed > 0 && cleanupJob.Status.Active == 0 {
		// Cleanup failed, but still allow deletion
		klog.ErrorS(nil, "Cleanup job failed, proceeding with deletion anyway", "model", model.Name)

		if err := r.Delete(ctx, cleanupJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !errors.IsNotFound(err) {
			klog.ErrorS(err, "Failed to delete failed cleanup job", "job", cleanupJobName)
		}

		controllerutil.RemoveFinalizer(model, ModelFinalizer)
		if err := r.Update(ctx, model); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Cleanup still in progress
	klog.InfoS("Waiting for cleanup job to complete", "model", model.Name, "job", cleanupJobName)
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// constructCleanupJob creates a Job to delete the model files from S3 and local paths
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

	// Build cleanup commands for local paths
	var localPathCleanup string
	for _, lp := range model.Status.LocalPaths {
		if lp.Path != "" {
			localPathCleanup += fmt.Sprintf(`
				echo "Cleaning up local path: %s"
				rm -rf %s || echo "Warning: Failed to clean up %s"
			`, lp.Path, lp.Path, lp.Path)
		}
	}

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
							Command: []string{
								"/bin/sh", "-c",
								fmt.Sprintf(`
									echo "Starting cleanup for model: %s"
									echo "Cleaning S3 path: %s"
									aws s3 rm %s --recursive --endpoint-url %s || echo "Warning: S3 cleanup failed"
									%s
									echo "Cleanup completed"
								`, model.Name, fullS3Path, fullS3Path, s3Endpoint, localPathCleanup),
							},
							Env: []corev1.EnvVar{
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

	return job, nil
}

func (r *ModelReconciler) handlePending(ctx context.Context, model *v1.Model) (ctrl.Result, error) {
	// Remote API models should already be Ready
	if model.IsRemoteAPI() {
		model.Status.Phase = v1.ModelPhaseReady
		model.Status.Message = "Remote API model is ready"
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
	for i := range model.Status.LocalPaths {
		lp := &model.Status.LocalPaths[i]
		if lp.Status == v1.LocalPathStatusReady {
			continue
		}
		if lp.Status == v1.LocalPathStatusFailed {
			continue
		}

		// Check/create download OpsJob for this workspace
		jobName := stringutil.NormalizeForDNS(fmt.Sprintf("%s-%s-%s", DownloadJobPrefix, model.Name, lp.Workspace))
		opsJob := &v1.OpsJob{}
		err := r.Get(ctx, client.ObjectKey{Name: jobName}, opsJob)

		if errors.IsNotFound(err) {
			// #region agent log - Hypothesis A: OpsJob creation
			klog.InfoS("[DEBUG] Creating OpsJob for download", "model", model.Name, "workspace", lp.Workspace, "path", lp.Path, "hypothesisId", "A")
			// #endregion
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

			lp.Status = v1.LocalPathStatusDownloading
			lp.Message = "Download OpsJob created"
			klog.InfoS("Local download OpsJob created", "model", model.Name, "workspace", lp.Workspace, "path", lp.Path)
		} else if err != nil {
			klog.ErrorS(err, "Failed to get local download OpsJob", "model", model.Name, "workspace", lp.Workspace)
			continue
		} else {
			// Check OpsJob status
			// #region agent log - Hypothesis D: OpsJob status check
			klog.InfoS("[DEBUG] OpsJob found, checking status", "model", model.Name, "opsJobName", opsJob.Name, "opsJobPhase", opsJob.Status.Phase, "conditions", opsJob.Status.Conditions, "hypothesisId", "D")
			// #endregion
			if opsJob.Status.Phase == v1.OpsJobSucceeded {
				lp.Status = v1.LocalPathStatusReady
				lp.Message = "Download completed"
				// #region agent log - Hypothesis D: Marking as Ready
				klog.InfoS("[DEBUG] Marking localPath as Ready based on OpsJob status", "model", model.Name, "workspace", lp.Workspace, "path", lp.Path, "hypothesisId", "D")
				// #endregion
				klog.InfoS("Local download completed", "model", model.Name, "workspace", lp.Workspace, "path", lp.Path)

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
	if downloadingCount > 0 {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// initializeLocalPaths initializes the local paths based on workspace configuration
// It deduplicates paths - if multiple workspaces share the same PFS path, only one download is needed
func (r *ModelReconciler) initializeLocalPaths(ctx context.Context, model *v1.Model) []v1.ModelLocalPath {
	var paths []v1.ModelLocalPath
	modelDir := model.GetSafeDisplayName()
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
			pfsPath := buildLocalModelPath(ws.PFSPath, subpath, modelDir)
			seenPaths[pfsPath] = append(seenPaths[pfsPath], ws.ID)
		}

		// Create one LocalPath entry per unique path
		// Use the first workspace ID as the "primary" for this path
		for pfsPath, wsIDs := range seenPaths {
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

		pfsPath := buildLocalModelPath(ws.PFSPath, subpath, modelDir)
		paths = append(paths, v1.ModelLocalPath{
			Workspace: ws.ID,
			Path:      pfsPath,
			Status:    v1.LocalPathStatusPending,
		})
	}

	return paths
}

// buildLocalModelPath assembles "<root>/[subpath/]models/<modelDir>".
func buildLocalModelPath(root, subpath, modelDir string) string {
	root = strings.TrimRight(root, "/")
	subpath = strings.Trim(subpath, "/")
	if subpath == "" {
		return fmt.Sprintf("%s/models/%s", root, modelDir)
	}
	return fmt.Sprintf("%s/%s/models/%s", root, subpath, modelDir)
}

// WorkspaceInfo represents basic workspace information
type WorkspaceInfo struct {
	ID      string
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
		PFSPath: pfsPath,
	}, nil
}

// constructLocalDownloadOpsJob creates an OpsJob to download from S3 to local PFS
func (r *ModelReconciler) constructLocalDownloadOpsJob(ctx context.Context, model *v1.Model, lp *v1.ModelLocalPath) (*v1.OpsJob, error) {
	// Get Workspace to retrieve Cluster ID
	workspace := &v1.Workspace{}
	if err := r.Get(ctx, client.ObjectKey{Name: lp.Workspace}, workspace); err != nil {
		return nil, fmt.Errorf("failed to get workspace %s: %w", lp.Workspace, err)
	}

	if workspace.Spec.Cluster == "" {
		return nil, fmt.Errorf("workspace %s has no cluster configured", lp.Workspace)
	}

	// Get S3 configuration
	if !commonconfig.IsS3Enable() {
		return nil, fmt.Errorf("S3 storage is not enabled")
	}
	s3Endpoint := commonconfig.GetS3Endpoint()
	s3Bucket := commonconfig.GetS3Bucket()

	// Validate and normalize S3 endpoint - must be HTTP/HTTPS URL
	// s3-downloader only supports HTTP/HTTPS schemes, not s3:// protocol
	if s3Endpoint == "" {
		return nil, fmt.Errorf("S3 endpoint is not configured")
	}
	// Remove trailing slash from endpoint
	s3Endpoint = strings.TrimSuffix(s3Endpoint, "/")
	// Ensure endpoint has HTTP/HTTPS scheme
	if !strings.HasPrefix(s3Endpoint, "http://") && !strings.HasPrefix(s3Endpoint, "https://") {
		// If endpoint looks like a hostname without scheme, add https://
		if strings.Contains(s3Endpoint, ".") || strings.Contains(s3Endpoint, ":") {
			s3Endpoint = "https://" + s3Endpoint
		} else {
			return nil, fmt.Errorf("S3 endpoint must be a valid HTTP/HTTPS URL, got: %s", s3Endpoint)
		}
	}

	// INPUT_URL/secret depend on whether this is a normal local model (HF→platform S3)
	// or an s3_sync import (read directly from the user's bucket).
	var inputURL, secretName string
	if isS3ImportModel(model) {
		userURL, err := buildHTTPURLFromS3URI(model)
		if err != nil {
			return nil, fmt.Errorf("s3 import: %w", err)
		}
		inputURL = userURL
		if model.Annotations != nil {
			if sn := strings.TrimSpace(model.Annotations[v1.ModelS3SourceSecretAnn]); sn != "" {
				secretName = sn
			}
		}
		if secretName == "" {
			// Public bucket / IAM-permitted access: fall back to platform secret.
			secretName = "primus-safe-s3"
		}
	} else {
		inputURL = fmt.Sprintf("%s/%s/%s/", s3Endpoint, s3Bucket, model.Status.S3Path)
		secretName = "primus-safe-s3"
	}

	// Use the OpsJob download image (configured in values.yaml)
	image := commonconfig.GetDownloadJoImage()

	// DEST_PATH: by default we send a relative path and the OpsJob webhook prefixes
	// it with the workspace's default PFS root. When the caller pinned a specific
	// volume via Spec.TargetVolume, we instead send the *absolute* path that we've
	// already recorded in lp.Path; the webhook is updated to leave absolute values
	// untouched.
	subpath := strings.Trim(model.Spec.TargetSubpath, "/")
	destPath := fmt.Sprintf("models/%s", model.GetSafeDisplayName())
	if subpath != "" {
		destPath = fmt.Sprintf("%s/models/%s", subpath, model.GetSafeDisplayName())
	}
	if strings.TrimSpace(model.Spec.TargetVolume) != "" && strings.HasPrefix(lp.Path, "/") {
		destPath = lp.Path
	}

	jobName := stringutil.NormalizeForDNS(fmt.Sprintf("%s-%s-%s", DownloadJobPrefix, model.Name, lp.Workspace))

	displayName := strings.ToLower(model.GetSafeDisplayName())

	// s3Path placeholder so the existing log line still makes sense.
	s3Path := inputURL

	klog.InfoS("Constructing OpsJob for model download", "model", model.Name, "workspace", lp.Workspace,
		"s3Path", s3Path, "destPath", destPath, "displayName", displayName,
		"lpPath", lp.Path, "image", image, "secretName", secretName)

	opsJob := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: jobName,
			Labels: map[string]string{
				v1.ClusterIdLabel:   workspace.Spec.Cluster,
				v1.WorkspaceIdLabel: lp.Workspace,
				v1.ModelIdLabel:     model.Name,
				v1.DisplayNameLabel: displayName,
				v1.UserIdLabel:      common.UserSystem,
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation: common.UserSystem,
			},
		},
		Spec: v1.OpsJobSpec{
			Type:                    v1.OpsJobDownloadType,
			Image:                   &image,
			TimeoutSecond:           10800, // 3 hours timeout for model download
			TTLSecondsAfterFinished: 60,
			Inputs: []v1.Parameter{
				// INPUT_URL: S3 path as the source (platform bucket for HF flow,
				// user bucket for s3_sync flow).
				{Name: v1.ParameterEndpoint, Value: inputURL},
				// DEST_PATH: relative path (will be prefixed with nfsPath)
				{Name: v1.ParameterDestPath, Value: destPath},
				// SECRET: reference to the S3 credentials secret (mounted to /etc/secrets/<secret-name>/)
				{Name: v1.ParameterSecret, Value: secretName},
				// WORKSPACE: workspace ID for validation and path resolution
				{Name: v1.ParameterWorkspace, Value: lp.Workspace},
			},
		},
	}

	if err := controllerutil.SetControllerReference(model, opsJob, r.Scheme()); err != nil {
		return nil, err
	}

	return opsJob, nil
}

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
func (r *ModelReconciler) tryFailover(ctx context.Context, model *v1.Model, lp *v1.ModelLocalPath) bool {
	failedWorkspace := lp.Workspace

	// Extract the base PFS path (e.g., "/wekafs" from "/wekafs/models/xxx")
	basePath := r.extractBasePath(lp.Path)
	if basePath == "" {
		klog.InfoS("Cannot determine base path for failover", "model", model.Name, "path", lp.Path)
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

	// Find all workspaces sharing the same base path
	allWorkspaces, err := commonworkspace.GetWorkspacesWithSamePath(r.Client, basePath)
	if err != nil {
		klog.ErrorS(err, "Failed to get workspaces with same path", "model", model.Name, "basePath", basePath)
		r.setTriedWorkspaces(model, basePath, triedWorkspaces)
		return false
	}

	// Filter out already tried workspaces
	var candidates []string
	for _, ws := range allWorkspaces {
		if !containsString(triedWorkspaces, ws) {
			candidates = append(candidates, ws)
		}
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

// extractBasePath extracts the base PFS path from a full model path.
// e.g., "/wekafs/models/llama-2-7b" -> "/wekafs"
func (r *ModelReconciler) extractBasePath(fullPath string) string {
	// Look for /models/ in the path to find the base
	idx := strings.Index(fullPath, "/models/")
	if idx > 0 {
		return fullPath[:idx]
	}
	// Fallback: find all workspaces and match against the path prefix
	return ""
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
