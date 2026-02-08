// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/github"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
)

const (
	// Ext keys for oneshot sync
	ExtKeySyncType      = "sync_type"
	ExtKeyFetchWorkflow = "fetch_workflow"
	ExtKeyFetchCommit   = "fetch_commit"
	ExtKeyFetchJobs     = "fetch_jobs"
	ExtKeyFetchLogs     = "fetch_logs"
	ExtKeyRunSummaryID  = "run_summary_id"
)

// InitialSyncExecutor performs one-shot sync when EphemeralRunner is created
type InitialSyncExecutor struct {
	task.BaseExecutor
}

// NewInitialSyncExecutor creates a new InitialSyncExecutor
func NewInitialSyncExecutor() *InitialSyncExecutor {
	return &InitialSyncExecutor{}
}

// GetTaskType returns the task type this executor handles
func (e *InitialSyncExecutor) GetTaskType() string {
	return constant.TaskTypeGithubInitialSync
}

// Validate validates task parameters
func (e *InitialSyncExecutor) Validate(t *model.WorkloadTaskState) error {
	if t.Ext == nil {
		return ErrInvalidTaskData
	}
	if e.GetExtInt(t, ExtKeyRunID) == 0 {
		return ErrInvalidTaskData
	}
	return nil
}

// Cancel cancels the task (no-op for one-shot sync)
func (e *InitialSyncExecutor) Cancel(ctx context.Context, t *model.WorkloadTaskState) error {
	return nil
}

// Execute performs one-shot initial sync
func (e *InitialSyncExecutor) Execute(ctx context.Context, execCtx *task.ExecutionContext) (*task.ExecutionResult, error) {
	runID := int64(e.GetExtInt(execCtx.Task, ExtKeyRunID))
	fetchWorkflow := e.GetExtBool(execCtx.Task, ExtKeyFetchWorkflow)
	fetchCommit := e.GetExtBool(execCtx.Task, ExtKeyFetchCommit)

	log.Infof("InitialSyncExecutor: starting initial sync for run %d", runID)

	runFacade := database.GetFacade().GetGithubWorkflowRun()
	runnerSetFacade := database.GetFacade().GetGithubRunnerSet()

	// Get run info
	run, err := runFacade.GetByID(ctx, runID)
	if err != nil || run == nil {
		return task.FailureResult("run not found", nil), nil
	}

	if run.GithubRunID == 0 {
		log.Debugf("InitialSyncExecutor: run %d has no GitHub run ID, skipping", runID)
		return task.SuccessResult(map[string]interface{}{
			"status": "skipped",
			"reason": "no github_run_id",
		}), nil
	}

	// Get runner set for GitHub credentials
	runnerSet, err := runnerSetFacade.GetByID(ctx, run.RunnerSetID)
	if err != nil || runnerSet == nil {
		return task.FailureResult("runner set not found", nil), nil
	}

	// Get GitHub client
	githubManager := github.GetGlobalManager()
	if githubManager == nil {
		return task.FailureResult("github manager not initialized", nil), nil
	}

	client, err := githubManager.GetClientForSecret(ctx, runnerSet.Namespace, runnerSet.GithubConfigSecret)
	if err != nil {
		return task.FailureResult(fmt.Sprintf("failed to get github client: %v", err), nil), nil
	}

	// 1. Fetch workflow run info
	ghRun, err := client.GetWorkflowRun(ctx, runnerSet.GithubOwner, runnerSet.GithubRepo, run.GithubRunID)
	if err != nil {
		return task.FailureResult(fmt.Sprintf("failed to get workflow run: %v", err), nil), nil
	}

	// 2. Update run record with sync fields only (avoid overwriting collection status)
	headSha := ""
	if run.HeadSha == "" {
		headSha = ghRun.HeadSHA
	}
	headBranch := ""
	if run.HeadBranch == "" {
		headBranch = ghRun.HeadBranch
	}
	var githubRunNumber int32
	if run.GithubRunNumber == 0 {
		githubRunNumber = int32(ghRun.RunNumber)
	}
	workflowName := ""
	if run.WorkflowName == "" {
		workflowName = ghRun.WorkflowName
	}

	if err := runFacade.UpdateSyncFields(ctx, runID,
		ghRun.Status, ghRun.Conclusion,
		headSha, headBranch, workflowName, githubRunNumber,
	); err != nil {
		log.Warnf("InitialSyncExecutor: failed to update run %d: %v", runID, err)
	}

	// 3. Fetch workflow file content if requested (store in run details)
	if fetchWorkflow && ghRun.WorkflowPath != "" {
		content, err := client.GetWorkflowFileContent(ctx, runnerSet.GithubOwner, runnerSet.GithubRepo, ghRun.WorkflowPath, ghRun.HeadSHA)
		if err != nil {
			log.Warnf("InitialSyncExecutor: failed to fetch workflow content for run %d: %v", runID, err)
		} else if content != "" {
			log.Debugf("InitialSyncExecutor: fetched workflow content for run %d (%d bytes)", runID, len(content))
		}
	}

	// 4. Fetch commit info if requested (for logging/debugging)
	if fetchCommit && ghRun.HeadSHA != "" {
		commitInfo, err := client.GetCommit(ctx, runnerSet.GithubOwner, runnerSet.GithubRepo, ghRun.HeadSHA)
		if err != nil {
			log.Warnf("InitialSyncExecutor: failed to fetch commit info for run %d: %v", runID, err)
		} else if commitInfo != nil {
			log.Debugf("InitialSyncExecutor: fetched commit info for run %d: %s", runID, commitInfo.Message)
		}
	}

	log.Infof("InitialSyncExecutor: completed initial sync for run %d (github_run: %d)", runID, run.GithubRunID)

	return task.SuccessResult(map[string]interface{}{
		"synced_at": time.Now().Format(time.RFC3339),
		"sync_type": "initial",
		"run_id":    runID,
	}), nil
}

// CompletionSyncExecutor performs one-shot sync when EphemeralRunner completes
type CompletionSyncExecutor struct {
	task.BaseExecutor
}

// NewCompletionSyncExecutor creates a new CompletionSyncExecutor
func NewCompletionSyncExecutor() *CompletionSyncExecutor {
	return &CompletionSyncExecutor{}
}

// GetTaskType returns the task type this executor handles
func (e *CompletionSyncExecutor) GetTaskType() string {
	return constant.TaskTypeGithubCompletionSync
}

// Validate validates task parameters
func (e *CompletionSyncExecutor) Validate(t *model.WorkloadTaskState) error {
	if t.Ext == nil {
		return ErrInvalidTaskData
	}
	if e.GetExtInt(t, ExtKeyRunID) == 0 {
		return ErrInvalidTaskData
	}
	return nil
}

// Cancel cancels the task (no-op for one-shot sync)
func (e *CompletionSyncExecutor) Cancel(ctx context.Context, t *model.WorkloadTaskState) error {
	return nil
}

// Execute performs one-shot completion sync
func (e *CompletionSyncExecutor) Execute(ctx context.Context, execCtx *task.ExecutionContext) (*task.ExecutionResult, error) {
	runID := int64(e.GetExtInt(execCtx.Task, ExtKeyRunID))
	fetchJobs := e.GetExtBool(execCtx.Task, ExtKeyFetchJobs)
	fetchLogs := e.GetExtBool(execCtx.Task, ExtKeyFetchLogs)

	log.Infof("CompletionSyncExecutor: starting completion sync for run %d", runID)

	runFacade := database.GetFacade().GetGithubWorkflowRun()
	runnerSetFacade := database.GetFacade().GetGithubRunnerSet()

	// Get run info
	run, err := runFacade.GetByID(ctx, runID)
	if err != nil || run == nil {
		return task.FailureResult("run not found", nil), nil
	}

	if run.GithubRunID == 0 {
		log.Debugf("CompletionSyncExecutor: run %d has no GitHub run ID, skipping", runID)
		return task.SuccessResult(map[string]interface{}{
			"status": "skipped",
			"reason": "no github_run_id",
		}), nil
	}

	// Get runner set for GitHub credentials
	runnerSet, err := runnerSetFacade.GetByID(ctx, run.RunnerSetID)
	if err != nil || runnerSet == nil {
		return task.FailureResult("runner set not found", nil), nil
	}

	// Get GitHub client
	githubManager := github.GetGlobalManager()
	if githubManager == nil {
		return task.FailureResult("github manager not initialized", nil), nil
	}

	client, err := githubManager.GetClientForSecret(ctx, runnerSet.Namespace, runnerSet.GithubConfigSecret)
	if err != nil {
		return task.FailureResult(fmt.Sprintf("failed to get github client: %v", err), nil), nil
	}

	// 1. Fetch workflow run with jobs
	ghRun, err := client.GetWorkflowRun(ctx, runnerSet.GithubOwner, runnerSet.GithubRepo, run.GithubRunID)
	if err != nil {
		return task.FailureResult(fmt.Sprintf("failed to get workflow run: %v", err), nil), nil
	}

	// 2. Update run sync fields only (avoid overwriting collection status set by CollectionExecutor)
	if err := runFacade.UpdateSyncFields(ctx, runID,
		ghRun.Status, ghRun.Conclusion,
		ghRun.HeadSHA, ghRun.HeadBranch, ghRun.WorkflowName, int32(ghRun.RunNumber),
	); err != nil {
		log.Warnf("CompletionSyncExecutor: failed to update run %d: %v", runID, err)
	}

	// 3. Fetch and sync jobs
	var jobCount int
	if fetchJobs {
		ghJobs, err := client.GetWorkflowRunJobs(ctx, runnerSet.GithubOwner, runnerSet.GithubRepo, run.GithubRunID)
		if err != nil {
			log.Warnf("CompletionSyncExecutor: failed to get jobs for run %d: %v", run.GithubRunID, err)
		} else {
			jobCount = len(ghJobs)
			// Sync jobs to the first run under this summary (one canonical copy).
			// github_workflow_jobs are per-workflow-run, not per-runner.
			targetRunID := run.ID
			if run.RunSummaryID > 0 {
				summaryRuns, listErr := runFacade.ListByRunSummaryID(ctx, run.RunSummaryID)
				if listErr == nil && len(summaryRuns) > 0 {
					targetRunID = summaryRuns[0].ID
				}
			}
			jobFacade := database.NewGithubWorkflowJobFacade()
			if err := jobFacade.SyncFromGitHub(ctx, targetRunID, ghJobs); err != nil {
				log.Warnf("CompletionSyncExecutor: failed to sync jobs for run %d: %v", targetRunID, err)
			}

			// 4. Fetch logs for completed jobs
			if fetchLogs {
				logsFacade := database.NewGithubWorkflowJobLogsFacade()
				for _, job := range ghJobs {
					if job.Status != "completed" {
						continue
					}

					// Check if we already have logs
					existingLog, _ := logsFacade.GetByJobID(ctx, run.ID, job.ID)
					if existingLog != nil {
						continue
					}

					// Create pending log entry
					if err := logsFacade.CreatePendingLogs(ctx, run.ID, job.ID, job.Name); err != nil {
						log.Warnf("CompletionSyncExecutor: failed to create pending log for job %d: %v", job.ID, err)
					}
				}

				// Trigger log fetch task
				if err := CreateLogFetchTask(ctx, run.ID); err != nil {
					log.Warnf("CompletionSyncExecutor: failed to create log fetch task for run %d: %v", runID, err)
				}
			}

			// Check for failures and trigger failure analysis
			e.checkAndTriggerFailureAnalysis(ctx, run, ghJobs)
		}
	}

	// 5. Backfill run summary GitHub info if available
	if run.RunSummaryID > 0 {
		runSummaryFacade := database.GetFacade().GetGithubWorkflowRunSummary()
		summary, summaryErr := runSummaryFacade.GetByID(ctx, run.RunSummaryID)
		if summaryErr == nil && summary != nil {
			updated := false
			if summary.WorkflowName == "" && ghRun.WorkflowName != "" {
				summary.WorkflowName = ghRun.WorkflowName
				updated = true
			}
			if summary.HeadSha == "" && ghRun.HeadSHA != "" {
				summary.HeadSha = ghRun.HeadSHA
				updated = true
			}
			if summary.HeadBranch == "" && ghRun.HeadBranch != "" {
				summary.HeadBranch = ghRun.HeadBranch
				updated = true
			}
			if summary.GithubRunNumber == 0 && ghRun.RunNumber > 0 {
				summary.GithubRunNumber = int32(ghRun.RunNumber)
				updated = true
			}
			// Backfill actor fields
			if summary.Actor == "" {
				if ghRun.Actor != nil && ghRun.Actor.Login != "" {
					summary.Actor = ghRun.Actor.Login
					updated = true
				} else if ghRun.TriggerActor != nil && ghRun.TriggerActor.Login != "" {
					summary.Actor = ghRun.TriggerActor.Login
					updated = true
				}
			}
			if summary.TriggeringActor == "" {
				if ghRun.TriggerActor != nil && ghRun.TriggerActor.Login != "" {
					summary.TriggeringActor = ghRun.TriggerActor.Login
					updated = true
				} else if ghRun.Actor != nil && ghRun.Actor.Login != "" {
					summary.TriggeringActor = ghRun.Actor.Login
					updated = true
				}
			}
			if summary.EventName == "" && ghRun.Event != "" {
				summary.EventName = ghRun.Event
				updated = true
			}
			// Backfill run timing from GitHub
			if summary.RunStartedAt.IsZero() && ghRun.RunStartedAt != nil {
				summary.RunStartedAt = *ghRun.RunStartedAt
				updated = true
			}
			if summary.RunCompletedAt.IsZero() && ghRun.RunCompletedAt != nil {
				summary.RunCompletedAt = *ghRun.RunCompletedAt
				updated = true
			}
			if updated {
				if err := runSummaryFacade.Update(ctx, summary); err != nil {
					log.Warnf("CompletionSyncExecutor: failed to backfill summary %d: %v", run.RunSummaryID, err)
				}
			}
		}
	}

	// 6. Schedule periodic sync if workflow not yet completed
	if ghRun.Status != "completed" && run.RunSummaryID > 0 {
		if err := CreatePeriodicSyncTask(ctx, run.RunSummaryID); err != nil {
			log.Warnf("CompletionSyncExecutor: failed to schedule periodic sync for run summary %d: %v", run.RunSummaryID, err)
		} else {
			log.Infof("CompletionSyncExecutor: scheduled periodic sync for run summary %d", run.RunSummaryID)
		}
	}

	log.Infof("CompletionSyncExecutor: completed sync for run %d (jobs: %d, workflow_status: %s)",
		runID, jobCount, ghRun.Status)

	return task.SuccessResult(map[string]interface{}{
		"synced_at":       time.Now().Format(time.RFC3339),
		"sync_type":       "completion",
		"run_id":          runID,
		"job_count":       jobCount,
		"workflow_status": ghRun.Status,
	}), nil
}

// checkAndTriggerFailureAnalysis checks for job failures and triggers analysis
func (e *CompletionSyncExecutor) checkAndTriggerFailureAnalysis(ctx context.Context, run *model.GithubWorkflowRuns, ghJobs []github.JobInfo) {
	if ghJobs == nil {
		return
	}

	hasFailure := false
	for _, job := range ghJobs {
		if job.Conclusion == "failure" {
			hasFailure = true
			break
		}
	}

	if !hasFailure || run.RunSummaryID == 0 {
		return
	}

	runSummaryFacade := database.GetFacade().GetGithubWorkflowRunSummary()
	summary, err := runSummaryFacade.GetByID(ctx, run.RunSummaryID)
	if err != nil || summary == nil || summary.FailureAnalysisTriggered {
		return
	}

	// Mark as triggered
	if err := runSummaryFacade.UpdateAnalysisTriggered(ctx, summary.ID, "failure", true); err != nil {
		log.Warnf("CompletionSyncExecutor: failed to update failure_analysis_triggered for run summary %d: %v", summary.ID, err)
		return
	}

	// Create failure analysis task
	taskFacade := database.NewWorkloadTaskFacade()
	taskUID := fmt.Sprintf("failure-analysis-%d-%d", summary.GithubRunID, time.Now().Unix())

	analysisTask := &model.WorkloadTaskState{
		WorkloadUID: taskUID,
		TaskType:    constant.TaskTypeGithubWorkflowAnalysis,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			"run_summary_id": summary.ID,
			"github_run_id":  summary.GithubRunID,
			"owner":          summary.Owner,
			"repo":           summary.Repo,
			"analysis_type":  "failure",
			"head_sha":       summary.HeadSha,
			"head_branch":    summary.HeadBranch,
			"workflow_name":  summary.WorkflowName,
			"repo_name":      summary.Owner + "/" + summary.Repo,
		},
	}

	if err := taskFacade.UpsertTask(ctx, analysisTask); err != nil {
		log.Warnf("CompletionSyncExecutor: failed to create failure analysis task: %v", err)
		return
	}

	log.Infof("CompletionSyncExecutor: triggered failure analysis task %s for run summary %d", taskUID, summary.ID)
}

// ManualSyncExecutor handles manual sync requests from UI
type ManualSyncExecutor struct {
	task.BaseExecutor
}

// NewManualSyncExecutor creates a new ManualSyncExecutor
func NewManualSyncExecutor() *ManualSyncExecutor {
	return &ManualSyncExecutor{}
}

// GetTaskType returns the task type this executor handles
func (e *ManualSyncExecutor) GetTaskType() string {
	return constant.TaskTypeGithubManualSync
}

// Validate validates task parameters
func (e *ManualSyncExecutor) Validate(t *model.WorkloadTaskState) error {
	if t.Ext == nil {
		return ErrInvalidTaskData
	}
	runID := e.GetExtInt(t, ExtKeyRunID)
	runSummaryID := e.GetExtInt(t, ExtKeyRunSummaryID)
	if runID == 0 && runSummaryID == 0 {
		return errors.New("either run_id or run_summary_id is required")
	}
	return nil
}

// Cancel cancels the task (no-op for manual sync)
func (e *ManualSyncExecutor) Cancel(ctx context.Context, t *model.WorkloadTaskState) error {
	return nil
}

// Execute performs manual sync
func (e *ManualSyncExecutor) Execute(ctx context.Context, execCtx *task.ExecutionContext) (*task.ExecutionResult, error) {
	runID := int64(e.GetExtInt(execCtx.Task, ExtKeyRunID))
	runSummaryID := int64(e.GetExtInt(execCtx.Task, ExtKeyRunSummaryID))

	log.Infof("ManualSyncExecutor: starting manual sync (run_id: %d, run_summary_id: %d)", runID, runSummaryID)

	// If we have run_summary_id, sync the entire workflow run
	if runSummaryID > 0 {
		return e.syncRunSummary(ctx, runSummaryID)
	}

	// Otherwise sync a single job run
	return e.syncSingleRun(ctx, runID)
}

func (e *ManualSyncExecutor) syncRunSummary(ctx context.Context, runSummaryID int64) (*task.ExecutionResult, error) {
	runSummaryFacade := database.GetFacade().GetGithubWorkflowRunSummary()
	summary, err := runSummaryFacade.GetByID(ctx, runSummaryID)
	if err != nil || summary == nil {
		return task.FailureResult("run summary not found", nil), nil
	}

	// Get GitHub client
	githubManager := github.GetGlobalManager()
	if githubManager == nil {
		return task.FailureResult("github manager not initialized", nil), nil
	}

	// Find runner set to get credentials using PrimaryRunnerSetID from summary
	runnerSetFacade := database.GetFacade().GetGithubRunnerSet()
	runnerSet, err := runnerSetFacade.GetByID(ctx, summary.PrimaryRunnerSetID)
	if err != nil || runnerSet == nil {
		return task.FailureResult("runner set not found", nil), nil
	}

	client, err := githubManager.GetClientForSecret(ctx, runnerSet.Namespace, runnerSet.GithubConfigSecret)
	if err != nil {
		return task.FailureResult(fmt.Sprintf("failed to get github client: %v", err), nil), nil
	}

	// Fetch workflow run status
	ghRun, err := client.GetWorkflowRun(ctx, summary.Owner, summary.Repo, summary.GithubRunID)
	if err != nil {
		return task.FailureResult(fmt.Sprintf("failed to get workflow run: %v", err), nil), nil
	}

	// Update summary
	summary.Status = ghRun.Status
	summary.Conclusion = ghRun.Conclusion
	summary.LastSyncedAt = time.Now()
	// Backfill run timing from GitHub
	if summary.RunStartedAt.IsZero() && ghRun.RunStartedAt != nil {
		summary.RunStartedAt = *ghRun.RunStartedAt
	}
	if summary.RunCompletedAt.IsZero() && ghRun.RunCompletedAt != nil {
		summary.RunCompletedAt = *ghRun.RunCompletedAt
	}

	if err := runSummaryFacade.Update(ctx, summary); err != nil {
		log.Warnf("ManualSyncExecutor: failed to update summary %d: %v", runSummaryID, err)
	}

	// Fetch jobs from GitHub and sync to database
	ghJobs, err := client.GetWorkflowRunJobs(ctx, summary.Owner, summary.Repo, summary.GithubRunID)
	if err != nil {
		log.Warnf("ManualSyncExecutor: failed to get jobs: %v", err)
	} else {
		// Match GitHub jobs to runner records and update workflow_status
		syncWorkflowStatusFromJobs(ctx, runSummaryID, ghJobs)

		// Sync job records to the first run under this summary (one canonical copy).
		// github_workflow_jobs are per-workflow-run, not per-runner, so we anchor
		// them to a single run_id to avoid duplicates.
		runFacade := database.GetFacade().GetGithubWorkflowRun()
		runs, listErr := runFacade.ListByRunSummaryID(ctx, runSummaryID)
		if listErr != nil {
			log.Warnf("ManualSyncExecutor: failed to list runs for summary %d: %v", runSummaryID, listErr)
		} else if len(runs) > 0 {
			jobFacade := database.NewGithubWorkflowJobFacade()
			targetRunID := runs[0].ID
			if syncErr := jobFacade.SyncFromGitHub(ctx, targetRunID, ghJobs); syncErr != nil {
				log.Warnf("ManualSyncExecutor: failed to sync jobs for run %d (summary %d): %v", targetRunID, runSummaryID, syncErr)
			}
			log.Infof("ManualSyncExecutor: synced %d jobs to run %d for summary %d", len(ghJobs), targetRunID, runSummaryID)
		}

		// Update job stats in summary (queries from DB)
		if err := runSummaryFacade.UpdateJobStats(ctx, summary.ID); err != nil {
			log.Warnf("ManualSyncExecutor: failed to update job stats for summary %d: %v", summary.ID, err)
		}
	}

	// Re-schedule periodic sync if workflow is not yet completed
	// (in case the periodic sync chain was broken by a restart)
	if ghRun.Status != "completed" {
		if err := CreatePeriodicSyncTask(ctx, runSummaryID); err != nil {
			log.Warnf("ManualSyncExecutor: failed to re-schedule periodic sync for summary %d: %v", runSummaryID, err)
		} else {
			log.Infof("ManualSyncExecutor: re-scheduled periodic sync for summary %d", runSummaryID)
		}
	}

	log.Infof("ManualSyncExecutor: completed manual sync for run summary %d (status: %s, jobs: %d)", runSummaryID, ghRun.Status, len(ghJobs))

	return task.SuccessResult(map[string]interface{}{
		"synced_at":       time.Now().Format(time.RFC3339),
		"sync_type":       "manual",
		"run_summary_id":  runSummaryID,
		"workflow_status": ghRun.Status,
	}), nil
}

func (e *ManualSyncExecutor) syncSingleRun(ctx context.Context, runID int64) (*task.ExecutionResult, error) {
	runFacade := database.GetFacade().GetGithubWorkflowRun()
	runnerSetFacade := database.GetFacade().GetGithubRunnerSet()

	run, err := runFacade.GetByID(ctx, runID)
	if err != nil || run == nil {
		return task.FailureResult("run not found", nil), nil
	}

	if run.GithubRunID == 0 {
		return task.SuccessResult(map[string]interface{}{
			"status": "skipped",
			"reason": "no github_run_id",
		}), nil
	}

	runnerSet, err := runnerSetFacade.GetByID(ctx, run.RunnerSetID)
	if err != nil || runnerSet == nil {
		return task.FailureResult("runner set not found", nil), nil
	}

	githubManager := github.GetGlobalManager()
	if githubManager == nil {
		return task.FailureResult("github manager not initialized", nil), nil
	}

	client, err := githubManager.GetClientForSecret(ctx, runnerSet.Namespace, runnerSet.GithubConfigSecret)
	if err != nil {
		return task.FailureResult(fmt.Sprintf("failed to get github client: %v", err), nil), nil
	}

	// Fetch and update
	ghRun, err := client.GetWorkflowRun(ctx, runnerSet.GithubOwner, runnerSet.GithubRepo, run.GithubRunID)
	if err != nil {
		return task.FailureResult(fmt.Sprintf("failed to get workflow run: %v", err), nil), nil
	}

	run.WorkflowStatus = ghRun.Status
	run.WorkflowConclusion = ghRun.Conclusion
	run.LastSyncedAt = time.Now()

	if err := runFacade.Update(ctx, run); err != nil {
		log.Warnf("ManualSyncExecutor: failed to update run %d: %v", runID, err)
	}

	// Sync jobs to the first run under this summary (one canonical copy)
	ghJobs, err := client.GetWorkflowRunJobs(ctx, runnerSet.GithubOwner, runnerSet.GithubRepo, run.GithubRunID)
	if err == nil {
		targetRunID := run.ID
		if run.RunSummaryID > 0 {
			summaryRuns, listErr := runFacade.ListByRunSummaryID(ctx, run.RunSummaryID)
			if listErr == nil && len(summaryRuns) > 0 {
				targetRunID = summaryRuns[0].ID
			}
		}
		jobFacade := database.NewGithubWorkflowJobFacade()
		if err := jobFacade.SyncFromGitHub(ctx, targetRunID, ghJobs); err != nil {
			log.Warnf("ManualSyncExecutor: failed to sync jobs for run %d: %v", targetRunID, err)
		}
	}

	log.Infof("ManualSyncExecutor: completed manual sync for run %d (status: %s)", runID, ghRun.Status)

	return task.SuccessResult(map[string]interface{}{
		"synced_at":       time.Now().Format(time.RFC3339),
		"sync_type":       "manual",
		"run_id":          runID,
		"workflow_status": ghRun.Status,
	}), nil
}

// CreateManualSyncTask creates a manual sync task for a workflow run summary.
// clusterName is optional; when empty, defaults to the exporter's local cluster.
func CreateManualSyncTask(ctx context.Context, runSummaryID int64, clusterName ...string) error {
	var taskFacade *database.WorkloadTaskFacade
	if len(clusterName) > 0 && clusterName[0] != "" {
		taskFacade = database.NewWorkloadTaskFacadeForCluster(clusterName[0])
	} else {
		taskFacade = database.NewWorkloadTaskFacade()
	}

	taskUID := fmt.Sprintf("manual-sync-%d-%d", runSummaryID, time.Now().Unix())

	syncTask := &model.WorkloadTaskState{
		WorkloadUID: taskUID,
		TaskType:    constant.TaskTypeGithubManualSync,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			ExtKeyRunSummaryID: runSummaryID,
			ExtKeySyncType:     "manual",
		},
	}

	return taskFacade.UpsertTask(ctx, syncTask)
}
