/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	sqrl "github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/require"
)

func TestSelectWorkloadsForList_FailedConditions(t *testing.T) {
	c, mock := newMockClient(t)
	for _, excluded := range []string{"pods", "nodes", "ranks", "env", "secrets", "images", "entrypoints"} {
		require.NotContains(t, workloadListColumns, excluded)
	}
	mock.ExpectQuery(`SELECT .*CASE WHEN phase = 'Failed' THEN conditions ELSE NULL END AS conditions FROM workload WHERE workspace = \$1 ORDER BY id DESC LIMIT 10 OFFSET 5`).
		WithArgs("test-workspace").WillReturnRows(sqlmock.NewRows([]string{"workload_id", "phase", "conditions"}).
		AddRow("failed-workload", "Failed", `[{"type":"AdminFailed","message":"registration failed"}]`).AddRow("running-workload", "Running", nil))
	rows, err := c.SelectWorkloadsForList(context.Background(), sqrl.Eq{"workspace": "test-workspace"}, []string{"id DESC"}, 10, 5)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.True(t, rows[0].Conditions.Valid)
	require.Contains(t, rows[0].Conditions.String, "registration failed")
	require.False(t, rows[1].Conditions.Valid)
	require.NoError(t, mock.ExpectationsWereMet())
}
