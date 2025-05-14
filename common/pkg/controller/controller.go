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
)

type Controller[T comparable] struct {
	queue         workqueue.TypedRateLimitingInterface[T]
	handler       Handler[T]
	MaxConcurrent int
}

type Result struct {
	Requeue      bool
	RequeueAfter time.Duration
}

type AddQueue[T comparable] func(item T)

type Handler[T comparable] interface {
	Do(ctx context.Context, item T) (Result, error)
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
	if result, err := c.handler.Do(ctx, req); err != nil {
		c.queue.AddRateLimited(req)
		return true
	} else if result.RequeueAfter > 0 {
		c.queue.Forget(req)
		c.queue.AddAfter(req, result.RequeueAfter)
		return true
	} else if result.Requeue {
		c.queue.AddRateLimited(req)
		return true
	}
	c.queue.Forget(req)
	return true
}

// AddQueue add object into queue
func (c *Controller[T]) Add(item T) {
	c.queue.Add(item)
}

func (c *Controller[T]) AddAfter(item T, duration time.Duration) {
	c.queue.AddAfter(item, duration)
}

func (c *Controller[T]) GetQueueSize() int {
	return c.queue.Len()
}
