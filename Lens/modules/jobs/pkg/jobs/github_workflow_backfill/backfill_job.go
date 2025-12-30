package github_workflow_backfill

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
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
)

func init() {
	prometheus.MustRegister(backfillTasksTotal)
	prometheus.MustRegister(backfillTasksActive)
	prometheus.MustRegister(backfillRunsCreated)
	prometheus.MustRegister(backfillRunsFailed)
}

const (
	// DefaultBatchSize is the default number of runs to process per batch
	DefaultBatchSize = 20
	// DefaultMaxConcurrent is the default max concurrent processing
	DefaultMaxConcurrent = 5
	// BackfillCheckInterval is how often to check for backfill tasks
	BackfillCheckInterval = "1m"
)

// BackfillStatus represents the status of a backfill task
type BackfillStatus string

const (
	BackfillStatusPending    BackfillStatus = "pending"
	BackfillStatusInProgress BackfillStatus = "in_progress"
	BackfillStatusCompleted  BackfillStatus = "completed"
	BackfillStatusFailed     BackfillStatus = "failed"
	BackfillStatusCancelled  BackfillStatus = "cancelled"
)

// BackfillTask represents a backfill task stored in memory (can be enhanced with DB storage)
type BackfillTask struct {
	ID            string         `json:"id"`
	ConfigID      int64          `json:"config_id"`
	StartTime     time.Time      `json:"start_time"`
	EndTime       time.Time      `json:"end_time"`
	WorkloadUIDs  []string       `json:"workload_uids,omitempty"` // Optional: specific workloads
	Status        BackfillStatus `json:"status"`
	TotalRuns     int            `json:"total_runs"`
	ProcessedRuns int            `json:"processed_runs"`
	FailedRuns    int            `json:"failed_runs"`
	CreatedAt     time.Time      `json:"created_at"`
	StartedAt     *time.Time     `json:"started_at,omitempty"`
	CompletedAt   *time.Time     `json:"completed_at,omitempty"`
	ErrorMessage  string         `json:"error_message,omitempty"`
	ClusterName   string         `json:"cluster_name"`
}

// BackfillTaskManager manages backfill tasks
type BackfillTaskManager struct {
	mu    sync.RWMutex
	tasks map[string]*BackfillTask
}

var (
	globalTaskManager     *BackfillTaskManager
	globalTaskManagerOnce sync.Once
)

// GetTaskManager returns the global backfill task manager
func GetTaskManager() *BackfillTaskManager {
	globalTaskManagerOnce.Do(func() {
		globalTaskManager = &BackfillTaskManager{
			tasks: make(map[string]*BackfillTask),
		}
	})
	return globalTaskManager
}

// CreateTask creates a new backfill task
func (m *BackfillTaskManager) CreateTask(configID int64, startTime, endTime time.Time, workloadUIDs []string, clusterName string) *BackfillTask {
	m.mu.Lock()
	defer m.mu.Unlock()

	taskID := fmt.Sprintf("backfill-%d-%d", configID, time.Now().UnixNano())
	task := &BackfillTask{
		ID:           taskID,
		ConfigID:     configID,
		StartTime:    startTime,
		EndTime:      endTime,
		WorkloadUIDs: workloadUIDs,
		Status:       BackfillStatusPending,
		CreatedAt:    time.Now(),
		ClusterName:  clusterName,
	}

	m.tasks[taskID] = task

	// Update metrics
	backfillTasksTotal.Inc()
	backfillTasksActive.Inc()

	return task
}

// GetTask returns a task by ID
func (m *BackfillTaskManager) GetTask(taskID string) *BackfillTask {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tasks[taskID]
}

// GetTasksByConfig returns all tasks for a config
func (m *BackfillTaskManager) GetTasksByConfig(configID int64) []*BackfillTask {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*BackfillTask
	for _, task := range m.tasks {
		if task.ConfigID == configID {
			result = append(result, task)
		}
	}
	return result
}

// GetPendingTasks returns all pending tasks
func (m *BackfillTaskManager) GetPendingTasks() []*BackfillTask {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*BackfillTask
	for _, task := range m.tasks {
		if task.Status == BackfillStatusPending {
			result = append(result, task)
		}
	}
	return result
}

// UpdateTaskStatus updates a task's status
func (m *BackfillTaskManager) UpdateTaskStatus(taskID string, status BackfillStatus, errorMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if task, ok := m.tasks[taskID]; ok {
		oldStatus := task.Status
		task.Status = status
		if errorMsg != "" {
			task.ErrorMessage = errorMsg
		}
		if status == BackfillStatusInProgress && task.StartedAt == nil {
			now := time.Now()
			task.StartedAt = &now
		}
		if status == BackfillStatusCompleted || status == BackfillStatusFailed || status == BackfillStatusCancelled {
			now := time.Now()
			task.CompletedAt = &now

			// Decrement active tasks when task finishes
			if oldStatus == BackfillStatusPending || oldStatus == BackfillStatusInProgress {
				backfillTasksActive.Dec()
			}
		}
	}
}

// UpdateTaskProgress updates a task's progress
func (m *BackfillTaskManager) UpdateTaskProgress(taskID string, total, processed, failed int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if task, ok := m.tasks[taskID]; ok {
		task.TotalRuns = total
		task.ProcessedRuns = processed
		task.FailedRuns = failed
	}
}

// CancelTask cancels a pending or in-progress task
func (m *BackfillTaskManager) CancelTask(taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if task.Status != BackfillStatusPending && task.Status != BackfillStatusInProgress {
		return fmt.Errorf("cannot cancel task in status: %s", task.Status)
	}

	task.Status = BackfillStatusCancelled
	now := time.Now()
	task.CompletedAt = &now

	return nil
}

// CleanupOldTasks removes tasks older than the specified duration
func (m *BackfillTaskManager) CleanupOldTasks(maxAge time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for id, task := range m.tasks {
		if task.CompletedAt != nil && task.CompletedAt.Before(cutoff) {
			delete(m.tasks, id)
			removed++
		}
	}

	return removed
}

// GithubWorkflowBackfillJob processes backfill tasks
type GithubWorkflowBackfillJob struct {
	taskManager *BackfillTaskManager
	batchSize   int
}

// NewGithubWorkflowBackfillJob creates a new backfill job
func NewGithubWorkflowBackfillJob() *GithubWorkflowBackfillJob {
	return &GithubWorkflowBackfillJob{
		taskManager: GetTaskManager(),
		batchSize:   DefaultBatchSize,
	}
}

// Run executes the backfill job
func (j *GithubWorkflowBackfillJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	startTime := time.Now()
	stats := common.NewExecutionStats()

	log.Info("GithubWorkflowBackfillJob: checking for pending backfill tasks")

	// Get pending tasks
	pendingTasks := j.taskManager.GetPendingTasks()
	if len(pendingTasks) == 0 {
		log.Debug("GithubWorkflowBackfillJob: no pending backfill tasks")
		return stats, nil
	}

	log.Infof("GithubWorkflowBackfillJob: found %d pending backfill tasks", len(pendingTasks))

	for _, task := range pendingTasks {
		// Check if cancelled
		if task.Status == BackfillStatusCancelled {
			continue
		}

		// Process this task
		err := j.processBackfillTask(ctx, task)
		if err != nil {
			log.Errorf("GithubWorkflowBackfillJob: failed to process task %s: %v", task.ID, err)
			j.taskManager.UpdateTaskStatus(task.ID, BackfillStatusFailed, err.Error())
			stats.ErrorCount++
		} else {
			stats.ItemsUpdated++
		}
	}

	// Cleanup old completed tasks (older than 7 days)
	removed := j.taskManager.CleanupOldTasks(7 * 24 * time.Hour)
	if removed > 0 {
		log.Infof("GithubWorkflowBackfillJob: cleaned up %d old tasks", removed)
	}

	stats.RecordsProcessed = int64(len(pendingTasks))
	stats.ProcessDuration = time.Since(startTime).Seconds()
	stats.AddMessage(fmt.Sprintf("Processed %d backfill tasks", len(pendingTasks)))

	return stats, nil
}

// processBackfillTask processes a single backfill task
func (j *GithubWorkflowBackfillJob) processBackfillTask(ctx context.Context, task *BackfillTask) error {
	// Mark as in progress
	j.taskManager.UpdateTaskStatus(task.ID, BackfillStatusInProgress, "")

	log.Infof("GithubWorkflowBackfillJob: processing task %s for config %d", task.ID, task.ConfigID)

	clusterName := task.ClusterName
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	// Get facades
	facade := database.GetFacadeForCluster(clusterName)
	configFacade := facade.GetGithubWorkflowConfig()
	runFacade := facade.GetGithubWorkflowRun()
	workloadFacade := facade.GetWorkload()

	// Get config
	config, err := configFacade.GetByID(ctx, task.ConfigID)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}
	if config == nil {
		return fmt.Errorf("config not found: %d", task.ConfigID)
	}

	// Find EphemeralRunners to process
	var workloads []*model.GpuWorkload

	if len(task.WorkloadUIDs) > 0 {
		// Process specific workloads
		for _, uid := range task.WorkloadUIDs {
			workload, err := workloadFacade.GetWorkloadByUID(ctx, uid)
			if err != nil {
				log.Warnf("GithubWorkflowBackfillJob: failed to get workload %s: %v", uid, err)
				continue
			}
			if workload != nil {
				workloads = append(workloads, workload)
			}
		}
	} else {
		// Find completed EphemeralRunners in the time range
		workloads, err = workloadFacade.ListCompletedWorkloadsByKindAndNamespace(
			ctx,
			"EphemeralRunner",
			config.RunnerSetNamespace,
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
			if !j.matchesRunnerSet(w, config) {
				continue
			}

			filtered = append(filtered, w)
		}
		workloads = filtered
	}

	if len(workloads) == 0 {
		log.Infof("GithubWorkflowBackfillJob: no workloads found for task %s", task.ID)
		j.taskManager.UpdateTaskStatus(task.ID, BackfillStatusCompleted, "")
		j.taskManager.UpdateTaskProgress(task.ID, 0, 0, 0)
		return nil
	}

	log.Infof("GithubWorkflowBackfillJob: found %d workloads to backfill for task %s", len(workloads), task.ID)
	j.taskManager.UpdateTaskProgress(task.ID, len(workloads), 0, 0)

	// Create run records for workloads not yet processed
	processed := 0
	failed := 0
	created := 0

	for _, workload := range workloads {
		// Check if cancelled
		currentTask := j.taskManager.GetTask(task.ID)
		if currentTask != nil && currentTask.Status == BackfillStatusCancelled {
			log.Infof("GithubWorkflowBackfillJob: task %s was cancelled", task.ID)
			return nil
		}

		// Check if already processed
		existingRun, err := runFacade.GetByConfigAndWorkload(ctx, task.ConfigID, workload.UID)
		if err != nil {
			log.Warnf("GithubWorkflowBackfillJob: failed to check existing run for workload %s: %v", workload.UID, err)
			failed++
			continue
		}

		if existingRun != nil {
			// Already processed
			processed++
			j.taskManager.UpdateTaskProgress(task.ID, len(workloads), processed, failed)
			continue
		}

		// Create new run record with pending status and backfill trigger
		run := &model.GithubWorkflowRuns{
			ConfigID:             task.ConfigID,
			WorkloadUID:          workload.UID,
			WorkloadName:         workload.Name,
			WorkloadNamespace:    workload.Namespace,
			Status:               database.WorkflowRunStatusPending,
			TriggerSource:        "backfill",
			WorkloadStartedAt:    workload.CreatedAt,
			WorkloadCompletedAt:  workload.EndAt,
			GithubRunID:          j.extractGithubRunID(workload),
			GithubJobID:          j.extractGithubJobID(workload),
			WorkflowName:         j.extractWorkflowName(workload),
			HeadBranch:           j.extractBranch(workload),
		}

		if err := runFacade.Create(ctx, run); err != nil {
			log.Warnf("GithubWorkflowBackfillJob: failed to create run for workload %s: %v", workload.UID, err)
			failed++
			backfillRunsFailed.Inc()
		} else {
			created++
			backfillRunsCreated.Inc()
		}

		processed++
		j.taskManager.UpdateTaskProgress(task.ID, len(workloads), processed, failed)
	}

	// Mark as completed
	j.taskManager.UpdateTaskStatus(task.ID, BackfillStatusCompleted, "")
	log.Infof("GithubWorkflowBackfillJob: completed task %s (total: %d, created: %d, failed: %d)",
		task.ID, len(workloads), created, failed)

	return nil
}

// matchesRunnerSet checks if a workload belongs to the configured AutoscalingRunnerSet
func (j *GithubWorkflowBackfillJob) matchesRunnerSet(workload *model.GpuWorkload, config *model.GithubWorkflowConfigs) bool {
	// Check labels for scale-set-name
	if workload.Labels != nil {
		if scaleSetName, ok := workload.Labels["actions.github.com/scale-set-name"].(string); ok {
			if scaleSetName == config.RunnerSetName {
				return true
			}
		}
	}

	// Check if parent UID matches (if we have it)
	if config.RunnerSetUID != "" && workload.ParentUID == config.RunnerSetUID {
		return true
	}

	return false
}

// extractGithubRunID extracts GitHub run ID from workload annotations
func (j *GithubWorkflowBackfillJob) extractGithubRunID(workload *model.GpuWorkload) int64 {
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
func (j *GithubWorkflowBackfillJob) extractGithubJobID(workload *model.GpuWorkload) int64 {
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
func (j *GithubWorkflowBackfillJob) extractWorkflowName(workload *model.GpuWorkload) string {
	if workload.Annotations == nil {
		return ""
	}
	if name, ok := workload.Annotations["actions.github.com/workflow-name"].(string); ok {
		return name
	}
	return ""
}

// extractBranch extracts branch from workload annotations
func (j *GithubWorkflowBackfillJob) extractBranch(workload *model.GpuWorkload) string {
	if workload.Annotations == nil {
		return ""
	}
	if branch, ok := workload.Annotations["actions.github.com/branch"].(string); ok {
		return branch
	}
	return ""
}

// Schedule returns the cron schedule for this job
func (j *GithubWorkflowBackfillJob) Schedule() string {
	return "@every " + BackfillCheckInterval
}

