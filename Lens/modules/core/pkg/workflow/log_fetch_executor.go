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
	// Task type for log fetching
	TaskTypeGithubWorkflowLogFetch = "github_workflow_log_fetch"

	// Ext keys for log fetch task
	ExtKeyLogFetchRunID      = "run_id"
	ExtKeyLogFetchJobCount   = "job_count"
	ExtKeyLogFetchedCount    = "fetched_count"
	ExtKeyLogFetchFailedCount = "failed_count"
)

// LogFetchExecutor fetches and stores job logs after workflow completion
type LogFetchExecutor struct {
	task.BaseExecutor
}

// NewLogFetchExecutor creates a new LogFetchExecutor
func NewLogFetchExecutor() *LogFetchExecutor {
	return &LogFetchExecutor{}
}

// GetTaskType returns the task type this executor handles
func (e *LogFetchExecutor) GetTaskType() string {
	return TaskTypeGithubWorkflowLogFetch
}

// Validate validates task parameters
func (e *LogFetchExecutor) Validate(t *model.WorkloadTaskState) error {
	if t.Ext == nil {
		return ErrInvalidTaskData
	}
	if e.GetExtInt(t, ExtKeyLogFetchRunID) == 0 {
		return ErrInvalidTaskData
	}
	return nil
}

// Execute fetches logs for all jobs in a workflow run
func (e *LogFetchExecutor) Execute(ctx context.Context, execCtx *task.ExecutionContext) (*task.ExecutionResult, error) {
	runID := int64(e.GetExtInt(execCtx.Task, ExtKeyLogFetchRunID))
	log.Infof("LogFetchExecutor: starting log fetch for run %d", runID)

	runFacade := database.GetFacade().GetGithubWorkflowRun()
	runnerSetFacade := database.GetFacade().GetGithubRunnerSet()
	logsFacade := database.NewGithubWorkflowJobLogsFacade()

	// Get run info
	run, err := runFacade.GetByID(ctx, runID)
	if err != nil || run == nil {
		return e.buildResult(runID, 0, 0, 0, "run_not_found"), nil
	}

	// Get runner set for GitHub credentials
	runnerSet, err := runnerSetFacade.GetByID(ctx, run.RunnerSetID)
	if err != nil || runnerSet == nil {
		return e.buildResult(runID, 0, 0, 0, "runner_set_not_found"), nil
	}

	// Get GitHub client
	githubManager := github.GetGlobalManager()
	if githubManager == nil {
		return e.buildResult(runID, 0, 0, 0, "github_manager_nil"), nil
	}

	client, err := githubManager.GetClientForSecret(ctx, runnerSet.Namespace, runnerSet.GithubConfigSecret)
	if err != nil {
		log.Errorf("LogFetchExecutor: failed to get GitHub client: %v", err)
		return e.buildResult(runID, 0, 0, 0, "github_client_error"), nil
	}

	// Get pending log entries for this run
	pendingLogs, err := logsFacade.GetByRunID(ctx, runID)
	if err != nil {
		log.Errorf("LogFetchExecutor: failed to get pending logs: %v", err)
		return e.buildResult(runID, 0, 0, 0, "db_error"), nil
	}

	jobCount := len(pendingLogs)
	fetchedCount := 0
	failedCount := 0

	for _, logEntry := range pendingLogs {
		// Skip already fetched logs
		if logEntry.FetchStatus == model.LogFetchStatusFetched {
			fetchedCount++
			continue
		}

		// Fetch logs from GitHub
		logs, err := client.GetJobLogs(ctx, runnerSet.GithubOwner, runnerSet.GithubRepo, logEntry.GithubJobID)
		if err != nil {
			log.Warnf("LogFetchExecutor: failed to fetch logs for job %d: %v", logEntry.GithubJobID, err)
			if updateErr := logsFacade.UpdateFetchError(ctx, runID, logEntry.GithubJobID, err.Error()); updateErr != nil {
				log.Errorf("LogFetchExecutor: failed to update fetch error: %v", updateErr)
			}
			failedCount++
			continue
		}

		// Store logs
		if err := logsFacade.UpdateLogsContent(ctx, runID, logEntry.GithubJobID, logs); err != nil {
			log.Errorf("LogFetchExecutor: failed to store logs for job %d: %v", logEntry.GithubJobID, err)
			failedCount++
			continue
		}

		fetchedCount++
		log.Infof("LogFetchExecutor: fetched logs for job %d (%d bytes)", logEntry.GithubJobID, len(logs))

		// Update progress
		execCtx.Task.Ext[ExtKeyLogFetchedCount] = fetchedCount
		execCtx.Task.Ext[ExtKeyLogFetchFailedCount] = failedCount
	}

	log.Infof("LogFetchExecutor: completed log fetch for run %d (%d/%d fetched, %d failed)",
		runID, fetchedCount, jobCount, failedCount)

	return e.buildResult(runID, jobCount, fetchedCount, failedCount, "completed"), nil
}

// Cancel cancels the log fetch task
func (e *LogFetchExecutor) Cancel(ctx context.Context, t *model.WorkloadTaskState) error {
	runID := int64(e.GetExtInt(t, ExtKeyLogFetchRunID))
	log.Infof("LogFetchExecutor: cancelling log fetch task for run %d", runID)
	return nil
}

// buildResult creates the final execution result
func (e *LogFetchExecutor) buildResult(runID int64, jobCount, fetchedCount, failedCount int, reason string) *task.ExecutionResult {
	return &task.ExecutionResult{
		Success: true,
		UpdateExt: map[string]interface{}{
			ExtKeyLogFetchRunID:       runID,
			ExtKeyLogFetchJobCount:    jobCount,
			ExtKeyLogFetchedCount:     fetchedCount,
			ExtKeyLogFetchFailedCount: failedCount,
			"finish_reason":           reason,
		},
		NewStatus: constant.TaskStatusCompleted,
	}
}

// CreateLogFetchTask creates a new log fetch task for a workflow run
func CreateLogFetchTask(ctx context.Context, runID int64) error {
	taskFacade := database.NewWorkloadTaskFacade()

	taskUID := fmt.Sprintf("workflow-log-fetch-%d", runID)

	// Check if task already exists
	existing, err := taskFacade.GetTask(ctx, taskUID, TaskTypeGithubWorkflowLogFetch)
	if err != nil {
		return err
	}
	if existing != nil {
		log.Debugf("LogFetchExecutor: log fetch task already exists for run %d", runID)
		return nil
	}

	taskState := &model.WorkloadTaskState{
		WorkloadUID: taskUID,
		TaskType:    TaskTypeGithubWorkflowLogFetch,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			ExtKeyLogFetchRunID: runID,
		},
		CreatedAt: time.Now(),
	}

	return taskFacade.UpsertTask(ctx, taskState)
}
