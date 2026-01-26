// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
	"github.com/AMD-AGI/Primus-SaFE/Lens/github-runners-exporter/pkg/collector"
)

const (
	// ExtKeyRunID is the key for workflow run ID in task ext
	ExtKeyRunID = "run_id"
	// ExtKeyRunnerSetID is the key for runner set ID in task ext
	ExtKeyRunnerSetID = "runner_set_id"
	// ExtKeyConfigID is the key for config ID in task ext
	ExtKeyConfigID = "config_id"
	// ExtKeyWorkloadName is the key for workload name in task ext
	ExtKeyWorkloadName = "workload_name"
	// ExtKeyCollectionResult is the key for collection result in task ext
	ExtKeyCollectionResult = "collection_result"
	// ExtKeyMetricsCount is the key for metrics count in task ext
	ExtKeyMetricsCount = "metrics_count"
	// ExtKeyErrorMessage is the key for error message in task ext
	ExtKeyErrorMessage = "error_message"
	// ExtKeyRetryCount is the key for retry count in task ext
	ExtKeyRetryCount = "retry_count"

	// MaxRetries is the maximum number of retries for a failed collection
	MaxRetries = 3
)

// CollectorInterface defines the interface for workflow metrics collection
// This allows for dependency injection and testing
type CollectorInterface interface {
	// CollectRun collects metrics for a single workflow run
	CollectRun(ctx context.Context, runID int64) (*CollectionResult, error)
}

// CollectionResult contains the result of a collection operation
type CollectionResult struct {
	Success      bool
	MetricsCount int
	SkipReason   string
	Error        string
}

// CollectionExecutor implements task.TaskExecutor for GitHub workflow collection
type CollectionExecutor struct {
	task.BaseExecutor
	clientSets *clientsets.K8SClientSet
	collector  CollectorInterface
}

// NewCollectionExecutor creates a new CollectionExecutor
func NewCollectionExecutor(clientSets *clientsets.K8SClientSet) *CollectionExecutor {
	return &CollectionExecutor{
		clientSets: clientSets,
	}
}

// SetCollector sets the collector implementation (for dependency injection)
func (e *CollectionExecutor) SetCollector(collector CollectorInterface) {
	e.collector = collector
}

// GetTaskType returns the task type this executor handles
func (e *CollectionExecutor) GetTaskType() string {
	return constant.TaskTypeGithubWorkflowCollection
}

// Validate validates task parameters
func (e *CollectionExecutor) Validate(taskState *model.WorkloadTaskState) error {
	runID := e.GetExtInt(taskState, ExtKeyRunID)
	if runID == 0 {
		return fmt.Errorf("missing required parameter: %s", ExtKeyRunID)
	}
	return nil
}

// Cancel cancels task execution
func (e *CollectionExecutor) Cancel(ctx context.Context, taskState *model.WorkloadTaskState) error {
	// Collection tasks are generally quick, no special cancellation needed
	log.Infof("CollectionExecutor: cancelling task for workload %s", taskState.WorkloadUID)
	return nil
}

// Execute executes the collection task
func (e *CollectionExecutor) Execute(ctx context.Context, execCtx *task.ExecutionContext) (*task.ExecutionResult, error) {
	taskState := execCtx.Task
	runID := int64(e.GetExtInt(taskState, ExtKeyRunID))
	workloadName := e.GetExtString(taskState, ExtKeyWorkloadName)
	retryCount := e.GetExtInt(taskState, ExtKeyRetryCount)

	log.Infof("CollectionExecutor: starting collection for run %d (workload: %s, retry: %d)",
		runID, workloadName, retryCount)

	// Get the workflow run from database
	runFacade := database.GetFacade().GetGithubWorkflowRun()
	run, err := runFacade.GetByID(ctx, runID)
	if err != nil {
		return task.FailureResult(
			fmt.Sprintf("failed to get workflow run %d: %v", runID, err),
			map[string]interface{}{ExtKeyErrorMessage: err.Error()},
		), nil
	}

	if run == nil {
		return task.FailureResult(
			fmt.Sprintf("workflow run %d not found", runID),
			map[string]interface{}{ExtKeyErrorMessage: "run not found"},
		), nil
	}

	// Check if run is still pending
	if run.Status != database.WorkflowRunStatusPending {
		log.Infof("CollectionExecutor: run %d is not pending (status: %s), skipping", runID, run.Status)
		return task.SuccessResult(map[string]interface{}{
			ExtKeyCollectionResult: "skipped",
			"skip_reason":          fmt.Sprintf("run status is %s", run.Status),
		}), nil
	}

	// Check if config exists
	if run.ConfigID == 0 {
		// No config means no collection needed - mark as completed
		log.Infof("CollectionExecutor: run %d has no config, marking as completed", runID)
		if err := runFacade.MarkCompleted(ctx, runID, 0, 0, 0); err != nil {
			log.Errorf("CollectionExecutor: failed to mark run %d as completed: %v", runID, err)
		}
		return task.SuccessResult(map[string]interface{}{
			ExtKeyCollectionResult: "no_config",
		}), nil
	}

	// Update run status to collecting
	if err := runFacade.UpdateStatus(ctx, runID, database.WorkflowRunStatusCollecting, ""); err != nil {
		log.Errorf("CollectionExecutor: failed to update run %d status to collecting: %v", runID, err)
	}

	// Perform collection
	result, err := e.performCollection(ctx, run)
	if err != nil {
		// Handle retry logic
		if retryCount < MaxRetries {
			log.Warnf("CollectionExecutor: collection failed for run %d, will retry (%d/%d): %v",
				runID, retryCount+1, MaxRetries, err)

			// Revert to pending for retry
			if updateErr := runFacade.UpdateStatus(ctx, runID, database.WorkflowRunStatusPending, ""); updateErr != nil {
				log.Errorf("CollectionExecutor: failed to revert run %d to pending: %v", runID, updateErr)
			}
			if updateErr := runFacade.IncrementRetryCount(ctx, runID); updateErr != nil {
				log.Errorf("CollectionExecutor: failed to increment retry count for run %d: %v", runID, updateErr)
			}

			return task.FailureResult(
				fmt.Sprintf("collection failed: %v", err),
				map[string]interface{}{
					ExtKeyErrorMessage: err.Error(),
					ExtKeyRetryCount:   retryCount + 1,
				},
			), nil
		}

		// Max retries exceeded
		log.Errorf("CollectionExecutor: collection failed for run %d after %d retries: %v",
			runID, MaxRetries, err)
		if updateErr := runFacade.MarkFailed(ctx, runID, err.Error()); updateErr != nil {
			log.Errorf("CollectionExecutor: failed to mark run %d as failed: %v", runID, updateErr)
		}

		return task.FailureResult(
			fmt.Sprintf("collection failed after %d retries: %v", MaxRetries, err),
			map[string]interface{}{
				ExtKeyErrorMessage: err.Error(),
				ExtKeyRetryCount:   retryCount,
			},
		), nil
	}

	// Collection succeeded
	log.Infof("CollectionExecutor: collection completed for run %d (metrics: %d)",
		runID, result.MetricsCount)

	// Mark run as completed
	if err := runFacade.MarkCompleted(ctx, runID, 0, 0, int32(result.MetricsCount)); err != nil {
		log.Errorf("CollectionExecutor: failed to mark run %d as completed: %v", runID, err)
	}

	// Create analysis task if enabled (chained task)
	if err := e.createAnalysisTask(ctx, run); err != nil {
		log.Warnf("CollectionExecutor: failed to create analysis task for run %d: %v", runID, err)
		// Don't fail the collection task if analysis task creation fails
	}

	return task.SuccessResult(map[string]interface{}{
		ExtKeyCollectionResult: "success",
		ExtKeyMetricsCount:     result.MetricsCount,
	}), nil
}

// performCollection performs the actual collection using the collector
func (e *CollectionExecutor) performCollection(ctx context.Context, run *model.GithubWorkflowRuns) (*CollectionResult, error) {
	// If collector is set (dependency injection for testing), use it
	if e.collector != nil {
		return e.collector.CollectRun(ctx, run.ID)
	}

	// Use the new migrated WorkflowCollector from collector package
	log.Infof("CollectionExecutor: performing collection for run %d using WorkflowCollector", run.ID)

	workflowCollector := collector.NewWorkflowCollector()

	// Initialize with K8s clients if available
	if e.clientSets != nil {
		workflowCollector.Initialize(e.clientSets)
	}

	// Perform collection
	metricsCount, err := workflowCollector.CollectRun(ctx, run)
	if err != nil {
		return nil, fmt.Errorf("collection failed: %w", err)
	}

	return &CollectionResult{
		Success:      true,
		MetricsCount: metricsCount,
	}, nil
}

// createAnalysisTask creates a chained analysis task for the completed run
func (e *CollectionExecutor) createAnalysisTask(ctx context.Context, run *model.GithubWorkflowRuns) error {
	// Check if analysis is enabled for this config
	if run.ConfigID == 0 {
		return nil
	}

	configFacade := database.GetFacade().GetGithubWorkflowConfig()
	config, err := configFacade.GetByID(ctx, run.ConfigID)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// Check if analysis is enabled in config
	// (This would be a config field like config.AnalysisEnabled)
	if config == nil || !config.Enabled {
		return nil
	}

	// Create analysis task
	taskFacade := database.NewWorkloadTaskFacade()
	analysisTask := &model.WorkloadTaskState{
		WorkloadUID: fmt.Sprintf("analysis-%d-%d", run.ID, time.Now().Unix()),
		TaskType:    constant.TaskTypeGithubWorkflowAnalysis,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			ExtKeyRunID:        run.ID,
			ExtKeyRunnerSetID:  run.RunnerSetID,
			ExtKeyConfigID:     run.ConfigID,
			ExtKeyWorkloadName: run.WorkloadName,
		},
	}

	if err := taskFacade.UpsertTask(ctx, analysisTask); err != nil {
		return fmt.Errorf("failed to create analysis task: %w", err)
	}

	log.Infof("CollectionExecutor: created analysis task for run %d", run.ID)
	return nil
}
