/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
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

// ListOptimizationTasks keeps only the tasks in the filter's scope: those of the scope's user or in
// one of its workspaces, those of the user alone, those of the workspaces alone, or none at all
// for the zero scope; without a scope it keeps every task.
func TestListOptimizationTasksScope(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scope *OptimizationTaskScope
		where string
		args  []driver.Value
	}{
		{"none", nil, `WHERE is_deleted = \$1 ORDER BY`, []driver.Value{false}},
		{"user and workspaces", &OptimizationTaskScope{UserID: "u1", Workspaces: []string{"ws1", "ws2"}},
			`WHERE is_deleted = \$1 AND \(+user_id = \$2 OR workspace IN \(\$3,\$4\)\)+ ORDER BY`,
			[]driver.Value{false, "u1", "ws1", "ws2"}},
		{"user", &OptimizationTaskScope{UserID: "u1"},
			`WHERE is_deleted = \$1 AND user_id = \$2 ORDER BY`, []driver.Value{false, "u1"}},
		{"workspaces", &OptimizationTaskScope{Workspaces: []string{"ws1"}},
			`WHERE is_deleted = \$1 AND workspace IN \(\$2\) ORDER BY`, []driver.Value{false, "ws1"}},
		{"empty", &OptimizationTaskScope{}, `WHERE is_deleted = \$1 AND FALSE ORDER BY`, []driver.Value{false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, mock := newMockClient(t)
			count := strings.Replace(tc.where, " ORDER BY", "$", 1)
			mock.ExpectQuery(`SELECT count\(\*\) FROM "optimization_task" ` + count).
				WithArgs(tc.args...).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			mock.ExpectQuery(`SELECT \* FROM "optimization_task" ` + tc.where + ` created_at DESC`).
				WithArgs(tc.args...).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("t1"))
			tasks, total, err := c.ListOptimizationTasks(context.Background(), OptimizationTaskFilter{Scope: tc.scope})
			require.NoError(t, err)
			require.Equal(t, int64(1), total)
			require.Len(t, tasks, 1)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
