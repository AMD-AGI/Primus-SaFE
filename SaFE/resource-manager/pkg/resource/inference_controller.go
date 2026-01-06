/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commons3 "github.com/AMD-AIG-AIMA/SAFE/common/pkg/s3"
	commonUtils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
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
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

const (
	// SyncInterval defines how often to sync workload status
	SyncInterval = 10 * time.Minute
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
// Note: Workload will be automatically deleted by Kubernetes garbage collection (OwnerReference)
func (r *InferenceReconciler) delete(ctx context.Context, inference *v1.Inference) error {
	// Clean up ApiKey Secret manually (cannot use OwnerReference because
	// Inference is cluster-scoped and Secret is namespace-scoped)
	if inference.Spec.Instance.ApiKey != nil && inference.Spec.Instance.ApiKey.Name != "" {
		apiKeySecret := &corev1.Secret{}
		apiKeySecretKey := client.ObjectKey{
			Name:      inference.Spec.Instance.ApiKey.Name,
			Namespace: common.PrimusSafeNamespace,
		}
		if err := r.Get(ctx, apiKeySecretKey, apiKeySecret); err != nil {
			if !errors.IsNotFound(err) {
				klog.ErrorS(err, "Failed to get apiKey secret", "secret", apiKeySecretKey.Name)
			}
		} else {
			if err := r.Delete(ctx, apiKeySecret); err != nil && !errors.IsNotFound(err) {
				klog.ErrorS(err, "Failed to delete apiKey secret", "secret", apiKeySecretKey.Name)
			} else {
				klog.InfoS("ApiKey secret deleted", "secret", apiKeySecretKey.Name, "inference", inference.Name)
			}
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
	case "", common.InferencePhasePending:
		return r.handlePending(ctx, inference)
	case common.InferencePhaseRunning:
		return r.handleRunning(ctx, inference)
	case common.InferencePhaseStopped:
		// Stopped: stop workload and delete inference
		return r.handleStopped(ctx, inference)
	case common.InferencePhaseFailure:
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

	// If workloadID is already set, skip download check and go directly to workload sync
	// This handles the case where workload was created but inference status wasn't updated
	if inference.Spec.Instance.WorkloadID != "" {
		workload := &v1.Workload{}
		if err := r.Get(ctx, client.ObjectKey{Name: inference.Spec.Instance.WorkloadID}, workload); err == nil {
			klog.Infof("Workload %s already exists for inference %s, syncing status", workload.Name, inference.Name)
			// Check workload status and sync to inference
			if workload.Status.Phase == v1.WorkloadRunning || workload.Status.Phase == v1.WorkloadFailed {
				return r.syncWorkloadStatus(ctx, inference, workload)
			}
			// Workload still pending
			if inference.Status.Message != "Workload created, waiting for running" {
				if _, err := r.updatePhase(ctx, inference, common.InferencePhasePending, "Workload created, waiting for running"); err != nil {
					return ctrlruntime.Result{}, err
				}
			}
			return ctrlruntime.Result{RequeueAfter: 10 * time.Second}, nil
		}
		// Workload not found, clear workloadID and continue with normal flow
		klog.Warningf("Workload %s not found for inference %s, clearing workloadID", inference.Spec.Instance.WorkloadID, inference.Name)
		originalInference := inference.DeepCopy()
		inference.Spec.Instance.WorkloadID = ""
		if err := r.Patch(ctx, inference, client.MergeFrom(originalInference)); err != nil {
			klog.ErrorS(err, "failed to clear workloadID")
			return ctrlruntime.Result{}, err
		}
	}

	// Create or get existing workload
	workload, err := r.createWorkload(ctx, inference)
	if err != nil {
		klog.ErrorS(err, "failed to create workload for inference", "inference", inference.Name)
		return r.updatePhase(ctx, inference, common.InferencePhaseFailure, fmt.Sprintf("Failed to create workload: %v", err))
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
		if _, err := r.updatePhase(ctx, inference, common.InferencePhasePending, "Workload created, waiting for running"); err != nil {
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
		return r.updatePhase(ctx, inference, common.InferencePhaseFailure, "No workload ID found")
	}

	// Get workload status
	workload := &v1.Workload{}
	err := r.Get(ctx, client.ObjectKey{Name: inference.Spec.Instance.WorkloadID}, workload)
	if err != nil {
		klog.ErrorS(err, "failed to get workload", "workload", inference.Spec.Instance.WorkloadID)
		return r.updatePhase(ctx, inference, common.InferencePhaseFailure, fmt.Sprintf("Workload not found: %v", err))
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

	// Build environment variables for workload
	env := map[string]string{
		"MODEL_NAME": inference.Spec.ModelName, // e.g., "model-fb7q8"
	}

	// Calculate MODEL_PATH automatically for local models
	modelPath := inference.Spec.Config.ModelPath
	if modelPath == "" && inference.IsFromModelSquare() {
		// Get Model to calculate path
		model := &v1.Model{}
		if err := r.Get(ctx, client.ObjectKey{Name: inference.Spec.ModelName}, model); err == nil {
			// Get workspace mount path
			workspace := &v1.Workspace{}
			if err := r.Get(ctx, client.ObjectKey{Name: inference.Spec.Resource.Workspace}, workspace); err == nil {
				localBasePath := getWorkspaceMountPath(workspace)
				if localBasePath != "" {
					modelPath = fmt.Sprintf("%s/models/%s", localBasePath, model.GetSafeDisplayName())
					klog.InfoS("Auto-calculated MODEL_PATH", "inference", inference.Name, "modelPath", modelPath)
				}
			}
		}
	}
	if modelPath != "" {
		env["MODEL_PATH"] = modelPath
	}

	// Generate presigned URLs and download script for model files (24h expiry)
	entryPoint := inference.Spec.Config.EntryPoint
	if inference.IsFromModelSquare() && commonconfig.IsS3Enable() {
		model := &v1.Model{}
		if err := r.Get(ctx, client.ObjectKey{Name: inference.Spec.ModelName}, model); err == nil {
			s3Prefix := model.GetS3Path()
			if s3Prefix != "" {
				s3Client, err := commons3.NewClient(ctx, commons3.Option{})
				if err != nil {
					return nil, fmt.Errorf("failed to create S3 client: %w", err)
				}
				urls, err := s3Client.PresignModelFiles(ctx, s3Prefix, 24)
				if err != nil {
					return nil, fmt.Errorf("failed to generate presigned URLs for model %s: %w", inference.Spec.ModelName, err)
				}
				if len(urls) > 0 {
					// Build download script (includes check: if file exists, skip download)
					downloadScript := buildModelDownloadScript(urls, modelPath)
					// Decode the original entrypoint (already base64 encoded)
					originalScript, err := base64.StdEncoding.DecodeString(inference.Spec.Config.EntryPoint)
					if err != nil {
						return nil, fmt.Errorf("failed to decode entrypoint for inference %s: %w", inference.Name, err)
					}
					// Prepend download script to original script, then re-encode
					fullScript := downloadScript + "\n" + string(originalScript)
					entryPoint = base64.StdEncoding.EncodeToString([]byte(fullScript))
					klog.InfoS("Added model download script to entrypoint", "inference", inference.Name, "fileCount", len(urls))
				}
			}
		}
	}

	// Get userName from User CR
	userName := inference.Spec.UserName
	if userName == "" && inference.Spec.UserID != "" {
		user := &v1.User{}
		if err := r.Get(ctx, client.ObjectKey{Name: inference.Spec.UserID}, user); err == nil {
			userName = v1.GetUserName(user)
		}
	}

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonUtils.GenerateName(inference.Name),
			Labels: map[string]string{
				v1.InferenceIdLabel: inference.Name,
				v1.UserIdLabel:      inference.Spec.UserID,
				v1.DisplayNameLabel: normalizedDisplayName,
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation: userName,
			},
		},
		Spec: v1.WorkloadSpec{
			Resources: []v1.WorkloadResource{{
				Replica: inference.Spec.Resource.Replica,
				CPU:     fmt.Sprintf("%d", inference.Spec.Resource.Cpu),
				Memory:  fmt.Sprintf("%dGi", inference.Spec.Resource.Memory),
				GPU:     inference.Spec.Resource.Gpu,
			}},
			Workspace:  inference.Spec.Resource.Workspace,
			Image:      inference.Spec.Config.Image,
			EntryPoint: entryPoint,
			Env:        env,
			GroupVersionKind: v1.GroupVersionKind{
				Kind:    common.DeploymentKind,
				Version: "v1",
			},
			Service: &v1.Service{
				Protocol:    corev1.ProtocolTCP,
				Port:        8000,
				TargetPort:  8000,
				ServiceType: corev1.ServiceTypeClusterIP,
			},
			Priority: 1,
		},
	}

	// Set Inference as owner of Workload for automatic cascade deletion
	if err := controllerutil.SetControllerReference(inference, workload, r.Client.Scheme()); err != nil {
		klog.ErrorS(err, "failed to set owner reference", "inference", inference.Name, "workload", workload.Name)
		return nil, err
	}

	if err := r.Create(ctx, workload); err != nil {
		return nil, err
	}

	klog.Infof("Created workload %s for inference %s", workload.Name, inference.Name)
	return workload, nil
}

// syncWorkloadStatus syncs the workload status to the inference
func (r *InferenceReconciler) syncWorkloadStatus(ctx context.Context, inference *v1.Inference, workload *v1.Workload) (ctrlruntime.Result, error) {
	var newPhase common.InferencePhaseType
	var message string
	var requeueAfter time.Duration

	switch workload.Status.Phase {
	case v1.WorkloadPending:
		newPhase = common.InferencePhasePending
		message = "Workload is pending"
		requeueAfter = 10 * time.Second
	case v1.WorkloadRunning:
		// Running is the normal state for inference services
		newPhase = common.InferencePhaseRunning
		message = "Inference service is running"
		requeueAfter = SyncInterval

		// Update inference instance information
		if err := r.updateInferenceInstance(ctx, inference, workload); err != nil {
			klog.ErrorS(err, "failed to update inference instance")
		}
	case v1.WorkloadFailed:
		// Failed is a terminal state for inference
		newPhase = common.InferencePhaseFailure
		message = fmt.Sprintf("Workload failed: %s", workload.Status.Message)
	case v1.WorkloadStopped:
		// Stopped is a terminal state for inference
		newPhase = common.InferencePhaseStopped
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

	newBaseUrl := fmt.Sprintf("https://%s/%s/%s/%s",
		commonconfig.GetSystemHost(), v1.GetClusterId(workload), workload.Spec.Workspace, workload.Name)

	if inference.Spec.Instance.BaseUrl != newBaseUrl {
		inference.Spec.Instance.BaseUrl = newBaseUrl
		needsUpdate = true
		klog.Infof("Updated inference %s baseUrl to %s", inference.Name, newBaseUrl)
	}

	if !needsUpdate {
		return nil
	}

	return r.Patch(ctx, inference, client.MergeFrom(originalInference))
}

// updatePhase updates the phase of an Inference resource.
func (r *InferenceReconciler) updatePhase(ctx context.Context, inference *v1.Inference, phase common.InferencePhaseType, message string) (ctrlruntime.Result, error) {
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

// buildModelDownloadScript generates a shell script to download model files from presigned URLs
func buildModelDownloadScript(urls map[string]string, modelPath string) string {
	if len(urls) == 0 || modelPath == "" {
		return ""
	}

	// Build download commands for each file
	var downloadCmds string
	for filename, url := range urls {
		// Escape special characters in URL for shell
		escapedURL := strings.ReplaceAll(url, "'", "'\\''")
		downloadCmds += fmt.Sprintf(`
  filepath="%s/%s"
  mkdir -p "$(dirname "$filepath")"
  if [ ! -f "$filepath" ]; then
    echo "Downloading %s..."
    curl -sSL -o "$filepath" '%s'
  fi`, modelPath, filename, filename, escapedURL)
	}

	script := fmt.Sprintf(`echo "=== Downloading model to %s ===" %s
echo "=== Model download completed ==="
`, modelPath, downloadCmds)

	return script
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
