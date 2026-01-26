// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package backfill

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/backfill"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// backfillTasksTotal tracks total backfill tasks created
	backfillTasksTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "primus_lens",
			Subsystem: "github_workflow",
			Name:      "backfill_tasks_total",
			Help:      "Total number of backfill tasks created",
		},
	)

	// backfillTasksActive tracks currently active backfill tasks
	backfillTasksActive = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "primus_lens",
			Subsystem: "github_workflow",
			Name:      "backfill_tasks_active",
			Help:      "Number of currently active (pending/in_progress) backfill tasks",
		},
	)

	// backfillRunsCreated tracks runs created by backfill
	backfillRunsCreated = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "primus_lens",
			Subsystem: "github_workflow",
			Name:      "backfill_runs_created_total",
			Help:      "Total number of runs created by backfill",
		},
	)

	// backfillRunsFailed tracks failed run creations by backfill
	backfillRunsFailed = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "primus_lens",
			Subsystem: "github_workflow",
			Name:      "backfill_runs_failed_total",
			Help:      "Total number of failed run creations by backfill",
		},
	)

	metricsOnce sync.Once
)

func registerMetrics() {
	metricsOnce.Do(func() {
		prometheus.MustRegister(backfillTasksTotal)
		prometheus.MustRegister(backfillTasksActive)
		prometheus.MustRegister(backfillRunsCreated)
		prometheus.MustRegister(backfillRunsFailed)

		// Set metrics callback for task manager
		backfill.GetTaskManager().SetMetricsCallback(&backfill.TaskManagerMetricsCallback{
			OnTaskCreated: func() {
				backfillTasksTotal.Inc()
			},
			OnTaskActive: func() {
				backfillTasksActive.Inc()
			},
			OnTaskCompleted: func() {
				backfillTasksActive.Dec()
			},
		})
	})
}

const (
	// DefaultBatchSize is the default number of runs to process per batch
	DefaultBatchSize = 20
	// DefaultMaxConcurrent is the default max concurrent processing
	DefaultMaxConcurrent = 5
	// DefaultCheckInterval is how often to check for backfill tasks
	DefaultCheckInterval = 1 * time.Minute
)

// WorkflowBackfillRunner manages backfill task processing
type WorkflowBackfillRunner struct {
	taskManager   *backfill.BackfillTaskManager
	batchSize     int
	checkInterval time.Duration
	stopCh        chan struct{}
	doneCh        chan struct{}
	running       bool
	mu            sync.Mutex
}

// NewWorkflowBackfillRunner creates a new backfill runner
func NewWorkflowBackfillRunner() *WorkflowBackfillRunner {
	registerMetrics()
	return &WorkflowBackfillRunner{
		taskManager:   backfill.GetTaskManager(),
		batchSize:     DefaultBatchSize,
		checkInterval: DefaultCheckInterval,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
}

// Start starts the backfill runner in a background goroutine
func (r *WorkflowBackfillRunner) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return fmt.Errorf("backfill runner already running")
	}
	r.running = true
	r.stopCh = make(chan struct{})
	r.doneCh = make(chan struct{})
	r.mu.Unlock()

	go r.runLoop(ctx)
	log.Info("WorkflowBackfillRunner started")
	return nil
}

// Stop stops the backfill runner
func (r *WorkflowBackfillRunner) Stop() error {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return nil
	}
	close(r.stopCh)
	r.mu.Unlock()

	// Wait for the loop to finish
	<-r.doneCh
	
	r.mu.Lock()
	r.running = false
	r.mu.Unlock()

	log.Info("WorkflowBackfillRunner stopped")
	return nil
}

// runLoop is the main loop that periodically checks for backfill tasks
func (r *WorkflowBackfillRunner) runLoop(ctx context.Context) {
	defer close(r.doneCh)

	ticker := time.NewTicker(r.checkInterval)
	defer ticker.Stop()

	// Run once immediately
	r.runOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info("WorkflowBackfillRunner: context cancelled, stopping")
			return
		case <-r.stopCh:
			log.Info("WorkflowBackfillRunner: stop signal received, stopping")
			return
		case <-ticker.C:
			r.runOnce(ctx)
		}
	}
}

// runOnce executes a single backfill check cycle
func (r *WorkflowBackfillRunner) runOnce(ctx context.Context) {
	log.Debug("WorkflowBackfillRunner: checking for pending backfill tasks")

	// Get pending tasks
	pendingTasks := r.taskManager.GetPendingTasks()
	if len(pendingTasks) == 0 {
		log.Debug("WorkflowBackfillRunner: no pending backfill tasks")
		return
	}

	log.Infof("WorkflowBackfillRunner: found %d pending backfill tasks", len(pendingTasks))

	for _, task := range pendingTasks {
		// Check if cancelled
		if task.Status == backfill.BackfillStatusCancelled {
			continue
		}

		// Process this task
		err := r.processBackfillTask(ctx, task)
		if err != nil {
			log.Errorf("WorkflowBackfillRunner: failed to process task %s: %v", task.ID, err)
			r.taskManager.UpdateTaskStatus(task.ID, backfill.BackfillStatusFailed, err.Error())
		}
	}

	// Cleanup old completed tasks (older than 7 days)
	removed := r.taskManager.CleanupOldTasks(7 * 24 * time.Hour)
	if removed > 0 {
		log.Infof("WorkflowBackfillRunner: cleaned up %d old tasks", removed)
	}
}

// processBackfillTask processes a single backfill task
func (r *WorkflowBackfillRunner) processBackfillTask(ctx context.Context, task *backfill.BackfillTask) error {
	// Mark as in progress
	r.taskManager.UpdateTaskStatus(task.ID, backfill.BackfillStatusInProgress, "")

	clusterName := task.ClusterName
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Get facades
	facade := database.GetFacadeForCluster(clusterName)
	configFacade := facade.GetGithubWorkflowConfig()
	runnerSetFacade := facade.GetGithubRunnerSet()
	runFacade := facade.GetGithubWorkflowRun()
	workloadFacade := facade.GetWorkload()

	// Determine namespace, runner set info, and config based on task type
	var namespace, runnerSetName string
	var runnerSetID, configID int64
	var config *model.GithubWorkflowConfigs

	if task.ConfigID != 0 {
		// Config-based backfill (existing logic)
		log.Infof("WorkflowBackfillRunner: processing config-based task %s for config %d", task.ID, task.ConfigID)
		
		var err error
		config, err = configFacade.GetByID(ctx, task.ConfigID)
		if err != nil {
			return fmt.Errorf("failed to get config: %w", err)
		}
		if config == nil {
			return fmt.Errorf("config not found: %d", task.ConfigID)
		}

		namespace = config.RunnerSetNamespace
		runnerSetName = config.RunnerSetName
		configID = task.ConfigID

		// Find runner set
		runnerSet, _ := runnerSetFacade.GetByNamespaceName(ctx, namespace, runnerSetName)
		if runnerSet != nil {
			runnerSetID = runnerSet.ID
		}
	} else if task.RunnerSetID != 0 {
		// Runner-set-based backfill (new logic)
		log.Infof("WorkflowBackfillRunner: processing runner-set-based task %s for runner_set %d", task.ID, task.RunnerSetID)
		
		runnerSet, err := runnerSetFacade.GetByID(ctx, task.RunnerSetID)
		if err != nil {
			return fmt.Errorf("failed to get runner set: %w", err)
		}
		if runnerSet == nil {
			return fmt.Errorf("runner set not found: %d", task.RunnerSetID)
		}

		namespace = runnerSet.Namespace
		runnerSetName = runnerSet.Name
		runnerSetID = task.RunnerSetID
		configID = 0 // No specific config

		// Try to find an enabled config for this runner set (optional)
		configs, _ := configFacade.ListByRunnerSet(ctx, namespace, runnerSetName)
		for _, cfg := range configs {
			if cfg.Enabled {
				config = cfg
				configID = cfg.ID
				break
			}
		}
	} else {
		return fmt.Errorf("task must have either ConfigID or RunnerSetID set")
	}

	// Find EphemeralRunners to process
	var workloads []*model.GpuWorkload

	if len(task.WorkloadUIDs) > 0 {
		// Process specific workloads
		for _, uid := range task.WorkloadUIDs {
			workload, err := workloadFacade.GetGpuWorkloadByUid(ctx, uid)
			if err != nil {
				log.Warnf("WorkflowBackfillRunner: failed to get workload %s: %v", uid, err)
				continue
			}
			if workload != nil {
				workloads = append(workloads, workload)
			}
		}
	} else {
		// Find completed EphemeralRunners in the time range
		var err error
		workloads, err = workloadFacade.ListCompletedWorkloadsByKindAndNamespace(
			ctx,
			"EphemeralRunner",
			namespace,
			task.StartTime,
			0, // No limit for backfill
		)
		if err != nil {
			return fmt.Errorf("failed to list workloads: %w", err)
		}

		// Filter by time range and parent
		filtered := make([]*model.GpuWorkload, 0)
		for _, w := range workloads {
			// Check time range
			if !w.EndAt.IsZero() && w.EndAt.Before(task.StartTime) {
				continue
			}
			if w.CreatedAt.After(task.EndTime) {
				continue
			}

			// Check parent matches the AutoscalingRunnerSet
			if !r.matchesRunnerSetByName(w, runnerSetName) {
				continue
			}

			filtered = append(filtered, w)
		}
		workloads = filtered
	}

	if len(workloads) == 0 {
		log.Infof("WorkflowBackfillRunner: no workloads found for task %s", task.ID)
		r.taskManager.UpdateTaskStatus(task.ID, backfill.BackfillStatusCompleted, "")
		r.taskManager.UpdateTaskProgress(task.ID, 0, 0, 0)
		return nil
	}

	log.Infof("WorkflowBackfillRunner: found %d workloads to backfill for task %s", len(workloads), task.ID)
	r.taskManager.UpdateTaskProgress(task.ID, len(workloads), 0, 0)

	// Create run records for workloads not yet processed
	processed := 0
	failed := 0
	created := 0

	for _, workload := range workloads {
		// Check if cancelled
		currentTask := r.taskManager.GetTask(task.ID)
		if currentTask != nil && currentTask.Status == backfill.BackfillStatusCancelled {
			log.Infof("WorkflowBackfillRunner: task %s was cancelled", task.ID)
			return nil
		}

		// Check if already processed (use runner_set_id for lookup)
		var existingRun *model.GithubWorkflowRuns
		var err error
		if runnerSetID != 0 {
			existingRun, err = runFacade.GetByRunnerSetAndWorkload(ctx, runnerSetID, workload.UID)
		} else {
			// Fallback to config-based lookup for backward compatibility
			existingRun, err = runFacade.GetByConfigAndWorkload(ctx, task.ConfigID, workload.UID)
		}
		if err != nil {
			log.Warnf("WorkflowBackfillRunner: failed to check existing run for workload %s: %v", workload.UID, err)
			failed++
			continue
		}

		if existingRun != nil {
			// Already processed
			processed++
			r.taskManager.UpdateTaskProgress(task.ID, len(workloads), processed, failed)
			continue
		}

		// Create new run record with pending status and backfill trigger
		run := &model.GithubWorkflowRuns{
			RunnerSetID:         runnerSetID,      // Required
			RunnerSetName:       runnerSetName,    // Required
			RunnerSetNamespace:  namespace,        // Required
			ConfigID:            configID,         // Optional (0 if runner-set-based)
			WorkloadUID:         workload.UID,
			WorkloadName:        workload.Name,
			WorkloadNamespace:   workload.Namespace,
			Status:              database.WorkflowRunStatusPending,
			TriggerSource:       "backfill",
			WorkloadStartedAt:   workload.CreatedAt,
			WorkloadCompletedAt: workload.EndAt,
			GithubRunID:         r.extractGithubRunID(workload),
			GithubJobID:         r.extractGithubJobID(workload),
			WorkflowName:        r.extractWorkflowName(workload),
			HeadBranch:          r.extractBranch(workload),
		}

		if err := runFacade.Create(ctx, run); err != nil {
			log.Warnf("WorkflowBackfillRunner: failed to create run for workload %s: %v", workload.UID, err)
			failed++
			backfillRunsFailed.Inc()
		} else {
			created++
			backfillRunsCreated.Inc()
		}

		processed++
		r.taskManager.UpdateTaskProgress(task.ID, len(workloads), processed, failed)
	}

	// Mark as completed
	r.taskManager.UpdateTaskStatus(task.ID, backfill.BackfillStatusCompleted, "")
	log.Infof("WorkflowBackfillRunner: completed task %s (total: %d, created: %d, failed: %d)",
		task.ID, len(workloads), created, failed)

	return nil
}

// matchesRunnerSetByName checks if a workload belongs to the specified runner set by name
func (r *WorkflowBackfillRunner) matchesRunnerSetByName(workload *model.GpuWorkload, runnerSetName string) bool {
	// Check labels for scale-set-name
	if workload.Labels != nil {
		if scaleSetName, ok := workload.Labels["actions.github.com/scale-set-name"].(string); ok {
			if scaleSetName == runnerSetName {
				return true
			}
		}
	}

	return false
}

// extractGithubRunID extracts GitHub run ID from workload annotations
func (r *WorkflowBackfillRunner) extractGithubRunID(workload *model.GpuWorkload) int64 {
	if workload.Annotations == nil {
		return 0
	}
	if runIDStr, ok := workload.Annotations["actions.github.com/run-id"].(string); ok {
		if runID, err := fmt.Sscanf(runIDStr, "%d", new(int64)); err == nil {
			return int64(runID)
		}
	}
	return 0
}

// extractGithubJobID extracts GitHub job ID from workload annotations
func (r *WorkflowBackfillRunner) extractGithubJobID(workload *model.GpuWorkload) int64 {
	if workload.Annotations == nil {
		return 0
	}
	if jobIDStr, ok := workload.Annotations["actions.github.com/job-id"].(string); ok {
		if jobID, err := fmt.Sscanf(jobIDStr, "%d", new(int64)); err == nil {
			return int64(jobID)
		}
	}
	return 0
}

// extractWorkflowName extracts workflow name from workload annotations
func (r *WorkflowBackfillRunner) extractWorkflowName(workload *model.GpuWorkload) string {
	if workload.Annotations == nil {
		return ""
	}
	if name, ok := workload.Annotations["actions.github.com/workflow-name"].(string); ok {
		return name
	}
	return ""
}

// extractBranch extracts branch from workload annotations
func (r *WorkflowBackfillRunner) extractBranch(workload *model.GpuWorkload) string {
	if workload.Annotations == nil {
		return ""
	}
	if branch, ok := workload.Annotations["actions.github.com/branch"].(string); ok {
		return branch
	}
	return ""
}
