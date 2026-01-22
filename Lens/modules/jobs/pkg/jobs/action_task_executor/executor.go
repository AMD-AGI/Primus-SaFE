// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package action_task_executor implements the data plane executor for action_tasks.
// It polls the database for pending tasks and executes them locally using the appropriate handlers.
// This enables cross-cluster communication via database as the communication medium.
package action_task_executor

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
)

const (
	// DefaultPollInterval is the default polling interval (300ms for <1s latency)
	DefaultPollInterval = 300 * time.Millisecond

	// DefaultBatchSize is the default number of tasks to fetch per poll
	DefaultBatchSize = 10

	// JobSchedule defines when the job runs (every 300ms for low latency)
	JobSchedule = "@every 300ms"
)

// ActionHandler is a function that handles a specific action type
// Parameters contains the action parameters, returns result data and error
type ActionHandler func(ctx context.Context, task *model.ActionTasks, k8sClient *clientsets.K8SClientSet) (interface{}, error)

// ActionTaskExecutor polls and executes action tasks from the database
type ActionTaskExecutor struct {
	clusterName  string
	pollInterval time.Duration
	batchSize    int
	handlers     map[string]ActionHandler
	mu           sync.RWMutex
	facade       database.ActionTaskFacadeInterface
}

// NewActionTaskExecutor creates a new ActionTaskExecutor
func NewActionTaskExecutor(clusterName string) *ActionTaskExecutor {
	return &ActionTaskExecutor{
		clusterName:  clusterName,
		pollInterval: DefaultPollInterval,
		batchSize:    DefaultBatchSize,
		handlers:     make(map[string]ActionHandler),
		facade:       database.NewActionTaskFacade(),
	}
}

// WithPollInterval sets the polling interval
func (e *ActionTaskExecutor) WithPollInterval(interval time.Duration) *ActionTaskExecutor {
	e.pollInterval = interval
	return e
}

// WithBatchSize sets the batch size for fetching tasks
func (e *ActionTaskExecutor) WithBatchSize(size int) *ActionTaskExecutor {
	e.batchSize = size
	return e
}

// RegisterHandler registers a handler for a specific action type
func (e *ActionTaskExecutor) RegisterHandler(actionType string, handler ActionHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers[actionType] = handler
	log.Infof("Registered action handler for type: %s", actionType)
}

// getHandler returns the handler for a specific action type
func (e *ActionTaskExecutor) getHandler(actionType string) (ActionHandler, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	handler, ok := e.handlers[actionType]
	return handler, ok
}

// Schedule returns the cron schedule for this job
func (e *ActionTaskExecutor) Schedule() string {
	return JobSchedule
}

// Run executes the job - polls for pending tasks and processes them
func (e *ActionTaskExecutor) Run(
	ctx context.Context,
	k8sClient *clientsets.K8SClientSet,
	storageClient *clientsets.StorageClientSet,
) (*common.ExecutionStats, error) {
	stats := &common.ExecutionStats{}

	// Fetch pending tasks for this cluster
	tasks, err := e.facade.GetPendingTasks(ctx, e.clusterName, e.batchSize)
	if err != nil {
		log.Errorf("Failed to fetch pending action tasks: %v", err)
		return stats, err
	}

	if len(tasks) == 0 {
		return stats, nil
	}

	log.Debugf("Found %d pending action tasks for cluster %s", len(tasks), e.clusterName)

	var processedCount, failedCount int64

	// Process each task
	for _, task := range tasks {
		if err := e.processTask(ctx, task, k8sClient); err != nil {
			log.Errorf("Failed to process action task %d: %v", task.ID, err)
			failedCount++
		} else {
			processedCount++
		}
	}

	stats.RecordsProcessed = processedCount
	stats.ItemsUpdated = processedCount
	stats.ItemsDeleted = failedCount // Use ItemsDeleted to track failures

	if processedCount > 0 || failedCount > 0 {
		log.Infof("ActionTaskExecutor completed: processed=%d, failed=%d, total=%d",
			processedCount, failedCount, len(tasks))
	}

	return stats, nil
}

// processTask processes a single action task
func (e *ActionTaskExecutor) processTask(ctx context.Context, task *model.ActionTasks, k8sClient *clientsets.K8SClientSet) error {
	// Check if task has timed out
	if e.isTaskTimedOut(task) {
		return e.markTaskTimeout(ctx, task, "task timed out before execution")
	}

	// Get handler for this action type
	handler, ok := e.getHandler(task.ActionType)
	if !ok {
		return e.markTaskFailed(ctx, task, "unknown action type: "+task.ActionType)
	}

	// Mark task as running
	if err := e.facade.MarkRunning(ctx, task.ID); err != nil {
		log.Warnf("Failed to mark task %d as running: %v", task.ID, err)
		// Continue anyway
	}

	log.Infof("Executing action task %d: type=%s, target=%s/%s, node=%s",
		task.ID, task.ActionType, task.TargetType, task.TargetID, task.TargetNode)

	// Execute the handler
	result, err := handler(ctx, task, k8sClient)
	if err != nil {
		return e.markTaskFailed(ctx, task, err.Error())
	}

	// Marshal result
	var resultExt model.ExtType
	if result != nil {
		resultBytes, err := json.Marshal(result)
		if err != nil {
			return e.markTaskFailed(ctx, task, "failed to marshal result: "+err.Error())
		}
		if err := json.Unmarshal(resultBytes, &resultExt); err != nil {
			return e.markTaskFailed(ctx, task, "failed to unmarshal result: "+err.Error())
		}
	}

	// Mark task as completed
	if err := e.facade.MarkCompleted(ctx, task.ID, resultExt); err != nil {
		log.Errorf("Failed to mark task %d as completed: %v", task.ID, err)
		return err
	}

	log.Infof("Action task %d completed successfully", task.ID)
	return nil
}

// isTaskTimedOut checks if a task has exceeded its timeout
func (e *ActionTaskExecutor) isTaskTimedOut(task *model.ActionTasks) bool {
	timeout := time.Duration(task.TimeoutSeconds) * time.Second
	return time.Since(task.CreatedAt) > timeout
}

// markTaskFailed marks a task as failed
func (e *ActionTaskExecutor) markTaskFailed(ctx context.Context, task *model.ActionTasks, errorMsg string) error {
	log.Errorf("Action task %d failed: %s", task.ID, errorMsg)
	if err := e.facade.MarkFailed(ctx, task.ID, errorMsg); err != nil {
		log.Errorf("Failed to mark task %d as failed: %v", task.ID, err)
		return err
	}
	return nil
}

// markTaskTimeout marks a task as timed out
func (e *ActionTaskExecutor) markTaskTimeout(ctx context.Context, task *model.ActionTasks, errorMsg string) error {
	log.Warnf("Action task %d timed out: %s", task.ID, errorMsg)
	if err := e.facade.MarkTimeout(ctx, task.ID, errorMsg); err != nil {
		log.Errorf("Failed to mark task %d as timeout: %v", task.ID, err)
		return err
	}
	return nil
}
