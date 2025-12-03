/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/constvar"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

const (
	// SyncInterval defines how often to sync workload status
	SyncInterval = 10 * time.Minute

	// InferenceDownloadJobPrefix is the prefix for inference model download jobs
	InferenceDownloadJobPrefix = "inference-download-"

	// InferenceCleanupJobPrefix is the prefix for inference model cleanup jobs
	InferenceCleanupJobPrefix = "inference-cleanup-"
)

// InferenceReconciler reconciles Inference resources
type InferenceReconciler struct {
	*ClusterBaseReconciler
}

// SetupInferenceController initializes and registers the InferenceReconciler with the controller manager.
func SetupInferenceController(mgr manager.Manager) error {
	r := &InferenceReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: mgr.GetClient(),
		},
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Inference{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, r.relevantChangePredicate()))).
		Watches(&v1.Workload{}, r.handleWorkloadEvent()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Inference Controller successfully")
	return nil
}

// relevantChangePredicate defines which Inference changes should trigger reconciliation.
func (r *InferenceReconciler) relevantChangePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldInf, ok1 := e.ObjectOld.(*v1.Inference)
			newInf, ok2 := e.ObjectNew.(*v1.Inference)
			if !ok1 || !ok2 {
				return false
			}
			// Trigger reconcile on deletion
			if oldInf.GetDeletionTimestamp().IsZero() && !newInf.GetDeletionTimestamp().IsZero() {
				return true
			}
			// Trigger reconcile on status change
			if oldInf.Status.Phase != newInf.Status.Phase {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}

// handleWorkloadEvent creates an event handler that enqueues Inference requests when related Workload resources change.
func (r *InferenceReconciler) handleWorkloadEvent() handler.EventHandler {
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, e event.CreateEvent, q v1.RequestWorkQueue) {
			workload, ok := e.Object.(*v1.Workload)
			if !ok {
				return
			}
			r.enqueueInferenceForWorkload(ctx, workload, q)
		},
		UpdateFunc: func(ctx context.Context, e event.UpdateEvent, q v1.RequestWorkQueue) {
			workload, ok := e.ObjectNew.(*v1.Workload)
			if !ok {
				return
			}
			r.enqueueInferenceForWorkload(ctx, workload, q)
		},
		DeleteFunc: func(ctx context.Context, e event.DeleteEvent, q v1.RequestWorkQueue) {
			workload, ok := e.Object.(*v1.Workload)
			if !ok {
				return
			}
			r.enqueueInferenceForWorkload(ctx, workload, q)
		},
	}
}

// enqueueInferenceForWorkload finds and enqueues the Inference that owns the workload
func (r *InferenceReconciler) enqueueInferenceForWorkload(ctx context.Context, workload *v1.Workload, q v1.RequestWorkQueue) {
	inferenceList := &v1.InferenceList{}
	if err := r.List(ctx, inferenceList); err != nil {
		klog.ErrorS(err, "failed to list inferences")
		return
	}
	for _, inf := range inferenceList.Items {
		if inf.Spec.Instance.WorkloadID == workload.Name {
			q.Add(ctrlruntime.Request{
				NamespacedName: client.ObjectKey{
					Name: inf.Name,
				},
			})
			return
		}
	}
}

// Reconcile is the main control loop for Inference resources.
func (r *InferenceReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	startTime := time.Now().UTC()
	defer func() {
		klog.V(4).Infof("Finished reconcile %s %s cost (%v)", v1.InferenceKind, req.Name, time.Since(startTime))
	}()

	inference := new(v1.Inference)
	if err := r.Get(ctx, req.NamespacedName, inference); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !inference.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, r.delete(ctx, inference)
	}

	// If from API, no processing needed
	if inference.IsFromAPI() {
		klog.V(4).Infof("Inference %s is from API, no controller action needed", inference.Name)
		return ctrlruntime.Result{}, nil
	}

	// Add finalizer if not present
	r.addFinalizerIfNeeded(ctx, inference)

	// Process ModelSquare inference
	return r.processModelSquareInference(ctx, inference)
}

// delete handles the deletion of an Inference resource.
func (r *InferenceReconciler) delete(ctx context.Context, inference *v1.Inference) error {
	// Delete associated workload if exists
	if inference.Spec.Instance.WorkloadID != "" {
		workload := &v1.Workload{}
		err := r.Get(ctx, client.ObjectKey{Name: inference.Spec.Instance.WorkloadID}, workload)
		if err == nil {
			klog.Infof("Deleting workload %s for inference %s", workload.Name, inference.Name)
			if err := r.Delete(ctx, workload); err != nil {
				klog.ErrorS(err, "failed to delete workload", "workload", workload.Name)
				return err
			}
		} else {
			klog.V(4).Infof("Workload %s not found, may already be deleted", inference.Spec.Instance.WorkloadID)
		}
	}

	// Clean up local model files if this is a local model
	if inference.IsFromModelSquare() {
		if err := r.cleanupLocalModel(ctx, inference); err != nil {
			klog.ErrorS(err, "failed to cleanup local model", "inference", inference.Name)
			// Don't block deletion on cleanup failure
		}
	}

	// Remove finalizer
	return utils.RemoveFinalizer(ctx, r.Client, inference, v1.InferenceFinalizer)
}

// addFinalizerIfNeeded adds the finalizer to the inference if not present
func (r *InferenceReconciler) addFinalizerIfNeeded(ctx context.Context, inference *v1.Inference) bool {
	if controllerutil.ContainsFinalizer(inference, v1.InferenceFinalizer) {
		return true
	}
	return controllerutil.AddFinalizer(inference, v1.InferenceFinalizer)
}

// processModelSquareInference processes an inference from ModelSquare
func (r *InferenceReconciler) processModelSquareInference(ctx context.Context, inference *v1.Inference) (ctrlruntime.Result, error) {
	switch inference.Status.Phase {
	case "", constvar.InferencePhasePending:
		return r.handlePending(ctx, inference)
	case constvar.InferencePhaseRunning:
		return r.handleRunning(ctx, inference)
	case constvar.InferencePhaseStopped:
		// Stopped: stop workload and delete inference
		return r.handleStopped(ctx, inference)
	case constvar.InferencePhaseFailure:
		// Failed: delete inference directly
		return r.handleTerminalState(ctx, inference)
	default:
		klog.Warningf("Unknown inference phase: %s", inference.Status.Phase)
		return ctrlruntime.Result{}, nil
	}
}

// handlePending creates a workload for the pending inference
func (r *InferenceReconciler) handlePending(ctx context.Context, inference *v1.Inference) (ctrlruntime.Result, error) {
	klog.Infof("Processing pending inference %s", inference.Name)

	// For ModelSquare models, download from S3 to local workspace first
	if inference.IsFromModelSquare() {
		downloaded, err := r.ensureModelDownloaded(ctx, inference)
		if err != nil {
			klog.ErrorS(err, "failed to download model for inference", "inference", inference.Name)
			return r.updatePhase(ctx, inference, constvar.InferencePhaseFailure, fmt.Sprintf("Failed to download model: %v", err))
		}
		if !downloaded {
			// Download job is still running, wait
			if inference.Status.Message != "Downloading model from S3 to local workspace" {
				if _, err := r.updatePhase(ctx, inference, constvar.InferencePhasePending, "Downloading model from S3 to local workspace"); err != nil {
					return ctrlruntime.Result{}, err
				}
			}
			return ctrlruntime.Result{RequeueAfter: 10 * time.Second}, nil
		}
	}

	// Create or get existing workload
	workload, err := r.createWorkload(ctx, inference)
	if err != nil {
		klog.ErrorS(err, "failed to create workload for inference", "inference", inference.Name)
		return r.updatePhase(ctx, inference, constvar.InferencePhaseFailure, fmt.Sprintf("Failed to create workload: %v", err))
	}

	// Update inference instance with workload ID if not set
	if inference.Spec.Instance.WorkloadID != workload.Name {
		originalInference := inference.DeepCopy()
		inference.Spec.Instance.WorkloadID = workload.Name
		if err := r.Patch(ctx, inference, client.MergeFrom(originalInference)); err != nil {
			klog.ErrorS(err, "failed to update inference with workload ID")
			return ctrlruntime.Result{}, err
		}
	}

	// Check workload status and sync to inference
	if workload.Status.Phase == v1.WorkloadRunning || workload.Status.Phase == v1.WorkloadFailed {
		return r.syncWorkloadStatus(ctx, inference, workload)
	}

	// Workload still pending, update phase and requeue
	if inference.Status.Message != "Workload created, waiting for running" {
		if _, err := r.updatePhase(ctx, inference, constvar.InferencePhasePending, "Workload created, waiting for running"); err != nil {
			return ctrlruntime.Result{}, err
		}
	}

	// Requeue to check workload status
	return ctrlruntime.Result{RequeueAfter: 10 * time.Second}, nil
}

// handleRunning monitors the running inference and syncs workload status
func (r *InferenceReconciler) handleRunning(ctx context.Context, inference *v1.Inference) (ctrlruntime.Result, error) {
	if inference.Spec.Instance.WorkloadID == "" {
		klog.Errorf("Running inference %s has no workload ID", inference.Name)
		return r.updatePhase(ctx, inference, constvar.InferencePhaseFailure, "No workload ID found")
	}

	// Get workload status
	workload := &v1.Workload{}
	err := r.Get(ctx, client.ObjectKey{Name: inference.Spec.Instance.WorkloadID}, workload)
	if err != nil {
		klog.ErrorS(err, "failed to get workload", "workload", inference.Spec.Instance.WorkloadID)
		return r.updatePhase(ctx, inference, constvar.InferencePhaseFailure, fmt.Sprintf("Workload not found: %v", err))
	}

	// Sync workload status
	return r.syncWorkloadStatus(ctx, inference, workload)
}

// handleStopped handles the stopped state - deletes inference (finalizer will delete workload)
func (r *InferenceReconciler) handleStopped(ctx context.Context, inference *v1.Inference) (ctrlruntime.Result, error) {
	klog.Infof("Handling stopped inference %s, will delete inference and workload", inference.Name)

	// Delete the inference CR
	// The delete() function (called by finalizer) will handle workload deletion
	if err := r.Delete(ctx, inference); err != nil {
		klog.ErrorS(err, "failed to delete inference", "inference", inference.Name)
		return ctrlruntime.Result{}, err
	}

	klog.Infof("Successfully initiated deletion of inference %s", inference.Name)
	return ctrlruntime.Result{}, nil
}

// handleTerminalState handles terminal state (Failed) - deletes inference (finalizer will delete workload)
func (r *InferenceReconciler) handleTerminalState(ctx context.Context, inference *v1.Inference) (ctrlruntime.Result, error) {
	klog.Infof("Handling terminal state (%s) for inference %s, will delete inference and workload", inference.Status.Phase, inference.Name)

	// Delete the inference CR
	// The delete() function (called by finalizer) will handle workload deletion
	if err := r.Delete(ctx, inference); err != nil {
		klog.ErrorS(err, "failed to delete inference", "inference", inference.Name)
		return ctrlruntime.Result{}, err
	}

	klog.Infof("Successfully initiated deletion of inference %s in terminal state %s", inference.Name, inference.Status.Phase)
	return ctrlruntime.Result{}, nil
}

// createWorkload creates a workload for the inference
func (r *InferenceReconciler) createWorkload(ctx context.Context, inference *v1.Inference) (*v1.Workload, error) {
	// Check if workload already exists for this inference
	existingWorkloads := &v1.WorkloadList{}
	if err := r.List(ctx, existingWorkloads, client.MatchingLabels{
		v1.InferenceIdLabel: inference.Name,
	}); err != nil {
		return nil, err
	}

	// If workload already exists, return it
	if len(existingWorkloads.Items) > 0 {
		klog.Infof("Workload %s already exists for inference %s", existingWorkloads.Items[0].Name, inference.Name)
		return &existingWorkloads.Items[0], nil
	}

	// Get normalized displayName from inference labels (already normalized in models.go)
	normalizedDisplayName := v1.GetDisplayName(inference)
	if normalizedDisplayName == "" {
		normalizedDisplayName = stringutil.NormalizeForDNS(inference.Spec.DisplayName)
	}

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("inference-%s-%d", inference.Name, time.Now().Unix()),
			Labels: map[string]string{
				v1.InferenceIdLabel: inference.Name,
				v1.UserIdLabel:      inference.Spec.UserID,
				v1.DisplayNameLabel: normalizedDisplayName,
			},
		},
		Spec: v1.WorkloadSpec{
			Resource: v1.WorkloadResource{
				Replica: inference.Spec.Resource.Replica,
				CPU:     fmt.Sprintf("%d", inference.Spec.Resource.Cpu),
				Memory:  fmt.Sprintf("%dGi", inference.Spec.Resource.Memory),
				GPU:     inference.Spec.Resource.Gpu,
			},
			Workspace:  inference.Spec.Resource.Workspace,
			Image:      inference.Spec.Config.Image,
			EntryPoint: inference.Spec.Config.EntryPoint,
			GroupVersionKind: v1.GroupVersionKind{
				Kind:    common.PytorchJobKind,
				Version: "v1",
			},
			Priority: 1,
		},
	}

	if err := r.Create(ctx, workload); err != nil {
		return nil, err
	}

	klog.Infof("Created workload %s for inference %s", workload.Name, inference.Name)
	return workload, nil
}

// syncWorkloadStatus syncs the workload status to the inference
func (r *InferenceReconciler) syncWorkloadStatus(ctx context.Context, inference *v1.Inference, workload *v1.Workload) (ctrlruntime.Result, error) {
	var newPhase constvar.InferencePhaseType
	var message string
	var requeueAfter time.Duration

	switch workload.Status.Phase {
	case v1.WorkloadPending:
		newPhase = constvar.InferencePhasePending
		message = "Workload is pending"
		requeueAfter = 10 * time.Second
	case v1.WorkloadRunning:
		// Running is the normal state for inference services
		newPhase = constvar.InferencePhaseRunning
		message = "Inference service is running"
		requeueAfter = SyncInterval

		// Update inference instance information
		if err := r.updateInferenceInstance(ctx, inference, workload); err != nil {
			klog.ErrorS(err, "failed to update inference instance")
		}
	case v1.WorkloadFailed:
		// Failed is a terminal state for inference
		newPhase = constvar.InferencePhaseFailure
		message = fmt.Sprintf("Workload failed: %s", workload.Status.Message)
	case v1.WorkloadStopped:
		// Stopped is a terminal state for inference
		newPhase = constvar.InferencePhaseStopped
		message = "Workload stopped"
	default:
		klog.Warningf("Unknown workload phase: %s", workload.Status.Phase)
		requeueAfter = 30 * time.Second
	}

	// Update phase if changed
	if inference.Status.Phase != newPhase {
		if _, err := r.updatePhase(ctx, inference, newPhase, message); err != nil {
			return ctrlruntime.Result{}, err
		}
	}

	// Add event
	if len(inference.Status.Events) == 0 || inference.Status.Events[len(inference.Status.Events)-1].WorkloadPhase != workload.Status.Phase {
		originalInference := inference.DeepCopy()
		inference.AddEvent(workload.Name, workload.Status.Phase, message)
		if err := r.Status().Patch(ctx, inference, client.MergeFrom(originalInference)); err != nil {
			klog.ErrorS(err, "failed to add event")
		}
	}

	return ctrlruntime.Result{RequeueAfter: requeueAfter}, nil
}

// updateInferenceInstance updates the inference instance information
func (r *InferenceReconciler) updateInferenceInstance(ctx context.Context, inference *v1.Inference, workload *v1.Workload) error {
	// Extract service information from workload
	originalInference := inference.DeepCopy()
	needsUpdate := false

	// Always update BaseUrl from current pod IP (pod IP may change after restart)
	if len(workload.Status.Pods) > 0 {
		pod := workload.Status.Pods[0]
		newBaseUrl := fmt.Sprintf("http://%s:8000", pod.PodIp)
		if inference.Spec.Instance.BaseUrl != newBaseUrl {
			inference.Spec.Instance.BaseUrl = newBaseUrl
			needsUpdate = true
			klog.Infof("Updated inference %s baseUrl to %s", inference.Name, newBaseUrl)
		}
	}

	if inference.Spec.Instance.ApiKey == "" {
		// Generate API key if not set
		inference.Spec.Instance.ApiKey = fmt.Sprintf("sk-%s", inference.Name)
		needsUpdate = true
	}

	if !needsUpdate {
		return nil
	}

	return r.Patch(ctx, inference, client.MergeFrom(originalInference))
}

// updatePhase updates the phase of an Inference resource.
func (r *InferenceReconciler) updatePhase(ctx context.Context, inference *v1.Inference, phase constvar.InferencePhaseType, message string) (ctrlruntime.Result, error) {
	if inference.Status.Phase == phase && inference.Status.Message == message {
		return ctrlruntime.Result{}, nil
	}

	originalInference := inference.DeepCopy()
	inference.Status.Phase = phase
	inference.Status.Message = message
	inference.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}

	if err := r.Status().Patch(ctx, inference, client.MergeFrom(originalInference)); err != nil {
		klog.ErrorS(err, "failed to update inference status", "inference", inference.Name)
		return ctrlruntime.Result{}, err
	}

	klog.Infof("Updated inference %s to phase %s: %s", inference.Name, phase, message)
	return ctrlruntime.Result{}, nil
}

// ensureModelDownloaded ensures the model is downloaded from S3 to local workspace
// Returns true if download is complete, false if still in progress
func (r *InferenceReconciler) ensureModelDownloaded(ctx context.Context, inference *v1.Inference) (bool, error) {
	// Get the Model CR to get S3 path information
	model := &v1.Model{}
	if err := r.Get(ctx, client.ObjectKey{Name: inference.Spec.ModelName}, model); err != nil {
		return false, fmt.Errorf("failed to get model %s: %w", inference.Spec.ModelName, err)
	}

	// Get workspace to determine mount path
	workspace := &v1.Workspace{}
	if err := r.Get(ctx, client.ObjectKey{Name: inference.Spec.Resource.Workspace}, workspace); err != nil {
		return false, fmt.Errorf("failed to get workspace %s: %w", inference.Spec.Resource.Workspace, err)
	}

	// Get mount path from workspace volumes, prioritizing PFS type
	localBasePath := getWorkspaceMountPath(workspace)
	if localBasePath == "" {
		return false, fmt.Errorf("workspace %s has no volumes with mount path", inference.Spec.Resource.Workspace)
	}

	// Local path: {mountPath}/models/{safeDisplayName}
	localModelPath := fmt.Sprintf("%s/models/%s", localBasePath, model.GetSafeDisplayName())

	// Check if download job already exists
	downloadJobName := stringutil.NormalizeForDNS(InferenceDownloadJobPrefix + inference.Name)
	downloadJob := &batchv1.Job{}
	err := r.Get(ctx, client.ObjectKey{Name: downloadJobName, Namespace: common.PrimusSafeNamespace}, downloadJob)

	if errors.IsNotFound(err) {
		// Create download job
		job, err := r.constructInferenceDownloadJob(inference, model, localBasePath, localModelPath)
		if err != nil {
			return false, fmt.Errorf("failed to construct download job: %w", err)
		}
		if err := r.Create(ctx, job); err != nil {
			return false, fmt.Errorf("failed to create download job: %w", err)
		}
		klog.Infof("Created download job %s for inference %s", downloadJobName, inference.Name)
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("failed to get download job: %w", err)
	}

	// Check job status
	if downloadJob.Status.Succeeded > 0 {
		klog.Infof("Download job %s completed for inference %s", downloadJobName, inference.Name)
		// Delete the job after success
		if err := r.Delete(ctx, downloadJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !errors.IsNotFound(err) {
			klog.ErrorS(err, "failed to delete completed download job", "job", downloadJobName)
		}
		return true, nil
	}

	if downloadJob.Status.Failed > 0 && downloadJob.Status.Active == 0 {
		// Job failed, delete it and return error
		if err := r.Delete(ctx, downloadJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !errors.IsNotFound(err) {
			klog.ErrorS(err, "failed to delete failed download job", "job", downloadJobName)
		}
		return false, fmt.Errorf("download job failed")
	}

	// Job still running
	return false, nil
}

// constructInferenceDownloadJob creates a job to download model from S3 to local workspace
func (r *InferenceReconciler) constructInferenceDownloadJob(inference *v1.Inference, model *v1.Model, localBasePath, localModelPath string) (*batchv1.Job, error) {
	// Get S3 configuration
	if !commonconfig.IsS3Enable() {
		return nil, fmt.Errorf("S3 storage is not enabled")
	}
	s3Endpoint := commonconfig.GetS3Endpoint()
	s3AccessKey := commonconfig.GetS3AccessKey()
	s3SecretKey := commonconfig.GetS3SecretKey()
	s3Bucket := commonconfig.GetS3Bucket()
	if s3Endpoint == "" || s3AccessKey == "" || s3SecretKey == "" || s3Bucket == "" {
		return nil, fmt.Errorf("S3 configuration is incomplete")
	}

	s3Path := fmt.Sprintf("s3://%s/%s", s3Bucket, model.GetS3Path())

	// Use the model-downloader image which has awscli installed
	image := "harbor.tas.primus-safe.amd.com/proxy/primussafe/model-downloader:latest"

	backoffLimit := int32(3)
	ttlSeconds := int32(60)

	jobName := stringutil.NormalizeForDNS(InferenceDownloadJobPrefix + inference.Name)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{
				"app":       "inference-download",
				"inference": inference.Name,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttlSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":       "inference-download",
						"inference": inference.Name,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:            "downloader",
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command: []string{
								"/bin/sh", "-c",
								fmt.Sprintf(`
									set -e
									echo "Downloading model from S3 to local workspace"
									echo "S3 Source: %s"
									echo "Local Target: %s"
									mkdir -p %s
									aws s3 sync %s %s --endpoint-url %s
									echo "Download completed successfully"
								`, s3Path, localModelPath, localModelPath, s3Path, localModelPath, s3Endpoint),
							},
							Env: []corev1.EnvVar{
								{Name: "AWS_ACCESS_KEY_ID", Value: s3AccessKey},
								{Name: "AWS_SECRET_ACCESS_KEY", Value: s3SecretKey},
								{Name: "AWS_DEFAULT_REGION", Value: "us-east-1"},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "workspace-storage",
									MountPath: localBasePath,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "workspace-storage",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: localBasePath,
								},
							},
						},
					},
				},
			},
		},
	}

	return job, nil
}

// cleanupLocalModel creates a job to clean up local model files when inference is deleted
func (r *InferenceReconciler) cleanupLocalModel(ctx context.Context, inference *v1.Inference) error {
	// Get the Model CR
	model := &v1.Model{}
	if err := r.Get(ctx, client.ObjectKey{Name: inference.Spec.ModelName}, model); err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("Model %s not found, skipping cleanup", inference.Spec.ModelName)
			return nil
		}
		return err
	}

	// Get workspace
	workspace := &v1.Workspace{}
	if err := r.Get(ctx, client.ObjectKey{Name: inference.Spec.Resource.Workspace}, workspace); err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("Workspace %s not found, skipping cleanup", inference.Spec.Resource.Workspace)
			return nil
		}
		return err
	}

	// Get mount path from workspace volumes, prioritizing PFS type
	localBasePath := getWorkspaceMountPath(workspace)
	if localBasePath == "" {
		klog.Infof("Workspace %s has no volumes with mount path, skipping cleanup", inference.Spec.Resource.Workspace)
		return nil
	}

	// Local path to clean
	localModelPath := fmt.Sprintf("%s/models/%s", localBasePath, model.GetSafeDisplayName())

	// Create cleanup job
	job, err := r.constructInferenceCleanupJob(inference, localBasePath, localModelPath)
	if err != nil {
		return err
	}

	// Check if cleanup job already exists
	existingJob := &batchv1.Job{}
	if err := r.Get(ctx, client.ObjectKey{Name: job.Name, Namespace: job.Namespace}, existingJob); err == nil {
		klog.Infof("Cleanup job %s already exists", job.Name)
		return nil
	}

	if err := r.Create(ctx, job); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	klog.Infof("Created cleanup job %s for inference %s, path: %s", job.Name, inference.Name, localModelPath)
	return nil
}

// constructInferenceCleanupJob creates a job to delete local model files
func (r *InferenceReconciler) constructInferenceCleanupJob(inference *v1.Inference, localBasePath, localModelPath string) (*batchv1.Job, error) {
	image := "alpine:3.18"

	backoffLimit := int32(1)
	ttlSeconds := int32(60)

	jobName := stringutil.NormalizeForDNS(InferenceCleanupJobPrefix + inference.Name)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{
				"app":       "inference-cleanup",
				"inference": inference.Name,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttlSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":       "inference-cleanup",
						"inference": inference.Name,
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
									echo "Starting cleanup for inference: %s"
									echo "Target path: %s"
									if [ -d "%s" ]; then
										rm -rf "%s"
										echo "Cleanup completed: directory deleted"
									else
										echo "Directory does not exist, nothing to clean"
									fi
								`, inference.Name, localModelPath, localModelPath, localModelPath),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "workspace-storage",
									MountPath: localBasePath,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "workspace-storage",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: localBasePath,
								},
							},
						},
					},
				},
			},
		},
	}

	return job, nil
}

// getWorkspaceMountPath retrieves mount path from workspace volumes.
// It prioritizes PFS type volumes for better storage performance,
// otherwise falls back to the first available volume's mount path.
func getWorkspaceMountPath(workspace *v1.Workspace) string {
	result := ""
	// Prioritize PFS type volumes (e.g., /wekafs)
	for _, vol := range workspace.Spec.Volumes {
		if vol.Type == v1.PFS && vol.MountPath != "" {
			result = vol.MountPath
			break
		}
	}
	// Fallback: use first volume with MountPath (e.g., /apps, /home)
	if result == "" {
		for _, vol := range workspace.Spec.Volumes {
			if vol.MountPath != "" {
				result = vol.MountPath
				break
			}
		}
	}
	return result
}
