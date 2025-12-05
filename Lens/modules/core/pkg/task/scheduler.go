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

// TaskScheduler 任务调度器
// 负责从数据库拉取任务并分发给对应的执行器
type TaskScheduler struct {
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	instanceID string

	// 数据库 facade
	taskFacade database.WorkloadTaskFacadeInterface

	// 执行器注册表 task_type -> executor
	executors map[string]TaskExecutor
	mu        sync.RWMutex

	// 运行中的任务 task_id -> context
	runningTasks map[int64]context.CancelFunc
	runningMu    sync.RWMutex

	// 配置
	config *SchedulerConfig
}

// SchedulerConfig 调度器配置
type SchedulerConfig struct {
	// 扫描间隔
	ScanInterval time.Duration

	// 锁持有时长
	LockDuration time.Duration

	// 心跳间隔
	HeartbeatInterval time.Duration

	// 最大并发任务数
	MaxConcurrentTasks int

	// 过期锁清理间隔
	StaleLockCleanupInterval time.Duration

	// 是否自动启动
	AutoStart bool
}

// DefaultSchedulerConfig 默认配置
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

// NewTaskScheduler 创建任务调度器
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

// RegisterExecutor 注册任务执行器
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

// GetExecutor 获取执行器
func (s *TaskScheduler) GetExecutor(taskType string) (TaskExecutor, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	executor, exists := s.executors[taskType]
	return executor, exists
}

// Start 启动调度器
func (s *TaskScheduler) Start() error {
	log.Infof("Starting task scheduler (instance: %s)", s.instanceID)

	// 1. 清理过期锁
	if err := s.cleanupStaleLocks(); err != nil {
		log.Warnf("Failed to cleanup stale locks on startup: %v", err)
	}

	// 2. 恢复未完成的任务
	if err := s.recoverTasks(); err != nil {
		log.Warnf("Failed to recover tasks on startup: %v", err)
	}

	// 3. 启动扫描循环
	s.wg.Add(1)
	go s.scanLoop()

	// 4. 启动过期锁清理循环
	s.wg.Add(1)
	go s.staleLockCleanupLoop()

	log.Info("Task scheduler started successfully")
	return nil
}

// Stop 停止调度器
func (s *TaskScheduler) Stop() error {
	log.Info("Stopping task scheduler...")

	// 1. 取消上下文
	s.cancel()

	// 2. 取消所有运行中的任务
	s.runningMu.Lock()
	for taskID, cancelFunc := range s.runningTasks {
		log.Infof("Cancelling task %d", taskID)
		cancelFunc()
	}
	s.runningMu.Unlock()

	// 3. 等待所有 goroutine 完成
	s.wg.Wait()

	// 4. 释放所有锁
	s.releaseAllLocks()

	log.Info("Task scheduler stopped")
	return nil
}

// scanLoop 扫描待执行任务的循环
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

// staleLockCleanupLoop 清理过期锁的循环
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

// scanAndExecuteTasks 扫描并执行任务
func (s *TaskScheduler) scanAndExecuteTasks() {
	// 检查当前运行任务数
	s.runningMu.RLock()
	runningCount := len(s.runningTasks)
	s.runningMu.RUnlock()

	if runningCount >= s.config.MaxConcurrentTasks {
		log.Debugf("Max concurrent tasks reached (%d), skipping scan", s.config.MaxConcurrentTasks)
		return
	}

	// 查询待执行的任务
	tasks, err := s.taskFacade.ListTasksByStatus(s.ctx, constant.TaskStatusPending)
	if err != nil {
		log.Errorf("Failed to list pending tasks: %v", err)
		return
	}

	if len(tasks) == 0 {
		return
	}

	log.Debugf("Found %d pending tasks", len(tasks))

	// 尝试执行每个任务
	for _, task := range tasks {
		// 检查是否达到最大并发数
		s.runningMu.RLock()
		runningCount := len(s.runningTasks)
		s.runningMu.RUnlock()

		if runningCount >= s.config.MaxConcurrentTasks {
			log.Debugf("Max concurrent tasks reached, stopping scan")
			break
		}

		// 尝试获取锁并执行任务
		s.tryExecuteTask(task)
	}
}

// tryExecuteTask 尝试获取锁并执行任务
func (s *TaskScheduler) tryExecuteTask(task *model.WorkloadTaskState) {
	// 1. 尝试获取锁
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

	// 2. 获取执行器
	executor, exists := s.GetExecutor(task.TaskType)
	if !exists {
		log.Warnf("No executor registered for task type: %s", task.TaskType)
		// 释放锁
		s.taskFacade.ReleaseLock(s.ctx, task.WorkloadUID, task.TaskType, s.instanceID)
		return
	}

	// 3. 验证任务参数
	if err := executor.Validate(task); err != nil {
		log.Errorf("Task validation failed for %s/%s: %v",
			task.WorkloadUID, task.TaskType, err)
		// 更新任务状态为失败
		s.taskFacade.UpdateTaskStatus(s.ctx, task.WorkloadUID, task.TaskType, constant.TaskStatusFailed)
		s.taskFacade.UpdateTaskExt(s.ctx, task.WorkloadUID, task.TaskType, model.ExtType{
			"error": fmt.Sprintf("validation failed: %v", err),
		})
		s.taskFacade.ReleaseLock(s.ctx, task.WorkloadUID, task.TaskType, s.instanceID)
		return
	}

	log.Infof("Acquired lock for task %s/%s, starting execution",
		task.WorkloadUID, task.TaskType)

	// 4. 启动任务执行
	s.wg.Add(1)
	go s.executeTask(task, executor)
}

// executeTask 执行任务
func (s *TaskScheduler) executeTask(task *model.WorkloadTaskState, executor TaskExecutor) {
	defer s.wg.Done()

	// 创建任务专用的 context
	taskCtx, taskCancel := context.WithCancel(s.ctx)
	defer taskCancel()

	// 注册到运行任务列表
	s.runningMu.Lock()
	s.runningTasks[task.ID] = taskCancel
	s.runningMu.Unlock()

	defer func() {
		s.runningMu.Lock()
		delete(s.runningTasks, task.ID)
		s.runningMu.Unlock()

		// 释放锁
		s.taskFacade.ReleaseLock(taskCtx, task.WorkloadUID, task.TaskType, s.instanceID)
	}()

	// 启动心跳
	heartbeatCtx, cancelHeartbeat := context.WithCancel(taskCtx)
	defer cancelHeartbeat()

	s.wg.Add(1)
	go s.heartbeatLoop(heartbeatCtx, task)

	// 构建执行上下文
	execCtx := &ExecutionContext{
		Task:       task,
		InstanceID: s.instanceID,
		Cancel:     taskCancel,
	}

	// 执行任务
	log.Infof("Executing task %s/%s (executor: %s)",
		task.WorkloadUID, task.TaskType, executor.GetTaskType())

	result, err := executor.Execute(taskCtx, execCtx)

	// 处理执行结果
	if err != nil {
		log.Errorf("Task execution failed %s/%s: %v",
			task.WorkloadUID, task.TaskType, err)

		// 更新任务状态
		s.handleTaskFailure(taskCtx, task, err.Error(), result)
	} else if result != nil {
		log.Infof("Task execution result %s/%s: success=%v, status=%s",
			task.WorkloadUID, task.TaskType, result.Success, result.NewStatus)

		// 更新任务状态和 ext
		s.handleTaskResult(taskCtx, task, result)
	}
}

// heartbeatLoop 心跳循环
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

// handleTaskResult 处理任务执行结果
func (s *TaskScheduler) handleTaskResult(ctx context.Context, task *model.WorkloadTaskState, result *ExecutionResult) {
	// 更新状态
	if result.NewStatus != "" {
		if err := s.taskFacade.UpdateTaskStatus(ctx, task.WorkloadUID, task.TaskType, result.NewStatus); err != nil {
			log.Errorf("Failed to update task status: %v", err)
		}
	}

	// 更新 ext 字段
	if len(result.UpdateExt) > 0 {
		if result.Error != "" {
			result.UpdateExt["error"] = result.Error
		}
		if err := s.taskFacade.UpdateTaskExt(ctx, task.WorkloadUID, task.TaskType, model.ExtType(result.UpdateExt)); err != nil {
			log.Errorf("Failed to update task ext: %v", err)
		}
	}
}

// handleTaskFailure 处理任务失败
func (s *TaskScheduler) handleTaskFailure(ctx context.Context, task *model.WorkloadTaskState, errorMsg string, result *ExecutionResult) {
	updates := model.ExtType{
		"error":      errorMsg,
		"failed_at":  time.Now().Format(time.RFC3339),
		"updated_by": s.instanceID,
	}

	// 合并 result 中的更新
	if result != nil && len(result.UpdateExt) > 0 {
		for k, v := range result.UpdateExt {
			updates[k] = v
		}
	}

	// 更新状态为失败
	s.taskFacade.UpdateTaskStatus(ctx, task.WorkloadUID, task.TaskType, constant.TaskStatusFailed)
	s.taskFacade.UpdateTaskExt(ctx, task.WorkloadUID, task.TaskType, updates)
}

// cleanupStaleLocks 清理过期锁
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

// recoverTasks 恢复未完成的任务
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

	// 重新调度这些任务
	for _, task := range tasks {
		// 重置状态为 pending
		s.taskFacade.UpdateTaskStatus(s.ctx, task.WorkloadUID, task.TaskType, constant.TaskStatusPending)
		log.Infof("Reset task %s/%s to pending for recovery", task.WorkloadUID, task.TaskType)
	}

	return nil
}

// releaseAllLocks 释放所有锁
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

// GetRunningTaskCount 获取运行中的任务数量
func (s *TaskScheduler) GetRunningTaskCount() int {
	s.runningMu.RLock()
	defer s.runningMu.RUnlock()
	return len(s.runningTasks)
}

// GetRegisteredExecutors 获取已注册的执行器类型
func (s *TaskScheduler) GetRegisteredExecutors() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	types := make([]string, 0, len(s.executors))
	for taskType := range s.executors {
		types = append(types, taskType)
	}
	return types
}
