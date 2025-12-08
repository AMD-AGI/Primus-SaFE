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
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

const (
	// ModelFinalizer is the finalizer for Model resources
	ModelFinalizer = "model.amd.com/finalizer"
	// CleanupJobPrefix is the prefix for cleanup job names
	CleanupJobPrefix = "cleanup-"
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
		Owns(&batchv1.Job{}).                               // Watch Jobs created by this controller
		Watches(&v1.Inference{}, r.handleInferenceEvent()). // Watch Inference status changes
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Model Controller successfully")
	return nil
}

// handleInferenceEvent creates an event handler that enqueues Model requests when related Inference resources change.
func (r *ModelReconciler) handleInferenceEvent() handler.EventHandler {
	return handler.Funcs{
		UpdateFunc: func(ctx context.Context, e event.UpdateEvent, q v1.RequestWorkQueue) {
			inference, ok := e.ObjectNew.(*v1.Inference)
			if !ok {
				return
			}
			// Find the Model that owns this Inference
			r.enqueueModelForInference(ctx, inference, q)
		},
	}
}

// enqueueModelForInference finds and enqueues the Model that owns the inference
func (r *ModelReconciler) enqueueModelForInference(ctx context.Context, inference *v1.Inference, q v1.RequestWorkQueue) {
	// inference.Spec.ModelName contains the Model name
	if inference.Spec.ModelName == "" {
		return
	}
	q.Add(ctrl.Request{
		NamespacedName: client.ObjectKey{
			Name: inference.Spec.ModelName,
		},
	})
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

	// 3. Add finalizer if needed (only for Local download type)
	if r.needsCleanup(model) && !controllerutil.ContainsFinalizer(model, ModelFinalizer) {
		controllerutil.AddFinalizer(model, ModelFinalizer)
		if err := r.Update(ctx, model); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// 4. Initialize Status
	if model.Status.Phase == "" {
		model.Status.Phase = v1.ModelPhasePending
		model.Status.Message = "Waiting for processing"
		if err := r.Status().Update(ctx, model); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// 5. Sync inference phase if model has an inference
	if model.Status.InferenceID != "" {
		if err := r.syncInferencePhase(ctx, model); err != nil {
			klog.ErrorS(err, "failed to sync inference phase", "model", model.Name)
		}
	}

	// 6. Processing logic based on Phase
	switch model.Status.Phase {
	case v1.ModelPhasePending:
		return r.handlePending(ctx, model)
	case v1.ModelPhasePulling:
		return r.handlePulling(ctx, model)
	case v1.ModelPhaseReady, v1.ModelPhaseFailed:
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// syncInferencePhase syncs the inference phase to the model status
func (r *ModelReconciler) syncInferencePhase(ctx context.Context, model *v1.Model) error {
	inference := &v1.Inference{}
	if err := r.Get(ctx, client.ObjectKey{Name: model.Status.InferenceID}, inference); err != nil {
		if errors.IsNotFound(err) {
			// Inference not found, clear the inference fields
			if model.Status.InferencePhase != "" || model.Status.InferenceID != "" {
				model.Status.InferenceID = ""
				model.Status.InferencePhase = ""
				return r.Status().Update(ctx, model)
			}
			return nil
		}
		return err
	}

	// Sync phase if changed
	newPhase := string(inference.Status.Phase)
	if model.Status.InferencePhase != newPhase {
		model.Status.InferencePhase = newPhase
		model.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
		klog.Infof("Syncing inference phase for model %s: %s -> %s", model.Name, model.Status.InferencePhase, newPhase)
		return r.Status().Update(ctx, model)
	}

	return nil
}

// needsCleanup checks if the model needs cleanup on deletion (only Local type needs cleanup)
func (r *ModelReconciler) needsCleanup(model *v1.Model) bool {
	// Remote API models don't need cleanup (no files downloaded to S3
	return model.Spec.Source.AccessMode != v1.AccessModeRemoteAPI
}

// handleDelete handles the deletion of a Model resource
// Note: Inference will be automatically deleted by Kubernetes garbage collection (OwnerReference)
func (r *ModelReconciler) handleDelete(ctx context.Context, model *v1.Model) (ctrl.Result, error) {
	// If no finalizer, nothing to do
	if !controllerutil.ContainsFinalizer(model, ModelFinalizer) {
		return ctrl.Result{}, nil
	}

	// Only cleanup for Local download type
	if !r.needsCleanup(model) {
		// No cleanup needed, just remove finalizer
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
		// Create cleanup job
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
		// Cleanup completed, delete the job and pods with cascade
		if err := r.Delete(ctx, cleanupJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !errors.IsNotFound(err) {
			klog.ErrorS(err, "Failed to delete cleanup job", "job", cleanupJobName)
		}

		controllerutil.RemoveFinalizer(model, ModelFinalizer)
		if err := r.Update(ctx, model); err != nil {
			return ctrl.Result{}, err
		}

		klog.InfoS("Model S3 cleanup completed and deleted", "model", model.Name, "s3Path", model.GetS3Path())
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

// constructCleanupJob creates a Job to delete the model files from S3
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

	s3Path := fmt.Sprintf("s3://%s/%s", s3Bucket, model.GetS3Path())

	// Use the model downloader image from config (has awscli installed)
	image := commonconfig.GetModelDownloaderImage()

	backoffLimit := int32(1)
	ttlSeconds := int32(60) // Auto-delete job and pod 60 seconds after completion

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
									echo "Starting S3 cleanup for model: %s"
									echo "Target S3 path: %s"
									aws s3 rm %s --recursive --endpoint-url %s || echo "Warning: S3 cleanup failed, but proceeding anyway"
									echo "S3 cleanup completed"
								`, model.Name, s3Path, s3Path, s3Endpoint),
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
	accessMode := model.Spec.Source.AccessMode

	// Case A: No download needed (Remote API)
	if accessMode == v1.AccessModeRemoteAPI {
		model.Status.Phase = v1.ModelPhaseReady
		model.Status.Message = fmt.Sprintf("Model ready (AccessMode: %s)", accessMode)
		klog.InfoS("Model marked as ready", "model", model.Name, "accessMode", accessMode)
		return ctrl.Result{}, r.Status().Update(ctx, model)
	}

	// Case B: Download needed (RemoteDownload or other modes requiring download)
	jobName := model.Name
	job := &batchv1.Job{}
	err := r.Get(ctx, client.ObjectKey{Name: jobName, Namespace: common.PrimusSafeNamespace}, job)

	if errors.IsNotFound(err) {
		// Construct download Job
		job, err = r.constructDownloadJob(model)
		if err != nil {
			klog.ErrorS(err, "Failed to construct download job",
				"model", model.Name,
				"url", model.Spec.Source.URL)

			model.Status.Phase = v1.ModelPhaseFailed
			model.Status.Message = fmt.Sprintf("Failed to construct download job: %v", err)

			return ctrl.Result{}, r.Status().Update(ctx, model)
		}

		// Create Job in Kubernetes
		if err := r.Create(ctx, job); err != nil {
			klog.ErrorS(err, "Failed to create download job", "model", model.Name, "jobName", jobName)

			// Check if it's a validation error or resource constraint
			if errors.IsInvalid(err) || errors.IsForbidden(err) {
				model.Status.Phase = v1.ModelPhaseFailed
				model.Status.Message = fmt.Sprintf("Failed to create download job: %v", err)
				return ctrl.Result{}, r.Status().Update(ctx, model)
			}

			// Transient error, retry
			return ctrl.Result{}, err
		}

		model.Status.Phase = v1.ModelPhasePulling
		model.Status.Message = fmt.Sprintf("Download job created: %s", jobName)
		klog.InfoS("Download job created", "model", model.Name, "jobName", jobName, "url", model.Spec.Source.URL)

		return ctrl.Result{}, r.Status().Update(ctx, model)
	} else if err != nil {
		// Unexpected error fetching Job
		klog.ErrorS(err, "Failed to get download job", "model", model.Name, "jobName", jobName)
		return ctrl.Result{}, err
	}

	// Job already exists, transition to Pulling
	model.Status.Phase = v1.ModelPhasePulling
	model.Status.Message = fmt.Sprintf("Download in progress (Job: %s)", jobName)
	klog.InfoS("Download job already exists", "model", model.Name, "jobName", jobName)

	return ctrl.Result{}, r.Status().Update(ctx, model)
}

func (r *ModelReconciler) handlePulling(ctx context.Context, model *v1.Model) (ctrl.Result, error) {
	jobName := model.Name
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
		model.Status.Phase = v1.ModelPhaseReady
		model.Status.Message = "Download completed successfully"
		klog.InfoS("Model download completed", "model", model.Name, "url", model.Spec.Source.URL)

		// Delete the completed job and pods to clean up resources
		if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !errors.IsNotFound(err) {
			klog.ErrorS(err, "Failed to delete completed job", "job", jobName)
		} else {
			klog.InfoS("Deleted completed download job", "job", jobName)
		}

		return ctrl.Result{}, r.Status().Update(ctx, model)
	}

	// Failure case - extract detailed error information
	if job.Status.Failed > 0 {
		// Check if all retries exhausted (no active pods)
		if job.Status.Active == 0 {
			// Extract failure reason from Job conditions
			failureReason := r.extractJobFailureReason(job)

			model.Status.Phase = v1.ModelPhaseFailed
			model.Status.Message = fmt.Sprintf("Download failed after %d attempts: %s", job.Status.Failed, failureReason)

			klog.ErrorS(nil, "Model download failed",
				"model", model.Name,
				"url", model.Spec.Source.URL,
				"attempts", job.Status.Failed,
				"reason", failureReason)

			// Delete the failed job and pods to allow retry
			if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !errors.IsNotFound(err) {
				klog.ErrorS(err, "Failed to delete failed job", "job", jobName)
			} else {
				klog.InfoS("Deleted failed download job", "job", jobName)
			}

			return ctrl.Result{}, r.Status().Update(ctx, model)
		}
		// Still retrying, continue waiting
		klog.InfoS("Download job failed but retrying",
			"model", model.Name,
			"failedAttempts", job.Status.Failed,
			"activeAttempts", job.Status.Active)
	}

	// Still in progress
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// extractJobFailureReason extracts detailed failure information from Job
func (r *ModelReconciler) extractJobFailureReason(job *batchv1.Job) string {
	// Check Job conditions first
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			if condition.Reason != "" {
				return fmt.Sprintf("%s: %s", condition.Reason, condition.Message)
			}
		}
	}

	// Common failure reasons based on Job status
	if job.Status.Failed >= *job.Spec.BackoffLimit {
		return "Maximum retry attempts exceeded. Possible causes: network timeout, authentication failure, repository not found, or insufficient disk space"
	}

	return "Unknown error during download. Check Job logs for details"
}

func (r *ModelReconciler) constructDownloadJob(model *v1.Model) (*batchv1.Job, error) {
	var envs []corev1.EnvVar

	// Validate Source URL
	if model.Spec.Source.URL == "" {
		return nil, fmt.Errorf("model source URL is empty")
	}

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

	// Use model downloader image from config (has huggingface-cli and awscli installed)
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

	// Add S3 credentials as environment variables
	envs = append(envs,
		corev1.EnvVar{Name: "AWS_ACCESS_KEY_ID", Value: s3AccessKey},
		corev1.EnvVar{Name: "AWS_SECRET_ACCESS_KEY", Value: s3SecretKey},
		corev1.EnvVar{Name: "AWS_DEFAULT_REGION", Value: "us-east-1"},
		corev1.EnvVar{Name: "S3_ENDPOINT", Value: s3Endpoint},
		corev1.EnvVar{Name: "S3_BUCKET", Value: s3Bucket},
	)

	// Download from HuggingFace to S3
	// Extract repo_id from URL (e.g., "https://huggingface.co/microsoft/phi-2" -> "microsoft/phi-2")
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
	ttlSeconds := int32(60) // Auto-delete job and pod 60 seconds after completion
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      model.Name,
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
// Examples:
//   - "https://huggingface.co/microsoft/phi-2" -> "microsoft/phi-2"
//   - "https://huggingface.co/gpt2" -> "gpt2"
//   - "microsoft/phi-2" -> "microsoft/phi-2" (already a repo_id)
func extractHFRepoId(url string) string {
	// Remove trailing slashes
	url = strings.TrimSuffix(url, "/")

	// Check if it's a full URL
	if strings.Contains(url, "huggingface.co/") {
		// Extract the part after "huggingface.co/"
		parts := strings.Split(url, "huggingface.co/")
		if len(parts) > 1 {
			return parts[1]
		}
	}

	return url
}
