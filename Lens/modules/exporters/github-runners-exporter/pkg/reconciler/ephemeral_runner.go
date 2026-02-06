// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package reconciler

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/github"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/github-runners-exporter/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// EphemeralRunnerReconciler watches EphemeralRunner resources and mirrors their
// K8s state to the github_ephemeral_runner_states table.
//
// This reconciler is intentionally lightweight - it only:
//  1. Gets the EphemeralRunner from K8s API
//  2. Manages finalizers (for deletion tracking)
//  3. Enriches with Pod status
//  4. Upserts raw state to the database
//
// All business logic (workflow_run lifecycle, task creation, summary management)
// is handled by the RunnerStateProcessor which reads from the runner_states table.
type EphemeralRunnerReconciler struct {
	client        *clientsets.K8SClientSet
	dynamicClient dynamic.Interface
	stateFacade   *database.GithubEphemeralRunnerStateFacade
}

// NewEphemeralRunnerReconciler creates a new reconciler
func NewEphemeralRunnerReconciler() *EphemeralRunnerReconciler {
	return &EphemeralRunnerReconciler{}
}

// Init initializes the reconciler with required clients
func (r *EphemeralRunnerReconciler) Init(ctx context.Context) error {
	clusterManager := clientsets.GetClusterManager()
	currentCluster := clusterManager.GetCurrentClusterClients()
	if currentCluster.K8SClientSet == nil {
		return fmt.Errorf("K8S client not initialized in ClusterManager")
	}
	r.client = currentCluster.K8SClientSet

	if r.client.Dynamic == nil {
		return fmt.Errorf("dynamic client not initialized in K8SClientSet")
	}
	r.dynamicClient = r.client.Dynamic

	// Initialize GitHub client manager
	if r.client.Clientsets != nil {
		github.InitGlobalManager(r.client.Clientsets)
	}

	// Initialize state facade
	r.stateFacade = database.NewGithubEphemeralRunnerStateFacade()

	log.Info("EphemeralRunnerReconciler initialized (lightweight mode)")
	return nil
}

// SetupWithManager sets up the controller with the Manager
func (r *EphemeralRunnerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create an unstructured object for EphemeralRunner
	erExample := &unstructured.Unstructured{}
	erExample.SetGroupVersionKind(types.EphemeralRunnerGVK)

	// Watch all EphemeralRunner resources
	return ctrl.NewControllerManagedBy(mgr).
		Named("ephemeral-runner-controller").
		For(erExample).
		Complete(r)
}

const finalizerName = "primus-lens.amd.com/workflow-run-tracker"

// Reconcile handles EphemeralRunner events.
// It mirrors K8s state to the database and nothing else.
func (r *EphemeralRunnerReconciler) Reconcile(ctx context.Context, req reconcile.Request) (result reconcile.Result, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic recovered: %v", rec)
			log.Errorf("Panic in EphemeralRunnerReconciler for %s/%s: %v\nStack trace:\n%s",
				req.Namespace, req.Name, rec, string(debug.Stack()))
		}
	}()

	log.Debugf("EphemeralRunnerReconciler: reconciling %s/%s", req.Namespace, req.Name)

	// Get the EphemeralRunner
	obj, err := r.dynamicClient.Resource(types.EphemeralRunnerGVR).
		Namespace(req.Namespace).
		Get(ctx, req.Name, metav1.GetOptions{})
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Resource was deleted, no action needed
			log.Debugf("EphemeralRunnerReconciler: %s/%s not found, skipping", req.Namespace, req.Name)
			return ctrl.Result{}, nil
		}
		log.Errorf("EphemeralRunnerReconciler: failed to get %s/%s: %v", req.Namespace, req.Name, err)
		return ctrl.Result{}, err
	}

	// Parse the object
	info := types.ParseEphemeralRunner(obj)

	// Add finalizer if not present (before deletion check, so we always have it)
	if obj.GetDeletionTimestamp() == nil && !containsFinalizer(obj.GetFinalizers(), finalizerName) {
		if err := r.addFinalizer(ctx, obj); err != nil {
			log.Errorf("EphemeralRunnerReconciler: failed to add finalizer to %s/%s: %v",
				req.Namespace, req.Name, err)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, err
		}
		log.Debugf("EphemeralRunnerReconciler: added finalizer to %s/%s", req.Namespace, req.Name)
	}

	// Determine runner type
	info.RunnerType = types.DetermineRunnerType(info.Name)

	// Enrich with Pod status (lightweight K8s API call)
	r.enrichPodStatus(ctx, info)

	// Resolve associated SaFE UnifiedJob workload (if any)
	r.resolveSafeWorkload(ctx, info)

	// Handle deletion: only finalize when the Pod is no longer running.
	// ARC sets deletionTimestamp early but keeps the Pod alive via its own finalizers
	// until the GitHub Actions job completes. We must continue tracking state during
	// this graceful shutdown period.
	if obj.GetDeletionTimestamp() != nil && containsFinalizer(obj.GetFinalizers(), finalizerName) {
		podDone := r.isPodTerminated(info)
		if podDone {
			// Pod is done - mark as deleted and remove our finalizer
			info.IsCompleted = true
			if err := r.markDeleted(ctx, info); err != nil {
				log.Errorf("EphemeralRunnerReconciler: failed to mark deleted for %s/%s: %v",
					req.Namespace, req.Name, err)
				return ctrl.Result{RequeueAfter: 5 * time.Second}, err
			}

			if err := r.removeFinalizer(ctx, obj); err != nil {
				log.Errorf("EphemeralRunnerReconciler: failed to remove finalizer from %s/%s: %v",
					req.Namespace, req.Name, err)
				return ctrl.Result{RequeueAfter: 5 * time.Second}, err
			}
			log.Infof("EphemeralRunnerReconciler: pod terminated, marked deleted and removed finalizer from %s/%s",
				req.Namespace, req.Name)
			return ctrl.Result{}, nil
		}

		// Pod still running during graceful shutdown - upsert latest state and requeue
		if err := r.upsertState(ctx, info); err != nil {
			log.Errorf("EphemeralRunnerReconciler: failed to upsert state for %s/%s: %v",
				req.Namespace, req.Name, err)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}
		log.Debugf("EphemeralRunnerReconciler: %s/%s has deletionTimestamp but pod still running (phase: %s), requeueing",
			req.Namespace, req.Name, info.PodPhase)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Normal path: upsert raw state to database
	if err := r.upsertState(ctx, info); err != nil {
		log.Errorf("EphemeralRunnerReconciler: failed to upsert state for %s/%s: %v",
			req.Namespace, req.Name, err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	log.Debugf("EphemeralRunnerReconciler: upserted state for %s/%s (phase: %s, pod: %s)",
		req.Namespace, req.Name, info.Phase, info.PodPhase)

	return ctrl.Result{}, nil
}

// upsertState writes the raw K8s state to the database
func (r *EphemeralRunnerReconciler) upsertState(ctx context.Context, info *types.EphemeralRunnerInfo) error {
	state := &model.GithubEphemeralRunnerStates{
		Namespace:         info.Namespace,
		Name:              info.Name,
		UID:               info.UID,
		RunnerSetName:     info.RunnerSetName,
		RunnerType:        info.RunnerType,
		Phase:             info.Phase,
		GithubRunID:       info.GithubRunID,
		GithubJobID:       info.GithubJobID,
		GithubRunNumber:   int32(info.GithubRunNumber),
		WorkflowName:      info.WorkflowName,
		HeadSha:           info.HeadSHA,
		HeadBranch:        info.Branch,
		Repository:        info.Repository,
		PodPhase:          info.PodPhase,
		PodCondition:      info.PodCondition,
		PodMessage:        info.PodMessage,
		SafeWorkloadID:    info.SafeWorkloadID,
		IsCompleted:       info.IsCompleted,
		CreationTimestamp:  info.CreationTimestamp.Time,
	}

	if !info.CompletionTime.IsZero() {
		state.CompletionTime = info.CompletionTime.Time
	}

	return r.stateFacade.Upsert(ctx, state)
}

// markDeleted marks a runner state as deleted in the database
func (r *EphemeralRunnerReconciler) markDeleted(ctx context.Context, info *types.EphemeralRunnerInfo) error {
	// First upsert the latest state (in case we missed an update)
	if err := r.upsertState(ctx, info); err != nil {
		log.Warnf("EphemeralRunnerReconciler: failed to upsert before marking deleted: %v", err)
	}

	// Then mark as deleted
	return r.stateFacade.MarkDeleted(ctx, info.Namespace, info.Name)
}

// containsFinalizer checks if a finalizer is present
func containsFinalizer(finalizers []string, finalizer string) bool {
	for _, f := range finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

// addFinalizer adds the finalizer to the object
func (r *EphemeralRunnerReconciler) addFinalizer(ctx context.Context, obj *unstructured.Unstructured) error {
	finalizers := obj.GetFinalizers()
	finalizers = append(finalizers, finalizerName)
	obj.SetFinalizers(finalizers)

	_, err := r.dynamicClient.Resource(types.EphemeralRunnerGVR).
		Namespace(obj.GetNamespace()).
		Update(ctx, obj, metav1.UpdateOptions{})
	return err
}

// removeFinalizer removes the finalizer from the object
func (r *EphemeralRunnerReconciler) removeFinalizer(ctx context.Context, obj *unstructured.Unstructured) error {
	finalizers := obj.GetFinalizers()
	newFinalizers := make([]string, 0, len(finalizers))
	for _, f := range finalizers {
		if f != finalizerName {
			newFinalizers = append(newFinalizers, f)
		}
	}
	obj.SetFinalizers(newFinalizers)

	_, err := r.dynamicClient.Resource(types.EphemeralRunnerGVR).
		Namespace(obj.GetNamespace()).
		Update(ctx, obj, metav1.UpdateOptions{})
	return err
}

// resolveSafeWorkload looks up the associated SaFE UnifiedJob workload for this EphemeralRunner.
//
// The association works in two steps (SaFE Workload CRD is cluster-scoped):
//  1. GET the SaFE Workload with the same name as this launcher EphemeralRunner
//  2. Read its primus-safe.scale.runner.id label (which is the worker pod name)
//  3. Find the UnifiedJob with the same scale.runner.id
//
// If found, the UnifiedJob workload name is stored in info.SafeWorkloadID.
// For runners without a matching UnifiedJob, this is a no-op.
func (r *EphemeralRunnerReconciler) resolveSafeWorkload(ctx context.Context, info *types.EphemeralRunnerInfo) {
	if r.dynamicClient == nil || info.Name == "" {
		return
	}

	// Only resolve for launcher-type runners (workers are sub-pods of the launcher)
	if info.RunnerType == "worker" {
		return
	}

	// Step 1: GET the SaFE Workload with the same name as this EphemeralRunner
	// SaFE Workload CRD is cluster-scoped, so no namespace needed
	safeWorkload, err := r.dynamicClient.Resource(types.SafeWorkloadGVR).
		Get(ctx, info.Name, metav1.GetOptions{})
	if err != nil {
		// No matching SaFE Workload - normal for runners without SaFE association
		log.Debugf("EphemeralRunnerReconciler: no SaFE workload found for %s: %v", info.Name, err)
		return
	}

	// Step 2: Read the scale.runner.id label (this is the worker pod name that links both workloads)
	safeLabels := safeWorkload.GetLabels()
	scaleRunnerID, ok := safeLabels[types.LabelSafeScaleRunnerID]
	if !ok || scaleRunnerID == "" {
		log.Debugf("EphemeralRunnerReconciler: SaFE workload %s has no scale.runner.id label", info.Name)
		return
	}

	// Step 3: Find the UnifiedJob with the same scale.runner.id
	labelSelector := labels.SelectorFromSet(map[string]string{
		types.LabelSafeScaleRunnerID: scaleRunnerID,
		types.LabelSafeWorkloadKind:  types.SafeUnifiedJobKind,
	}).String()

	workloadList, err := r.dynamicClient.Resource(types.SafeWorkloadGVR).
		List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
			Limit:         1,
		})
	if err != nil {
		log.Debugf("EphemeralRunnerReconciler: failed to list UnifiedJob for scaleRunnerId %s: %v",
			scaleRunnerID, err)
		return
	}

	if len(workloadList.Items) > 0 {
		info.SafeWorkloadID = workloadList.Items[0].GetName()
		log.Infof("EphemeralRunnerReconciler: resolved SaFE UnifiedJob %q for runner %s (via scaleRunnerId %s)",
			info.SafeWorkloadID, info.Name, scaleRunnerID)
	}
}

// isPodTerminated checks if the Pod is no longer actively running.
// Returns true when the pod is Succeeded, Failed, not found, or in an unrecoverable error state.
func (r *EphemeralRunnerReconciler) isPodTerminated(info *types.EphemeralRunnerInfo) bool {
	switch info.PodPhase {
	case "Succeeded", "Failed":
		return true
	case "Unknown":
		// Pod not found or unreachable - treat as terminated
		return true
	case "Running":
		// Check if the pod is in a terminal error condition even though phase is Running
		switch info.PodCondition {
		case "OOMKilled", "ContainerCannotRun":
			return true
		}
		return false
	case "Pending":
		return false
	default:
		// Empty or unexpected phase - if we couldn't fetch the pod, it's likely gone
		if info.PodPhase == "" {
			return true
		}
		return false
	}
}

// enrichPodStatus fetches Pod status and enriches the EphemeralRunnerInfo
func (r *EphemeralRunnerReconciler) enrichPodStatus(ctx context.Context, info *types.EphemeralRunnerInfo) {
	if r.client == nil || r.client.Clientsets == nil {
		return
	}

	pod, err := r.client.Clientsets.CoreV1().Pods(info.Namespace).Get(ctx, info.Name, metav1.GetOptions{})
	if err != nil {
		log.Debugf("EphemeralRunnerReconciler: failed to get pod %s/%s: %v", info.Namespace, info.Name, err)
		info.PodPhase = "Unknown"
		info.PodMessage = fmt.Sprintf("Failed to get pod: %v", err)
		return
	}

	info.PodPhase = string(pod.Status.Phase)

	// Check container statuses for waiting/terminated states
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil {
			info.PodCondition = cs.State.Waiting.Reason
			info.PodMessage = cs.State.Waiting.Message
			return
		}
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			info.PodCondition = cs.State.Terminated.Reason
			info.PodMessage = cs.State.Terminated.Message
			return
		}
	}

	// Check Pod conditions
	for _, pc := range pod.Status.Conditions {
		if pc.Type == corev1.PodReady && pc.Status == corev1.ConditionTrue {
			info.PodCondition = database.PodConditionReady
			return
		}
	}
}
