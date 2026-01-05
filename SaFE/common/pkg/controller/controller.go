/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	ctrlruntime "sigs.k8s.io/controller-runtime"
)

// Custom controller structure including work queue, processor, and concurrency control parameters
type Controller[T comparable] struct {
	queue         workqueue.TypedRateLimitingInterface[T]
	handler       Handler[T]
	MaxConcurrent int
}

// Type definition for queue processing function
type QueueHandler[T comparable] func(message T)

// Controller processor interface defining actual business logic
type Handler[T comparable] interface {
	Do(ctx context.Context, message T) (ctrlruntime.Result, error)
}

// NewController create a new controller instance
// Parameters:
//
//	h: Processor implementing the Handler interface
//	concurrent: Maximum concurrent processing count
//
// Returns: Initialized Controller instance
func NewController[T comparable](h Handler[T], concurrent int) *Controller[T] {
	return &Controller[T]{
		handler: h,
		queue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[T](),
			workqueue.TypedRateLimitingQueueConfig[T]{}),
		MaxConcurrent: concurrent,
	}
}

// NewControllerWithQueue similar to the above, creates a new controller instance using the provided queue
func NewControllerWithQueue[T comparable](h Handler[T], queue workqueue.TypedRateLimitingInterface[T], concurrent int) *Controller[T] {
	return &Controller[T]{
		handler:       h,
		queue:         queue,
		MaxConcurrent: concurrent,
	}
}

// Run start the controller run loop
// Parameters:
//
//	ctx: Context for controlling goroutine lifecycle
//
// Functionality: Starts background goroutine to continuously process messages in the queue
func (c *Controller[T]) Run(ctx context.Context) {
	go wait.UntilWithContext(ctx, func(ctx context.Context) {
		for {
			if !c.processNext(ctx) {
				break
			}
		}
	}, time.Second*10)
}

// processNext process the next message in the queue
// Parameters:
//
//	ctx: Context
//
// Return value:
//
//	true: Continue processing next message
//	false: Stop processing (queue closed)
//
// Functionality: Gets message from queue and calls processor to handle it, decides whether to requeue based on result
func (c *Controller[T]) processNext(ctx context.Context) bool {
	req, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(req)

	result, err := c.handler.Do(ctx, req)
	switch {
	case err != nil:
		c.queue.AddRateLimited(req)
	case result.RequeueAfter > 0:
		c.queue.AddAfter(req, result.RequeueAfter)
		if result.Requeue {
			c.queue.AddRateLimited(req)
		} else {
			c.queue.Forget(req)
		}
	case result.Requeue:
		c.queue.AddRateLimited(req)
	default:
		c.queue.Forget(req)
	}
	return true
}

// Add add message to processing queue
// Parameters:
//
//	message: Message object to be processe
func (c *Controller[T]) Add(message T) {
	c.queue.Add(message)
}

// AddAfter add message to queue after specified delay
// Parameters:
//
//	message: Message object to be processed
//	duration: Delay time
func (c *Controller[T]) AddAfter(message T, duration time.Duration) {
	c.queue.AddAfter(message, duration)
}

// GetQueueSize get current queue length
// Return value: Number of messages waiting to be processed in the queue
func (c *Controller[T]) GetQueueSize() int {
	return c.queue.Len()
}
