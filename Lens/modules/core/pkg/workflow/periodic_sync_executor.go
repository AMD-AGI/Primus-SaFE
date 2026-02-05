// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package workflow

import (
	"context"
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
	// PeriodicSyncInterval is the interval between periodic syncs
	PeriodicSyncInterval = 5 * time.Minute

	// MaxPeriodicSyncTime is the maximum time to keep syncing a workflow
	MaxPeriodicSyncTime = 6 * time.Hour
)

// PeriodicSyncExecutor performs periodic sync every 5 minutes until workflow completes
type PeriodicSyncExecutor struct {
	task.BaseExecutor
}

// NewPeriodicSyncExecutor creates a new PeriodicSyncExecutor
func NewPeriodicSyncExecutor() *PeriodicSyncExecutor {
	return &PeriodicSyncExecutor{}
}

// GetTaskType returns the task type this executor handles
func (e *PeriodicSyncExecutor) GetTaskType() string {
	return constant.TaskTypeGithubPeriodicSync
}

// Validate validates task parameters
func (e *PeriodicSyncExecutor) Validate(t *model.WorkloadTaskState) error {
	if t.Ext == nil {
		return ErrInvalidTaskData
	}
	if e.GetExtInt(t, ExtKeyRunSummaryID) == 0 {
		return ErrInvalidTaskData
	}
	return nil
}

// Execute performs periodic sync
func (e *PeriodicSyncExecutor) Execute(ctx context.Context, execCtx *task.ExecutionContext) (*task.ExecutionResult, error) {
	runSummaryID := int64(e.GetExtInt(execCtx.Task, ExtKeyRunSummaryID))

	log.Infof("PeriodicSyncExecutor: starting periodic sync for run summary %d", runSummaryID)

	runSummaryFacade := database.NewGithubWorkflowRunSummaryFacade()

	// Get run summary
	summary, err := runSummaryFacade.GetByID(ctx, runSummaryID)
	if err != nil || summary == nil {
		return task.FailureResult("run summary not found", nil), nil
	}

	// Check if exceeded max sync time
	if time.Since(summary.CreatedAt) > MaxPeriodicSyncTime {
		log.Warnf("PeriodicSyncExecutor: exceeded max sync time for run summary %d", runSummaryID)
		return task.SuccessResult(map[string]interface{}{
			"status":  "timeout",
			"message": "exceeded max periodic sync time",
		}), nil
	}

	// Check if already completed
	if summary.Status == "completed" {
		log.Infof("PeriodicSyncExecutor: run summary %d already completed, stopping periodic sync", runSummaryID)
		return task.SuccessResult(map[string]interface{}{
			"status":     "completed",
			"conclusion": summary.Conclusion,
		}), nil
	}

	// Get GitHub client via runner set
	runFacade := database.GetFacade().GetGithubWorkflowRun()
	runs, err := runFacade.GetByRunSummaryID(ctx, runSummaryID)
	if err != nil || len(runs) == 0 {
		return task.FailureResult("no runs found for summary", nil), nil
	}

	runnerSetFacade := database.GetFacade().GetGithubRunnerSet()
	runnerSet, err := runnerSetFacade.GetByID(ctx, runs[0].RunnerSetID)
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

	// Fetch workflow run status from GitHub
	ghRun, err := client.GetWorkflowRun(ctx, summary.Owner, summary.Repo, summary.GithubRunID)
	if err != nil {
		log.Warnf("PeriodicSyncExecutor: failed to get workflow run %d: %v", summary.GithubRunID, err)
		// Schedule next sync despite error (transient failure)
		e.scheduleNextSync(ctx, runSummaryID)
		return task.SuccessResult(map[string]interface{}{
			"status": "retry",
			"error":  err.Error(),
		}), nil
	}

	// Update summary status
	summary.Status = ghRun.Status
	summary.Conclusion = ghRun.Conclusion
	summary.LastSyncedAt = time.Now()

	if err := runSummaryFacade.Update(ctx, summary); err != nil {
		log.Warnf("PeriodicSyncExecutor: failed to update run summary %d: %v", runSummaryID, err)
	}

	// Fetch jobs and update
	ghJobs, err := client.GetWorkflowRunJobs(ctx, summary.Owner, summary.Repo, summary.GithubRunID)
	if err == nil {
		// Update all related runs
		for _, run := range runs {
			run.WorkflowStatus = ghRun.Status
			run.WorkflowConclusion = ghRun.Conclusion
			run.LastSyncedAt = time.Now()
			if err := runFacade.Update(ctx, run); err != nil {
				log.Warnf("PeriodicSyncExecutor: failed to update run %d: %v", run.ID, err)
			}

			// Sync jobs
			jobFacade := database.NewGithubWorkflowJobFacade()
			if err := jobFacade.SyncFromGitHub(ctx, run.ID, ghJobs); err != nil {
				log.Warnf("PeriodicSyncExecutor: failed to sync jobs for run %d: %v", run.ID, err)
			}
		}
	}

	// If workflow not completed, schedule next sync
	if ghRun.Status != "completed" {
		e.scheduleNextSync(ctx, runSummaryID)
		log.Infof("PeriodicSyncExecutor: synced run summary %d (status: %s), scheduled next sync in %v",
			runSummaryID, ghRun.Status, PeriodicSyncInterval)
		return task.SuccessResult(map[string]interface{}{
			"status":    ghRun.Status,
			"next_sync": time.Now().Add(PeriodicSyncInterval).Format(time.RFC3339),
		}), nil
	}

	// Workflow completed - trigger data analysis if needed
	log.Infof("PeriodicSyncExecutor: workflow %d completed (conclusion: %s), stopping periodic sync",
		summary.GithubRunID, ghRun.Conclusion)

	// Trigger data analysis on workflow completion
	e.triggerDataAnalysisOnCompletion(ctx, summary, ghRun.Conclusion)

	return task.SuccessResult(map[string]interface{}{
		"status":     "completed",
		"conclusion": ghRun.Conclusion,
	}), nil
}

// scheduleNextSync schedules the next periodic sync
func (e *PeriodicSyncExecutor) scheduleNextSync(ctx context.Context, runSummaryID int64) {
	taskFacade := database.NewWorkloadTaskFacade()

	taskUID := fmt.Sprintf("periodic-sync-%d-%d", runSummaryID, time.Now().Unix())

	syncTask := &model.WorkloadTaskState{
		WorkloadUID: taskUID,
		TaskType:    constant.TaskTypeGithubPeriodicSync,
		Status:      constant.TaskStatusPending,
		ScheduledAt: time.Now().Add(PeriodicSyncInterval),
		Ext: model.ExtType{
			ExtKeyRunSummaryID: runSummaryID,
			ExtKeySyncType:     "periodic",
		},
	}

	if err := taskFacade.UpsertTask(ctx, syncTask); err != nil {
		log.Warnf("PeriodicSyncExecutor: failed to schedule next sync for run summary %d: %v", runSummaryID, err)
	}
}

// triggerDataAnalysisOnCompletion triggers data analysis when workflow completes
func (e *PeriodicSyncExecutor) triggerDataAnalysisOnCompletion(ctx context.Context, summary *model.GithubWorkflowRunSummaries, conclusion string) {
	runSummaryFacade := database.NewGithubWorkflowRunSummaryFacade()

	// Skip if already triggered
	if summary.DataAnalysisTriggered {
		return
	}

	// Mark as triggered
	if err := runSummaryFacade.UpdateAnalysisTriggered(ctx, summary.ID, "data", true); err != nil {
		log.Warnf("PeriodicSyncExecutor: failed to update data_analysis_triggered for run summary %d: %v", summary.ID, err)
		return
	}

	// Create data analysis task
	taskFacade := database.NewWorkloadTaskFacade()
	taskUID := fmt.Sprintf("data-analysis-%d-%d", summary.GithubRunID, time.Now().Unix())

	analysisTask := &model.WorkloadTaskState{
		WorkloadUID: taskUID,
		TaskType:    constant.TaskTypeGithubWorkflowAnalysis,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			"run_summary_id": summary.ID,
			"github_run_id":  summary.GithubRunID,
			"owner":          summary.Owner,
			"repo":           summary.Repo,
			"analysis_type":  "data",
			"conclusion":     conclusion,
			"head_sha":       summary.HeadSha,
			"head_branch":    summary.HeadBranch,
		},
	}

	if err := taskFacade.UpsertTask(ctx, analysisTask); err != nil {
		log.Warnf("PeriodicSyncExecutor: failed to create data analysis task: %v", err)
		return
	}

	log.Infof("PeriodicSyncExecutor: triggered data analysis task %s for run summary %d (conclusion: %s)",
		taskUID, summary.ID, conclusion)
}

// CreatePeriodicSyncTask creates a periodic sync task for a workflow run summary
func CreatePeriodicSyncTask(ctx context.Context, runSummaryID int64) error {
	taskFacade := database.NewWorkloadTaskFacade()

	taskUID := fmt.Sprintf("periodic-sync-%d-%d", runSummaryID, time.Now().Unix())

	syncTask := &model.WorkloadTaskState{
		WorkloadUID: taskUID,
		TaskType:    constant.TaskTypeGithubPeriodicSync,
		Status:      constant.TaskStatusPending,
		ScheduledAt: time.Now().Add(PeriodicSyncInterval),
		Ext: model.ExtType{
			ExtKeyRunSummaryID: runSummaryID,
			ExtKeySyncType:     "periodic",
		},
	}

	return taskFacade.UpsertTask(ctx, syncTask)
}

// CreateInitialSyncTask creates an initial sync task for a workflow run
func CreateInitialSyncTask(ctx context.Context, runID int64, fetchWorkflow, fetchCommit bool) error {
	taskFacade := database.NewWorkloadTaskFacade()

	taskUID := fmt.Sprintf("initial-sync-%d-%d", runID, time.Now().Unix())

	syncTask := &model.WorkloadTaskState{
		WorkloadUID: taskUID,
		TaskType:    constant.TaskTypeGithubInitialSync,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			ExtKeyRunID:         runID,
			ExtKeySyncType:      "initial",
			ExtKeyFetchWorkflow: fetchWorkflow,
			ExtKeyFetchCommit:   fetchCommit,
		},
	}

	return taskFacade.UpsertTask(ctx, syncTask)
}

// CreateCompletionSyncTask creates a completion sync task for a workflow run
func CreateCompletionSyncTask(ctx context.Context, runID int64, fetchJobs, fetchLogs bool) error {
	taskFacade := database.NewWorkloadTaskFacade()

	taskUID := fmt.Sprintf("completion-sync-%d-%d", runID, time.Now().Unix())

	syncTask := &model.WorkloadTaskState{
		WorkloadUID: taskUID,
		TaskType:    constant.TaskTypeGithubCompletionSync,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			ExtKeyRunID:     runID,
			ExtKeySyncType:  "completion",
			ExtKeyFetchJobs: fetchJobs,
			ExtKeyFetchLogs: fetchLogs,
		},
	}

	return taskFacade.UpsertTask(ctx, syncTask)
}
