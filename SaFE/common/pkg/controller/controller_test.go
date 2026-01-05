/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package controller

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/util/workqueue"
	ctrlruntime "sigs.k8s.io/controller-runtime"
)

// mockHandler is a mock implementation of Handler interface for testing
type mockHandler struct {
	mu            sync.Mutex
	processedMsgs []string
	results       map[string]ctrlruntime.Result
	errors        map[string]error
	callCount     atomic.Int32
}

func newMockHandler() *mockHandler {
	return &mockHandler{
		processedMsgs: make([]string, 0),
		results:       make(map[string]ctrlruntime.Result),
		errors:        make(map[string]error),
	}
}

func (m *mockHandler) Do(ctx context.Context, message string) (ctrlruntime.Result, error) {
	m.callCount.Add(1)
	m.mu.Lock()
	m.processedMsgs = append(m.processedMsgs, message)
	m.mu.Unlock()

	if err, ok := m.errors[message]; ok {
		return ctrlruntime.Result{}, err
	}
	if result, ok := m.results[message]; ok {
		return result, nil
	}
	return ctrlruntime.Result{}, nil
}

func (m *mockHandler) getProcessedMessages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.processedMsgs))
	copy(result, m.processedMsgs)
	return result
}

func (m *mockHandler) setResult(msg string, result ctrlruntime.Result) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results[msg] = result
}

func (m *mockHandler) setError(msg string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[msg] = err
}

// TestNewController tests the NewController function
func TestNewController(t *testing.T) {
	tests := []struct {
		name       string
		concurrent int
	}{
		{
			name:       "create controller with concurrency 1",
			concurrent: 1,
		},
		{
			name:       "create controller with concurrency 5",
			concurrent: 5,
		},
		{
			name:       "create controller with concurrency 10",
			concurrent: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newMockHandler()
			ctrl := NewController[string](handler, tt.concurrent)

			assert.NotNil(t, ctrl)
			assert.NotNil(t, ctrl.queue)
			assert.NotNil(t, ctrl.handler)
			assert.Equal(t, tt.concurrent, ctrl.MaxConcurrent)
		})
	}
}

// TestNewControllerWithQueue tests the NewControllerWithQueue function
func TestNewControllerWithQueue(t *testing.T) {
	handler := newMockHandler()
	queue := workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[string](),
		workqueue.TypedRateLimitingQueueConfig[string]{},
	)

	ctrl := NewControllerWithQueue[string](handler, queue, 3)

	assert.NotNil(t, ctrl)
	assert.Equal(t, queue, ctrl.queue)
	assert.NotNil(t, ctrl.handler)
	assert.Equal(t, 3, ctrl.MaxConcurrent)
}

// TestControllerAdd tests the Add method
func TestControllerAdd(t *testing.T) {
	handler := newMockHandler()
	ctrl := NewController[string](handler, 1)

	// Add messages to the queue
	ctrl.Add("msg1")
	ctrl.Add("msg2")
	ctrl.Add("msg3")

	assert.Equal(t, 3, ctrl.GetQueueSize())
}

// TestControllerAddAfter tests the AddAfter method
func TestControllerAddAfter(t *testing.T) {
	handler := newMockHandler()
	ctrl := NewController[string](handler, 1)

	// Add a message with delay
	ctrl.AddAfter("delayed-msg", 50*time.Millisecond)

	// Initially the queue should be empty (message is delayed)
	assert.Equal(t, 0, ctrl.GetQueueSize())

	// Wait for the delay to pass
	time.Sleep(100 * time.Millisecond)

	// Now the message should be in the queue
	assert.Equal(t, 1, ctrl.GetQueueSize())
}

// TestControllerGetQueueSize tests the GetQueueSize method
func TestControllerGetQueueSize(t *testing.T) {
	handler := newMockHandler()
	ctrl := NewController[string](handler, 1)

	// Empty queue
	assert.Equal(t, 0, ctrl.GetQueueSize())

	// Add messages
	ctrl.Add("msg1")
	assert.Equal(t, 1, ctrl.GetQueueSize())

	ctrl.Add("msg2")
	assert.Equal(t, 2, ctrl.GetQueueSize())

	ctrl.Add("msg3")
	assert.Equal(t, 3, ctrl.GetQueueSize())
}

// TestControllerProcessNextSuccess tests successful message processing
func TestControllerProcessNextSuccess(t *testing.T) {
	handler := newMockHandler()
	ctrl := NewController[string](handler, 1)

	ctrl.Add("test-msg")

	// Process the message
	ctx := context.Background()
	result := ctrl.processNext(ctx)

	assert.True(t, result)
	assert.Equal(t, 0, ctrl.GetQueueSize())
	assert.Contains(t, handler.getProcessedMessages(), "test-msg")
}

// TestControllerProcessNextWithError tests message processing with error (should requeue)
func TestControllerProcessNextWithError(t *testing.T) {
	handler := newMockHandler()
	handler.setError("error-msg", errors.New("processing error"))
	ctrl := NewController[string](handler, 1)

	ctrl.Add("error-msg")

	// Process the message
	ctx := context.Background()
	result := ctrl.processNext(ctx)

	assert.True(t, result)
	// Message should be requeued due to error (rate limited)
	assert.Contains(t, handler.getProcessedMessages(), "error-msg")
}

// TestControllerProcessNextWithRequeue tests message processing with Requeue=true
func TestControllerProcessNextWithRequeue(t *testing.T) {
	handler := newMockHandler()
	handler.setResult("requeue-msg", ctrlruntime.Result{Requeue: true})
	ctrl := NewController[string](handler, 1)

	ctrl.Add("requeue-msg")

	ctx := context.Background()
	result := ctrl.processNext(ctx)

	assert.True(t, result)
	assert.Contains(t, handler.getProcessedMessages(), "requeue-msg")
}

// TestControllerProcessNextWithRequeueAfter tests message processing with RequeueAfter
func TestControllerProcessNextWithRequeueAfter(t *testing.T) {
	handler := newMockHandler()
	handler.setResult("requeue-after-msg", ctrlruntime.Result{RequeueAfter: 50 * time.Millisecond})
	ctrl := NewController[string](handler, 1)

	ctrl.Add("requeue-after-msg")

	ctx := context.Background()
	result := ctrl.processNext(ctx)

	assert.True(t, result)
	assert.Contains(t, handler.getProcessedMessages(), "requeue-after-msg")

	// Queue should be empty immediately after processing
	assert.Equal(t, 0, ctrl.GetQueueSize())

	// Wait for requeue delay
	time.Sleep(100 * time.Millisecond)

	// Message should be requeued
	assert.Equal(t, 1, ctrl.GetQueueSize())
}

// TestControllerProcessNextShutdown tests processNext when queue is shutdown
func TestControllerProcessNextShutdown(t *testing.T) {
	handler := newMockHandler()
	ctrl := NewController[string](handler, 1)

	// Shutdown the queue
	ctrl.queue.ShutDown()

	ctx := context.Background()
	result := ctrl.processNext(ctx)

	// Should return false when queue is shutdown
	assert.False(t, result)
}

// TestControllerRun tests the Run method with context cancellation
func TestControllerRun(t *testing.T) {
	handler := newMockHandler()
	ctrl := NewController[string](handler, 1)

	ctx, cancel := context.WithCancel(context.Background())

	// Start the controller
	ctrl.Run(ctx)

	// Add messages
	ctrl.Add("run-msg1")
	ctrl.Add("run-msg2")

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Verify messages were processed
	processed := handler.getProcessedMessages()
	assert.Contains(t, processed, "run-msg1")
	assert.Contains(t, processed, "run-msg2")

	// Cancel context to stop the controller
	cancel()
}

// TestControllerRunMultipleMessages tests processing multiple messages
func TestControllerRunMultipleMessages(t *testing.T) {
	handler := newMockHandler()
	ctrl := NewController[string](handler, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctrl.Run(ctx)

	// Add multiple messages
	messages := []string{"msg1", "msg2", "msg3", "msg4", "msg5"}
	for _, msg := range messages {
		ctrl.Add(msg)
	}

	// Wait for all messages to be processed
	time.Sleep(500 * time.Millisecond)

	// Verify all messages were processed
	processed := handler.getProcessedMessages()
	for _, msg := range messages {
		assert.Contains(t, processed, msg)
	}
}

// TestControllerProcessNextWithRequeueAfterAndRequeue tests RequeueAfter with Requeue=true
func TestControllerProcessNextWithRequeueAfterAndRequeue(t *testing.T) {
	handler := newMockHandler()
	handler.setResult("both-requeue-msg", ctrlruntime.Result{
		RequeueAfter: 50 * time.Millisecond,
		Requeue:      true,
	})
	ctrl := NewController[string](handler, 1)

	ctrl.Add("both-requeue-msg")

	ctx := context.Background()
	result := ctrl.processNext(ctx)

	assert.True(t, result)
	assert.Contains(t, handler.getProcessedMessages(), "both-requeue-msg")
}

// TestControllerWithIntegerType tests controller with integer type
func TestControllerWithIntegerType(t *testing.T) {
	intHandler := &intMockHandler{
		processedMsgs: make([]int, 0),
	}
	ctrl := NewController[int](intHandler, 1)

	ctrl.Add(1)
	ctrl.Add(2)
	ctrl.Add(3)

	assert.Equal(t, 3, ctrl.GetQueueSize())

	ctx := context.Background()
	ctrl.processNext(ctx)
	ctrl.processNext(ctx)
	ctrl.processNext(ctx)

	assert.Equal(t, 0, ctrl.GetQueueSize())
	assert.ElementsMatch(t, []int{1, 2, 3}, intHandler.processedMsgs)
}

// intMockHandler is a mock handler for integer type
type intMockHandler struct {
	mu            sync.Mutex
	processedMsgs []int
}

func (m *intMockHandler) Do(ctx context.Context, message int) (ctrlruntime.Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processedMsgs = append(m.processedMsgs, message)
	return ctrlruntime.Result{}, nil
}

// TestControllerConcurrentAdd tests adding messages concurrently
func TestControllerConcurrentAdd(t *testing.T) {
	handler := newMockHandler()
	ctrl := NewController[string](handler, 5)

	var wg sync.WaitGroup
	messageCount := 100

	// Add messages concurrently
	for i := 0; i < messageCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctrl.Add("concurrent-msg")
		}(i)
	}

	wg.Wait()

	// All messages should be added (some might be deduplicated by the queue)
	assert.Greater(t, ctrl.GetQueueSize(), 0)
}

// TestControllerDuplicateMessages tests handling of duplicate messages
func TestControllerDuplicateMessages(t *testing.T) {
	handler := newMockHandler()
	ctrl := NewController[string](handler, 1)

	// Add the same message multiple times
	ctrl.Add("duplicate")
	ctrl.Add("duplicate")
	ctrl.Add("duplicate")

	// Queue deduplicates by default, so should only have 1
	assert.Equal(t, 1, ctrl.GetQueueSize())
}
