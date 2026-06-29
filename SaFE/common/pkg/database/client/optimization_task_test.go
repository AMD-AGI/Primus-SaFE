/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestOptimizationTaskCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 60)
	ctx := context.Background()

	_ = c.UpsertOptimizationTask(ctx, &OptimizationTask{ID: "t1"})
	_, _ = c.GetOptimizationTask(ctx, "t1")
	_, _, _ = c.ListOptimizationTasks(ctx, OptimizationTaskFilter{
		Workspace: "ws", Status: "running", ModelID: "m", UserID: "u", Search: "x", Limit: 10, Offset: 0,
	})
	_ = c.UpdateOptimizationTaskStatus(ctx, "t1", OptimizationTaskStatusRunning, 1, "msg")
	_ = c.UpdateOptimizationTaskStatus(ctx, "t1", OptimizationTaskStatusSucceeded, 2, "done")
	_ = c.UpdateOptimizationTaskStatus(ctx, "t1", OptimizationTaskStatus("pending"), 0, "")
	_ = c.UpdateOptimizationTaskClawSession(ctx, "t1", "sess")
	_ = c.UpdateOptimizationTaskResult(ctx, "t1", "{}", "/report")
	_ = c.DeleteOptimizationTask(ctx, "t1")
	_, _ = c.CountRunningOptimizationTasks(ctx, "ws")
	_ = c.AppendOptimizationEvent(ctx, &OptimizationEvent{TaskID: "t1"})
	_, _ = c.ListOptimizationEvents(ctx, "t1", 0, 10)
	_, _, _ = c.OptimizationEventSeq(ctx, "t1", "t1-1")
	_, _ = c.LatestOptimizationEventSeq(ctx, "t1")
}

func TestOptimizationTaskGormNotInitialized(t *testing.T) {
	c := &Client{}
	ctx := context.Background()
	_, err := c.GetOptimizationTask(ctx, "t1")
	if err == nil {
		t.Fatal("expected error when gorm not initialized")
	}
}

func TestAppendOptimizationEventDuplicate(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery("INSERT INTO .*optimization_event").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	err := c.AppendOptimizationEvent(context.Background(), &OptimizationEvent{
		EventID: "event-1",
		TaskID:  "t1",
		Type:    "log",
		Payload: "{}",
		Seq:     1,
	})
	if !errors.Is(err, ErrOptimizationEventDuplicate) {
		t.Fatalf("expected duplicate sentinel, got %v", err)
	}
}

func TestOptimizationEventSeqFoundAndMissing(t *testing.T) {
	ctx := context.Background()

	c, mock := newMockClient(t)
	mock.ExpectQuery("SELECT .*seq.* FROM .*optimization_event").
		WillReturnRows(sqlmock.NewRows([]string{"seq"}).AddRow(int64(42)))
	seq, ok, err := c.OptimizationEventSeq(ctx, "t1", "event-1")
	if err != nil || !ok || seq != 42 {
		t.Fatalf("expected seq=42 ok=true err=nil, got seq=%d ok=%v err=%v", seq, ok, err)
	}

	c2, mock2 := newMockClient(t)
	mock2.ExpectQuery("SELECT .*seq.* FROM .*optimization_event").
		WillReturnRows(sqlmock.NewRows([]string{"seq"}))
	seq, ok, err = c2.OptimizationEventSeq(ctx, "t1", "missing")
	if err != nil || ok || seq != 0 {
		t.Fatalf("expected missing event to return seq=0 ok=false err=nil, got seq=%d ok=%v err=%v", seq, ok, err)
	}
}
