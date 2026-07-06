/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package controller

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	ctrlruntime "sigs.k8s.io/controller-runtime"
)

// fnHandler adapts a func into a Handler for tests.
type fnHandler struct {
	fn func(m *kmsg)
}

func (h fnHandler) Do(_ context.Context, m *kmsg) (ctrlruntime.Result, error) {
	h.fn(m)
	return ctrlruntime.Result{}, nil
}

// recordMax bumps *max to n if n is larger (lock-free).
func recordMax(max *int32, n int32) {
	for {
		old := atomic.LoadInt32(max)
		if n <= old || atomic.CompareAndSwapInt32(max, old, n) {
			return
		}
	}
}

type kmsg struct {
	key string
	val int
	del bool
}

type keyedMockHandler struct {
	mu        sync.Mutex
	processed []kmsg
	errOnce   map[string]bool
}

func (h *keyedMockHandler) Do(_ context.Context, m *kmsg) (ctrlruntime.Result, error) {
	h.mu.Lock()
	h.processed = append(h.processed, *m)
	fail := h.errOnce[m.key]
	if fail {
		h.errOnce[m.key] = false
	}
	h.mu.Unlock()
	if fail {
		return ctrlruntime.Result{}, errors.New("transient")
	}
	return ctrlruntime.Result{}, nil
}

func (h *keyedMockHandler) snapshot() []kmsg {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]kmsg, len(h.processed))
	copy(out, h.processed)
	return out
}

func kmsgKey(m *kmsg) string { return m.key }

func kmsgMergeDeletePriority(existing *kmsg, ok bool, incoming *kmsg) *kmsg {
	if ok && existing.del && !incoming.del {
		return existing
	}
	return incoming
}

// TestKeyedControllerCoalesce verifies that multiple stages of the same key keep
// only the latest payload.
func TestKeyedControllerCoalesce(t *testing.T) {
	c := NewKeyedController[*kmsg](&keyedMockHandler{}, kmsgKey, nil, 1)
	c.stage(&kmsg{key: "a", val: 1})
	c.stage(&kmsg{key: "a", val: 2})
	key := c.stage(&kmsg{key: "a", val: 3})
	assert.Equal(t, "a", key)

	msg, ok := c.take(key)
	assert.True(t, ok)
	assert.Equal(t, 3, msg.val)

	// Drained: a second take returns nothing.
	_, ok = c.take(key)
	assert.False(t, ok)
}

// TestKeyedControllerDeletePriority verifies a pending delete is not overwritten
// by a later non-delete event.
func TestKeyedControllerDeletePriority(t *testing.T) {
	c := NewKeyedController[*kmsg](&keyedMockHandler{}, kmsgKey, kmsgMergeDeletePriority, 1)
	c.stage(&kmsg{key: "b", del: true})
	c.stage(&kmsg{key: "b", val: 9, del: false})

	msg, ok := c.take("b")
	assert.True(t, ok)
	assert.True(t, msg.del, "pending delete must survive a later non-delete event")

	// Latest-wins still holds among non-deletes.
	c2 := NewKeyedController[*kmsg](&keyedMockHandler{}, kmsgKey, kmsgMergeDeletePriority, 1)
	c2.stage(&kmsg{key: "c", val: 1})
	c2.stage(&kmsg{key: "c", val: 2})
	m2, _ := c2.take("c")
	assert.Equal(t, 2, m2.val)
}

// TestKeyedControllerProcessesLatest runs a worker and verifies the coalesced
// latest payload is delivered to the handler.
func TestKeyedControllerProcessesLatest(t *testing.T) {
	h := &keyedMockHandler{}
	c := NewKeyedController[*kmsg](h, kmsgKey, nil, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < c.MaxConcurrent; i++ {
		c.Run(ctx)
	}

	c.Add(&kmsg{key: "k", val: 7})
	assert.Eventually(t, func() bool {
		for _, m := range h.snapshot() {
			if m.key == "k" && m.val == 7 {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)
}

// TestKeyedControllerRetriesOnError verifies a handler error requeues the key and
// the message is processed again (restaged payload survives the retry).
func TestKeyedControllerRetriesOnError(t *testing.T) {
	h := &keyedMockHandler{errOnce: map[string]bool{"e": true}}
	c := NewKeyedController[*kmsg](h, kmsgKey, nil, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Run(ctx)

	c.Add(&kmsg{key: "e", val: 1})
	assert.Eventually(t, func() bool {
		count := 0
		for _, m := range h.snapshot() {
			if m.key == "e" {
				count++
			}
		}
		return count >= 2 // first errors, retry succeeds
	}, 3*time.Second, 10*time.Millisecond)
}

// TestKeyedControllerSerializesSameKey verifies the core safety property that
// enables running multiple workers: the same key is never processed by two
// workers concurrently, even when re-enqueued while in flight. A self-sustaining
// re-enqueue chain drives a fixed number of processings; if serialization were
// broken the concurrent-run counter would exceed 1.
func TestKeyedControllerSerializesSameKey(t *testing.T) {
	const target = 6
	var running, maxRunning, calls int32
	var c *KeyedController[*kmsg]
	h := fnHandler{fn: func(m *kmsg) {
		recordMax(&maxRunning, atomic.AddInt32(&running, 1))
		time.Sleep(10 * time.Millisecond)
		// Re-enqueue the same key while still in flight (running not yet
		// decremented) to exercise the workqueue's dirty/requeue path.
		if atomic.AddInt32(&calls, 1) < target {
			c.Add(&kmsg{key: "same"})
		}
		atomic.AddInt32(&running, -1)
	}}
	c = NewKeyedController[*kmsg](h, kmsgKey, nil, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < c.MaxConcurrent; i++ {
		c.Run(ctx)
	}

	c.Add(&kmsg{key: "same"})
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&calls) >= target
	}, 3*time.Second, 5*time.Millisecond)
	// Multiple processings happened, and none overlapped.
	assert.Equal(t, int32(1), atomic.LoadInt32(&maxRunning))
}

// TestKeyedControllerParallelDifferentKeys verifies distinct keys are processed
// concurrently across workers.
func TestKeyedControllerParallelDifferentKeys(t *testing.T) {
	var running, maxRunning int32
	h := fnHandler{fn: func(m *kmsg) {
		recordMax(&maxRunning, atomic.AddInt32(&running, 1))
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&running, -1)
	}}
	c := NewKeyedController[*kmsg](h, kmsgKey, nil, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < c.MaxConcurrent; i++ {
		c.Run(ctx)
	}

	for i := 0; i < 4; i++ {
		c.Add(&kmsg{key: fmt.Sprintf("k%d", i)})
	}
	// Distinct keys must overlap across workers.
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&maxRunning) >= 2
	}, 2*time.Second, 5*time.Millisecond)
}
