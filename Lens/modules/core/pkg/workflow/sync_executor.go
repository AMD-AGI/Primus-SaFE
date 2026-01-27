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

var (
	// ErrInvalidTaskData is returned when task data is invalid or missing required fields
	ErrInvalidTaskData = errors.New("invalid task data")
)

const (
	// Task type for workflow sync
	TaskTypeGithubWorkflowSync = "github_workflow_sync"

	// Ext keys
	ExtKeyRunID           = "run_id"
	ExtKeySyncCount       = "sync_count"
	ExtKeyLastSyncAt      = "last_sync_at"
	ExtKeyWorkflowStatus  = "workflow_status"
	ExtKeyProgressPercent = "progress_percent"

	// Sync intervals
	DefaultSyncInterval   = 3 * time.Second
	QueuedSyncInterval    = 10 * time.Second
	ActiveStepInterval    = 2 * time.Second
	MaxSyncDuration       = 6 * time.Hour // Maximum time to sync a single workflow
)

// SyncExecutor is a long-running TaskExecutor for github_workflow_sync tasks
// It runs continuously, polling GitHub API and updating the database
type SyncExecutor struct {
	task.BaseExecutor
}

// NewSyncExecutor creates a new SyncExecutor
func NewSyncExecutor() *SyncExecutor {
	return &SyncExecutor{}
}

// GetTaskType returns the task type this executor handles
func (e *SyncExecutor) GetTaskType() string {
	return TaskTypeGithubWorkflowSync
}

// Validate validates task parameters
func (e *SyncExecutor) Validate(t *model.WorkloadTaskState) error {
	if t.Ext == nil {
		return ErrInvalidTaskData
	}
	if e.GetExtInt(t, ExtKeyRunID) == 0 {
		return ErrInvalidTaskData
	}
	return nil
}

// Execute runs the sync loop until workflow completes or context is cancelled
func (e *SyncExecutor) Execute(ctx context.Context, execCtx *task.ExecutionContext) (*task.ExecutionResult, error) {
	runID := int64(e.GetExtInt(execCtx.Task, ExtKeyRunID))
	log.Infof("SyncExecutor: starting sync loop for run %d", runID)

	startTime := time.Now()
	syncCount := 0

	// Main sync loop
	for {
		select {
		case <-ctx.Done():
			log.Infof("SyncExecutor: context cancelled for run %d after %d syncs", runID, syncCount)
			return e.buildResult(runID, syncCount, "cancelled"), nil

		default:
			// Check max duration
			if time.Since(startTime) > MaxSyncDuration {
				log.Warnf("SyncExecutor: max sync duration exceeded for run %d", runID)
				return e.buildResult(runID, syncCount, "timeout"), nil
			}

			// Perform sync
			state, err := e.syncOnce(ctx, runID)
			syncCount++

			if err != nil {
				log.Warnf("SyncExecutor: sync failed for run %d: %v", runID, err)
				// Continue trying, don't fail the task
			}

			// Update heartbeat with progress info
			if state != nil {
				execCtx.Task.Ext[ExtKeySyncCount] = syncCount
				execCtx.Task.Ext[ExtKeyLastSyncAt] = time.Now().Format(time.RFC3339)
				execCtx.Task.Ext[ExtKeyWorkflowStatus] = state.WorkflowStatus
				execCtx.Task.Ext[ExtKeyProgressPercent] = state.ProgressPercent
			}

			// Check if workflow completed
			if state != nil && state.WorkflowStatus == "completed" {
				log.Infof("SyncExecutor: workflow %d completed (conclusion: %s), stopping sync after %d syncs",
					runID, state.WorkflowConclusion, syncCount)
				return e.buildResult(runID, syncCount, "completed"), nil
			}

			// Calculate next sync interval
			interval := e.calculateInterval(state)
			
			// Sleep before next sync
			select {
			case <-ctx.Done():
				return e.buildResult(runID, syncCount, "cancelled"), nil
			case <-time.After(interval):
				// Continue to next sync
			}
		}
	}
}

// syncOnce performs a single sync from GitHub API to database
func (e *SyncExecutor) syncOnce(ctx context.Context, runID int64) (*WorkflowLiveState, error) {
	runFacade := database.GetFacade().GetGithubWorkflowRun()
	runnerSetFacade := database.GetFacade().GetGithubRunnerSet()

	// Get run info
	run, err := runFacade.GetByID(ctx, runID)
	if err != nil || run == nil {
		return nil, err
	}

	if run.GithubRunID == 0 {
		log.Debugf("SyncExecutor: run %d has no GitHub run ID, skipping", runID)
		return nil, nil
	}

	// Get runner set for GitHub credentials
	runnerSet, err := runnerSetFacade.GetByID(ctx, run.RunnerSetID)
	if err != nil || runnerSet == nil {
		return nil, err
	}

	// Get GitHub client
	githubManager := github.GetGlobalManager()
	if githubManager == nil {
		return nil, nil
	}

	client, err := githubManager.GetClientForSecret(ctx, runnerSet.Namespace, runnerSet.GithubConfigSecret)
	if err != nil {
		return nil, err
	}

	// Fetch from GitHub
	ghRun, err := client.GetWorkflowRun(ctx, runnerSet.GithubOwner, runnerSet.GithubRepo, run.GithubRunID)
	if err != nil {
		return nil, err
	}

	ghJobs, err := client.GetWorkflowRunJobs(ctx, runnerSet.GithubOwner, runnerSet.GithubRepo, run.GithubRunID)
	if err != nil {
		log.Warnf("SyncExecutor: failed to get jobs for run %d: %v", run.GithubRunID, err)
		ghJobs = nil
	}

	// Build state
	state := e.buildState(run, ghRun, ghJobs)

	// Update database
	if err := e.updateDatabase(ctx, run, state, ghJobs); err != nil {
		log.Errorf("SyncExecutor: failed to update database for run %d: %v", runID, err)
	}

	return state, nil
}

// buildState builds WorkflowLiveState from GitHub data
func (e *SyncExecutor) buildState(run *model.GithubWorkflowRuns, ghRun *github.WorkflowRunInfo, ghJobs []github.JobInfo) *WorkflowLiveState {
	state := &WorkflowLiveState{
		RunID:              run.ID,
		GithubRunID:        run.GithubRunID,
		GithubRunNumber:    ghRun.RunNumber,
		WorkflowName:       ghRun.WorkflowName,
		HeadSHA:            ghRun.HeadSHA,
		HeadBranch:         ghRun.HeadBranch,
		WorkflowStatus:     ghRun.Status,
		WorkflowConclusion: ghRun.Conclusion,
		CollectionStatus:   run.Status,
		StartedAt:          ghRun.RunStartedAt,
		LastSyncedAt:       time.Now(),
		UpdatedAt:          time.Now(),
	}

	if state.StartedAt != nil {
		state.ElapsedSeconds = int(time.Since(*state.StartedAt).Seconds())
	}

	// Process jobs
	if ghJobs != nil {
		state.Jobs = make([]*JobLiveState, len(ghJobs))
		totalSteps := 0
		completedSteps := 0

		for i, ghJob := range ghJobs {
			jobState := &JobLiveState{
				GithubJobID: ghJob.ID,
				Name:        ghJob.Name,
				Status:      ghJob.Status,
				Conclusion:  ghJob.Conclusion,
				StartedAt:   ghJob.StartedAt,
				CompletedAt: ghJob.CompletedAt,
				RunnerName:  ghJob.RunnerName,
				Steps:       make([]*StepLiveState, len(ghJob.Steps)),
			}

			if ghJob.StartedAt != nil {
				endTime := time.Now()
				if ghJob.CompletedAt != nil {
					endTime = *ghJob.CompletedAt
				}
				jobState.DurationSeconds = int(endTime.Sub(*ghJob.StartedAt).Seconds())
			}

			for j, ghStep := range ghJob.Steps {
				stepState := &StepLiveState{
					Number:     ghStep.Number,
					Name:       ghStep.Name,
					Status:     ghStep.Status,
					Conclusion: ghStep.Conclusion,
					StartedAt:  ghStep.StartedAt,
					CompletedAt: ghStep.CompletedAt,
				}

				if ghStep.StartedAt != nil {
					endTime := time.Now()
					if ghStep.CompletedAt != nil {
						endTime = *ghStep.CompletedAt
					}
					stepState.DurationSeconds = int(endTime.Sub(*ghStep.StartedAt).Seconds())
				}

				if ghStep.Status == "in_progress" {
					jobState.CurrentStepNumber = ghStep.Number
					jobState.CurrentStepName = ghStep.Name
				}

				jobState.Steps[j] = stepState
				totalSteps++

				if ghStep.Conclusion == "success" || ghStep.Conclusion == "skipped" {
					completedSteps++
				}
			}

			if ghJob.Status == "in_progress" {
				state.CurrentJobName = ghJob.Name
				if jobState.CurrentStepName != "" {
					state.CurrentStepName = jobState.CurrentStepName
				}
			}

			state.Jobs[i] = jobState
		}

		if totalSteps > 0 {
			state.ProgressPercent = (completedSteps * 100) / totalSteps
		}
	}

	return state
}

// updateDatabase updates run, jobs, and steps in database
func (e *SyncExecutor) updateDatabase(ctx context.Context, run *model.GithubWorkflowRuns, state *WorkflowLiveState, ghJobs []github.JobInfo) error {
	runFacade := database.GetFacade().GetGithubWorkflowRun()

	// Update run record with sync info
	run.CurrentJobName = state.CurrentJobName
	run.CurrentStepName = state.CurrentStepName
	run.ProgressPercent = int32(state.ProgressPercent)
	run.LastSyncedAt = state.LastSyncedAt

	// Update workflow status from GitHub API
	run.WorkflowStatus = state.WorkflowStatus
	run.WorkflowConclusion = state.WorkflowConclusion

	// Update head SHA and branch if not already set
	if run.HeadSha == "" && state.HeadSHA != "" {
		run.HeadSha = state.HeadSHA
	}
	if run.HeadBranch == "" && state.HeadBranch != "" {
		run.HeadBranch = state.HeadBranch
	}

	// Update run number if not already set
	if run.GithubRunNumber == 0 && state.GithubRunNumber != 0 {
		run.GithubRunNumber = int32(state.GithubRunNumber)
	}

	if err := runFacade.Update(ctx, run); err != nil {
		return err
	}

	// Sync jobs and steps
	if ghJobs != nil {
		jobFacade := database.NewGithubWorkflowJobFacade()
		if err := jobFacade.SyncFromGitHub(ctx, run.ID, ghJobs); err != nil {
			return err
		}
	}

	return nil
}

// calculateInterval determines the next sync interval based on state
func (e *SyncExecutor) calculateInterval(state *WorkflowLiveState) time.Duration {
	if state == nil {
		return DefaultSyncInterval
	}

	// Workflow completed - no need to sync
	if state.WorkflowStatus == "completed" {
		return 0
	}

	// Queued - slower polling
	if state.WorkflowStatus == "queued" {
		return QueuedSyncInterval
	}

	// Check for actively running steps (poll faster)
	if state.Jobs != nil {
		for _, job := range state.Jobs {
			for _, step := range job.Steps {
				if step.Status == "in_progress" && step.DurationSeconds < 10 {
					return ActiveStepInterval
				}
			}
		}
	}

	return DefaultSyncInterval
}

// buildResult creates the final execution result
func (e *SyncExecutor) buildResult(runID int64, syncCount int, reason string) *task.ExecutionResult {
	return &task.ExecutionResult{
		Success: true,
		UpdateExt: map[string]interface{}{
			ExtKeyRunID:      runID,
			ExtKeySyncCount:  syncCount,
			ExtKeyLastSyncAt: time.Now().Format(time.RFC3339),
			"finish_reason":  reason,
		},
		NewStatus: constant.TaskStatusCompleted,
	}
}

// Cancel stops the sync task
func (e *SyncExecutor) Cancel(ctx context.Context, t *model.WorkloadTaskState) error {
	runID := int64(e.GetExtInt(t, ExtKeyRunID))
	log.Infof("SyncExecutor: cancelling sync task for run %d", runID)
	return nil
}

// CreateSyncTask creates a new sync task for a workflow run
func CreateSyncTask(ctx context.Context, runID int64) error {
	taskFacade := database.NewWorkloadTaskFacade()

	taskUID := generateSyncTaskUID(runID)

	// Check if task already exists
	existing, err := taskFacade.GetTask(ctx, taskUID, TaskTypeGithubWorkflowSync)
	if err != nil {
		return err
	}
	if existing != nil && existing.Status == constant.TaskStatusRunning {
		log.Debugf("SyncExecutor: sync task already running for run %d", runID)
		return nil
	}

	taskState := &model.WorkloadTaskState{
		WorkloadUID: taskUID,
		TaskType:    TaskTypeGithubWorkflowSync,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			ExtKeyRunID: runID,
		},
	}

	return taskFacade.UpsertTask(ctx, taskState)
}

// generateSyncTaskUID generates a unique task UID for sync tasks
func generateSyncTaskUID(runID int64) string {
	return fmt.Sprintf("workflow-sync-%d", runID)
}
