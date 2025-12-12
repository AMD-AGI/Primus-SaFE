package task

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// TaskScheduler task scheduler
// Responsible for pulling tasks from database and dispatching them to corresponding executors
type TaskScheduler struct {
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	instanceID string

	// Database facade
	taskFacade database.WorkloadTaskFacadeInterface

	// Executor registry: task_type -> executor
	executors map[string]TaskExecutor
	mu        sync.RWMutex

	// Running tasks: task_id -> context
	runningTasks map[int64]context.CancelFunc
	runningMu    sync.RWMutex

	// Configuration
	config *SchedulerConfig
}

// SchedulerConfig scheduler configuration
type SchedulerConfig struct {
	// Scan interval
	ScanInterval time.Duration

	// Lock hold duration
	LockDuration time.Duration

	// Heartbeat interval
	HeartbeatInterval time.Duration

	// Maximum concurrent tasks
	MaxConcurrentTasks int

	// Stale lock cleanup interval
	StaleLockCleanupInterval time.Duration

	// Whether to auto start
	AutoStart bool
}

// DefaultSchedulerConfig default configuration
func DefaultSchedulerConfig() *SchedulerConfig {
	return &SchedulerConfig{
		ScanInterval:             10 * time.Second,
		LockDuration:             5 * time.Minute,
		HeartbeatInterval:        30 * time.Second,
		MaxConcurrentTasks:       10,
		StaleLockCleanupInterval: 1 * time.Minute,
		AutoStart:                true,
	}
}

// NewTaskScheduler creates task scheduler
func NewTaskScheduler(instanceID string, config *SchedulerConfig) *TaskScheduler {
	if config == nil {
		config = DefaultSchedulerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TaskScheduler{
		ctx:          ctx,
		cancel:       cancel,
		instanceID:   instanceID,
		taskFacade:   database.NewWorkloadTaskFacade(),
		executors:    make(map[string]TaskExecutor),
		runningTasks: make(map[int64]context.CancelFunc),
		config:       config,
	}
}

// RegisterExecutor registers task executor
func (s *TaskScheduler) RegisterExecutor(executor TaskExecutor) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	taskType := executor.GetTaskType()
	if _, exists := s.executors[taskType]; exists {
		return fmt.Errorf("executor for task type %s already registered", taskType)
	}

	s.executors[taskType] = executor
	log.Infof("Registered executor for task type: %s", taskType)
	return nil
}

// GetExecutor gets executor
func (s *TaskScheduler) GetExecutor(taskType string) (TaskExecutor, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	executor, exists := s.executors[taskType]
	return executor, exists
}

// Start starts scheduler
func (s *TaskScheduler) Start() error {
	log.Infof("Starting task scheduler (instance: %s)", s.instanceID)

	// 1. Cleanup stale locks
	if err := s.cleanupStaleLocks(); err != nil {
		log.Warnf("Failed to cleanup stale locks on startup: %v", err)
	}

	// 2. Recover unfinished tasks
	if err := s.recoverTasks(); err != nil {
		log.Warnf("Failed to recover tasks on startup: %v", err)
	}

	// 3. Start scan loop
	s.wg.Add(1)
	go s.scanLoop()

	// 4. Start stale lock cleanup loop
	s.wg.Add(1)
	go s.staleLockCleanupLoop()

	log.Info("Task scheduler started successfully")
	return nil
}

// Stop stops scheduler
func (s *TaskScheduler) Stop() error {
	log.Info("Stopping task scheduler...")

	// 1. Cancel context
	s.cancel()

	// 2. Cancel all running tasks
	s.runningMu.Lock()
	for taskID, cancelFunc := range s.runningTasks {
		log.Infof("Cancelling task %d", taskID)
		cancelFunc()
	}
	s.runningMu.Unlock()

	// 3. Wait for all goroutines to complete
	s.wg.Wait()

	// 4. Release all locks
	s.releaseAllLocks()

	log.Info("Task scheduler stopped")
	return nil
}

// scanLoop scan loop for pending tasks
func (s *TaskScheduler) scanLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.ScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.scanAndExecuteTasks()
		}
	}
}

// staleLockCleanupLoop loop for cleaning up stale locks
func (s *TaskScheduler) staleLockCleanupLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.StaleLockCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if err := s.cleanupStaleLocks(); err != nil {
				log.Errorf("Failed to cleanup stale locks: %v", err)
			}
		}
	}
}

// scanAndExecuteTasks scans and executes tasks
func (s *TaskScheduler) scanAndExecuteTasks() {
	// Check current running task count
	s.runningMu.RLock()
	runningCount := len(s.runningTasks)
	s.runningMu.RUnlock()

	if runningCount >= s.config.MaxConcurrentTasks {
		log.Debugf("Max concurrent tasks reached (%d), skipping scan", s.config.MaxConcurrentTasks)
		return
	}

	// Query pending tasks
	tasks, err := s.taskFacade.ListTasksByStatus(s.ctx, constant.TaskStatusPending)
	if err != nil {
		log.Errorf("Failed to list pending tasks: %v", err)
		return
	}

	if len(tasks) == 0 {
		return
	}

	log.Debugf("Found %d pending tasks", len(tasks))

	// Try to execute each task
	for _, task := range tasks {
		// Check if maximum concurrency reached
		s.runningMu.RLock()
		runningCount := len(s.runningTasks)
		s.runningMu.RUnlock()

		if runningCount >= s.config.MaxConcurrentTasks {
			log.Debugf("Max concurrent tasks reached, stopping scan")
			break
		}

		// Try to acquire lock and execute task
		s.tryExecuteTask(task)
	}
}

// tryExecuteTask tries to acquire lock and execute task
func (s *TaskScheduler) tryExecuteTask(task *model.WorkloadTaskState) {
	// 1. Try to acquire lock
	acquired, err := s.taskFacade.TryAcquireLock(
		s.ctx,
		task.WorkloadUID,
		task.TaskType,
		s.instanceID,
		s.config.LockDuration,
	)

	if err != nil {
		log.Errorf("Failed to acquire lock for task %s/%s: %v",
			task.WorkloadUID, task.TaskType, err)
		return
	}

	if !acquired {
		log.Debugf("Task %s/%s is locked by another instance",
			task.WorkloadUID, task.TaskType)
		return
	}

	// 2. Get executor
	executor, exists := s.GetExecutor(task.TaskType)
	if !exists {
		log.Warnf("No executor registered for task type: %s", task.TaskType)
		// Release lock
		s.taskFacade.ReleaseLock(s.ctx, task.WorkloadUID, task.TaskType, s.instanceID)
		return
	}

	// 3. Validate task parameters
	if err := executor.Validate(task); err != nil {
		log.Errorf("Task validation failed for %s/%s: %v",
			task.WorkloadUID, task.TaskType, err)
		// Update task status to failed
		s.taskFacade.UpdateTaskStatus(s.ctx, task.WorkloadUID, task.TaskType, constant.TaskStatusFailed)
		s.taskFacade.UpdateTaskExt(s.ctx, task.WorkloadUID, task.TaskType, model.ExtType{
			"error": fmt.Sprintf("validation failed: %v", err),
		})
		s.taskFacade.ReleaseLock(s.ctx, task.WorkloadUID, task.TaskType, s.instanceID)
		return
	}

	log.Infof("Acquired lock for task %s/%s, starting execution",
		task.WorkloadUID, task.TaskType)

	// 4. Start task execution
	s.wg.Add(1)
	go s.executeTask(task, executor)
}

// executeTask executes task
func (s *TaskScheduler) executeTask(task *model.WorkloadTaskState, executor TaskExecutor) {
	defer s.wg.Done()

	// Create task-specific context
	taskCtx, taskCancel := context.WithCancel(s.ctx)
	defer taskCancel()

	// Register to running tasks list
	s.runningMu.Lock()
	s.runningTasks[task.ID] = taskCancel
	s.runningMu.Unlock()

	defer func() {
		s.runningMu.Lock()
		delete(s.runningTasks, task.ID)
		s.runningMu.Unlock()

		// Release lock
		s.taskFacade.ReleaseLock(taskCtx, task.WorkloadUID, task.TaskType, s.instanceID)
	}()

	// Start heartbeat
	heartbeatCtx, cancelHeartbeat := context.WithCancel(taskCtx)
	defer cancelHeartbeat()

	s.wg.Add(1)
	go s.heartbeatLoop(heartbeatCtx, task)

	// Build execution context
	execCtx := &ExecutionContext{
		Task:       task,
		InstanceID: s.instanceID,
		Cancel:     taskCancel,
	}

	// Execute task
	log.Infof("Executing task %s/%s (executor: %s)",
		task.WorkloadUID, task.TaskType, executor.GetTaskType())

	result, err := executor.Execute(taskCtx, execCtx)

	// Handle execution result
	if err != nil {
		log.Errorf("Task execution failed %s/%s: %v",
			task.WorkloadUID, task.TaskType, err)

		// Update task status
		s.handleTaskFailure(taskCtx, task, err.Error(), result)
	} else if result != nil {
		log.Infof("Task execution result %s/%s: success=%v, status=%s",
			task.WorkloadUID, task.TaskType, result.Success, result.NewStatus)

		// Update task status and ext
		s.handleTaskResult(taskCtx, task, result)
	}
}

// heartbeatLoop heartbeat loop
func (s *TaskScheduler) heartbeatLoop(ctx context.Context, task *model.WorkloadTaskState) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			extended, err := s.taskFacade.ExtendLock(
				ctx,
				task.WorkloadUID,
				task.TaskType,
				s.instanceID,
				s.config.LockDuration,
			)

			if err != nil {
				log.Errorf("Failed to extend lock for task %s/%s: %v",
					task.WorkloadUID, task.TaskType, err)
				return
			}

			if !extended {
				log.Warnf("Failed to extend lock for task %s/%s, another instance may have taken over",
					task.WorkloadUID, task.TaskType)
				return
			}

			log.Debugf("Extended lock for task %s/%s", task.WorkloadUID, task.TaskType)
		}
	}
}

// handleTaskResult handles task execution result
func (s *TaskScheduler) handleTaskResult(ctx context.Context, task *model.WorkloadTaskState, result *ExecutionResult) {
	// Update status
	if result.NewStatus != "" {
		if err := s.taskFacade.UpdateTaskStatus(ctx, task.WorkloadUID, task.TaskType, result.NewStatus); err != nil {
			log.Errorf("Failed to update task status: %v", err)
		}
	}

	// Update ext field
	if len(result.UpdateExt) > 0 {
		if result.Error != "" {
			result.UpdateExt["error"] = result.Error
		}
		if err := s.taskFacade.UpdateTaskExt(ctx, task.WorkloadUID, task.TaskType, model.ExtType(result.UpdateExt)); err != nil {
			log.Errorf("Failed to update task ext: %v", err)
		}
	}
}

// handleTaskFailure handles task failure
func (s *TaskScheduler) handleTaskFailure(ctx context.Context, task *model.WorkloadTaskState, errorMsg string, result *ExecutionResult) {
	updates := model.ExtType{
		"error":      errorMsg,
		"failed_at":  time.Now().Format(time.RFC3339),
		"updated_by": s.instanceID,
	}

	// Merge updates from result
	if result != nil && len(result.UpdateExt) > 0 {
		for k, v := range result.UpdateExt {
			updates[k] = v
		}
	}

	// Update status to failed
	s.taskFacade.UpdateTaskStatus(ctx, task.WorkloadUID, task.TaskType, constant.TaskStatusFailed)
	s.taskFacade.UpdateTaskExt(ctx, task.WorkloadUID, task.TaskType, updates)
}

// cleanupStaleLocks cleans up stale locks
func (s *TaskScheduler) cleanupStaleLocks() error {
	released, err := s.taskFacade.ReleaseStaleLocks(s.ctx)
	if err != nil {
		return err
	}

	if released > 0 {
		log.Infof("Released %d stale locks", released)
	}

	return nil
}

// recoverTasks recovers unfinished tasks
func (s *TaskScheduler) recoverTasks() error {
	tasks, err := s.taskFacade.ListRecoverableTasks(s.ctx)
	if err != nil {
		return fmt.Errorf("failed to list recoverable tasks: %w", err)
	}

	if len(tasks) == 0 {
		log.Info("No tasks to recover")
		return nil
	}

	log.Infof("Found %d recoverable tasks", len(tasks))

	// Reschedule these tasks
	for _, task := range tasks {
		// Reset status to pending
		s.taskFacade.UpdateTaskStatus(s.ctx, task.WorkloadUID, task.TaskType, constant.TaskStatusPending)
		log.Infof("Reset task %s/%s to pending for recovery", task.WorkloadUID, task.TaskType)
	}

	return nil
}

// releaseAllLocks releases all locks
func (s *TaskScheduler) releaseAllLocks() {
	tasks, err := s.taskFacade.ListTasksByStatus(s.ctx, constant.TaskStatusRunning)
	if err != nil {
		log.Errorf("Failed to list running tasks: %v", err)
		return
	}

	for _, task := range tasks {
		if task.LockOwner == s.instanceID {
			if err := s.taskFacade.ReleaseLock(s.ctx, task.WorkloadUID, task.TaskType, s.instanceID); err != nil {
				log.Errorf("Failed to release lock for task %s/%s: %v",
					task.WorkloadUID, task.TaskType, err)
			} else {
				log.Infof("Released lock for task %s/%s", task.WorkloadUID, task.TaskType)
			}
		}
	}
}

// GetRunningTaskCount gets running task count
func (s *TaskScheduler) GetRunningTaskCount() int {
	s.runningMu.RLock()
	defer s.runningMu.RUnlock()
	return len(s.runningTasks)
}

// GetRegisteredExecutors gets registered executor types
func (s *TaskScheduler) GetRegisteredExecutors() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	types := make([]string, 0, len(s.executors))
	for taskType := range s.executors {
		types = append(types, taskType)
	}
	return types
}
