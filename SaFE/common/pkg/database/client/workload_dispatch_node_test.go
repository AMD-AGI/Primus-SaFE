/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestGetWorkloadDispatchNodeValidation(t *testing.T) {
	c, _ := newMockClient(t)
	_, err := c.GetWorkloadDispatchNode(context.Background(), "", 0)
	require.Error(t, err)
	_, err = c.GetWorkloadDispatchNode(context.Background(), "w1", -1)
	require.Error(t, err)
}

func TestGetWorkloadDispatchNodeSuccess(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery("SELECT \\* FROM workload_dispatch_node WHERE workload_id =").
		WithArgs("w1", 2).
		WillReturnRows(sqlmock.NewRows([]string{"workload_id", "dispatch_index", "nodes", "ranks", "updated_at"}).
			AddRow("w1", 2, `["n1"]`, `["0"]`, pq.NullTime{Time: time.Now(), Valid: true}))

	row, err := c.GetWorkloadDispatchNode(context.Background(), "w1", 2)
	require.NoError(t, err)
	require.Equal(t, "w1", row.WorkloadId)
	require.Equal(t, 2, row.DispatchIndex)
	require.Equal(t, `["n1"]`, row.Nodes.String)
}
