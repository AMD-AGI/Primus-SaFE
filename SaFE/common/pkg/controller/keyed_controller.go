/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package controller

import (
	"context"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
)

// KeyedController is a work-queue controller that keys the queue by a stable
// object identity string instead of by the message value itself. This gives two
// properties the plain Controller[T] (keyed by the message) cannot:
//   - Per-object serialization: messages that resolve to the same key are never
//     processed by two workers at once (safe to run MaxConcurrent > 1).
//   - Coalescing: rapid events for the same object collapse to the latest
//     payload, so a burst (e.g. mass Pod deletes) does not flood the workers.
//
// The latest payload for each key is held in a side map; the work queue only
// carries keys.
type KeyedController[T comparable] struct {
	queue   workqueue.TypedRateLimitingInterface[string]
	handler Handler[T]
	keyFn   KeyFunc[T]
	mergeFn MergeFunc[T]

	MaxConcurrent int

	mu      sync.Mutex
	pending map[string]T
}

// KeyFunc derives a stable, comparable key for a message. Messages with the same
// key are serialized and coalesced.
type KeyFunc[T comparable] func(message T) string

// MergeFunc decides the payload kept for a key when a new message arrives while
// one is still pending. existingOK reports whether a payload was already pending.
// When nil, the latest message always wins.
type MergeFunc[T comparable] func(existing T, existingOK bool, incoming T) T

// NewKeyedController creates a KeyedController. keyFn is required; mergeFn may be
// nil (latest-wins).
func NewKeyedController[T comparable](h Handler[T], keyFn KeyFunc[T], mergeFn MergeFunc[T], concurrent int) *KeyedController[T] {
	return &KeyedController[T]{
		queue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[string](),
			workqueue.TypedRateLimitingQueueConfig[string]{}),
		handler:       h,
		keyFn:         keyFn,
		mergeFn:       mergeFn,
		MaxConcurrent: concurrent,
		pending:       make(map[string]T),
	}
}

// Run starts a worker loop. Call it MaxConcurrent times for parallelism.
func (c *KeyedController[T]) Run(ctx context.Context) {
	go wait.UntilWithContext(ctx, func(ctx context.Context) {
		for {
			if !c.processNext(ctx) {
				break
			}
		}
	}, time.Second*10)
}

// stage stores/merges the latest payload for a key.
func (c *KeyedController[T]) stage(message T) string {
	key := c.keyFn(message)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mergeFn != nil {
		existing, ok := c.pending[key]
		c.pending[key] = c.mergeFn(existing, ok, message)
	} else {
		c.pending[key] = message
	}
	return key
}

// restage re-stores a payload for a requeued key only when nothing newer is
// pending, so an in-flight retry never overwrites a freshly-arrived event.
func (c *KeyedController[T]) restage(key string, message T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.pending[key]; !ok {
		c.pending[key] = message
	}
}

// take removes and returns the pending payload for a key.
func (c *KeyedController[T]) take(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	message, ok := c.pending[key]
	if ok {
		delete(c.pending, key)
	}
	return message, ok
}

// Add stages a message and enqueues its key.
func (c *KeyedController[T]) Add(message T) {
	c.queue.Add(c.stage(message))
}

// AddAfter stages a message and enqueues its key after the given delay.
func (c *KeyedController[T]) AddAfter(message T, duration time.Duration) {
	c.queue.AddAfter(c.stage(message), duration)
}

// GetQueueSize returns the number of keys waiting to be processed.
func (c *KeyedController[T]) GetQueueSize() int {
	return c.queue.Len()
}

func (c *KeyedController[T]) processNext(ctx context.Context) bool {
	key, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(key)

	message, ok := c.take(key)
	if !ok {
		// The payload was already consumed by a coalesced run; nothing to do.
		c.queue.Forget(key)
		return true
	}

	result, err := c.handler.Do(ctx, message)
	switch {
	case err != nil:
		c.restage(key, message)
		c.queue.AddRateLimited(key)
	case result.RequeueAfter > 0:
		c.restage(key, message)
		c.queue.AddAfter(key, result.RequeueAfter)
		if result.Requeue {
			c.queue.AddRateLimited(key)
		} else {
			c.queue.Forget(key)
		}
	case result.Requeue:
		c.restage(key, message)
		c.queue.AddRateLimited(key)
	default:
		c.queue.Forget(key)
	}
	return true
}
