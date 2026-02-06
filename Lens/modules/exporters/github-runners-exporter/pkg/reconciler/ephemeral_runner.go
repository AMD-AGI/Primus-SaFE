// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package reconciler

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/github"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/workflow"
	"github.com/AMD-AGI/Primus-SaFE/Lens/github-runners-exporter/pkg/types"
	corev1 "k8s.io/api/core/v1"
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
	runSummaryFacade := database.GetFacade().GetGithubWorkflowRunSummary()

	// Find the runner set for this ephemeral runner
	runnerSet, err := runnerSetFacade.GetByNamespaceName(ctx, info.Namespace, info.RunnerSetName)
	if err != nil {
		return fmt.Errorf("failed to get runner set %s/%s: %w", info.Namespace, info.RunnerSetName, err)
	}

	if runnerSet == nil {
		log.Debugf("EphemeralRunnerReconciler: runner set not found for %s/%s, skipping", info.Namespace, info.RunnerSetName)
		return nil
	}

	// Determine runner type (launcher vs worker)
	info.RunnerType = types.DetermineRunnerType(info.Name)

	// Get Pod status for visibility
	r.enrichPodStatus(ctx, info)

	// Enrich with GitHub API info if we have a run ID but missing details
	if info.GithubRunID != 0 && info.HeadSHA == "" {
		r.enrichWithGitHubInfo(ctx, info, runnerSet)
	}

	// Process Run Summary (Run-level aggregation)
	var runSummary *model.GithubWorkflowRunSummaries
	var isNewSummary bool
	if info.GithubRunID != 0 && runnerSet.GithubOwner != "" && runnerSet.GithubRepo != "" {
		// Has GitHub run ID - get or create regular summary
		runSummary, isNewSummary, err = runSummaryFacade.GetOrCreateByRunID(ctx, info.GithubRunID, runnerSet.GithubOwner, runnerSet.GithubRepo, runnerSet.ID)
		if err != nil {
			log.Warnf("EphemeralRunnerReconciler: failed to get/create run summary for github_run_id %d: %v", info.GithubRunID, err)
		} else if runSummary != nil {
			// Trigger GraphFetch if new or not yet fetched
			if isNewSummary || !runSummary.GraphFetched {
				r.triggerGraphFetch(ctx, runSummary, runnerSet)
			}
		}
	} else if runnerSet.GithubOwner != "" && runnerSet.GithubRepo != "" {
		// No GitHub run ID yet - get or create placeholder summary
		runSummary, err = r.getOrCreatePlaceholderSummary(ctx, runnerSet, runSummaryFacade)
		if err != nil {
			log.Warnf("EphemeralRunnerReconciler: failed to get/create placeholder summary for runner set %s: %v", runnerSet.Name, err)
		}
	}

	// Map EphemeralRunner phase to our status (considering pod errors)
	status := r.mapPhaseToStatus(info.Phase, info.IsCompleted)
	// If pod has error condition, mark as error status
	if r.isPodErrorCondition(info.PodCondition) {
		status = database.WorkflowRunStatusError
	}

	// Check if we already have a run record for this workload
	// Use workload_name instead of UID for uniqueness - this allows same-named runners
	// in the same repo to be aggregated together across different runs
	existingRun, err := runFacade.GetByRunnerSetAndWorkloadName(ctx, runnerSet.ID, info.Name)
	if err != nil {
		return fmt.Errorf("failed to check existing run for %s: %w", info.Name, err)
	}

	if existingRun != nil {
		needsUpdate := false
		oldStatus := existingRun.Status
		oldSummaryID := existingRun.RunSummaryID

		// Update WorkloadUID if changed (same name but different instance)
		if existingRun.WorkloadUID != info.UID {
			oldUID := existingRun.WorkloadUID
			existingRun.WorkloadUID = info.UID
			existingRun.WorkloadStartedAt = info.CreationTimestamp.Time
			needsUpdate = true
			log.Infof("EphemeralRunnerReconciler: updating UID for %s (old: %s, new: %s)",
				info.Name, oldUID, info.UID)
		}

		// Update runner type if not set or changed
		if existingRun.RunnerType != info.RunnerType {
			existingRun.RunnerType = info.RunnerType
			needsUpdate = true
		}

		// Update pod status
		if existingRun.PodPhase != info.PodPhase || existingRun.PodCondition != info.PodCondition || existingRun.PodMessage != info.PodMessage {
			existingRun.PodPhase = info.PodPhase
			existingRun.PodCondition = info.PodCondition
			existingRun.PodMessage = info.PodMessage
			needsUpdate = true
		}

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

			// When GithubRunID becomes available, upgrade from placeholder to real summary
			if runnerSet.GithubOwner != "" && runnerSet.GithubRepo != "" {
				summary, isNew, summaryErr := runSummaryFacade.GetOrCreateByRunID(ctx, info.GithubRunID, runnerSet.GithubOwner, runnerSet.GithubRepo, runnerSet.ID)
				if summaryErr != nil {
					log.Warnf("EphemeralRunnerReconciler: failed to create run summary for existing run %d (github_run_id %d): %v",
						existingRun.ID, info.GithubRunID, summaryErr)
				} else if summary != nil {
					existingRun.RunSummaryID = summary.ID
					log.Infof("EphemeralRunnerReconciler: upgraded run %d from placeholder to run summary %d (github_run_id: %d, new: %v)",
						existingRun.ID, summary.ID, info.GithubRunID, isNew)

					// Trigger GraphFetch if new or not yet fetched
					if isNew || !summary.GraphFetched {
						r.triggerGraphFetch(ctx, summary, runnerSet)
					}

					// Update runSummary variable for later use (code analysis trigger)
					runSummary = summary
					isNewSummary = isNew
				}
			}
		} else if existingRun.RunSummaryID == 0 && runSummary != nil {
			// Associate with placeholder summary if not already associated
			existingRun.RunSummaryID = runSummary.ID
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

			// Cleanup orphan placeholder summary if we upgraded to a real summary
			if oldSummaryID != 0 && existingRun.RunSummaryID != oldSummaryID {
				r.cleanupPlaceholderSummaryIfOrphan(ctx, oldSummaryID, runFacade, runSummaryFacade)
			}

			if existingRun.Status != oldStatus {
				log.Infof("EphemeralRunnerReconciler: updated run record %d for %s/%s (status: %s -> %s, type: %s)",
					existingRun.ID, info.Namespace, info.Name, oldStatus, status, info.RunnerType)

				// Refresh run summary status when job status changes
				if existingRun.RunSummaryID > 0 {
					if err := runSummaryFacade.RefreshStatusFromJobs(ctx, existingRun.RunSummaryID); err != nil {
						log.Warnf("EphemeralRunnerReconciler: failed to refresh run summary status for %d: %v",
							existingRun.RunSummaryID, err)
					}
				}
			} else {
				log.Debugf("EphemeralRunnerReconciler: updated GitHub info for run record %d", existingRun.ID)
			}

			// Trigger initial sync when runner starts running (event-driven, one-shot)
			// This replaces the old high-frequency polling SyncExecutor
			if status == database.WorkflowRunStatusWorkloadRunning && existingRun.GithubRunID > 0 {
				r.triggerInitialSync(ctx, existingRun)
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
		RunnerType:         info.RunnerType,
		PodPhase:           info.PodPhase,
		PodCondition:       info.PodCondition,
		PodMessage:         info.PodMessage,
	}

	// Set run_summary_id if we have a run summary
	if runSummary != nil {
		run.RunSummaryID = runSummary.ID
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

	log.Infof("EphemeralRunnerReconciler: created run record %d for %s/%s (runner_set: %s, status: %s, run_summary: %d)",
		run.ID, info.Namespace, info.Name, runnerSet.Name, status, run.RunSummaryID)

	// Refresh run summary status when a new job is added
	if run.RunSummaryID > 0 {
		if err := runSummaryFacade.RefreshStatusFromJobs(ctx, run.RunSummaryID); err != nil {
			log.Warnf("EphemeralRunnerReconciler: failed to refresh run summary status for %d: %v",
				run.RunSummaryID, err)
		}
	}

	// Trigger code analysis on first job of this run (Run-level trigger)
	if runSummary != nil && !runSummary.CodeAnalysisTriggered {
		r.triggerCodeAnalysis(ctx, runSummary, runnerSet)
	}

	// Trigger initial sync when runner starts running (event-driven, one-shot)
	// This replaces the old high-frequency polling SyncExecutor
	if status == database.WorkflowRunStatusWorkloadRunning && run.GithubRunID > 0 {
		r.triggerInitialSync(ctx, run)
	}

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

// triggerInitialSync triggers one-shot initial sync when runner starts running
// This replaces the old high-frequency polling SyncExecutor
func (r *EphemeralRunnerReconciler) triggerInitialSync(ctx context.Context, run *model.GithubWorkflowRuns) {
	if run.GithubRunID == 0 {
		return
	}

	// Create initial sync task (one-shot, not polling)
	if err := workflow.CreateInitialSyncTask(ctx, run.ID, true, true); err != nil {
		log.Warnf("EphemeralRunnerReconciler: failed to create initial sync task for run %d: %v", run.ID, err)
		return
	}

	log.Infof("EphemeralRunnerReconciler: triggered initial sync for run %d (github_run: %d)", run.ID, run.GithubRunID)
}

// triggerCompletionSync triggers one-shot completion sync when runner finishes
// This syncs the final job status, logs, and schedules periodic sync if workflow not done
func (r *EphemeralRunnerReconciler) triggerCompletionSync(ctx context.Context, run *model.GithubWorkflowRuns) {
	if run.GithubRunID == 0 {
		return
	}

	// Create completion sync task (one-shot, fetches jobs and logs)
	if err := workflow.CreateCompletionSyncTask(ctx, run.ID, true, true); err != nil {
		log.Warnf("EphemeralRunnerReconciler: failed to create completion sync task for run %d: %v", run.ID, err)
		return
	}

	log.Infof("EphemeralRunnerReconciler: triggered completion sync for run %d (github_run: %d)", run.ID, run.GithubRunID)
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
// When a runner is deleted, we mark it as ready for collection and submit a collection task
func (r *EphemeralRunnerReconciler) processDeletion(ctx context.Context, info *types.EphemeralRunnerInfo) error {
	runnerSetFacade := database.GetFacade().GetGithubRunnerSet()
	runFacade := database.GetFacade().GetGithubWorkflowRun()
	runSummaryFacade := database.GetFacade().GetGithubWorkflowRunSummary()

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
	// Use workload_name instead of UID for consistency with processRunner
	existingRun, err := runFacade.GetByRunnerSetAndWorkloadName(ctx, runnerSet.ID, info.Name)
	if err != nil {
		return fmt.Errorf("failed to check existing run for %s: %w", info.Name, err)
	}

	if existingRun == nil {
		log.Debugf("EphemeralRunnerReconciler: no run record found for %s, skipping deletion processing", info.Name)
		return nil
	}

	// Store old summary ID for potential cleanup
	oldSummaryID := existingRun.RunSummaryID

	// If already in a terminal state, just update pod status to reflect deletion
	if existingRun.Status == database.WorkflowRunStatusCompleted ||
		existingRun.Status == database.WorkflowRunStatusFailed ||
		existingRun.Status == database.WorkflowRunStatusSkipped {
		log.Debugf("EphemeralRunnerReconciler: run %d already in terminal state %s", existingRun.ID, existingRun.Status)

		// Update pod status to reflect that pod has been terminated/deleted
		needsUpdate := false
		if existingRun.PodPhase != "Succeeded" && existingRun.PodPhase != "Failed" {
			if existingRun.Status == database.WorkflowRunStatusCompleted {
				existingRun.PodPhase = "Succeeded"
			} else {
				existingRun.PodPhase = "Failed"
			}
			existingRun.PodCondition = "Terminated"
			existingRun.PodMessage = ""
			needsUpdate = true
		}

		if needsUpdate {
			if err := runFacade.Update(ctx, existingRun); err != nil {
				log.Warnf("EphemeralRunnerReconciler: failed to update pod status for terminal run %d: %v", existingRun.ID, err)
			} else {
				log.Debugf("EphemeralRunnerReconciler: updated pod status for terminal run %d to %s", existingRun.ID, existingRun.PodPhase)
			}
		}

		// Still try to cleanup placeholder summary even if run is in terminal state
		if oldSummaryID > 0 {
			r.cleanupPlaceholderSummaryIfOrphan(ctx, oldSummaryID, runFacade, runSummaryFacade)
		}
		return nil
	}

	// Mark as pending (ready for collection) and update pod status
	oldStatus := existingRun.Status
	existingRun.Status = database.WorkflowRunStatusPending
	existingRun.WorkloadCompletedAt = time.Now()
	// Update pod status to reflect deletion
	existingRun.PodPhase = "Succeeded"
	existingRun.PodCondition = "Terminated"
	existingRun.PodMessage = ""

	if err := runFacade.Update(ctx, existingRun); err != nil {
		return fmt.Errorf("failed to update run record for deletion %s: %w", info.Name, err)
	}

	log.Infof("EphemeralRunnerReconciler: marked run %d as pending for collection on deletion (status: %s -> %s, pod: %s)",
		existingRun.ID, oldStatus, existingRun.Status, existingRun.PodPhase)

	// Cleanup placeholder summary if this was the last run referencing it
	if oldSummaryID > 0 {
		r.cleanupPlaceholderSummaryIfOrphan(ctx, oldSummaryID, runFacade, runSummaryFacade)
	}

	// Trigger completion sync (event-driven, one-shot)
	// This replaces the continuous SyncExecutor polling with a one-time sync
	r.triggerCompletionSync(ctx, existingRun)

	// Submit collection task to TaskScheduler
	if err := r.submitCollectionTask(ctx, existingRun); err != nil {
		// Log error but don't fail - the run is already marked as pending
		// and will be picked up by the job scheduler if task submission fails
		log.Warnf("EphemeralRunnerReconciler: failed to submit collection task for run %d: %v", existingRun.ID, err)
	}

	return nil
}

// submitCollectionTask creates a collection task for the workflow run
func (r *EphemeralRunnerReconciler) submitCollectionTask(ctx context.Context, run *model.GithubWorkflowRuns) error {
	taskFacade := database.NewWorkloadTaskFacade()

	// Create unique workload UID for the task
	taskUID := fmt.Sprintf("collection-%d-%d", run.ID, time.Now().Unix())

	collectionTask := &model.WorkloadTaskState{
		WorkloadUID: taskUID,
		TaskType:    constant.TaskTypeGithubWorkflowCollection,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			"run_id":        run.ID,
			"runner_set_id": run.RunnerSetID,
			"config_id":     run.ConfigID,
			"workload_name": run.WorkloadName,
		},
	}

	if err := taskFacade.UpsertTask(ctx, collectionTask); err != nil {
		return fmt.Errorf("failed to create collection task: %w", err)
	}

	log.Infof("EphemeralRunnerReconciler: submitted collection task %s for run %d", taskUID, run.ID)
	return nil
}

// triggerGraphFetch triggers a GraphFetch task to fetch workflow graph from GitHub
func (r *EphemeralRunnerReconciler) triggerGraphFetch(ctx context.Context, summary *model.GithubWorkflowRunSummaries, runnerSet *model.GithubRunnerSets) {
	taskFacade := database.NewWorkloadTaskFacade()

	taskUID := fmt.Sprintf("graph-fetch-%d-%d", summary.GithubRunID, time.Now().Unix())

	graphFetchTask := &model.WorkloadTaskState{
		WorkloadUID: taskUID,
		TaskType:    constant.TaskTypeGithubGraphFetch,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			"run_summary_id":       summary.ID,
			"github_run_id":        summary.GithubRunID,
			"owner":                summary.Owner,
			"repo":                 summary.Repo,
			"runner_set_namespace": runnerSet.Namespace,
			"runner_set_name":      runnerSet.Name,
		},
	}

	if err := taskFacade.UpsertTask(ctx, graphFetchTask); err != nil {
		log.Warnf("EphemeralRunnerReconciler: failed to create graph fetch task for run summary %d: %v", summary.ID, err)
		return
	}

	log.Infof("EphemeralRunnerReconciler: triggered graph fetch task %s for run summary %d (github_run_id: %d)",
		taskUID, summary.ID, summary.GithubRunID)
}

// triggerCodeAnalysis triggers code analysis on first job of a run (Run-level trigger)
func (r *EphemeralRunnerReconciler) triggerCodeAnalysis(ctx context.Context, summary *model.GithubWorkflowRunSummaries, runnerSet *model.GithubRunnerSets) {
	runSummaryFacade := database.GetFacade().GetGithubWorkflowRunSummary()

	// Mark as triggered first to prevent duplicate triggers
	if err := runSummaryFacade.UpdateAnalysisTriggered(ctx, summary.ID, "code", true); err != nil {
		log.Warnf("EphemeralRunnerReconciler: failed to update code_analysis_triggered for run summary %d: %v", summary.ID, err)
		return
	}

	taskFacade := database.NewWorkloadTaskFacade()
	taskUID := fmt.Sprintf("code-analysis-%d-%d", summary.GithubRunID, time.Now().Unix())

	analysisTask := &model.WorkloadTaskState{
		WorkloadUID: taskUID,
		TaskType:    constant.TaskTypeGithubCodeIndexing, // Code indexing for AI-Me
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			"run_summary_id": summary.ID,
			"github_run_id":  summary.GithubRunID,
			"owner":          summary.Owner,
			"repo":           summary.Repo,
			"head_sha":       summary.HeadSha,
			"head_branch":    summary.HeadBranch,
		},
	}

	if err := taskFacade.UpsertTask(ctx, analysisTask); err != nil {
		log.Warnf("EphemeralRunnerReconciler: failed to create code analysis task for run summary %d: %v", summary.ID, err)
		return
	}

	log.Infof("EphemeralRunnerReconciler: triggered code analysis task %s for run summary %d (first job of run)",
		taskUID, summary.ID)
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

// getOrCreatePlaceholderSummary creates or gets an existing placeholder summary for a runner set
func (r *EphemeralRunnerReconciler) getOrCreatePlaceholderSummary(ctx context.Context, runnerSet *model.GithubRunnerSets, facade *database.GithubWorkflowRunSummaryFacade) (*model.GithubWorkflowRunSummaries, error) {
	// Try to find existing active placeholder for this runner set
	summary, err := facade.GetActivePlaceholderByRunnerSet(ctx, runnerSet.ID)
	if err != nil {
		return nil, err
	}
	if summary != nil {
		return summary, nil
	}

	// Create new placeholder summary
	// Use negative runner_set_id as github_run_id to avoid unique constraint conflict
	// The unique constraint is on (github_run_id, github_run_attempt)
	placeholder := &model.GithubWorkflowRunSummaries{
		GithubRunID:        -int64(runnerSet.ID),
		GithubRunNumber:    0,
		Owner:              runnerSet.GithubOwner,
		Repo:               runnerSet.GithubRepo,
		Status:             database.RunSummaryStatusQueued,
		IsPlaceholder:      true,
		PrimaryRunnerSetID: runnerSet.ID,
		WorkflowName:       "Waiting for job assignment...",
		CollectionStatus:   database.RunSummaryCollectionPending,
	}

	created, err := facade.Create(ctx, placeholder)
	if err != nil {
		return nil, fmt.Errorf("failed to create placeholder summary: %w", err)
	}

	log.Infof("EphemeralRunnerReconciler: created placeholder summary %d for runner set %s/%s",
		created.ID, runnerSet.Namespace, runnerSet.Name)
	return created, nil
}

// isPodErrorCondition checks if the pod condition indicates an error
func (r *EphemeralRunnerReconciler) isPodErrorCondition(condition string) bool {
	switch condition {
	case database.PodConditionImagePullBackOff,
		database.PodConditionCrashLoopBackOff,
		database.PodConditionOOMKilled:
		return true
	default:
		return false
	}
}

// cleanupPlaceholderSummaryIfOrphan deletes a placeholder summary if no runs reference it
func (r *EphemeralRunnerReconciler) cleanupPlaceholderSummaryIfOrphan(ctx context.Context, summaryID int64, runFacade database.GithubWorkflowRunFacadeInterface, summaryFacade *database.GithubWorkflowRunSummaryFacade) {
	summary, err := summaryFacade.GetByID(ctx, summaryID)
	if err != nil || summary == nil || !summary.IsPlaceholder {
		return // Not a placeholder summary, skip
	}

	// Check if any runs still reference this placeholder
	count, err := runFacade.CountByRunSummaryID(ctx, summaryID)
	if err != nil {
		log.Warnf("EphemeralRunnerReconciler: failed to count runs for placeholder summary %d: %v", summaryID, err)
		return
	}

	if count == 0 {
		// No references, safe to delete
		if err := summaryFacade.Delete(ctx, summaryID); err != nil {
			log.Warnf("EphemeralRunnerReconciler: failed to delete orphan placeholder summary %d: %v", summaryID, err)
			return
		}
		log.Infof("EphemeralRunnerReconciler: cleaned up orphan placeholder summary %d", summaryID)
	}
}

