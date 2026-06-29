/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestBuildEvent(t *testing.T) {
	h := &Handler{}
	hub := newTaskHub("t1", 0)
	ev := h.buildEvent("t1", hub, EventTypePhase, PhaseEventPayload{Phase: 3, PhaseName: "Profile"})
	assert.Equal(t, "t1-1", ev.ID)
	assert.Equal(t, int64(1), ev.Seq)
	assert.Equal(t, EventTypePhase, ev.Type)
	assert.Contains(t, string(ev.Payload), "Profile")
}

func TestBuildEventFromClawUsesStableID(t *testing.T) {
	h := &Handler{}
	raw := ClawSSEEvent{ID: "claw-event-1", Event: "tool_used", Data: "{}"}

	ev1 := h.buildEventFromClaw("t1", newTaskHub("t1", 0), raw, 0, EventTypeLog, LogEventPayload{Message: "a"})
	ev2 := h.buildEventFromClaw("t1", newTaskHub("t1", 99), raw, 0, EventTypeLog, LogEventPayload{Message: "a"})
	ev3 := h.buildEventFromClaw("t1", newTaskHub("t1", 0), raw, 1, EventTypeLog, LogEventPayload{Message: "a"})

	assert.Equal(t, ev1.ID, ev2.ID)
	assert.NotEqual(t, ev1.ID, ev3.ID)
	assert.Equal(t, int64(1), ev1.Seq)
	assert.Equal(t, int64(100), ev2.Seq)
}

func TestPersistAndBroadcast(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock_client.NewMockInterface(ctrl)
	mockDB.EXPECT().AppendOptimizationEvent(gomock.Any(), gomock.Any()).Return(nil)

	h := &Handler{dbClient: mockDB}
	hub := newTaskHub("t1", 0)
	ch, _ := hub.subscribe("s", 0)

	ev := Event{ID: "t1-1", TaskID: "t1", Type: EventTypeLog, Seq: 1}
	assert.True(t, h.persistAndBroadcast("t1", hub, ev))

	got := <-ch
	assert.Equal(t, "t1-1", got.ID)
}

func TestPersistAndBroadcastSkipsDuplicate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock_client.NewMockInterface(ctrl)
	mockDB.EXPECT().AppendOptimizationEvent(gomock.Any(), gomock.Any()).Return(dbclient.ErrOptimizationEventDuplicate)

	h := &Handler{dbClient: mockDB}
	hub := newTaskHub("t1", 0)
	ch, _ := hub.subscribe("s", 0)

	ev := Event{ID: "t1-c-dup-0", TaskID: "t1", Type: EventTypeLog, Seq: 2}
	assert.False(t, h.persistAndBroadcast("t1", hub, ev))
	select {
	case got := <-ch:
		t.Fatalf("duplicate event should not broadcast, got %#v", got)
	case <-time.After(10 * time.Millisecond):
	}
}

func TestResolveAfterSeqUsesPersistedEventID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock_client.NewMockInterface(ctrl)
	mockDB.EXPECT().OptimizationEventSeq(gomock.Any(), "t1", "stable-id").Return(int64(42), true, nil)

	h := &Handler{dbClient: mockDB}
	assert.Equal(t, int64(42), h.resolveAfterSeq(context.Background(), "t1", "stable-id"))
}

func TestResolveAfterSeqFallsBackToLegacyID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock_client.NewMockInterface(ctrl)
	mockDB.EXPECT().OptimizationEventSeq(gomock.Any(), "t1", "t1-7").Return(int64(0), false, nil)

	h := &Handler{dbClient: mockDB}
	assert.Equal(t, int64(7), h.resolveAfterSeq(context.Background(), "t1", "t1-7"))
}

func TestMaybeUpdateTaskStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock_client.NewMockInterface(ctrl)
	mockDB.EXPECT().UpdateOptimizationTaskStatus(
		gomock.Any(), "t1", dbclient.OptimizationTaskStatusRunning, 3, gomock.Any(),
	).Return(nil)

	h := &Handler{dbClient: mockDB}
	// Phase event -> triggers status update.
	h.maybeUpdateTaskStatus("t1", ParsedEvent{
		Type:    EventTypePhase,
		Payload: PhaseEventPayload{Phase: 3, PhaseName: "Profile"},
	})

	// Non-phase event -> no DB call (mock would fail if called).
	h.maybeUpdateTaskStatus("t1", ParsedEvent{Type: EventTypeLog})
}

func TestResolveStatusFromClawEmptySession(t *testing.T) {
	h := &Handler{clawClient: NewClawClient("", "")}
	status, msg := h.resolveStatusFromClaw("", errors.New("stream dropped"), "bearer", false)
	assert.Equal(t, dbclient.OptimizationTaskStatusFailed, status)
	assert.Contains(t, msg, "stream dropped")
}
