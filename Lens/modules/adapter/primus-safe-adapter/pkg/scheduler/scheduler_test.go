// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockTask implements the Task interface for testing
type mockTask struct {
	name          string
	runCount      int32
	runFunc       func(ctx context.Context) error
	mu            sync.Mutex
	executionTime time.Duration
}

func newMockTask(name string) *mockTask {
	return &mockTask{
		name: name,
		runFunc: func(ctx context.Context) error {
			return nil
		},
	}
}

func (m *mockTask) Name() string {
	return m.name
}

func (m *mockTask) Run(ctx context.Context) error {
	atomic.AddInt32(&m.runCount, 1)

	if m.executionTime > 0 {
		time.Sleep(m.executionTime)
	}

	return m.runFunc(ctx)
}

func (m *mockTask) GetRunCount() int32 {
	return atomic.LoadInt32(&m.runCount)
}

func (m *mockTask) SetRunFunc(fn func(ctx context.Context) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runFunc = fn
}

func (m *mockTask) SetExecutionTime(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executionTime = d
}

func TestNewScheduler(t *testing.T) {
	s := NewScheduler()

	assert.NotNil(t, s, "Scheduler should not be nil")
	assert.NotNil(t, s.tasks, "Tasks map should not be nil")
	assert.NotNil(t, s.stopChan, "Stop channel should not be nil")
	assert.Equal(t, 0, len(s.tasks), "Initial tasks should be empty")
}

func TestScheduler_AddTask(t *testing.T) {
	s := NewScheduler()
	task := newMockTask("test-task")

	// Add task
	s.AddTask(task, 1*time.Second)

	s.mu.RLock()
	defer s.mu.RUnlock()

	assert.Equal(t, 1, len(s.tasks), "Should have 1 task")
	assert.NotNil(t, s.tasks["test-task"], "Task should exist in map")
	assert.Equal(t, 1*time.Second, s.tasks["test-task"].interval, "Interval should match")
}

func TestScheduler_AddTask_Replace(t *testing.T) {
	s := NewScheduler()
	task1 := newMockTask("test-task")
	task2 := newMockTask("test-task")

	// Add first task
	s.AddTask(task1, 1*time.Second)

	// Add second task with same name (should replace)
	s.AddTask(task2, 2*time.Second)

	s.mu.RLock()
	defer s.mu.RUnlock()

	assert.Equal(t, 1, len(s.tasks), "Should still have 1 task")
	assert.Equal(t, 2*time.Second, s.tasks["test-task"].interval, "Interval should be updated")
}

func TestScheduler_StartAndStop(t *testing.T) {
	s := NewScheduler()
	task := newMockTask("test-task")

	s.AddTask(task, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start scheduler
	s.Start(ctx)

	// Wait for at least one execution
	time.Sleep(150 * time.Millisecond)

	// Stop scheduler
	s.Stop()

	// Task should have run at least once (immediate execution)
	assert.GreaterOrEqual(t, task.GetRunCount(), int32(1), "Task should have run at least once")
}

func TestScheduler_TaskExecutesImmediately(t *testing.T) {
	s := NewScheduler()
	task := newMockTask("immediate-task")

	executionChan := make(chan struct{})
	task.SetRunFunc(func(ctx context.Context) error {
		executionChan <- struct{}{}
		return nil
	})

	s.AddTask(task, 1*time.Hour) // Long interval to ensure only immediate execution

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	// Wait for immediate execution
	select {
	case <-executionChan:
		// Success - task executed immediately
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Task did not execute immediately")
	}

	s.Stop()
}

func TestScheduler_TaskExecutesPeriodically(t *testing.T) {
	s := NewScheduler()
	task := newMockTask("periodic-task")

	s.AddTask(task, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	// Wait for multiple executions
	time.Sleep(350 * time.Millisecond)

	s.Stop()

	runCount := task.GetRunCount()
	// Should have run: immediate + at least 2-3 periodic executions
	assert.GreaterOrEqual(t, runCount, int32(3), "Task should have run multiple times")
	assert.LessOrEqual(t, runCount, int32(5), "Task should not run too many times")
}

func TestScheduler_TaskErrorHandling(t *testing.T) {
	s := NewScheduler()
	task := newMockTask("error-task")

	expectedErr := errors.New("test error")
	task.SetRunFunc(func(ctx context.Context) error {
		return expectedErr
	})

	s.AddTask(task, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	// Wait for executions
	time.Sleep(250 * time.Millisecond)

	s.Stop()

	// Task should continue running even after errors
	assert.GreaterOrEqual(t, task.GetRunCount(), int32(2), "Task should continue after errors")
}

func TestScheduler_ContextCancellation(t *testing.T) {
	s := NewScheduler()
	task := newMockTask("context-task")

	s.AddTask(task, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	s.Start(ctx)

	// Wait for some executions
	time.Sleep(120 * time.Millisecond)

	runCountBefore := task.GetRunCount()

	// Cancel context
	cancel()

	// Wait a bit to ensure task stops
	time.Sleep(200 * time.Millisecond)

	runCountAfter := task.GetRunCount()

	s.Stop()

	// Task should have stopped after context cancellation
	// Allow at most 1 additional execution due to race condition between context cancellation and ticker
	assert.LessOrEqual(t, runCountAfter-runCountBefore, int32(1), "Task should stop after context cancellation (allowing 1 execution due to race condition)")
	
	// Verify task eventually stopped (no more than 1 extra execution)
	time.Sleep(100 * time.Millisecond)
	runCountFinal := task.GetRunCount()
	assert.Equal(t, runCountAfter, runCountFinal, "Task should not execute after stopping")
}

func TestScheduler_MultipleTasks(t *testing.T) {
	s := NewScheduler()

	task1 := newMockTask("task-1")
	task2 := newMockTask("task-2")
	task3 := newMockTask("task-3")

	s.AddTask(task1, 100*time.Millisecond)
	s.AddTask(task2, 100*time.Millisecond)
	s.AddTask(task3, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	time.Sleep(250 * time.Millisecond)

	s.Stop()

	// All tasks should have run
	assert.GreaterOrEqual(t, task1.GetRunCount(), int32(2), "Task 1 should have run")
	assert.GreaterOrEqual(t, task2.GetRunCount(), int32(2), "Task 2 should have run")
	assert.GreaterOrEqual(t, task3.GetRunCount(), int32(2), "Task 3 should have run")
}

func TestScheduler_RemoveTaskLocked(t *testing.T) {
	s := NewScheduler()
	task := newMockTask("removable-task")

	s.AddTask(task, 1*time.Second)

	// Lock and remove
	s.mu.Lock()
	s.removeTaskLocked("removable-task")
	s.mu.Unlock()

	s.mu.RLock()
	defer s.mu.RUnlock()

	assert.Equal(t, 0, len(s.tasks), "Task should be removed")
}

func TestScheduler_StopWithMultipleTasks(t *testing.T) {
	s := NewScheduler()

	task1 := newMockTask("stop-task-1")
	task2 := newMockTask("stop-task-2")

	s.AddTask(task1, 100*time.Millisecond)
	s.AddTask(task2, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	time.Sleep(50 * time.Millisecond)

	// Stop should clean up all tasks
	s.Stop()

	s.mu.RLock()
	defer s.mu.RUnlock()

	assert.Equal(t, 0, len(s.tasks), "All tasks should be removed after stop")
}

func TestScheduler_TaskWithDifferentIntervals(t *testing.T) {
	s := NewScheduler()

	fastTask := newMockTask("fast-task")
	slowTask := newMockTask("slow-task")

	s.AddTask(fastTask, 50*time.Millisecond)
	s.AddTask(slowTask, 200*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	time.Sleep(250 * time.Millisecond)

	s.Stop()

	fastCount := fastTask.GetRunCount()
	slowCount := slowTask.GetRunCount()

	// Fast task should run more times than slow task
	assert.Greater(t, fastCount, slowCount, "Fast task should run more frequently")
	assert.GreaterOrEqual(t, fastCount, int32(3), "Fast task should have multiple executions")
	assert.GreaterOrEqual(t, slowCount, int32(1), "Slow task should have at least one execution")
}

func TestScheduler_ConcurrentAccess(t *testing.T) {
	s := NewScheduler()

	var wg sync.WaitGroup

	// Concurrently add tasks
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			task := newMockTask("concurrent-task-" + string(rune('0'+idx)))
			s.AddTask(task, 100*time.Millisecond)
		}(i)
	}

	wg.Wait()

	s.mu.RLock()
	taskCount := len(s.tasks)
	s.mu.RUnlock()

	assert.Equal(t, 10, taskCount, "Should have 10 tasks added concurrently")
}

func TestScheduler_LongRunningTask(t *testing.T) {
	s := NewScheduler()
	task := newMockTask("long-task")

	// Set task to take longer than interval
	task.SetExecutionTime(150 * time.Millisecond)

	s.AddTask(task, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	time.Sleep(400 * time.Millisecond)

	s.Stop()

	// Task should still execute multiple times, but limited by execution time
	runCount := task.GetRunCount()
	assert.GreaterOrEqual(t, runCount, int32(1), "Long task should execute at least once")
	// Should be fewer executions than if it was fast
	assert.LessOrEqual(t, runCount, int32(5), "Long task should not queue up excessively")
}

// TestScheduler_ZeroInterval tests zero interval case
// Note: time.NewTicker does not support zero or negative intervals and will panic
// In actual use, the caller should ensure the interval value is greater than zero
// This test case has been removed because it tests an invalid scenario that should not occur
