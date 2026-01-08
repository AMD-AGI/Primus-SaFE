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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// EphemeralRunnerReconciler watches EphemeralRunner resources and creates workflow run records
type EphemeralRunnerReconciler struct {
	client        *clientsets.K8SClientSet
	dynamicClient dynamic.Interface
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

	log.Info("EphemeralRunnerReconciler initialized")
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

// Reconcile handles EphemeralRunner events
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

	// Handle finalizer for tracking completion
	if obj.GetDeletionTimestamp() != nil {
		// Object is being deleted
		if containsFinalizer(obj.GetFinalizers(), finalizerName) {
			// Process final state before deletion
			log.Infof("EphemeralRunnerReconciler: processing deletion for %s/%s (phase: %s)",
				req.Namespace, req.Name, info.Phase)

			// When being deleted, mark as completed regardless of phase
			// The runner is being cleaned up, so it must have finished
			info.IsCompleted = true
			if err := r.processDeletion(ctx, info); err != nil {
				log.Errorf("EphemeralRunnerReconciler: failed to process deletion for %s/%s: %v",
					req.Namespace, req.Name, err)
				return ctrl.Result{RequeueAfter: 5 * time.Second}, err
			}

			// Remove finalizer to allow deletion
			if err := r.removeFinalizer(ctx, obj); err != nil {
				log.Errorf("EphemeralRunnerReconciler: failed to remove finalizer from %s/%s: %v",
					req.Namespace, req.Name, err)
				return ctrl.Result{RequeueAfter: 5 * time.Second}, err
			}
			log.Infof("EphemeralRunnerReconciler: removed finalizer from %s/%s", req.Namespace, req.Name)
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !containsFinalizer(obj.GetFinalizers(), finalizerName) {
		if err := r.addFinalizer(ctx, obj); err != nil {
			log.Errorf("EphemeralRunnerReconciler: failed to add finalizer to %s/%s: %v",
				req.Namespace, req.Name, err)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, err
		}
		log.Debugf("EphemeralRunnerReconciler: added finalizer to %s/%s", req.Namespace, req.Name)
	}

	// Process the runner - track all state changes
	if err := r.processRunner(ctx, info); err != nil {
		log.Errorf("EphemeralRunnerReconciler: failed to process %s/%s: %v",
			req.Namespace, req.Name, err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	log.Debugf("EphemeralRunnerReconciler: processed %s/%s (phase: %s, workflow: %s)",
		req.Namespace, req.Name, info.Phase, info.WorkflowName)

	return ctrl.Result{}, nil
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

// processRunner processes an EphemeralRunner at any state
func (r *EphemeralRunnerReconciler) processRunner(ctx context.Context, info *types.EphemeralRunnerInfo) error {
	runnerSetFacade := database.GetFacade().GetGithubRunnerSet()
	runFacade := database.GetFacade().GetGithubWorkflowRun()

	// Find the runner set for this ephemeral runner
	runnerSet, err := runnerSetFacade.GetByNamespaceName(ctx, info.Namespace, info.RunnerSetName)
	if err != nil {
		return fmt.Errorf("failed to get runner set %s/%s: %w", info.Namespace, info.RunnerSetName, err)
	}

	if runnerSet == nil {
		log.Debugf("EphemeralRunnerReconciler: runner set not found for %s/%s, skipping", info.Namespace, info.RunnerSetName)
		return nil
	}

	// Enrich with GitHub API info if we have a run ID but missing details
	if info.GithubRunID != 0 && info.HeadSHA == "" {
		r.enrichWithGitHubInfo(ctx, info, runnerSet)
	}

	// Map EphemeralRunner phase to our status
	status := r.mapPhaseToStatus(info.Phase, info.IsCompleted)

	// Check if we already have a run record for this workload
	existingRun, err := runFacade.GetByRunnerSetAndWorkload(ctx, runnerSet.ID, info.UID)
	if err != nil {
		return fmt.Errorf("failed to check existing run for %s: %w", info.UID, err)
	}

	if existingRun != nil {
		needsUpdate := false
		oldStatus := existingRun.Status

		// Update status if changed
		if existingRun.Status != status && r.shouldUpdateStatus(oldStatus, status) {
			existingRun.Status = status
			needsUpdate = true
			if info.IsCompleted && existingRun.WorkloadCompletedAt.IsZero() {
				if !info.CompletionTime.IsZero() {
					existingRun.WorkloadCompletedAt = info.CompletionTime.Time
				} else {
					existingRun.WorkloadCompletedAt = time.Now()
				}
			}
		}

		// Update GitHub info if it becomes available (might not be present initially)
		if existingRun.GithubRunID == 0 && info.GithubRunID != 0 {
			existingRun.GithubRunID = info.GithubRunID
			needsUpdate = true
		}
		if existingRun.WorkflowName == "" && info.WorkflowName != "" {
			existingRun.WorkflowName = info.WorkflowName
			needsUpdate = true
		}
		if existingRun.HeadBranch == "" && info.Branch != "" {
			existingRun.HeadBranch = info.Branch
			needsUpdate = true
		}
		if existingRun.HeadSha == "" && info.HeadSHA != "" {
			existingRun.HeadSha = info.HeadSHA
			needsUpdate = true
		}
		if existingRun.GithubRunNumber == 0 && info.GithubRunNumber != 0 {
			existingRun.GithubRunNumber = int32(info.GithubRunNumber)
			needsUpdate = true
		}

		if needsUpdate {
			if err := runFacade.Update(ctx, existingRun); err != nil {
				return fmt.Errorf("failed to update run record for %s: %w", info.Name, err)
			}
			if existingRun.Status != oldStatus {
				log.Infof("EphemeralRunnerReconciler: updated run record %d for %s/%s (status: %s -> %s)",
					existingRun.ID, info.Namespace, info.Name, oldStatus, status)
			} else {
				log.Debugf("EphemeralRunnerReconciler: updated GitHub info for run record %d", existingRun.ID)
			}
		}
		return nil
	}

	// Find matching config for additional metadata (optional)
	var configID int64
	configFacade := database.GetFacade().GetGithubWorkflowConfig()
	configs, err := configFacade.ListByRunnerSetID(ctx, runnerSet.ID)
	if err != nil {
		log.Warnf("EphemeralRunnerReconciler: failed to list configs for runner set %d: %v", runnerSet.ID, err)
	} else if len(configs) > 0 {
		for _, config := range configs {
			if config.Enabled && r.matchesConfig(info, config) {
				configID = config.ID
				break
			}
		}
	}

	// Create a new run record
	run := &model.GithubWorkflowRuns{
		RunnerSetID:        runnerSet.ID,
		RunnerSetName:      runnerSet.Name,
		RunnerSetNamespace: runnerSet.Namespace,
		ConfigID:           configID,
		WorkloadUID:        info.UID,
		WorkloadName:       info.Name,
		WorkloadNamespace:  info.Namespace,
		GithubRunID:        info.GithubRunID,
		GithubRunNumber:    int32(info.GithubRunNumber),
		GithubJobID:        info.GithubJobID,
		HeadSha:            info.HeadSHA,
		HeadBranch:         info.Branch,
		WorkflowName:       info.WorkflowName,
		Status:             status,
		TriggerSource:      database.WorkflowRunTriggerRealtime,
		WorkloadStartedAt:  info.CreationTimestamp.Time,
	}

	// Set completion time if completed
	if info.IsCompleted {
		if !info.CompletionTime.IsZero() {
			run.WorkloadCompletedAt = info.CompletionTime.Time
		} else {
			run.WorkloadCompletedAt = time.Now()
		}
	}

	if err := runFacade.Create(ctx, run); err != nil {
		return fmt.Errorf("failed to create run record for %s: %w", info.Name, err)
	}

	log.Infof("EphemeralRunnerReconciler: created run record %d for %s/%s (runner_set: %s, status: %s)",
		run.ID, info.Namespace, info.Name, runnerSet.Name, status)

	return nil
}

// mapPhaseToStatus maps EphemeralRunner phase to workflow run status
func (r *EphemeralRunnerReconciler) mapPhaseToStatus(phase string, isCompleted bool) string {
	switch phase {
	case types.EphemeralRunnerPhasePending, "":
		return database.WorkflowRunStatusWorkloadPending
	case types.EphemeralRunnerPhaseRunning:
		return database.WorkflowRunStatusWorkloadRunning
	case types.EphemeralRunnerPhaseSucceeded, types.EphemeralRunnerPhaseFailed:
		// Completed runners are ready for collection
		return database.WorkflowRunStatusPending
	default:
		if isCompleted {
			return database.WorkflowRunStatusPending
		}
		return database.WorkflowRunStatusWorkloadRunning
	}
}

// shouldUpdateStatus checks if we should update from oldStatus to newStatus
func (r *EphemeralRunnerReconciler) shouldUpdateStatus(oldStatus, newStatus string) bool {
	// Define status priority (higher = later in lifecycle)
	priority := map[string]int{
		database.WorkflowRunStatusWorkloadPending: 1,
		database.WorkflowRunStatusWorkloadRunning: 2,
		database.WorkflowRunStatusPending:         3,
		database.WorkflowRunStatusCollecting:      4,
		database.WorkflowRunStatusExtracting:      5,
		database.WorkflowRunStatusCompleted:       6,
		database.WorkflowRunStatusFailed:          6,
		database.WorkflowRunStatusSkipped:         6,
	}

	oldPriority, oldOK := priority[oldStatus]
	newPriority, newOK := priority[newStatus]

	// Only allow forward transitions
	if !oldOK || !newOK {
		return true // Allow if status is unknown
	}
	return newPriority > oldPriority
}

// matchesConfig checks if an EphemeralRunner matches a workflow config
func (r *EphemeralRunnerReconciler) matchesConfig(info *types.EphemeralRunnerInfo, config *model.GithubWorkflowConfigs) bool {
	// Check namespace matches
	if config.RunnerSetNamespace != "" && config.RunnerSetNamespace != info.Namespace {
		return false
	}

	// Check runner set name if specified
	if config.RunnerSetName != "" && config.RunnerSetName != info.RunnerSetName {
		return false
	}

	// Check workflow filter if set
	if config.WorkflowFilter != "" && info.WorkflowName != "" && info.WorkflowName != config.WorkflowFilter {
		return false
	}

	// Check branch filter if set
	if config.BranchFilter != "" && info.Branch != "" && info.Branch != config.BranchFilter {
		return false
	}

	return true
}

// enrichWithGitHubInfo fetches additional info from GitHub API and enriches the EphemeralRunnerInfo
func (r *EphemeralRunnerReconciler) enrichWithGitHubInfo(ctx context.Context, info *types.EphemeralRunnerInfo, runnerSet *model.GithubRunnerSets) {
	// Skip if no GitHub run ID available
	if info.GithubRunID == 0 {
		return
	}

	// Skip if we already have head SHA (already enriched)
	if info.HeadSHA != "" {
		return
	}

	// Need GitHub owner and repo from runner set
	if runnerSet.GithubOwner == "" || runnerSet.GithubRepo == "" {
		log.Debugf("EphemeralRunnerReconciler: no GitHub owner/repo for runner set %s, skipping GitHub API call", runnerSet.Name)
		return
	}

	// Get GitHub client
	githubManager := github.GetGlobalManager()
	if githubManager == nil {
		log.Debugf("EphemeralRunnerReconciler: GitHub client manager not initialized")
		return
	}

	// Get client using the runner set's config secret
	if runnerSet.GithubConfigSecret == "" {
		log.Debugf("EphemeralRunnerReconciler: no GitHub config secret for runner set %s", runnerSet.Name)
		return
	}

	client, err := githubManager.GetClientForSecret(ctx, runnerSet.Namespace, runnerSet.GithubConfigSecret)
	if err != nil {
		log.Warnf("EphemeralRunnerReconciler: failed to get GitHub client for %s/%s: %v",
			runnerSet.Namespace, runnerSet.GithubConfigSecret, err)
		return
	}

	// Fetch workflow run info from GitHub
	runInfo, err := client.GetWorkflowRun(ctx, runnerSet.GithubOwner, runnerSet.GithubRepo, info.GithubRunID)
	if err != nil {
		log.Warnf("EphemeralRunnerReconciler: failed to get workflow run %d from GitHub: %v",
			info.GithubRunID, err)
		return
	}

	// Enrich the info with data from GitHub API
	if runInfo.HeadSHA != "" {
		info.HeadSHA = runInfo.HeadSHA
	}
	if runInfo.RunNumber != 0 && info.GithubRunNumber == 0 {
		info.GithubRunNumber = runInfo.RunNumber
	}
	if runInfo.HeadBranch != "" && info.Branch == "" {
		info.Branch = runInfo.HeadBranch
	}
	if runInfo.WorkflowName != "" && info.WorkflowName == "" {
		info.WorkflowName = runInfo.WorkflowName
	}

	log.Debugf("EphemeralRunnerReconciler: enriched %s with GitHub info (sha: %s, run_number: %d)",
		info.Name, info.HeadSHA, info.GithubRunNumber)
}

// processDeletion handles the deletion of an EphemeralRunner
// When a runner is deleted, we mark it as completed and ready for collection
func (r *EphemeralRunnerReconciler) processDeletion(ctx context.Context, info *types.EphemeralRunnerInfo) error {
	runnerSetFacade := database.GetFacade().GetGithubRunnerSet()
	runFacade := database.GetFacade().GetGithubWorkflowRun()

	// Find the runner set for this ephemeral runner
	runnerSet, err := runnerSetFacade.GetByNamespaceName(ctx, info.Namespace, info.RunnerSetName)
	if err != nil {
		return fmt.Errorf("failed to get runner set %s/%s: %w", info.Namespace, info.RunnerSetName, err)
	}

	if runnerSet == nil {
		log.Debugf("EphemeralRunnerReconciler: runner set not found for %s/%s, skipping deletion processing",
			info.Namespace, info.RunnerSetName)
		return nil
	}

	// Check if we have a run record for this workload
	existingRun, err := runFacade.GetByRunnerSetAndWorkload(ctx, runnerSet.ID, info.UID)
	if err != nil {
		return fmt.Errorf("failed to check existing run for %s: %w", info.UID, err)
	}

	if existingRun == nil {
		log.Debugf("EphemeralRunnerReconciler: no run record found for %s, skipping deletion processing", info.Name)
		return nil
	}

	// Only update if not already in a terminal state
	if existingRun.Status == database.WorkflowRunStatusCompleted ||
		existingRun.Status == database.WorkflowRunStatusFailed ||
		existingRun.Status == database.WorkflowRunStatusSkipped {
		log.Debugf("EphemeralRunnerReconciler: run %d already in terminal state %s", existingRun.ID, existingRun.Status)
		return nil
	}

	// Mark as pending (ready for collection)
	oldStatus := existingRun.Status
	existingRun.Status = database.WorkflowRunStatusPending
	existingRun.WorkloadCompletedAt = time.Now()

	if err := runFacade.Update(ctx, existingRun); err != nil {
		return fmt.Errorf("failed to update run record for deletion %s: %w", info.Name, err)
	}

	log.Infof("EphemeralRunnerReconciler: marked run %d as pending for collection on deletion (status: %s -> %s)",
		existingRun.ID, oldStatus, existingRun.Status)

	return nil
}

