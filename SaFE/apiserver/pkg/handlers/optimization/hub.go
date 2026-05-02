/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Buffered event channel size per subscriber. Slow consumers are dropped
// instead of back-pressuring the upstream Claw SSE loop.
const subscriberBuffer = 256

// taskHub holds the shared broadcast state for a single optimization task.
// Exactly one goroutine per task consumes the upstream Claw SSE stream and
// fans out events to all local HTTP SSE subscribers.
type taskHub struct {
	taskID string

	mu          sync.RWMutex
	subscribers map[string]*subscriber
	// seq is a monotonic counter used to order persisted events.
	seq atomic.Int64
	// lastEvent caches the latest Event so newcomers can initialize their
	// local state (phase/status) without replaying the full history.
	lastEvent *Event

	// Closed when the hub has completed (task succeeded/failed) and no
	// further events will be produced.
	done     chan struct{}
	doneOnce sync.Once
}

type subscriber struct {
	id     string
	events chan Event
	// afterSeq records the seq this subscriber was created with; used if
	// events that were persisted before subscription need to be replayed.
	afterSeq int64
}

func newTaskHub(taskID string, initialSeq int64) *taskHub {
	h := &taskHub{
		taskID:      taskID,
		subscribers: make(map[string]*subscriber),
		done:        make(chan struct{}),
	}
	h.seq.Store(initialSeq)
	return h
}

// nextSeq returns the next monotonic sequence number. Thread-safe.
func (h *taskHub) nextSeq() int64 {
	return h.seq.Add(1)
}

// subscribe registers a new subscriber and returns the channel to read Events
// from plus an unsubscribe function.
func (h *taskHub) subscribe(id string, afterSeq int64) (<-chan Event, func()) {
	s := &subscriber{
		id:       id,
		events:   make(chan Event, subscriberBuffer),
		afterSeq: afterSeq,
	}
	h.mu.Lock()
	h.subscribers[id] = s
	h.mu.Unlock()
	return s.events, func() { h.unsubscribe(id) }
}

func (h *taskHub) unsubscribe(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok := h.subscribers[id]; ok {
		delete(h.subscribers, id)
		close(s.events)
	}
}

// broadcast sends an event to every subscriber. Slow subscribers (buffer
// full) get their pending event dropped rather than blocking the producer.
func (h *taskHub) broadcast(ev Event) {
	h.mu.RLock()
	subs := make([]*subscriber, 0, len(h.subscribers))
	for _, s := range h.subscribers {
		subs = append(subs, s)
	}
	h.mu.RUnlock()

	for _, s := range subs {
		select {
		case s.events <- ev:
		default:
			// Drop into the ether; the subscriber can reconnect with
			// after_event_id to catch up from the DB.
		}
	}

	h.mu.Lock()
	evCopy := ev
	h.lastEvent = &evCopy
	h.mu.Unlock()
}

// close signals the end of the stream and tears down all subscribers. Safe
// to call multiple times.
func (h *taskHub) close() {
	h.doneOnce.Do(func() {
		close(h.done)
		h.mu.Lock()
		for _, s := range h.subscribers {
			close(s.events)
		}
		h.subscribers = map[string]*subscriber{}
		h.mu.Unlock()
	})
}

// Done returns a channel closed when the hub is torn down.
func (h *taskHub) Done() <-chan struct{} { return h.done }

// ── hubRegistry: process-wide index ─────────────────────────────────────

// hubRegistry maps task id → hub. All methods are safe for concurrent use.
type hubRegistry struct {
	mu   sync.Mutex
	hubs map[string]*taskHub
}

func newHubRegistry() *hubRegistry {
	return &hubRegistry{hubs: map[string]*taskHub{}}
}

func (r *hubRegistry) get(taskID string) *taskHub {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hubs[taskID]
}

// getOrCreate atomically looks up an existing hub or creates one with
// initialSeq. The boolean return is true when a new hub was created.
func (r *hubRegistry) getOrCreate(taskID string, initialSeq int64) (*taskHub, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if h, ok := r.hubs[taskID]; ok {
		return h, false
	}
	h := newTaskHub(taskID, initialSeq)
	r.hubs[taskID] = h
	return h, true
}

func (r *hubRegistry) remove(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if h, ok := r.hubs[taskID]; ok {
		h.close()
		delete(r.hubs, taskID)
	}
}

// waitForDrain is a small helper to pause until a hub completes or ctx is
// cancelled. Useful during graceful shutdown of the apiserver process.
func (r *hubRegistry) waitForDrain(ctx context.Context, taskID string) {
	h := r.get(taskID)
	if h == nil {
		return
	}
	select {
	case <-h.Done():
	case <-ctx.Done():
	}
}

// nowMillis is exported for tests that need to predict event timestamps.
var nowMillis = func() int64 { return time.Now().UnixNano() / int64(time.Millisecond) }
