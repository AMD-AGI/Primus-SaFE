// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package task

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// ============ Mock Objects ============

// MockWorkloadTaskFacade mocks the task facade interface
type MockWorkloadTaskFacade struct {
	mock.Mock
}

func (m *MockWorkloadTaskFacade) UpsertTask(ctx context.Context, task *model.WorkloadTaskState) error {
	args := m.Called(ctx, task)
	return args.Error(0)
}

func (m *MockWorkloadTaskFacade) GetTask(ctx context.Context, workloadUID, taskType string) (*model.WorkloadTaskState, error) {
	args := m.Called(ctx, workloadUID, taskType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.WorkloadTaskState), args.Error(1)
}

func (m *MockWorkloadTaskFacade) DeleteTask(ctx context.Context, workloadUID, taskType string) error {
	args := m.Called(ctx, workloadUID, taskType)
	return args.Error(0)
}

func (m *MockWorkloadTaskFacade) ListTasksByWorkload(ctx context.Context, workloadUID string) ([]*model.WorkloadTaskState, error) {
	args := m.Called(ctx, workloadUID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.WorkloadTaskState), args.Error(1)
}

func (m *MockWorkloadTaskFacade) ListTasksByStatus(ctx context.Context, status string) ([]*model.WorkloadTaskState, error) {
	args := m.Called(ctx, status)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.WorkloadTaskState), args.Error(1)
}

func (m *MockWorkloadTaskFacade) ListRecoverableTasks(ctx context.Context) ([]*model.WorkloadTaskState, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.WorkloadTaskState), args.Error(1)
}

func (m *MockWorkloadTaskFacade) UpdateTaskStatus(ctx context.Context, workloadUID, taskType, status string) error {
	args := m.Called(ctx, workloadUID, taskType, status)
	return args.Error(0)
}

func (m *MockWorkloadTaskFacade) UpdateTaskExt(ctx context.Context, workloadUID, taskType string, extData model.ExtType) error {
	args := m.Called(ctx, workloadUID, taskType, extData)
	return args.Error(0)
}

func (m *MockWorkloadTaskFacade) TryAcquireLock(ctx context.Context, workloadUID, taskType, lockOwner string, lockDuration time.Duration) (bool, error) {
	args := m.Called(ctx, workloadUID, taskType, lockOwner, lockDuration)
	return args.Bool(0), args.Error(1)
}

func (m *MockWorkloadTaskFacade) ExtendLock(ctx context.Context, workloadUID, taskType, lockOwner string, lockDuration time.Duration) (bool, error) {
	args := m.Called(ctx, workloadUID, taskType, lockOwner, lockDuration)
	return args.Bool(0), args.Error(1)
}

func (m *MockWorkloadTaskFacade) ReleaseLock(ctx context.Context, workloadUID, taskType, lockOwner string) error {
	args := m.Called(ctx, workloadUID, taskType, lockOwner)
	return args.Error(0)
}

func (m *MockWorkloadTaskFacade) ReleaseStaleLocks(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockWorkloadTaskFacade) CleanupOldTasks(ctx context.Context, retentionDays int) (int64, error) {
	args := m.Called(ctx, retentionDays)
	return args.Get(0).(int64), args.Error(1)
}

// MockTaskExecutor mocks the task executor interface
type MockTaskExecutor struct {
	mock.Mock
	taskType string
}

func NewMockTaskExecutor(taskType string) *MockTaskExecutor {
	return &MockTaskExecutor{taskType: taskType}
}

func (m *MockTaskExecutor) GetTaskType() string {
	return m.taskType
}

func (m *MockTaskExecutor) Execute(ctx context.Context, execCtx *ExecutionContext) (*ExecutionResult, error) {
	args := m.Called(ctx, execCtx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ExecutionResult), args.Error(1)
}

func (m *MockTaskExecutor) Cancel(ctx context.Context, task *model.WorkloadTaskState) error {
	args := m.Called(ctx, task)
	return args.Error(0)
}

func (m *MockTaskExecutor) Validate(task *model.WorkloadTaskState) error {
	args := m.Called(task)
	return args.Error(0)
}

// ============ Test Helpers ============

func createTestTask(workloadUID, taskType string) *model.WorkloadTaskState {
	return &model.WorkloadTaskState{
		ID:          1,
		WorkloadUID: workloadUID,
		TaskType:    taskType,
		Status:      constant.TaskStatusPending,
		Ext:         model.ExtType{},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

func createTestScheduler(facade *MockWorkloadTaskFacade) *TaskScheduler {
	config := &SchedulerConfig{
		ScanInterval:             100 * time.Millisecond,
		LockDuration:             1 * time.Minute,
		HeartbeatInterval:        10 * time.Second, // Long interval to avoid heartbeat during tests
		MaxConcurrentTasks:       5,
		StaleLockCleanupInterval: 200 * time.Millisecond,
		AutoStart:                false,
	}

	ctx, cancel := context.WithCancel(context.Background())
	scheduler := &TaskScheduler{
		ctx:          ctx,
		cancel:       cancel,
		instanceID:   "test-instance",
		taskFacade:   facade,
		executors:    make(map[string]TaskExecutor),
		runningTasks: make(map[int64]context.CancelFunc),
		config:       config,
	}

	return scheduler
}

// waitForTasks waits for all running tasks to complete with a timeout
func waitForTasks(scheduler *TaskScheduler, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		scheduler.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// ============ Basic Tests ============

func TestDefaultSchedulerConfig(t *testing.T) {
	config := DefaultSchedulerConfig()

	assert.NotNil(t, config)
	assert.Equal(t, 10*time.Second, config.ScanInterval)
	assert.Equal(t, 5*time.Minute, config.LockDuration)
	assert.Equal(t, 30*time.Second, config.HeartbeatInterval)
	assert.Equal(t, 20, config.MaxConcurrentTasks)
	assert.Equal(t, 1*time.Minute, config.StaleLockCleanupInterval)
	assert.True(t, config.AutoStart)
}

func TestNewTaskScheduler(t *testing.T) {
	t.Run("with custom config", func(t *testing.T) {
		config := &SchedulerConfig{
			ScanInterval:       5 * time.Second,
			MaxConcurrentTasks: 20,
		}

		scheduler := NewTaskScheduler("test-instance", config)

		assert.NotNil(t, scheduler)
		assert.Equal(t, "test-instance", scheduler.instanceID)
		assert.Equal(t, config, scheduler.config)
		assert.NotNil(t, scheduler.executors)
		assert.NotNil(t, scheduler.runningTasks)
	})

	t.Run("with nil config", func(t *testing.T) {
		scheduler := NewTaskScheduler("test-instance", nil)

		assert.NotNil(t, scheduler)
		assert.NotNil(t, scheduler.config)
		assert.Equal(t, 10*time.Second, scheduler.config.ScanInterval)
	})
}

// ============ Executor Registration Tests ============

func TestRegisterExecutor(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)
		executor := NewMockTaskExecutor("test-task")

		err := scheduler.RegisterExecutor(executor)

		assert.NoError(t, err)
		assert.Len(t, scheduler.executors, 1)
	})

	t.Run("duplicate registration", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)
		executor1 := NewMockTaskExecutor("test-task")
		executor2 := NewMockTaskExecutor("test-task")

		err1 := scheduler.RegisterExecutor(executor1)
		err2 := scheduler.RegisterExecutor(executor2)

		assert.NoError(t, err1)
		assert.Error(t, err2)
		assert.Contains(t, err2.Error(), "already registered")
	})
}

func TestGetExecutor(t *testing.T) {
	t.Run("executor exists", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)
		executor := NewMockTaskExecutor("test-task")
		scheduler.RegisterExecutor(executor)

		result, exists := scheduler.GetExecutor("test-task")

		assert.True(t, exists)
		assert.Equal(t, executor, result)
	})

	t.Run("executor not found", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		result, exists := scheduler.GetExecutor("non-existent")

		assert.False(t, exists)
		assert.Nil(t, result)
	})
}

func TestGetRegisteredExecutors(t *testing.T) {
	mockFacade := new(MockWorkloadTaskFacade)
	scheduler := createTestScheduler(mockFacade)

	executor1 := NewMockTaskExecutor("task-type-1")
	executor2 := NewMockTaskExecutor("task-type-2")

	scheduler.RegisterExecutor(executor1)
	scheduler.RegisterExecutor(executor2)

	types := scheduler.GetRegisteredExecutors()

	assert.Len(t, types, 2)
	assert.Contains(t, types, "task-type-1")
	assert.Contains(t, types, "task-type-2")
}

// ============ Task Execution Tests ============

func TestTryExecuteTask(t *testing.T) {
	t.Run("success - acquire lock and execute", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		task := createTestTask("workload-1", "test-task")
		executor := NewMockTaskExecutor("test-task")
		scheduler.RegisterExecutor(executor)

		mockFacade.On("TryAcquireLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID, scheduler.config.LockDuration).
			Return(true, nil)
		executor.On("Validate", task).Return(nil)
		executor.On("Execute", mock.Anything, mock.Anything).
			Return(SuccessResult(map[string]interface{}{"completed": true}), nil)
		mockFacade.On("UpdateTaskStatus", mock.Anything, task.WorkloadUID, task.TaskType, constant.TaskStatusCompleted).
			Return(nil)
		mockFacade.On("UpdateTaskExt", mock.Anything, task.WorkloadUID, task.TaskType, mock.Anything).
			Return(nil)
		mockFacade.On("ExtendLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID, scheduler.config.LockDuration).
			Return(true, nil).Maybe()
		mockFacade.On("ReleaseLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID).
			Return(nil)

		scheduler.tryExecuteTask(task)

		time.Sleep(100 * time.Millisecond)

		mockFacade.AssertCalled(t, "TryAcquireLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID, scheduler.config.LockDuration)
		executor.AssertCalled(t, "Validate", task)
	})

	t.Run("lock acquisition failed", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		task := createTestTask("workload-1", "test-task")

		mockFacade.On("TryAcquireLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID, scheduler.config.LockDuration).
			Return(false, nil)

		scheduler.tryExecuteTask(task)

		mockFacade.AssertCalled(t, "TryAcquireLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID, scheduler.config.LockDuration)
		mockFacade.AssertNotCalled(t, "UpdateTaskStatus")
	})

	t.Run("lock acquisition error", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		task := createTestTask("workload-1", "test-task")

		mockFacade.On("TryAcquireLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID, scheduler.config.LockDuration).
			Return(false, errors.New("database error"))

		scheduler.tryExecuteTask(task)

		mockFacade.AssertCalled(t, "TryAcquireLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID, scheduler.config.LockDuration)
	})

	t.Run("no executor registered", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		task := createTestTask("workload-1", "unknown-task")

		mockFacade.On("TryAcquireLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID, scheduler.config.LockDuration).
			Return(true, nil)
		mockFacade.On("ReleaseLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID).
			Return(nil)

		scheduler.tryExecuteTask(task)

		time.Sleep(50 * time.Millisecond)

		mockFacade.AssertCalled(t, "ReleaseLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID)
	})

	t.Run("validation failed", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		task := createTestTask("workload-1", "test-task")
		executor := NewMockTaskExecutor("test-task")
		scheduler.RegisterExecutor(executor)

		mockFacade.On("TryAcquireLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID, scheduler.config.LockDuration).
			Return(true, nil)
		executor.On("Validate", task).Return(errors.New("invalid parameters"))
		mockFacade.On("UpdateTaskStatus", mock.Anything, task.WorkloadUID, task.TaskType, constant.TaskStatusFailed).
			Return(nil)
		mockFacade.On("UpdateTaskExt", mock.Anything, task.WorkloadUID, task.TaskType, mock.Anything).
			Return(nil)
		mockFacade.On("ReleaseLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID).
			Return(nil)

		scheduler.tryExecuteTask(task)

		time.Sleep(50 * time.Millisecond)

		mockFacade.AssertCalled(t, "UpdateTaskStatus", mock.Anything, task.WorkloadUID, task.TaskType, constant.TaskStatusFailed)
		executor.AssertNotCalled(t, "Execute")
	})
}

func TestExecuteTask(t *testing.T) {
	t.Run("successful execution", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		task := createTestTask("workload-1", "test-task")
		executor := NewMockTaskExecutor("test-task")

		result := SuccessResult(map[string]interface{}{"result": "success"})
		executor.On("Execute", mock.Anything, mock.Anything).Return(result, nil)

		mockFacade.On("UpdateTaskStatus", mock.Anything, task.WorkloadUID, task.TaskType, constant.TaskStatusCompleted).
			Return(nil)
		mockFacade.On("UpdateTaskExt", mock.Anything, task.WorkloadUID, task.TaskType, mock.Anything).
			Return(nil)
		mockFacade.On("ExtendLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID, scheduler.config.LockDuration).
			Return(true, nil).Maybe()
		mockFacade.On("ReleaseLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID).
			Return(nil)

		scheduler.wg.Add(1)
		go scheduler.executeTask(task, executor)

		// Wait for task to complete
		completed := waitForTasks(scheduler, 500*time.Millisecond)
		assert.True(t, completed, "task should complete within timeout")

		executor.AssertCalled(t, "Execute", mock.Anything, mock.Anything)
		mockFacade.AssertCalled(t, "UpdateTaskStatus", mock.Anything, task.WorkloadUID, task.TaskType, constant.TaskStatusCompleted)
	})

	t.Run("execution failure", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		task := createTestTask("workload-1", "test-task")
		executor := NewMockTaskExecutor("test-task")

		executor.On("Execute", mock.Anything, mock.Anything).Return(nil, errors.New("execution failed"))

		mockFacade.On("UpdateTaskStatus", mock.Anything, task.WorkloadUID, task.TaskType, constant.TaskStatusFailed).
			Return(nil)
		mockFacade.On("UpdateTaskExt", mock.Anything, task.WorkloadUID, task.TaskType, mock.Anything).
			Return(nil)
		mockFacade.On("ExtendLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID, scheduler.config.LockDuration).
			Return(true, nil).Maybe()
		mockFacade.On("ReleaseLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID).
			Return(nil)

		scheduler.wg.Add(1)
		go scheduler.executeTask(task, executor)

		// Wait for task to complete
		completed := waitForTasks(scheduler, 500*time.Millisecond)
		assert.True(t, completed, "task should complete within timeout")

		mockFacade.AssertCalled(t, "UpdateTaskStatus", mock.Anything, task.WorkloadUID, task.TaskType, constant.TaskStatusFailed)
	})
}

// ============ Scan and Execute Tests ============

func TestScanAndExecuteTasks(t *testing.T) {
	t.Run("no pending tasks", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		mockFacade.On("ListTasksByStatus", mock.Anything, constant.TaskStatusPending).
			Return([]*model.WorkloadTaskState{}, nil)

		scheduler.scanAndExecuteTasks()

		mockFacade.AssertCalled(t, "ListTasksByStatus", mock.Anything, constant.TaskStatusPending)
	})

	t.Run("max concurrent tasks reached", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)
		scheduler.config.MaxConcurrentTasks = 1

		scheduler.runningMu.Lock()
		scheduler.runningTasks[1] = func() {}
		scheduler.runningMu.Unlock()

		scheduler.scanAndExecuteTasks()

		mockFacade.AssertNotCalled(t, "ListTasksByStatus")
	})

	t.Run("list tasks error", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		mockFacade.On("ListTasksByStatus", mock.Anything, constant.TaskStatusPending).
			Return([]*model.WorkloadTaskState(nil), errors.New("database error"))

		scheduler.scanAndExecuteTasks()

		mockFacade.AssertCalled(t, "ListTasksByStatus", mock.Anything, constant.TaskStatusPending)
	})

	t.Run("execute multiple tasks", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)
		scheduler.config.MaxConcurrentTasks = 2

		task1 := createTestTask("workload-1", "test-task")
		task2 := createTestTask("workload-2", "test-task")
		tasks := []*model.WorkloadTaskState{task1, task2}

		executor := NewMockTaskExecutor("test-task")
		scheduler.RegisterExecutor(executor)

		mockFacade.On("ListTasksByStatus", mock.Anything, constant.TaskStatusPending).
			Return(tasks, nil)
		mockFacade.On("TryAcquireLock", mock.Anything, mock.Anything, mock.Anything, scheduler.instanceID, scheduler.config.LockDuration).
			Return(true, nil)
		executor.On("Validate", mock.Anything).Return(nil)
		executor.On("Execute", mock.Anything, mock.Anything).
			Return(SuccessResult(map[string]interface{}{}), nil)
		mockFacade.On("UpdateTaskStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		mockFacade.On("UpdateTaskExt", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		mockFacade.On("ExtendLock", mock.Anything, mock.Anything, mock.Anything, scheduler.instanceID, scheduler.config.LockDuration).
			Return(true, nil).Maybe()
		mockFacade.On("ReleaseLock", mock.Anything, mock.Anything, mock.Anything, scheduler.instanceID).
			Return(nil)

		scheduler.scanAndExecuteTasks()

		// Wait for tasks to complete
		completed := waitForTasks(scheduler, 500*time.Millisecond)
		assert.True(t, completed, "tasks should complete within timeout")

		mockFacade.AssertCalled(t, "TryAcquireLock", mock.Anything, task1.WorkloadUID, task1.TaskType, scheduler.instanceID, scheduler.config.LockDuration)
	})
}

// ============ Lock Management Tests ============

func TestCleanupStaleLocks(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		mockFacade.On("ReleaseStaleLocks", mock.Anything).Return(int64(3), nil)

		err := scheduler.cleanupStaleLocks()

		assert.NoError(t, err)
		mockFacade.AssertCalled(t, "ReleaseStaleLocks", mock.Anything)
	})

	t.Run("error", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		mockFacade.On("ReleaseStaleLocks", mock.Anything).Return(int64(0), errors.New("database error"))

		err := scheduler.cleanupStaleLocks()

		assert.Error(t, err)
	})
}

func TestReleaseAllLocks(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		task1 := createTestTask("workload-1", "test-task")
		task1.LockOwner = scheduler.instanceID
		task2 := createTestTask("workload-2", "test-task")
		task2.LockOwner = "other-instance"

		tasks := []*model.WorkloadTaskState{task1, task2}

		mockFacade.On("ListTasksByStatus", mock.Anything, constant.TaskStatusRunning).
			Return(tasks, nil)
		mockFacade.On("ReleaseLock", mock.Anything, task1.WorkloadUID, task1.TaskType, scheduler.instanceID).
			Return(nil)

		scheduler.releaseAllLocks()

		mockFacade.AssertCalled(t, "ReleaseLock", mock.Anything, task1.WorkloadUID, task1.TaskType, scheduler.instanceID)
		mockFacade.AssertNotCalled(t, "ReleaseLock", mock.Anything, task2.WorkloadUID, task2.TaskType, scheduler.instanceID)
	})

	t.Run("list tasks error", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		mockFacade.On("ListTasksByStatus", mock.Anything, constant.TaskStatusRunning).
			Return([]*model.WorkloadTaskState(nil), errors.New("database error"))

		scheduler.releaseAllLocks()

		mockFacade.AssertCalled(t, "ListTasksByStatus", mock.Anything, constant.TaskStatusRunning)
	})

	t.Run("release lock error", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		task := createTestTask("workload-1", "test-task")
		task.LockOwner = scheduler.instanceID

		mockFacade.On("ListTasksByStatus", mock.Anything, constant.TaskStatusRunning).
			Return([]*model.WorkloadTaskState{task}, nil)
		mockFacade.On("ReleaseLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID).
			Return(errors.New("release error"))

		scheduler.releaseAllLocks()

		mockFacade.AssertCalled(t, "ReleaseLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID)
	})
}

// ============ Task Recovery Tests ============

func TestRecoverTasks(t *testing.T) {
	t.Run("no tasks to recover", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		mockFacade.On("ListRecoverableTasks", mock.Anything).
			Return([]*model.WorkloadTaskState{}, nil)

		err := scheduler.recoverTasks()

		assert.NoError(t, err)
		mockFacade.AssertCalled(t, "ListRecoverableTasks", mock.Anything)
	})

	t.Run("recover tasks successfully", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		task1 := createTestTask("workload-1", "test-task")
		task1.Status = constant.TaskStatusRunning
		task2 := createTestTask("workload-2", "test-task")
		task2.Status = constant.TaskStatusRunning

		tasks := []*model.WorkloadTaskState{task1, task2}

		mockFacade.On("ListRecoverableTasks", mock.Anything).
			Return(tasks, nil)
		mockFacade.On("UpdateTaskStatus", mock.Anything, mock.Anything, mock.Anything, constant.TaskStatusPending).
			Return(nil)

		err := scheduler.recoverTasks()

		assert.NoError(t, err)
		mockFacade.AssertNumberOfCalls(t, "UpdateTaskStatus", 2)
	})

	t.Run("list recoverable tasks error", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		mockFacade.On("ListRecoverableTasks", mock.Anything).
			Return([]*model.WorkloadTaskState(nil), errors.New("database error"))

		err := scheduler.recoverTasks()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list recoverable tasks")
	})
}

// ============ Task Result Handling Tests ============

func TestHandleTaskResult(t *testing.T) {
	t.Run("update status only", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		task := createTestTask("workload-1", "test-task")
		result := &ExecutionResult{
			Success:   true,
			NewStatus: constant.TaskStatusCompleted,
		}

		mockFacade.On("UpdateTaskStatus", mock.Anything, task.WorkloadUID, task.TaskType, constant.TaskStatusCompleted).
			Return(nil)

		scheduler.handleTaskResult(context.Background(), task, result)

		mockFacade.AssertCalled(t, "UpdateTaskStatus", mock.Anything, task.WorkloadUID, task.TaskType, constant.TaskStatusCompleted)
		mockFacade.AssertNotCalled(t, "UpdateTaskExt")
	})

	t.Run("update status and ext", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		task := createTestTask("workload-1", "test-task")
		result := &ExecutionResult{
			Success:   true,
			NewStatus: constant.TaskStatusCompleted,
			UpdateExt: map[string]interface{}{
				"result": "success",
			},
		}

		mockFacade.On("UpdateTaskStatus", mock.Anything, task.WorkloadUID, task.TaskType, constant.TaskStatusCompleted).
			Return(nil)
		mockFacade.On("UpdateTaskExt", mock.Anything, task.WorkloadUID, task.TaskType, mock.Anything).
			Return(nil)

		scheduler.handleTaskResult(context.Background(), task, result)

		mockFacade.AssertCalled(t, "UpdateTaskStatus", mock.Anything, task.WorkloadUID, task.TaskType, constant.TaskStatusCompleted)
		mockFacade.AssertCalled(t, "UpdateTaskExt", mock.Anything, task.WorkloadUID, task.TaskType, mock.Anything)
	})

	t.Run("update status error", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		task := createTestTask("workload-1", "test-task")
		result := &ExecutionResult{
			Success:   true,
			NewStatus: constant.TaskStatusCompleted,
		}

		mockFacade.On("UpdateTaskStatus", mock.Anything, task.WorkloadUID, task.TaskType, constant.TaskStatusCompleted).
			Return(errors.New("update error"))

		scheduler.handleTaskResult(context.Background(), task, result)

		mockFacade.AssertCalled(t, "UpdateTaskStatus", mock.Anything, task.WorkloadUID, task.TaskType, constant.TaskStatusCompleted)
	})

	t.Run("update ext with error message", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		task := createTestTask("workload-1", "test-task")
		result := &ExecutionResult{
			Success:   false,
			Error:     "execution error",
			NewStatus: constant.TaskStatusFailed,
			UpdateExt: map[string]interface{}{
				"attempts": 3,
			},
		}

		mockFacade.On("UpdateTaskStatus", mock.Anything, task.WorkloadUID, task.TaskType, constant.TaskStatusFailed).
			Return(nil)
		mockFacade.On("UpdateTaskExt", mock.Anything, task.WorkloadUID, task.TaskType, mock.Anything).
			Return(nil)

		scheduler.handleTaskResult(context.Background(), task, result)

		mockFacade.AssertCalled(t, "UpdateTaskExt", mock.Anything, task.WorkloadUID, task.TaskType, mock.Anything)
	})
}

func TestHandleTaskFailure(t *testing.T) {
	t.Run("basic failure", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		task := createTestTask("workload-1", "test-task")

		mockFacade.On("UpdateTaskStatus", mock.Anything, task.WorkloadUID, task.TaskType, constant.TaskStatusFailed).
			Return(nil)
		mockFacade.On("UpdateTaskExt", mock.Anything, task.WorkloadUID, task.TaskType, mock.Anything).
			Return(nil)

		scheduler.handleTaskFailure(context.Background(), task, "execution failed", nil)

		mockFacade.AssertCalled(t, "UpdateTaskStatus", mock.Anything, task.WorkloadUID, task.TaskType, constant.TaskStatusFailed)
		mockFacade.AssertCalled(t, "UpdateTaskExt", mock.Anything, task.WorkloadUID, task.TaskType, mock.Anything)
	})

	t.Run("failure with result", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		task := createTestTask("workload-1", "test-task")
		result := &ExecutionResult{
			UpdateExt: map[string]interface{}{
				"attempts": 3,
				"last_error_code": 500,
			},
		}

		mockFacade.On("UpdateTaskStatus", mock.Anything, task.WorkloadUID, task.TaskType, constant.TaskStatusFailed).
			Return(nil)
		mockFacade.On("UpdateTaskExt", mock.Anything, task.WorkloadUID, task.TaskType, mock.Anything).
			Return(nil)

		scheduler.handleTaskFailure(context.Background(), task, "execution failed", result)

		mockFacade.AssertCalled(t, "UpdateTaskExt", mock.Anything, task.WorkloadUID, task.TaskType, mock.Anything)
	})
}

// ============ Heartbeat Tests ============

func TestHeartbeatLoop(t *testing.T) {
	t.Run("extends lock successfully", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)
		scheduler.config.HeartbeatInterval = 50 * time.Millisecond // Short interval for testing

		task := createTestTask("workload-1", "test-task")

		mockFacade.On("ExtendLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID, scheduler.config.LockDuration).
			Return(true, nil).Maybe()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		scheduler.wg.Add(1)
		go scheduler.heartbeatLoop(ctx, task)

		time.Sleep(100 * time.Millisecond)
		cancel()
		scheduler.wg.Wait()

		mockFacade.AssertCalled(t, "ExtendLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID, scheduler.config.LockDuration)
	})

	t.Run("extend lock fails", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)
		scheduler.config.HeartbeatInterval = 50 * time.Millisecond

		task := createTestTask("workload-1", "test-task")

		mockFacade.On("ExtendLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID, scheduler.config.LockDuration).
			Return(false, nil).Once()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		scheduler.wg.Add(1)
		go scheduler.heartbeatLoop(ctx, task)

		time.Sleep(100 * time.Millisecond)
		scheduler.wg.Wait()

		mockFacade.AssertCalled(t, "ExtendLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID, scheduler.config.LockDuration)
	})

	t.Run("extend lock error", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)
		scheduler.config.HeartbeatInterval = 50 * time.Millisecond

		task := createTestTask("workload-1", "test-task")

		mockFacade.On("ExtendLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID, scheduler.config.LockDuration).
			Return(false, errors.New("database error")).Once()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		scheduler.wg.Add(1)
		go scheduler.heartbeatLoop(ctx, task)

		time.Sleep(100 * time.Millisecond)
		scheduler.wg.Wait()

		mockFacade.AssertCalled(t, "ExtendLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID, scheduler.config.LockDuration)
	})
}

// ============ Start/Stop Tests ============

func TestStart(t *testing.T) {
	t.Run("start successfully", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		mockFacade.On("ReleaseStaleLocks", mock.Anything).Return(int64(0), nil)
		mockFacade.On("ListRecoverableTasks", mock.Anything).
			Return([]*model.WorkloadTaskState{}, nil)
		mockFacade.On("ListTasksByStatus", mock.Anything, mock.Anything).
			Return([]*model.WorkloadTaskState{}, nil).Maybe()

		err := scheduler.Start()
		assert.NoError(t, err)

		time.Sleep(50 * time.Millisecond)

		scheduler.Stop()

		mockFacade.AssertCalled(t, "ReleaseStaleLocks", mock.Anything)
		mockFacade.AssertCalled(t, "ListRecoverableTasks", mock.Anything)
	})

	t.Run("cleanup stale locks error on startup", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		mockFacade.On("ReleaseStaleLocks", mock.Anything).Return(int64(0), errors.New("cleanup error"))
		mockFacade.On("ListRecoverableTasks", mock.Anything).
			Return([]*model.WorkloadTaskState{}, nil)
		mockFacade.On("ListTasksByStatus", mock.Anything, mock.Anything).
			Return([]*model.WorkloadTaskState{}, nil).Maybe()

		err := scheduler.Start()
		assert.NoError(t, err)

		time.Sleep(50 * time.Millisecond)

		scheduler.Stop()
	})

	t.Run("recover tasks error on startup", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		mockFacade.On("ReleaseStaleLocks", mock.Anything).Return(int64(0), nil)
		mockFacade.On("ListRecoverableTasks", mock.Anything).
			Return([]*model.WorkloadTaskState(nil), errors.New("recovery error"))
		mockFacade.On("ListTasksByStatus", mock.Anything, mock.Anything).
			Return([]*model.WorkloadTaskState{}, nil).Maybe()

		err := scheduler.Start()
		assert.NoError(t, err)

		time.Sleep(50 * time.Millisecond)

		scheduler.Stop()
	})
}

func TestStop(t *testing.T) {
	t.Run("stop successfully", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		mockFacade.On("ReleaseStaleLocks", mock.Anything).Return(int64(0), nil).Maybe()
		mockFacade.On("ListRecoverableTasks", mock.Anything).
			Return([]*model.WorkloadTaskState{}, nil).Maybe()
		mockFacade.On("ListTasksByStatus", mock.Anything, mock.Anything).
			Return([]*model.WorkloadTaskState{}, nil).Maybe()

		scheduler.Start()
		time.Sleep(50 * time.Millisecond)

		err := scheduler.Stop()

		assert.NoError(t, err)
		mockFacade.AssertCalled(t, "ListTasksByStatus", mock.Anything, constant.TaskStatusRunning)
	})

	t.Run("stop with running tasks", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		task := createTestTask("workload-1", "test-task")
		task.LockOwner = scheduler.instanceID

		mockFacade.On("ListTasksByStatus", mock.Anything, constant.TaskStatusRunning).
			Return([]*model.WorkloadTaskState{task}, nil)
		mockFacade.On("ReleaseLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID).
			Return(nil)

		// Add a running task
		cancelCalled := false
		cancelFunc := func() { cancelCalled = true }
		scheduler.runningMu.Lock()
		scheduler.runningTasks[task.ID] = cancelFunc
		scheduler.runningMu.Unlock()

		err := scheduler.Stop()

		assert.NoError(t, err)
		assert.True(t, cancelCalled, "cancel function should have been called")
		
		// Verify the release lock was attempted
		mockFacade.AssertExpectations(t)
	})
}

// ============ Utility Tests ============

func TestGetRunningTaskCount(t *testing.T) {
	mockFacade := new(MockWorkloadTaskFacade)
	scheduler := createTestScheduler(mockFacade)

	assert.Equal(t, 0, scheduler.GetRunningTaskCount())

	scheduler.runningMu.Lock()
	scheduler.runningTasks[1] = func() {}
	scheduler.runningTasks[2] = func() {}
	scheduler.runningMu.Unlock()

	assert.Equal(t, 2, scheduler.GetRunningTaskCount())
}

// ============ Integration Tests ============

func TestScanLoop(t *testing.T) {
	t.Run("scan loop processes tasks", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		task := createTestTask("workload-1", "test-task")
		executor := NewMockTaskExecutor("test-task")
		scheduler.RegisterExecutor(executor)

		callCount := 0
		mockFacade.On("ListTasksByStatus", mock.Anything, constant.TaskStatusPending).
			Run(func(args mock.Arguments) {
				callCount++
			}).
			Return([]*model.WorkloadTaskState{task}, nil).Once()
		mockFacade.On("ListTasksByStatus", mock.Anything, constant.TaskStatusPending).
			Return([]*model.WorkloadTaskState{}, nil)

		mockFacade.On("TryAcquireLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID, scheduler.config.LockDuration).
			Return(true, nil).Once()
		executor.On("Validate", task).Return(nil).Once()
		executor.On("Execute", mock.Anything, mock.Anything).
			Return(SuccessResult(map[string]interface{}{}), nil).Once()
		mockFacade.On("UpdateTaskStatus", mock.Anything, task.WorkloadUID, task.TaskType, constant.TaskStatusCompleted).
			Return(nil).Once()
		mockFacade.On("UpdateTaskExt", mock.Anything, task.WorkloadUID, task.TaskType, mock.Anything).
			Return(nil).Maybe()
		mockFacade.On("ExtendLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID, scheduler.config.LockDuration).
			Return(true, nil).Maybe()
		mockFacade.On("ReleaseLock", mock.Anything, task.WorkloadUID, task.TaskType, scheduler.instanceID).
			Return(nil).Once()

		scheduler.wg.Add(1)
		go scheduler.scanLoop()

		time.Sleep(250 * time.Millisecond)

		scheduler.cancel()
		scheduler.wg.Wait()

		assert.GreaterOrEqual(t, callCount, 1)
	})
}

func TestStaleLockCleanupLoop(t *testing.T) {
	t.Run("cleanup loop runs periodically", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		callCount := 0
		mockFacade.On("ReleaseStaleLocks", mock.Anything).
			Run(func(args mock.Arguments) {
				callCount++
			}).
			Return(int64(1), nil)

		scheduler.wg.Add(1)
		go scheduler.staleLockCleanupLoop()

		time.Sleep(450 * time.Millisecond)

		scheduler.cancel()
		scheduler.wg.Wait()

		assert.GreaterOrEqual(t, callCount, 2)
	})

	t.Run("cleanup loop handles errors", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)

		mockFacade.On("ReleaseStaleLocks", mock.Anything).
			Return(int64(0), errors.New("cleanup error"))

		scheduler.wg.Add(1)
		go scheduler.staleLockCleanupLoop()

		time.Sleep(250 * time.Millisecond)

		scheduler.cancel()
		scheduler.wg.Wait()

		mockFacade.AssertCalled(t, "ReleaseStaleLocks", mock.Anything)
	})
}

// ============ Edge Cases and Concurrent Tests ============

func TestConcurrentTaskExecution(t *testing.T) {
	t.Run("executes tasks concurrently", func(t *testing.T) {
		mockFacade := new(MockWorkloadTaskFacade)
		scheduler := createTestScheduler(mockFacade)
		scheduler.config.MaxConcurrentTasks = 3

		task1 := createTestTask("workload-1", "test-task")
		task2 := createTestTask("workload-2", "test-task")
		task1.ID = 1
		task2.ID = 2

		executor := NewMockTaskExecutor("test-task")
		scheduler.RegisterExecutor(executor)

		mockFacade.On("TryAcquireLock", mock.Anything, mock.Anything, mock.Anything, scheduler.instanceID, scheduler.config.LockDuration).
			Return(true, nil)
		executor.On("Validate", mock.Anything).Return(nil)
		executor.On("Execute", mock.Anything, mock.Anything).
			Return(SuccessResult(map[string]interface{}{}), nil)
		mockFacade.On("UpdateTaskStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		mockFacade.On("UpdateTaskExt", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		mockFacade.On("ExtendLock", mock.Anything, mock.Anything, mock.Anything, scheduler.instanceID, scheduler.config.LockDuration).
			Return(true, nil).Maybe()
		mockFacade.On("ReleaseLock", mock.Anything, mock.Anything, mock.Anything, scheduler.instanceID).
			Return(nil)

		scheduler.tryExecuteTask(task1)
		scheduler.tryExecuteTask(task2)

		// Wait for tasks to complete
		completed := waitForTasks(scheduler, 500*time.Millisecond)
		assert.True(t, completed, "tasks should complete within timeout")

		// Both tasks should have been executed
		executor.AssertCalled(t, "Execute", mock.Anything, mock.Anything)
	})
}

