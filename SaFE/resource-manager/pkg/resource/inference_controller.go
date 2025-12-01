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

	// Create workload
	workload, err := r.createWorkload(ctx, inference)
	if err != nil {
		klog.ErrorS(err, "failed to create workload for inference", "inference", inference.Name)
		return r.updatePhase(ctx, inference, constvar.InferencePhaseFailure, fmt.Sprintf("Failed to create workload: %v", err))
	}

	// Update inference instance with workload ID
	originalInference := inference.DeepCopy()
	inference.Spec.Instance.WorkloadID = workload.Name
	if err := r.Patch(ctx, inference, client.MergeFrom(originalInference)); err != nil {
		klog.ErrorS(err, "failed to update inference with workload ID")
		return ctrlruntime.Result{}, err
	}

	// Update phase to pending (wait for workload to start)
	if _, err := r.updatePhase(ctx, inference, constvar.InferencePhasePending, "Workload created, waiting for running"); err != nil {
		return ctrlruntime.Result{}, err
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

	// Get normalized displayName from inference labels
	normalizedDisplayName := v1.GetDisplayName(inference)

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
	// TODO: Implement proper service discovery
	// For now, use mock data
	originalInference := inference.DeepCopy()

	if inference.Spec.Instance.BaseUrl == "" && len(workload.Status.Pods) > 0 {
		// Mock base URL from pod IP
		pod := workload.Status.Pods[0]
		inference.Spec.Instance.BaseUrl = fmt.Sprintf("http://%s:8000", pod.PodIp)
	}

	if inference.Spec.Instance.ApiKey == "" {
		// Generate mock API key
		inference.Spec.Instance.ApiKey = fmt.Sprintf("sk-%s", inference.Name)
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
