// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package processor implements the RunnerStateProcessor which reads raw K8s state
// from the github_ephemeral_runner_states table and drives workflow_run lifecycle,
// run_summary management, and task creation.
//
// This decouples business logic from the reconciler, making the reconciler lightweight
// and ensuring reliable state processing with crash recovery.
package processor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/github"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/workflow"
)

const (
	// DefaultScanInterval is the default interval between state scans
	DefaultScanInterval = 3 * time.Second

	// DefaultBatchSize is the default number of states to process per scan
	DefaultBatchSize = 100

	// DefaultCleanupInterval is the interval for cleaning up old deleted states
	DefaultCleanupInterval = 1 * time.Hour

	// DefaultCleanupRetentionDays is how long to keep deleted states
	DefaultCleanupRetentionDays = 7
)

// RunnerStateProcessor reads raw K8s state from the database and drives:
// 1. workflow_run record lifecycle (create, status transitions)
// 2. run_summary management (create, upgrade from placeholder, cleanup)
// 3. Task creation based on lifecycle transitions (sync, collection, graph-fetch, etc.)
//
// Design principles:
//   - Idempotent: re-processing the same state produces no side effects
//   - Recoverable: after crash, unprocessed states are automatically re-scanned
//   - Decoupled: no K8s API calls, no GitHub API calls - purely DB-driven
type RunnerStateProcessor struct {
	scanInterval    time.Duration
	batchSize       int
	cleanupInterval time.Duration
	retentionDays   int

	stateFacade *database.GithubEphemeralRunnerStateFacade

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ProcessorConfig holds configuration for the RunnerStateProcessor
type ProcessorConfig struct {
	ScanInterval    time.Duration
	BatchSize       int
	CleanupInterval time.Duration
	RetentionDays   int
}

// NewRunnerStateProcessor creates a new processor with the given config
func NewRunnerStateProcessor(cfg *ProcessorConfig) *RunnerStateProcessor {
	if cfg == nil {
		cfg = &ProcessorConfig{}
	}
	if cfg.ScanInterval <= 0 {
		cfg.ScanInterval = DefaultScanInterval
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultBatchSize
	}
	if cfg.CleanupInterval <= 0 {
		cfg.CleanupInterval = DefaultCleanupInterval
	}
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = DefaultCleanupRetentionDays
	}

	return &RunnerStateProcessor{
		scanInterval:    cfg.ScanInterval,
		batchSize:       cfg.BatchSize,
		cleanupInterval: cfg.CleanupInterval,
		retentionDays:   cfg.RetentionDays,
		stateFacade:     database.NewGithubEphemeralRunnerStateFacade(),
	}
}

// Start begins the background processing loop
func (p *RunnerStateProcessor) Start(ctx context.Context) error {
	ctx, p.cancel = context.WithCancel(ctx)

	// Start main processing loop
	p.wg.Add(1)
	go p.processLoop(ctx)

	// Start cleanup loop
	p.wg.Add(1)
	go p.cleanupLoop(ctx)

	log.Infof("RunnerStateProcessor started (scan_interval: %v, batch_size: %d)", p.scanInterval, p.batchSize)
	return nil
}

// Stop gracefully stops the processor
func (p *RunnerStateProcessor) Stop() error {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	log.Info("RunnerStateProcessor stopped")
	return nil
}

// processLoop is the main processing loop
func (p *RunnerStateProcessor) processLoop(ctx context.Context) {
	defer p.wg.Done()

	ticker := time.NewTicker(p.scanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.scanAndProcess(ctx)
		}
	}
}

// cleanupLoop periodically cleans up old deleted states
func (p *RunnerStateProcessor) cleanupLoop(ctx context.Context) {
	defer p.wg.Done()

	ticker := time.NewTicker(p.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deleted, err := p.stateFacade.CleanupOldDeleted(ctx, p.retentionDays)
			if err != nil {
				log.Warnf("RunnerStateProcessor: cleanup error: %v", err)
			} else if deleted > 0 {
				log.Infof("RunnerStateProcessor: cleaned up %d old deleted states", deleted)
			}
		}
	}
}

// scanAndProcess reads unprocessed states and processes them
func (p *RunnerStateProcessor) scanAndProcess(ctx context.Context) {
	states, err := p.stateFacade.ListUnprocessed(ctx, p.batchSize)
	if err != nil {
		log.Warnf("RunnerStateProcessor: failed to list unprocessed states: %v", err)
		return
	}

	for _, state := range states {
		if ctx.Err() != nil {
			return
		}
		p.processState(ctx, state)
	}
}

// processState processes a single runner state change
func (p *RunnerStateProcessor) processState(ctx context.Context, state *model.GithubEphemeralRunnerStates) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("RunnerStateProcessor: panic processing state %d (%s/%s): %v",
				state.ID, state.Namespace, state.Name, r)
		}
	}()

	// Skip worker-type runners - they are compute nodes managed by a launcher
	// and have no independent GitHub identity (github_run_id=0).
	// Only launcher runners represent actual workflow runs.
	if state.RunnerType == "worker" {
		if err := p.stateFacade.MarkProcessed(ctx, state.ID, 0, 0, "skipped_worker"); err != nil {
			log.Warnf("RunnerStateProcessor: failed to mark worker state %d as processed: %v", state.ID, err)
		}
		return
	}

	runnerSetFacade := database.GetFacade().GetGithubRunnerSet()
	runFacade := database.GetFacade().GetGithubWorkflowRun()
	runSummaryFacade := database.GetFacade().GetGithubWorkflowRunSummary()

	// 1. Find the runner set
	runnerSet, err := runnerSetFacade.GetByNamespaceName(ctx, state.Namespace, state.RunnerSetName)
	if err != nil {
		log.Warnf("RunnerStateProcessor: failed to get runner set %s/%s: %v",
			state.Namespace, state.RunnerSetName, err)
		return // Retry on next scan
	}
	if runnerSet == nil {
		log.Debugf("RunnerStateProcessor: runner set not found for %s/%s, marking processed",
			state.Namespace, state.RunnerSetName)
		p.stateFacade.MarkProcessed(ctx, state.ID, 0, 0, "")
		return
	}

	// 2. Determine status from K8s phase
	newStatus := p.mapPhaseToStatus(state.Phase, state.IsCompleted, state.IsDeleted, state.PodCondition)

	// 3. Get or create workflow_run record
	run, err := p.getOrCreateWorkflowRun(ctx, state, runnerSet, runFacade, newStatus)
	if err != nil {
		log.Errorf("RunnerStateProcessor: failed to get/create workflow_run for %s/%s: %v",
			state.Namespace, state.Name, err)
		return // Retry on next scan
	}

	// 4. Handle run summary lifecycle
	var runSummary *model.GithubWorkflowRunSummaries
	runSummaryID := state.RunSummaryID

	if state.GithubRunID != 0 && runnerSet.GithubOwner != "" && runnerSet.GithubRepo != "" {
		runSummary, runSummaryID = p.handleRunSummary(ctx, state, run, runnerSet, runSummaryFacade, runFacade)
	} else if runnerSet.GithubOwner != "" && runnerSet.GithubRepo != "" {
		// No GitHub run ID yet - handle placeholder
		runSummary, runSummaryID = p.handlePlaceholderSummary(ctx, state, run, runnerSet, runSummaryFacade, runFacade)
	}

	// 5. Update workflow_run fields if changed
	p.updateWorkflowRunFields(ctx, state, run, runFacade, newStatus, runSummaryID)

	// 6. Detect lifecycle transitions and create tasks
	p.handleTransitions(ctx, state, run, runnerSet, runSummary, newStatus)

	// 7. Mark state as processed
	if err := p.stateFacade.MarkProcessed(ctx, state.ID, run.ID, runSummaryID, newStatus); err != nil {
		log.Warnf("RunnerStateProcessor: failed to mark processed for state %d: %v", state.ID, err)
	}
}

// getOrCreateWorkflowRun gets an existing or creates a new workflow_run record
func (p *RunnerStateProcessor) getOrCreateWorkflowRun(
	ctx context.Context,
	state *model.GithubEphemeralRunnerStates,
	runnerSet *model.GithubRunnerSets,
	runFacade database.GithubWorkflowRunFacadeInterface,
	status string,
) (*model.GithubWorkflowRuns, error) {

	// Try to get by cached workflow_run_id
	if state.WorkflowRunID > 0 {
		run, err := runFacade.GetByID(ctx, state.WorkflowRunID)
		if err == nil && run != nil {
			return run, nil
		}
	}

	// Try to find by runner_set + workload_name
	existingRun, err := runFacade.GetByRunnerSetAndWorkloadName(ctx, runnerSet.ID, state.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup existing run: %w", err)
	}
	if existingRun != nil {
		return existingRun, nil
	}

	// Find matching config for additional metadata
	var configID int64
	configFacade := database.GetFacade().GetGithubWorkflowConfig()
	configs, err := configFacade.ListByRunnerSetID(ctx, runnerSet.ID)
	if err == nil {
		for _, config := range configs {
			if config.Enabled && matchesConfig(state, config) {
				configID = config.ID
				break
			}
		}
	}

	// Create new run record
	run := &model.GithubWorkflowRuns{
		RunnerSetID:        runnerSet.ID,
		RunnerSetName:      runnerSet.Name,
		RunnerSetNamespace: runnerSet.Namespace,
		ConfigID:           configID,
		WorkloadUID:        state.UID,
		WorkloadName:       state.Name,
		WorkloadNamespace:  state.Namespace,
		GithubRunID:        state.GithubRunID,
		GithubRunNumber:    state.GithubRunNumber,
		GithubJobID:        state.GithubJobID,
		HeadSha:            state.HeadSha,
		HeadBranch:         state.HeadBranch,
		WorkflowName:       state.WorkflowName,
		Status:             status,
		TriggerSource:      database.WorkflowRunTriggerRealtime,
		WorkloadStartedAt:  state.CreationTimestamp,
		RunnerType:         state.RunnerType,
		PodPhase:           state.PodPhase,
		PodCondition:       state.PodCondition,
		PodMessage:         state.PodMessage,
		SafeWorkloadID:     state.SafeWorkloadID,
	}

	if state.IsCompleted {
		if !state.CompletionTime.IsZero() {
			run.WorkloadCompletedAt = state.CompletionTime
		} else {
			run.WorkloadCompletedAt = time.Now()
		}
	}

	if err := runFacade.Create(ctx, run); err != nil {
		return nil, fmt.Errorf("failed to create workflow_run: %w", err)
	}

	log.Infof("RunnerStateProcessor: created workflow_run %d for %s/%s (runner_set: %s, status: %s)",
		run.ID, state.Namespace, state.Name, runnerSet.Name, status)

	return run, nil
}

// handleRunSummary manages the run_summary lifecycle when github_run_id is available
func (p *RunnerStateProcessor) handleRunSummary(
	ctx context.Context,
	state *model.GithubEphemeralRunnerStates,
	run *model.GithubWorkflowRuns,
	runnerSet *model.GithubRunnerSets,
	summaryFacade *database.GithubWorkflowRunSummaryFacade,
	runFacade database.GithubWorkflowRunFacadeInterface,
) (*model.GithubWorkflowRunSummaries, int64) {

	summary, isNew, err := summaryFacade.GetOrCreateByRunID(
		ctx, state.GithubRunID, runnerSet.GithubOwner, runnerSet.GithubRepo, runnerSet.ID,
	)
	if err != nil {
		log.Warnf("RunnerStateProcessor: failed to get/create run summary for github_run_id %d: %v",
			state.GithubRunID, err)
		return nil, state.RunSummaryID
	}
	if summary == nil {
		return nil, state.RunSummaryID
	}

	// Trigger graph fetch if new summary or not yet fetched
	if isNew || !summary.GraphFetched {
		p.triggerGraphFetch(ctx, summary, runnerSet)
	}

	// Trigger code analysis on first job of this run
	if !summary.CodeAnalysisTriggered {
		p.triggerCodeAnalysis(ctx, summary, runnerSet)
	}

	// If the run's summary was a placeholder, handle upgrade
	oldSummaryID := run.RunSummaryID
	if run.RunSummaryID != summary.ID {
		runFacade.UpdateFields(ctx, run.ID, map[string]interface{}{
			"run_summary_id": summary.ID,
		})
		run.RunSummaryID = summary.ID

		log.Infof("RunnerStateProcessor: associated run %d with run summary %d (github_run_id: %d)",
			run.ID, summary.ID, state.GithubRunID)

		// Cleanup old placeholder if orphaned
		if oldSummaryID > 0 && oldSummaryID != summary.ID {
			p.cleanupPlaceholderIfOrphan(ctx, oldSummaryID, runFacade, summaryFacade)
		}
	}

	return summary, summary.ID
}

// handlePlaceholderSummary creates or gets a placeholder summary when no github_run_id is available
func (p *RunnerStateProcessor) handlePlaceholderSummary(
	ctx context.Context,
	state *model.GithubEphemeralRunnerStates,
	run *model.GithubWorkflowRuns,
	runnerSet *model.GithubRunnerSets,
	summaryFacade *database.GithubWorkflowRunSummaryFacade,
	runFacade database.GithubWorkflowRunFacadeInterface,
) (*model.GithubWorkflowRunSummaries, int64) {

	if run.RunSummaryID > 0 {
		// Already has a summary association
		return nil, run.RunSummaryID
	}

	// Try to find existing placeholder
	summary, err := summaryFacade.GetActivePlaceholderByRunnerSet(ctx, runnerSet.ID)
	if err != nil {
		log.Warnf("RunnerStateProcessor: failed to get placeholder summary: %v", err)
		return nil, 0
	}

	if summary == nil {
		// Create new placeholder
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

		summary, err = summaryFacade.Create(ctx, placeholder)
		if err != nil {
			log.Warnf("RunnerStateProcessor: failed to create placeholder summary: %v", err)
			return nil, 0
		}

		log.Infof("RunnerStateProcessor: created placeholder summary %d for runner set %s/%s",
			summary.ID, runnerSet.Namespace, runnerSet.Name)
	}

	// Associate run with placeholder
	runFacade.UpdateFields(ctx, run.ID, map[string]interface{}{
		"run_summary_id": summary.ID,
	})
	run.RunSummaryID = summary.ID

	return summary, summary.ID
}

// updateWorkflowRunFields updates the workflow_run record with current state
func (p *RunnerStateProcessor) updateWorkflowRunFields(
	ctx context.Context,
	state *model.GithubEphemeralRunnerStates,
	run *model.GithubWorkflowRuns,
	runFacade database.GithubWorkflowRunFacadeInterface,
	newStatus string,
	runSummaryID int64,
) {
	fields := make(map[string]interface{})

	// Update UID if changed (same name but new instance)
	if run.WorkloadUID != state.UID && state.UID != "" {
		fields["workload_uid"] = state.UID
		fields["workload_started_at"] = state.CreationTimestamp
		log.Infof("RunnerStateProcessor: updating UID for run %d (old: %s, new: %s)",
			run.ID, run.WorkloadUID, state.UID)
	}

	// Update runner type
	if run.RunnerType != state.RunnerType && state.RunnerType != "" {
		fields["runner_type"] = state.RunnerType
	}

	// Update pod state
	if run.PodPhase != state.PodPhase || run.PodCondition != state.PodCondition || run.PodMessage != state.PodMessage {
		fields["pod_phase"] = state.PodPhase
		fields["pod_condition"] = state.PodCondition
		fields["pod_message"] = state.PodMessage
	}

	// Update status (only forward transitions)
	if run.Status != newStatus && shouldUpdateStatus(run.Status, newStatus) {
		fields["status"] = newStatus
		if state.IsCompleted && run.WorkloadCompletedAt.IsZero() {
			if !state.CompletionTime.IsZero() {
				fields["workload_completed_at"] = state.CompletionTime
			} else {
				fields["workload_completed_at"] = time.Now()
			}
		}

		log.Infof("RunnerStateProcessor: status transition for run %d: %s -> %s",
			run.ID, run.Status, newStatus)
	}

	// Update GitHub info if it becomes available from K8s labels
	if run.GithubRunID == 0 && state.GithubRunID != 0 {
		fields["github_run_id"] = state.GithubRunID
	}
	if run.GithubJobID == 0 && state.GithubJobID != 0 {
		fields["github_job_id"] = state.GithubJobID
	}
	if run.WorkflowName == "" && state.WorkflowName != "" {
		fields["workflow_name"] = state.WorkflowName
	}
	if run.HeadBranch == "" && state.HeadBranch != "" {
		fields["head_branch"] = state.HeadBranch
	}
	if run.HeadSha == "" && state.HeadSha != "" {
		fields["head_sha"] = state.HeadSha
	}
	if run.GithubRunNumber == 0 && state.GithubRunNumber != 0 {
		fields["github_run_number"] = state.GithubRunNumber
	}

	// Update SaFE workload association if it becomes available
	if run.SafeWorkloadID == "" && state.SafeWorkloadID != "" {
		fields["safe_workload_id"] = state.SafeWorkloadID
		log.Infof("RunnerStateProcessor: associated SaFE UnifiedJob %q with run %d (%s)",
			state.SafeWorkloadID, run.ID, state.Name)
	}

	// Update run_summary_id if changed
	if run.RunSummaryID != runSummaryID && runSummaryID > 0 {
		fields["run_summary_id"] = runSummaryID
	}

	if len(fields) > 0 {
		if err := runFacade.UpdateFields(ctx, run.ID, fields); err != nil {
			log.Warnf("RunnerStateProcessor: failed to update workflow_run %d: %v", run.ID, err)
		}

		// Refresh run summary status when status changes
		statusVal, statusChanged := fields["status"]
		if statusChanged {
			summaryID := runSummaryID
			if summaryID == 0 {
				summaryID = run.RunSummaryID
			}
			if summaryID > 0 {
				summaryFacade := database.GetFacade().GetGithubWorkflowRunSummary()
				if err := summaryFacade.RefreshStatusFromJobs(ctx, summaryID); err != nil {
					log.Warnf("RunnerStateProcessor: failed to refresh summary status for %d: %v", summaryID, err)
				}
			}

			// Update run's in-memory status for transition detection
			if s, ok := statusVal.(string); ok {
				run.Status = s
			}
		}
	}
}

// handleTransitions detects lifecycle transitions and creates tasks accordingly
func (p *RunnerStateProcessor) handleTransitions(
	ctx context.Context,
	state *model.GithubEphemeralRunnerStates,
	run *model.GithubWorkflowRuns,
	runnerSet *model.GithubRunnerSets,
	runSummary *model.GithubWorkflowRunSummaries,
	newStatus string,
) {
	oldStatus := state.LastStatus

	// Transition: runner started running -> trigger initial sync
	if oldStatus != database.WorkflowRunStatusWorkloadRunning &&
		newStatus == database.WorkflowRunStatusWorkloadRunning &&
		run.GithubRunID > 0 {
		p.triggerInitialSync(ctx, run)
	}

	// Transition: runner deleted -> trigger completion sync + collection
	if state.IsDeleted && oldStatus != database.WorkflowRunStatusPending &&
		(newStatus == database.WorkflowRunStatusPending || state.LastStatus == "") {
		p.triggerCompletionSync(ctx, run)
		p.submitCollectionTask(ctx, run)
	}
}

// ============================================================================
// Task creation methods (moved from reconciler, now using stable UIDs)
// ============================================================================

// triggerInitialSync triggers one-shot initial sync when runner starts running
func (p *RunnerStateProcessor) triggerInitialSync(ctx context.Context, run *model.GithubWorkflowRuns) {
	if run.GithubRunID == 0 {
		return
	}

	// Use stable UID for idempotency
	if err := workflow.CreateInitialSyncTask(ctx, run.ID, true, true); err != nil {
		log.Warnf("RunnerStateProcessor: failed to create initial sync task for run %d: %v", run.ID, err)
		return
	}

	log.Infof("RunnerStateProcessor: triggered initial sync for run %d (github_run: %d)", run.ID, run.GithubRunID)
}

// triggerCompletionSync triggers one-shot completion sync when runner finishes
func (p *RunnerStateProcessor) triggerCompletionSync(ctx context.Context, run *model.GithubWorkflowRuns) {
	if run.GithubRunID == 0 {
		return
	}

	if err := workflow.CreateCompletionSyncTask(ctx, run.ID, true, true); err != nil {
		log.Warnf("RunnerStateProcessor: failed to create completion sync task for run %d: %v", run.ID, err)
		return
	}

	log.Infof("RunnerStateProcessor: triggered completion sync for run %d (github_run: %d)", run.ID, run.GithubRunID)
}

// submitCollectionTask creates a collection task for data extraction
func (p *RunnerStateProcessor) submitCollectionTask(ctx context.Context, run *model.GithubWorkflowRuns) {
	taskFacade := database.NewWorkloadTaskFacade()

	// Use stable UID for idempotency
	taskUID := fmt.Sprintf("collection-%d", run.ID)

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
		log.Warnf("RunnerStateProcessor: failed to create collection task for run %d: %v", run.ID, err)
		return
	}

	log.Infof("RunnerStateProcessor: submitted collection task %s for run %d", taskUID, run.ID)
}

// triggerGraphFetch triggers a graph-fetch task to get workflow graph from GitHub
func (p *RunnerStateProcessor) triggerGraphFetch(ctx context.Context, summary *model.GithubWorkflowRunSummaries, runnerSet *model.GithubRunnerSets) {
	taskFacade := database.NewWorkloadTaskFacade()

	// Use stable UID for idempotency
	taskUID := fmt.Sprintf("graph-fetch-%d", summary.GithubRunID)

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
		log.Warnf("RunnerStateProcessor: failed to create graph fetch task for summary %d: %v", summary.ID, err)
		return
	}

	log.Infof("RunnerStateProcessor: triggered graph fetch for summary %d (github_run_id: %d)",
		summary.ID, summary.GithubRunID)
}

// triggerCodeAnalysis triggers code analysis on first job of a run
func (p *RunnerStateProcessor) triggerCodeAnalysis(ctx context.Context, summary *model.GithubWorkflowRunSummaries, runnerSet *model.GithubRunnerSets) {
	summaryFacade := database.GetFacade().GetGithubWorkflowRunSummary()

	// Mark as triggered first to prevent duplicate triggers
	if err := summaryFacade.UpdateAnalysisTriggered(ctx, summary.ID, "code", true); err != nil {
		log.Warnf("RunnerStateProcessor: failed to update code_analysis_triggered for summary %d: %v", summary.ID, err)
		return
	}

	// Resolve GitHub token from the runner set secret so that downstream
	// consumers (lens-workflow-analysis, ai-me) can clone private repos.
	var githubToken string
	if runnerSet.GithubConfigSecret != "" && runnerSet.Namespace != "" {
		if mgr := github.GetGlobalManager(); mgr != nil {
			token, err := mgr.GetTokenForSecret(ctx, runnerSet.Namespace, runnerSet.GithubConfigSecret)
			if err != nil {
				log.Warnf("RunnerStateProcessor: failed to resolve github token for summary %d: %v", summary.ID, err)
			} else {
				githubToken = token
			}
		}
	}

	taskFacade := database.NewWorkloadTaskFacade()

	// Use stable UID for idempotency
	taskUID := fmt.Sprintf("code-analysis-%d", summary.GithubRunID)

	ext := model.ExtType{
		"run_summary_id": summary.ID,
		"github_run_id":  summary.GithubRunID,
		"owner":          summary.Owner,
		"repo":           summary.Repo,
		"head_sha":       summary.HeadSha,
		"head_branch":    summary.HeadBranch,
		"workflow_name":  summary.WorkflowName,
		"repo_name":      summary.Owner + "/" + summary.Repo,
		"analysis_type":  "code",
	}
	if githubToken != "" {
		ext["github_token"] = githubToken
	}

	analysisTask := &model.WorkloadTaskState{
		WorkloadUID: taskUID,
		TaskType:    constant.TaskTypeGithubCodeIndexing,
		Status:      constant.TaskStatusPending,
		Ext:         ext,
	}

	if err := taskFacade.UpsertTask(ctx, analysisTask); err != nil {
		log.Warnf("RunnerStateProcessor: failed to create code analysis task for summary %d: %v", summary.ID, err)
		return
	}

	log.Infof("RunnerStateProcessor: triggered code analysis for summary %d (first job of run)", summary.ID)
}

// ============================================================================
// Status mapping and utility methods (moved from reconciler)
// ============================================================================

// mapPhaseToStatus maps K8s EphemeralRunner phase to workflow run status
func (p *RunnerStateProcessor) mapPhaseToStatus(phase string, isCompleted, isDeleted bool, podCondition string) string {
	// Pod error conditions override phase-based status
	if isPodErrorCondition(podCondition) {
		return database.WorkflowRunStatusError
	}

	// Deletion means runner is finished, ready for collection
	if isDeleted {
		return database.WorkflowRunStatusPending
	}

	switch phase {
	case "Pending", "":
		return database.WorkflowRunStatusWorkloadPending
	case "Running":
		return database.WorkflowRunStatusWorkloadRunning
	case "Succeeded", "Failed":
		return database.WorkflowRunStatusPending
	default:
		if isCompleted {
			return database.WorkflowRunStatusPending
		}
		return database.WorkflowRunStatusWorkloadRunning
	}
}

// shouldUpdateStatus checks if we should update from oldStatus to newStatus
// Only allows forward transitions in the lifecycle
func shouldUpdateStatus(oldStatus, newStatus string) bool {
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

	if !oldOK || !newOK {
		return true // Allow if status is unknown
	}
	return newPriority > oldPriority
}

// isPodErrorCondition checks if a pod condition indicates an error
func isPodErrorCondition(condition string) bool {
	switch condition {
	case database.PodConditionImagePullBackOff,
		database.PodConditionCrashLoopBackOff,
		database.PodConditionOOMKilled:
		return true
	default:
		return false
	}
}

// matchesConfig checks if a runner state matches a workflow config
func matchesConfig(state *model.GithubEphemeralRunnerStates, config *model.GithubWorkflowConfigs) bool {
	if config.RunnerSetNamespace != "" && config.RunnerSetNamespace != state.Namespace {
		return false
	}
	if config.RunnerSetName != "" && config.RunnerSetName != state.RunnerSetName {
		return false
	}
	if config.WorkflowFilter != "" && state.WorkflowName != "" && state.WorkflowName != config.WorkflowFilter {
		return false
	}
	if config.BranchFilter != "" && state.HeadBranch != "" && state.HeadBranch != config.BranchFilter {
		return false
	}
	return true
}

// cleanupPlaceholderIfOrphan removes a placeholder summary if no runs reference it
func (p *RunnerStateProcessor) cleanupPlaceholderIfOrphan(
	ctx context.Context,
	summaryID int64,
	runFacade database.GithubWorkflowRunFacadeInterface,
	summaryFacade *database.GithubWorkflowRunSummaryFacade,
) {
	summary, err := summaryFacade.GetByID(ctx, summaryID)
	if err != nil || summary == nil || !summary.IsPlaceholder {
		return
	}

	count, err := runFacade.CountByRunSummaryID(ctx, summaryID)
	if err != nil {
		log.Warnf("RunnerStateProcessor: failed to count runs for placeholder summary %d: %v", summaryID, err)
		return
	}

	if count == 0 {
		if err := summaryFacade.Delete(ctx, summaryID); err != nil {
			log.Warnf("RunnerStateProcessor: failed to delete orphan placeholder summary %d: %v", summaryID, err)
			return
		}
		log.Infof("RunnerStateProcessor: cleaned up orphan placeholder summary %d", summaryID)
	}
}
