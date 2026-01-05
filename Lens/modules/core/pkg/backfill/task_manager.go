package backfill

import (
	"fmt"
	"sync"
	"time"
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
	UpdatedAt     time.Time      `json:"updated_at"`
	StartedAt     *time.Time     `json:"started_at,omitempty"`
	CompletedAt   *time.Time     `json:"completed_at,omitempty"`
	ErrorMessage  string         `json:"error_message,omitempty"`
	ClusterName   string         `json:"cluster_name"`
	DryRun        bool           `json:"dry_run"`
}

// TaskManagerMetricsCallback allows external code to update metrics
type TaskManagerMetricsCallback struct {
	OnTaskCreated   func()
	OnTaskActive    func()
	OnTaskCompleted func()
}

// BackfillTaskManager manages backfill tasks
type BackfillTaskManager struct {
	mu      sync.RWMutex
	tasks   map[string]*BackfillTask
	metrics *TaskManagerMetricsCallback
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

// SetMetricsCallback sets the metrics callback for the task manager
func (m *BackfillTaskManager) SetMetricsCallback(cb *TaskManagerMetricsCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics = cb
}

// CreateTask creates a new backfill task
func (m *BackfillTaskManager) CreateTask(configID int64, startTime, endTime time.Time, workloadUIDs []string, clusterName string, dryRun bool) *BackfillTask {
	m.mu.Lock()
	defer m.mu.Unlock()

	taskID := fmt.Sprintf("backfill-%d-%d", configID, time.Now().UnixNano())
	now := time.Now()
	task := &BackfillTask{
		ID:           taskID,
		ConfigID:     configID,
		StartTime:    startTime,
		EndTime:      endTime,
		WorkloadUIDs: workloadUIDs,
		Status:       BackfillStatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
		ClusterName:  clusterName,
		DryRun:       dryRun,
	}

	m.tasks[taskID] = task

	// Update metrics via callback
	if m.metrics != nil {
		if m.metrics.OnTaskCreated != nil {
			m.metrics.OnTaskCreated()
		}
		if m.metrics.OnTaskActive != nil {
			m.metrics.OnTaskActive()
		}
	}

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

// GetAllTasks returns all tasks
func (m *BackfillTaskManager) GetAllTasks() []*BackfillTask {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*BackfillTask, 0, len(m.tasks))
	for _, task := range m.tasks {
		result = append(result, task)
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
		task.UpdatedAt = time.Now()
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
			if (oldStatus == BackfillStatusPending || oldStatus == BackfillStatusInProgress) && m.metrics != nil && m.metrics.OnTaskCompleted != nil {
				m.metrics.OnTaskCompleted()
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
		task.UpdatedAt = time.Now()
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

	oldStatus := task.Status
	task.Status = BackfillStatusCancelled
	now := time.Now()
	task.CompletedAt = &now
	task.UpdatedAt = now

	// Decrement active tasks
	if (oldStatus == BackfillStatusPending || oldStatus == BackfillStatusInProgress) && m.metrics != nil && m.metrics.OnTaskCompleted != nil {
		m.metrics.OnTaskCompleted()
	}

	return nil
}

// CleanupOldTasks removes tasks older than the specified duration
func (m *BackfillTaskManager) CleanupOldTasks(maxAge time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	count := 0

	for taskID, task := range m.tasks {
		// Only cleanup completed/failed/cancelled tasks
		if task.Status == BackfillStatusCompleted || task.Status == BackfillStatusFailed || task.Status == BackfillStatusCancelled {
			if task.CompletedAt != nil && task.CompletedAt.Before(cutoff) {
				delete(m.tasks, taskID)
				count++
			}
		}
	}

	return count
}

// HasActiveTaskForConfig checks if there's an active task for the given config
func (m *BackfillTaskManager) HasActiveTaskForConfig(configID int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, task := range m.tasks {
		if task.ConfigID == configID && (task.Status == BackfillStatusPending || task.Status == BackfillStatusInProgress) {
			return true
		}
	}
	return false
}
