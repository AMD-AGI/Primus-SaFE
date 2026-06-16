/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"testing"
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
