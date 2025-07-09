/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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

type Controller[T comparable] struct {
	queue         workqueue.TypedRateLimitingInterface[T]
	handler       Handler[T]
	MaxConcurrent int
}

type QueueHandler[T comparable] func(message T)

type Handler[T comparable] interface {
	Do(ctx context.Context, message T) (ctrlruntime.Result, error)
}

func NewController[T comparable](h Handler[T], concurrent int) *Controller[T] {
	return &Controller[T]{
		handler: h,
		queue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[T](),
			workqueue.TypedRateLimitingQueueConfig[T]{}),
		MaxConcurrent: concurrent,
	}
}

func NewControllerWithQueue[T comparable](h Handler[T], queue workqueue.TypedRateLimitingInterface[T], concurrent int) *Controller[T] {
	return &Controller[T]{
		handler:       h,
		queue:         queue,
		MaxConcurrent: concurrent,
	}
}

func (c *Controller[T]) Run(ctx context.Context) {
	go wait.UntilWithContext(ctx, func(ctx context.Context) {
		for {
			if !c.processNext(ctx) {
				break
			}
		}
	}, time.Second*10)
}

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

func (c *Controller[T]) Add(message T) {
	c.queue.Add(message)
}

func (c *Controller[T]) AddAfter(message T, duration time.Duration) {
	c.queue.AddAfter(message, duration)
}

func (c *Controller[T]) GetQueueSize() int {
	return c.queue.Len()
}
