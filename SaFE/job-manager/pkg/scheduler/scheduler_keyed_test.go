/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
)

// countingHandler records how many times Do runs per workspace key.
type countingHandler struct {
	client ctrlruntime.Client
	mu     sync.Mutex
	counts map[string]int
}

func (h *countingHandler) Do(_ context.Context, m *SchedulerMessage) (ctrlruntime.Result, error) {
	key := schedulerMessageKey(m)
	h.mu.Lock()
	h.counts[key]++
	h.mu.Unlock()
	// Exercise the real path: missing workspace is a no-op error-wise.
	r := &SchedulerReconciler{Client: h.client}
	return r.Do(context.Background(), m)
}

func (h *countingHandler) get(key string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.counts[key]
}

// TestSchedulerKeyedCoalesce verifies duplicate workspace events collapse to one run.
func TestSchedulerKeyedCoalesce(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	h := &countingHandler{client: cl, counts: make(map[string]int)}
	c := controller.NewKeyedController[*SchedulerMessage](h, schedulerMessageKey, nil, 1)

	msg := &SchedulerMessage{ClusterId: "c1", WorkspaceId: "ws1"}
	c.Add(msg)
	c.Add(msg)
	c.Add(msg)
	assert.Equal(t, 1, c.GetQueueSize())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Run(ctx)
	deadline := time.Now().Add(2 * time.Second)
	for c.GetQueueSize() > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 1, h.get("c1|ws1"))
}

// TestSchedulerKeyedParallelWorkspaces verifies distinct workspace keys fan out.
func TestSchedulerKeyedParallelWorkspaces(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	var maxParallel int32
	var inFlight int32
	h := &countingHandler{client: cl, counts: make(map[string]int)}
	wrapped := &parallelHandler{
		inner:     h,
		inFlight:  &inFlight,
		maxParallel: &maxParallel,
	}
	c := controller.NewKeyedController[*SchedulerMessage](wrapped, schedulerMessageKey, nil, 4)

	for i := 0; i < 4; i++ {
		c.Add(&SchedulerMessage{ClusterId: "c1", WorkspaceId: "ws-a"})
		c.Add(&SchedulerMessage{ClusterId: "c1", WorkspaceId: "ws-b"})
		c.Add(&SchedulerMessage{ClusterId: "c1", WorkspaceId: "ws-c"})
		c.Add(&SchedulerMessage{ClusterId: "c1", WorkspaceId: "ws-d"})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < 4; i++ {
		c.Run(ctx)
	}
	deadline := time.Now().Add(3 * time.Second)
	for c.GetQueueSize() > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	assert.GreaterOrEqual(t, int(atomic.LoadInt32(&maxParallel)), 2)
	assert.Equal(t, 1, h.get("c1|ws-a"))
	assert.Equal(t, 1, h.get("c1|ws-b"))
}

type parallelHandler struct {
	inner       *countingHandler
	inFlight    *int32
	maxParallel *int32
}

func (h *parallelHandler) Do(ctx context.Context, m *SchedulerMessage) (ctrlruntime.Result, error) {
	cur := atomic.AddInt32(h.inFlight, 1)
	defer atomic.AddInt32(h.inFlight, -1)
	for {
		old := atomic.LoadInt32(h.maxParallel)
		if cur <= old || atomic.CompareAndSwapInt32(h.maxParallel, old, cur) {
			break
		}
	}
	time.Sleep(30 * time.Millisecond)
	return h.inner.Do(ctx, m)
}

func TestSchedulerMessageKey(t *testing.T) {
	assert.Equal(t, "cluster-a|ws-1", schedulerMessageKey(&SchedulerMessage{ClusterId: "cluster-a", WorkspaceId: "ws-1"}))
}
