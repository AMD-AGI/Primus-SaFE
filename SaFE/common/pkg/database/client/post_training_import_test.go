/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestListPostTrainingImportsOfRunsReadsOnlyWhatTheListShows(t *testing.T) {
	c, mock := newMockClient(t)
	ctx := context.Background()
	none, err := c.ListPostTrainingImportsOfRuns(ctx, nil)
	require.NoError(t, err)
	require.Empty(t, none, "a page without imported runs makes no query")

	mock.ExpectQuery(`SELECT "run_id","source_path","slurm_job_id","via","verdict","summary","created_at" ` +
		`FROM "post_training_import" WHERE run_id IN \(\$1,\$2\) AND state = \$3 AND is_deleted = \$4`).
		WillReturnRows(sqlmock.NewRows([]string{"run_id", "source_path", "verdict"}).
			AddRow("spur-a", "/runs/a", "PASS").AddRow("spur-b", "/runs/b", "FAIL"))
	got, err := c.ListPostTrainingImportsOfRuns(ctx, []string{"spur-a", "spur-b"})
	require.NoError(t, err)
	require.Equal(t, "/runs/a", got["spur-a"].SourcePath)
	require.Equal(t, "FAIL", got["spur-b"].Verdict)
	require.NoError(t, mock.ExpectationsWereMet())
}
