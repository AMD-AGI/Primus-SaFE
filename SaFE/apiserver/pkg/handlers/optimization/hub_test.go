/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTaskHubSubscribeBroadcast(t *testing.T) {
	h := newTaskHub("task-1", 5)
	assert.Equal(t, int64(6), h.nextSeq())
	assert.Equal(t, int64(7), h.nextSeq())

	ch, unsub := h.subscribe("sub-1", 0)
	ev := Event{ID: "task-1-1", TaskID: "task-1", Type: EventTypeLog}
	h.broadcast(ev)

	select {
	case got := <-ch:
		assert.Equal(t, "task-1-1", got.ID)
	case <-time.After(time.Second):
		t.Fatal("expected broadcast event")
	}

	// lastEvent cached.
	h.mu.RLock()
	assert.NotNil(t, h.lastEvent)
	h.mu.RUnlock()

	// Unsubscribe closes the channel.
	unsub()
	_, ok := <-ch
	assert.False(t, ok)
}

func TestTaskHubBroadcastDropsSlowSubscriber(t *testing.T) {
	h := newTaskHub("task-1", 0)
	// Subscribe but never drain; fill beyond buffer to exercise the drop path.
	_, _ = h.subscribe("slow", 0)
	for i := 0; i < subscriberBuffer+10; i++ {
		h.broadcast(Event{ID: "x", TaskID: "task-1"})
	}
}

func TestTaskHubClose(t *testing.T) {
	h := newTaskHub("task-1", 0)
	ch, _ := h.subscribe("s", 0)
	h.close()
	h.close() // idempotent

	// Subscriber channel closed.
	_, ok := <-ch
	assert.False(t, ok)

	// Done channel closed.
	select {
	case <-h.Done():
	case <-time.After(time.Second):
		t.Fatal("Done channel should be closed")
	}
}

func TestHubRegistry(t *testing.T) {
	r := newHubRegistry()
	assert.Nil(t, r.get("missing"))

	h, created := r.getOrCreate("t1", 0)
	assert.True(t, created)
	assert.NotNil(t, h)

	h2, created2 := r.getOrCreate("t1", 0)
	assert.False(t, created2)
	assert.Equal(t, h, h2)

	assert.Equal(t, h, r.get("t1"))

	r.remove("t1")
	assert.Nil(t, r.get("t1"))

	// waitForDrain on a missing hub returns immediately.
	r.waitForDrain(context.Background(), "missing")

	// waitForDrain returns when ctx is cancelled.
	r.getOrCreate("t2", 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.waitForDrain(ctx, "t2")
}

func TestWithClawBearer(t *testing.T) {
	ctx := context.Background()
	// Empty bearer leaves ctx unchanged.
	assert.Equal(t, "", clawBearerFromContext(WithClawBearer(ctx, "  ")))
	// Set bearer round-trips.
	ctx2 := WithClawBearer(ctx, "tok-123")
	assert.Equal(t, "tok-123", clawBearerFromContext(ctx2))
	// Nil context.
	assert.Nil(t, WithClawBearer(nil, "x"))
	assert.Equal(t, "", clawBearerFromContext(nil))
}
