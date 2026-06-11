/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"errors"
	"testing"

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
	assert.Equal(t, EventTypePhase, ev.Type)
	assert.Contains(t, string(ev.Payload), "Profile")
}

func TestPersistAndBroadcast(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock_client.NewMockInterface(ctrl)
	mockDB.EXPECT().AppendOptimizationEvent(gomock.Any(), gomock.Any()).Return(nil)

	h := &Handler{dbClient: mockDB}
	hub := newTaskHub("t1", 0)
	ch, _ := hub.subscribe("s", 0)

	ev := Event{ID: "t1-1", TaskID: "t1", Type: EventTypeLog}
	h.persistAndBroadcast("t1", hub, ev)

	got := <-ch
	assert.Equal(t, "t1-1", got.ID)
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
	status, msg := h.resolveStatusFromClaw("", errors.New("stream dropped"))
	assert.Equal(t, dbclient.OptimizationTaskStatusFailed, status)
	assert.Contains(t, msg, "stream dropped")
}
