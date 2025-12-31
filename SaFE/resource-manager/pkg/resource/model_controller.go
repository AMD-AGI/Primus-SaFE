/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
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
)

const (
	// ModelFinalizer is the finalizer for Model resources
	ModelFinalizer = "model.amd.com/finalizer"
	// CleanupJobPrefix is the prefix for cleanup job names
	CleanupJobPrefix = "cleanup-"
	// DownloadJobPrefix is the prefix for download job names
	DownloadJobPrefix = "download-"
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

	// 4. Initialize Status if needed
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
	// Check if all local paths are ready
	allReady := true
	anyFailed := false

	for i := range model.Status.LocalPaths {
		lp := &model.Status.LocalPaths[i]
		if lp.Status == v1.LocalPathStatusReady {
			continue
		}
		if lp.Status == v1.LocalPathStatusFailed {
			anyFailed = true
			continue
		}

		// Check/create download OpsJob for this workspace
		allReady = false
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
				anyFailed = true
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
				lp.Status = v1.LocalPathStatusFailed
				lp.Message = r.extractOpsJobFailureReason(opsJob)
				anyFailed = true
				klog.ErrorS(nil, "Local download failed", "model", model.Name, "workspace", lp.Workspace)

				// Delete failed OpsJob
				if err := r.Delete(ctx, opsJob); err != nil && !errors.IsNotFound(err) {
					klog.ErrorS(err, "Failed to delete failed OpsJob", "job", jobName)
				}
			}
		}
	}

	// Update status
	model.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}

	if allReady {
		model.Status.Phase = v1.ModelPhaseReady
		model.Status.Message = "Model is ready in all workspaces"
		klog.InfoS("Model is ready", "model", model.Name)
	} else if anyFailed {
		// Some downloads failed - check if any succeeded
		hasReady := false
		for _, lp := range model.Status.LocalPaths {
			if lp.Status == v1.LocalPathStatusReady {
				hasReady = true
				break
			}
		}
		if hasReady {
			model.Status.Phase = v1.ModelPhaseReady
			model.Status.Message = "Model is ready (some workspaces failed)"
		} else {
			model.Status.Phase = v1.ModelPhaseFailed
			model.Status.Message = "All local downloads failed"
		}
	}

	if err := r.Status().Update(ctx, model); err != nil {
		return ctrl.Result{}, err
	}

	if !allReady && !anyFailed {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// initializeLocalPaths initializes the local paths based on workspace configuration
// It deduplicates paths - if multiple workspaces share the same PFS path, only one download is needed
func (r *ModelReconciler) initializeLocalPaths(ctx context.Context, model *v1.Model) []v1.ModelLocalPath {
	var paths []v1.ModelLocalPath
	modelDir := model.GetSafeDisplayName()

	// Track unique paths to avoid duplicate downloads
	// Key: PFS path, Value: list of workspace IDs sharing this path
	seenPaths := make(map[string][]string)

	if model.IsPublic() {
		// Public model: download to all workspaces (but deduplicate same paths)
		workspaces, err := r.listWorkspaces(ctx)
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
			pfsPath := fmt.Sprintf("%s/models/%s", ws.PFSPath, modelDir)
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
		ws, err := r.getWorkspace(ctx, model.Spec.Workspace)
		if err != nil {
			klog.ErrorS(err, "Failed to get workspace for model", "model", model.Name, "workspace", model.Spec.Workspace)
			return paths
		}

		pfsPath := fmt.Sprintf("%s/models/%s", ws.PFSPath, modelDir)
		paths = append(paths, v1.ModelLocalPath{
			Workspace: ws.ID,
			Path:      pfsPath,
			Status:    v1.LocalPathStatusPending,
		})
	}

	return paths
}

// WorkspaceInfo represents basic workspace information
type WorkspaceInfo struct {
	ID      string
	PFSPath string
}

// listWorkspaces returns all available workspaces
func (r *ModelReconciler) listWorkspaces(ctx context.Context) ([]WorkspaceInfo, error) {
	// List Workspace CRs
	workspaceList := &v1.WorkspaceList{}
	if err := r.List(ctx, workspaceList); err != nil {
		return nil, err
	}

	var workspaces []WorkspaceInfo
	for _, ws := range workspaceList.Items {
		pfsPath := getPFSPathFromWorkspace(&ws)
		workspaces = append(workspaces, WorkspaceInfo{
			ID:      ws.Name,
			PFSPath: pfsPath,
		})
	}

	return workspaces, nil
}

// getPFSPathFromWorkspace extracts the storage mount path from workspace volumes.
// It prioritizes PFS type volumes, otherwise falls back to the first available volume's mount path.
func getPFSPathFromWorkspace(ws *v1.Workspace) string {
	result := ""
	for _, vol := range ws.Spec.Volumes {
		if vol.Type == v1.PFS {
			result = vol.MountPath
			break
		}
	}
	// If no PFS volume, use the first available volume
	if result == "" && len(ws.Spec.Volumes) > 0 {
		result = ws.Spec.Volumes[0].MountPath
	}
	return result
}

// getWorkspace returns workspace info by ID
func (r *ModelReconciler) getWorkspace(ctx context.Context, workspaceID string) (*WorkspaceInfo, error) {
	ws := &v1.Workspace{}
	if err := r.Get(ctx, client.ObjectKey{Name: workspaceID}, ws); err != nil {
		return nil, err
	}

	pfsPath := getPFSPathFromWorkspace(ws)

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
	s3AccessKey := commonconfig.GetS3AccessKey()
	s3SecretKey := commonconfig.GetS3SecretKey()
	s3Bucket := commonconfig.GetS3Bucket()

	// INPUT_URL: S3 path for the model
	s3Path := fmt.Sprintf("s3://%s/%s", s3Bucket, model.Status.S3Path)

	// Use the OpsJob download image (configured in values.yaml)
	image := commonconfig.GetDownloadJoImage()
	if image == "" {
		// Fallback to model downloader image if download image not configured
		image = commonconfig.GetModelDownloaderImage()
	}

	// DEST_PATH: relative path (will be prefixed with workspace nfsPath by download_job_controller)
	// e.g., "models/llama-2-7b" -> final path: "/wekafs/models/llama-2-7b"
	destPath := fmt.Sprintf("models/%s", model.GetSafeDisplayName())

	jobName := stringutil.NormalizeForDNS(fmt.Sprintf("%s-%s-%s", DownloadJobPrefix, model.Name, lp.Workspace))

	displayName := strings.ToLower(model.GetSafeDisplayName())

	// #region agent log - Hypothesis C/E: Check paths passed to OpsJob
	klog.InfoS("[DEBUG] Constructing OpsJob", "model", model.Name, "workspace", lp.Workspace,
		"s3Path", s3Path, "destPath", destPath, "displayName", displayName,
		"lpPath", lp.Path, "image", image, "hypothesisId", "C/E")
	// #endregion

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
			TimeoutSecond:           3600, // 1 hour timeout for model download
			TTLSecondsAfterFinished: 60,
			Inputs: []v1.Parameter{
				// INPUT_URL: S3 path as the source
				{Name: v1.ParameterEndpoint, Value: s3Path},
				// DEST_PATH: relative path (will be prefixed with nfsPath)
				{Name: v1.ParameterDestPath, Value: destPath},
				// SECRET_PATH: empty since we use env vars for S3 auth
				{Name: v1.ParameterSecret, Value: ""},
			},
			// Custom env vars for S3 download (passed to Workload.Spec.Env)
			// Note: The download image needs to support these env vars for S3 downloads
			Env: map[string]string{
				"AWS_ACCESS_KEY_ID":     s3AccessKey,
				"AWS_SECRET_ACCESS_KEY": s3SecretKey,
				"AWS_DEFAULT_REGION":    "us-east-1",
				"S3_ENDPOINT":           s3Endpoint,
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

func (r *ModelReconciler) constructDownloadJob(model *v1.Model) (*batchv1.Job, error) {
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
