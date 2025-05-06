/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
)

type Controller struct {
	queue         workqueue.RateLimitingInterface
	handler       Handler
	MaxConcurrent int
}

type Result struct {
	Requeue      bool
	RequeueAfter time.Duration
}

type AddQueue func(item interface{})

type Handler interface {
	Do(ctx context.Context, item interface{}) (Result, error)
}

// NewController new Controller Object func
func NewControllerWithQueue(h Handler, queue workqueue.RateLimitingInterface, concurrent int) *Controller {
	return &Controller{
		handler:       h,
		queue:         queue,
		MaxConcurrent: concurrent,
	}
}

func NewController(h Handler, concurrent int) *Controller {
	return &Controller{
		handler:       h,
		queue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "default"),
		MaxConcurrent: concurrent,
	}
}

func (c *Controller) Run(ctx context.Context) {
	go wait.UntilWithContext(ctx, func(ctx context.Context) {
		for {
			if !c.processNext(ctx) {
				break
			}
		}
	}, time.Minute)
}

func (c *Controller) processNext(ctx context.Context) bool {
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
func (c *Controller) Add(item interface{}) {
	c.queue.Add(item)
}

func (c *Controller) AddAfter(item interface{}, duration time.Duration) {
	c.queue.AddAfter(item, duration)
}

func (c *Controller) GetQueueSize() int {
	return c.queue.Len()
}
