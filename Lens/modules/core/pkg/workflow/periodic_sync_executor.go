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

	// SlowPeriodicSyncInterval is the interval for long-running workflows (after MaxPeriodicSyncTime)
	SlowPeriodicSyncInterval = 30 * time.Minute

	// MaxPeriodicSyncTime is the threshold after which syncing switches to slow mode
	MaxPeriodicSyncTime = 6 * time.Hour

	// AbsoluteMaxSyncTime is the absolute maximum time to keep syncing a workflow (7 days)
	AbsoluteMaxSyncTime = 7 * 24 * time.Hour
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

// Cancel cancels the task (no-op for periodic sync)
func (e *PeriodicSyncExecutor) Cancel(ctx context.Context, t *model.WorkloadTaskState) error {
	return nil
}

// Execute performs periodic sync
func (e *PeriodicSyncExecutor) Execute(ctx context.Context, execCtx *task.ExecutionContext) (*task.ExecutionResult, error) {
	runSummaryID := int64(e.GetExtInt(execCtx.Task, ExtKeyRunSummaryID))

	log.Infof("PeriodicSyncExecutor: starting periodic sync for run summary %d", runSummaryID)

	runSummaryFacade := database.GetFacade().GetGithubWorkflowRunSummary()

	// Get run summary
	summary, err := runSummaryFacade.GetByID(ctx, runSummaryID)
	if err != nil || summary == nil {
		return task.FailureResult("run summary not found", nil), nil
	}

	// Check if exceeded absolute max sync time (7 days)
	summaryAge := time.Since(summary.CreatedAt)
	if summaryAge > AbsoluteMaxSyncTime {
		log.Warnf("PeriodicSyncExecutor: exceeded absolute max sync time (%v) for run summary %d, giving up",
			AbsoluteMaxSyncTime, runSummaryID)
		return task.SuccessResult(map[string]interface{}{
			"status":  "timeout",
			"message": "exceeded absolute max periodic sync time",
		}), nil
	}

	// Determine sync interval: slow mode after MaxPeriodicSyncTime
	isSlowMode := summaryAge > MaxPeriodicSyncTime
	syncInterval := PeriodicSyncInterval
	if isSlowMode {
		syncInterval = SlowPeriodicSyncInterval
	}

	// Check if already completed
	if summary.Status == "completed" {
		log.Infof("PeriodicSyncExecutor: run summary %d already completed, stopping periodic sync", runSummaryID)
		return task.SuccessResult(map[string]interface{}{
			"status":     "completed",
			"conclusion": summary.Conclusion,
		}), nil
	}

	// Get GitHub client via runner set using PrimaryRunnerSetID from summary
	runnerSetFacade := database.GetFacade().GetGithubRunnerSet()
	runnerSetID := summary.PrimaryRunnerSetID

	// Fallback: if PrimaryRunnerSetID is 0 (legacy data), look it up from workflow runs
	if runnerSetID == 0 {
		log.Warnf("PeriodicSyncExecutor: PrimaryRunnerSetID is 0 for summary %d, looking up from workflow runs", runSummaryID)
		runFacade := database.GetFacade().GetGithubWorkflowRun()
		runs, lookupErr := runFacade.ListByRunSummaryID(ctx, runSummaryID)
		if lookupErr == nil && len(runs) > 0 {
			for _, r := range runs {
				if r.RunnerSetID > 0 {
					runnerSetID = r.RunnerSetID
					// Backfill PrimaryRunnerSetID in the summary
					summary.PrimaryRunnerSetID = runnerSetID
					runSummaryFacade.Update(ctx, summary)
					log.Infof("PeriodicSyncExecutor: backfilled PrimaryRunnerSetID=%d for summary %d from run %d",
						runnerSetID, runSummaryID, r.ID)
					break
				}
			}
		}
		if runnerSetID == 0 {
			return task.FailureResult("runner set not found: PrimaryRunnerSetID is 0 and no runs have a valid RunnerSetID", nil), nil
		}
	}

	runnerSet, err := runnerSetFacade.GetByID(ctx, runnerSetID)
	if err != nil || runnerSet == nil {
		return task.FailureResult(fmt.Sprintf("runner set %d not found", runnerSetID), nil), nil
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
		e.scheduleNextSyncWithInterval(ctx, runSummaryID, syncInterval)
		return task.SuccessResult(map[string]interface{}{
			"status": "retry",
			"error":  err.Error(),
		}), nil
	}

	// Update summary status and backfill missing GitHub info
	summary.Status = ghRun.Status
	summary.Conclusion = ghRun.Conclusion
	summary.LastSyncedAt = time.Now()

	// Backfill workflow details if not yet populated
	if summary.WorkflowName == "" && ghRun.WorkflowName != "" {
		summary.WorkflowName = ghRun.WorkflowName
	}
	if summary.HeadSha == "" && ghRun.HeadSHA != "" {
		summary.HeadSha = ghRun.HeadSHA
	}
	if summary.HeadBranch == "" && ghRun.HeadBranch != "" {
		summary.HeadBranch = ghRun.HeadBranch
	}
	if summary.GithubRunNumber == 0 && ghRun.RunNumber > 0 {
		summary.GithubRunNumber = int32(ghRun.RunNumber)
	}
	// Backfill actor fields
	if summary.Actor == "" {
		if ghRun.Actor != nil && ghRun.Actor.Login != "" {
			summary.Actor = ghRun.Actor.Login
		} else if ghRun.TriggerActor != nil && ghRun.TriggerActor.Login != "" {
			summary.Actor = ghRun.TriggerActor.Login
		}
	}
	if summary.TriggeringActor == "" {
		if ghRun.TriggerActor != nil && ghRun.TriggerActor.Login != "" {
			summary.TriggeringActor = ghRun.TriggerActor.Login
		} else if ghRun.Actor != nil && ghRun.Actor.Login != "" {
			summary.TriggeringActor = ghRun.Actor.Login
		}
	}
	if summary.EventName == "" && ghRun.Event != "" {
		summary.EventName = ghRun.Event
	}

	if err := runSummaryFacade.Update(ctx, summary); err != nil {
		log.Warnf("PeriodicSyncExecutor: failed to update run summary %d: %v", runSummaryID, err)
	}

	// Fetch jobs and update job stats in summary
	ghJobs, jobsErr := client.GetWorkflowRunJobs(ctx, summary.Owner, summary.Repo, summary.GithubRunID)
	if jobsErr == nil {
		// Match GitHub jobs to individual runner records and update workflow_status
		syncWorkflowStatusFromJobs(ctx, summary.ID, ghJobs)

		// Update job stats in summary (queries from DB)
		if err := runSummaryFacade.UpdateJobStats(ctx, summary.ID); err != nil {
			log.Warnf("PeriodicSyncExecutor: failed to update job stats for summary %d: %v", summary.ID, err)
		}
	} else {
		log.Warnf("PeriodicSyncExecutor: failed to get jobs for summary %d: %v", summary.ID, jobsErr)
	}

	// If workflow not completed, schedule next sync with appropriate interval
	if ghRun.Status != "completed" {
		e.scheduleNextSyncWithInterval(ctx, runSummaryID, syncInterval)
		if isSlowMode {
			log.Infof("PeriodicSyncExecutor: synced run summary %d (status: %s, slow mode), scheduled next sync in %v",
				runSummaryID, ghRun.Status, syncInterval)
		} else {
			log.Infof("PeriodicSyncExecutor: synced run summary %d (status: %s), scheduled next sync in %v",
				runSummaryID, ghRun.Status, syncInterval)
		}
		return task.SuccessResult(map[string]interface{}{
			"status":    ghRun.Status,
			"next_sync": time.Now().Add(syncInterval).Format(time.RFC3339),
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

// scheduleNextSyncWithInterval schedules the next periodic sync with the given interval
func (e *PeriodicSyncExecutor) scheduleNextSyncWithInterval(ctx context.Context, runSummaryID int64, interval time.Duration) {
	taskFacade := database.NewWorkloadTaskFacade()

	scheduledAt := time.Now().Add(interval)
	taskUID := fmt.Sprintf("periodic-sync-%d-%d", runSummaryID, scheduledAt.Unix())

	syncTask := &model.WorkloadTaskState{
		WorkloadUID: taskUID,
		TaskType:    constant.TaskTypeGithubPeriodicSync,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			ExtKeyRunSummaryID: runSummaryID,
			ExtKeySyncType:     "periodic",
			"scheduled_at":     scheduledAt.Format(time.RFC3339),
		},
	}

	if err := taskFacade.UpsertTask(ctx, syncTask); err != nil {
		log.Warnf("PeriodicSyncExecutor: failed to schedule next sync for run summary %d: %v", runSummaryID, err)
	}
}

// triggerDataAnalysisOnCompletion triggers data analysis when workflow completes
func (e *PeriodicSyncExecutor) triggerDataAnalysisOnCompletion(ctx context.Context, summary *model.GithubWorkflowRunSummaries, conclusion string) {
	// Only trigger for successful workflows
	if conclusion != "success" {
		log.Debugf("PeriodicSyncExecutor: skipping data analysis for run summary %d (conclusion: %s)", summary.ID, conclusion)
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

// RecoverOrphanedPeriodicSyncs finds in_progress summaries whose periodic sync
// chain was broken (e.g. due to exporter restart) and re-creates their tasks.
// This should be called once during exporter startup.
func RecoverOrphanedPeriodicSyncs(ctx context.Context) {
	runSummaryFacade := database.GetFacade().GetGithubWorkflowRunSummary()
	taskFacade := database.NewWorkloadTaskFacade()

	summaries, err := runSummaryFacade.ListInProgress(ctx, 200)
	if err != nil {
		log.Warnf("RecoverOrphanedPeriodicSyncs: failed to list in-progress summaries: %v", err)
		return
	}

	if len(summaries) == 0 {
		log.Info("RecoverOrphanedPeriodicSyncs: no in-progress summaries found")
		return
	}

	// Check which summaries already have a pending periodic sync task
	pendingTasks, err := taskFacade.ListPendingTasksByType(ctx, constant.TaskTypeGithubPeriodicSync)
	if err != nil {
		log.Warnf("RecoverOrphanedPeriodicSyncs: failed to list pending periodic tasks: %v", err)
		pendingTasks = nil // proceed anyway
	}

	// Build set of summary IDs that already have pending tasks
	hasPendingSync := make(map[int64]bool)
	for _, t := range pendingTasks {
		if t.Ext != nil {
			if sid, ok := t.Ext[ExtKeyRunSummaryID]; ok {
				switch v := sid.(type) {
				case float64:
					hasPendingSync[int64(v)] = true
				case int64:
					hasPendingSync[v] = true
				case int:
					hasPendingSync[int64(v)] = true
				}
			}
		}
	}

	recovered := 0
	staleThreshold := 2 * PeriodicSyncInterval // 10 minutes
	for _, summary := range summaries {
		if hasPendingSync[summary.ID] {
			continue // already has a pending task
		}
		// Only recover summaries that haven't been synced recently
		if !summary.LastSyncedAt.IsZero() && time.Since(summary.LastSyncedAt) < staleThreshold {
			continue
		}
		// Skip summaries older than AbsoluteMaxSyncTime
		if time.Since(summary.CreatedAt) > AbsoluteMaxSyncTime {
			continue
		}
		if err := CreatePeriodicSyncTask(ctx, summary.ID); err != nil {
			log.Warnf("RecoverOrphanedPeriodicSyncs: failed to create task for summary %d: %v", summary.ID, err)
		} else {
			recovered++
			log.Infof("RecoverOrphanedPeriodicSyncs: re-created periodic sync for orphaned summary %d (last_synced: %v)",
				summary.ID, summary.LastSyncedAt)
		}
	}

	log.Infof("RecoverOrphanedPeriodicSyncs: checked %d in-progress summaries, recovered %d orphaned syncs",
		len(summaries), recovered)
}

// BackfillCompletedSummaries finds recently completed summaries that still have
// workflow runs with github_job_id=0 (meaning syncWorkflowStatusFromJobs never ran
// for them) and creates manual sync tasks to backfill the missing data.
// This handles historical data that completed before the backfill code was deployed.
func BackfillCompletedSummaries(ctx context.Context) {
	runSummaryFacade := database.GetFacade().GetGithubWorkflowRunSummary()
	runFacade := database.GetFacade().GetGithubWorkflowRun()

	// Find summaries completed in the last 7 days
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	var summaries []*model.GithubWorkflowRunSummaries
	err := database.GetFacade().GetSystemConfig().GetDB().WithContext(ctx).
		Where("status = ? AND created_at > ?", database.RunSummaryStatusCompleted, cutoff).
		Order("created_at DESC").
		Limit(200).
		Find(&summaries).Error
	if err != nil {
		log.Warnf("BackfillCompletedSummaries: failed to list completed summaries: %v", err)
		return
	}
	_ = runSummaryFacade // used indirectly

	if len(summaries) == 0 {
		log.Info("BackfillCompletedSummaries: no recently completed summaries to check")
		return
	}

	backfilled := 0
	for _, summary := range summaries {
		if summary.GithubRunID == 0 {
			continue
		}
		// Check if any runs under this summary still have github_job_id=0
		runs, err := runFacade.ListByRunSummaryID(ctx, summary.ID)
		if err != nil || len(runs) == 0 {
			continue
		}

		needsBackfill := false
		for _, run := range runs {
			if run.GithubJobID == 0 {
				needsBackfill = true
				break
			}
		}

		if !needsBackfill {
			continue
		}

		// Create a manual sync task for this summary
		if err := CreateManualSyncTaskInternal(ctx, summary.ID); err != nil {
			log.Warnf("BackfillCompletedSummaries: failed to create task for summary %d: %v", summary.ID, err)
		} else {
			backfilled++
			log.Infof("BackfillCompletedSummaries: created backfill sync for completed summary %d", summary.ID)
		}
	}

	log.Infof("BackfillCompletedSummaries: checked %d completed summaries, created %d backfill tasks",
		len(summaries), backfilled)
}

// CreateManualSyncTaskInternal creates a manual sync task without cluster context (for exporter use)
func CreateManualSyncTaskInternal(ctx context.Context, runSummaryID int64) error {
	taskFacade := database.NewWorkloadTaskFacade()
	taskUID := fmt.Sprintf("backfill-sync-%d-%d", runSummaryID, time.Now().Unix())

	syncTask := &model.WorkloadTaskState{
		WorkloadUID: taskUID,
		TaskType:    constant.TaskTypeGithubManualSync,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			ExtKeyRunSummaryID: runSummaryID,
			ExtKeySyncType:     "backfill",
		},
	}

	return taskFacade.UpsertTask(ctx, syncTask)
}

// CreatePeriodicSyncTask creates a periodic sync task for a workflow run summary
func CreatePeriodicSyncTask(ctx context.Context, runSummaryID int64) error {
	taskFacade := database.NewWorkloadTaskFacade()

	scheduledAt := time.Now().Add(PeriodicSyncInterval)
	taskUID := fmt.Sprintf("periodic-sync-%d-%d", runSummaryID, scheduledAt.Unix())

	syncTask := &model.WorkloadTaskState{
		WorkloadUID: taskUID,
		TaskType:    constant.TaskTypeGithubPeriodicSync,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			ExtKeyRunSummaryID: runSummaryID,
			ExtKeySyncType:     "periodic",
			"scheduled_at":     scheduledAt.Format(time.RFC3339),
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

// syncWorkflowStatusFromJobs matches GitHub jobs to individual github_workflow_runs
// records (by runner_name == workload_name) and updates each record's workflow_status
// and workflow_conclusion to reflect the actual GitHub job status.
func syncWorkflowStatusFromJobs(ctx context.Context, summaryID int64, ghJobs []github.JobInfo) {
	if len(ghJobs) == 0 {
		return
	}

	runFacade := database.GetFacade().GetGithubWorkflowRun()
	runs, err := runFacade.ListByRunSummaryID(ctx, summaryID)
	if err != nil || len(runs) == 0 {
		return
	}

	// Build a map of runner_name -> job info for quick lookup
	jobByRunner := make(map[string]*github.JobInfo, len(ghJobs))
	for i := range ghJobs {
		if ghJobs[i].RunnerName != "" {
			jobByRunner[ghJobs[i].RunnerName] = &ghJobs[i]
		}
	}

	for _, run := range runs {
		job, ok := jobByRunner[run.WorkloadName]
		if !ok {
			continue
		}

		fields := make(map[string]interface{})

		// Sync workflow status if changed
		if run.WorkflowStatus != job.Status {
			fields["workflow_status"] = job.Status
		}
		if run.WorkflowConclusion != job.Conclusion {
			fields["workflow_conclusion"] = job.Conclusion
		}

		// Backfill github_job_id if not set (enables GitHub Job name display on frontend)
		if run.GithubJobID == 0 && job.ID != 0 {
			fields["github_job_id"] = job.ID
		}

		if len(fields) == 0 {
			continue
		}

		if err := runFacade.UpdateFields(ctx, run.ID, fields); err != nil {
			log.Warnf("syncWorkflowStatusFromJobs: failed to update run %d: %v", run.ID, err)
		}
	}
}
