/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Each run method runs its query.
func TestPostTrainingRunCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 40)
	ctx := context.Background()

	_ = c.CreatePostTrainingRun(ctx, &PostTrainingRun{RunID: "r1", WorkloadID: "w1"})
	_, _ = c.GetPostTrainingRun(ctx, "r1")
	_, _, _ = c.ListPostTrainingRuns(ctx, PostTrainingRunFilter{
		Workspace: "ws", Framework: "slime", TrainType: "sft", Method: "full", Statuses: []string{"Running"},
		Stage: "train", BaseModelID: "m", UserID: "u", TemplateID: "t", ExperimentID: "e", Search: "x",
		Limit: 10, Offset: 10,
	})
	_ = c.SetPostTrainingRunDeleted(ctx, "r1")
	_, _ = c.ListPostTrainingRunMetrics(ctx, "r1", []string{"train/loss"})
	_, _ = c.ListPostTrainingRunMetricNames(ctx, "r1")
	_ = c.DeletePostTrainingRunMetrics(ctx, "r1")
}

// The run search matches a pasted run ID as well as the display name.
func TestARunIsFoundByItsIDToo(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(`SELECT count\(\*\) FROM "post_training_run" WHERE is_deleted = \$1 AND `+
		`\(display_name ILIKE \$2 OR run_id = \$3\)`).
		WithArgs(false, "%pt-sft-ab12%", "pt-sft-ab12").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery(`SELECT \* FROM "post_training_run" WHERE is_deleted = \$1 AND ` +
		`\(display_name ILIKE \$2 OR run_id = \$3\)`).
		WillReturnRows(sqlmock.NewRows([]string{"run_id"}))
	_, _, err := c.ListPostTrainingRuns(context.Background(), PostTrainingRunFilter{Search: " pt-sft-ab12 "})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet(), "a pasted run ID finds its run")
}

// A limited curve read takes the newest points and returns them by name and step.
func TestACurveRequestReadsTheNewestPoints(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(`SELECT \* FROM "post_training_run_metric" WHERE run_id = \$1 AND name IN \(\$2,\$3\) `+
		`ORDER BY step DESC LIMIT \$4`).
		WithArgs("r1", "a", "b", 3).
		WillReturnRows(sqlmock.NewRows([]string{"run_id", "name", "step", "value"}).
			AddRow("r1", "b", 9, 1.0).AddRow("r1", "a", 9, 2.0).AddRow("r1", "a", 8, 3.0))
	got, err := c.ListLatestPostTrainingRunMetrics(context.Background(), "r1", []string{"a", "b"}, 3)
	require.NoError(t, err)
	var order []string
	for _, p := range got {
		order = append(order, fmt.Sprintf("%s@%d", p.Name, p.Step))
	}
	require.Equal(t, []string{"a@8", "a@9", "b@9"}, order, "the newest points come back by name and step")
	require.NoError(t, mock.ExpectationsWereMet())
}

// The run methods fail without a database.
func TestPostTrainingRunInputs(t *testing.T) {
	c := &Client{}
	ctx := context.Background()

	require.Error(t, c.CreatePostTrainingRun(ctx, nil))
	_, err := c.GetPostTrainingRun(ctx, "r1")
	require.Error(t, err, "expected error when gorm not initialized")
	require.NoError(t, c.UpdatePostTrainingRun(ctx, "r1", nil))
	require.NoError(t, c.SavePostTrainingRunMetrics(ctx, "r1", []*PostTrainingRunMetric{nil}, nil))
	require.Error(t, c.DeletePostTrainingRunMetrics(ctx, "r1"))
}

// A checkpoint cleanup request is taken only while the run has none.
func TestRequestPostTrainingRunCleanupTakesOneRequest(t *testing.T) {
	c, mock := newMockClient(t)
	ctx := context.Background()
	mock.ExpectExec(`UPDATE "post_training_run" SET "cleanup_request"=\$1,"updated_at"=\$2 WHERE .*\(cleanup_request IS NULL OR cleanup_request = ''\)`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	taken, err := c.RequestPostTrainingRunCleanup(ctx, "pt-1", `{"keep":"none"}`)
	require.NoError(t, err)
	require.True(t, taken)
	mock.ExpectExec(`UPDATE "post_training_run" SET "cleanup_request"`).WillReturnResult(sqlmock.NewResult(0, 0))
	taken, err = c.RequestPostTrainingRunCleanup(ctx, "pt-1", `{"keep":"best"}`)
	require.NoError(t, err)
	require.False(t, taken, "a run that has a request keeps it")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Updating a missing run is record not found.
func TestUpdatePostTrainingRunNotFound(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectExec(`UPDATE "post_training_run" SET`).WillReturnResult(sqlmock.NewResult(0, 0))

	err := c.UpdatePostTrainingRun(context.Background(), "missing", map[string]interface{}{"stage": "train"})
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Metric points and run columns are written in one transaction, a repeated point once.
func TestSavePostTrainingRunMetricsCommits(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "post_training_run_metric" .* ON CONFLICT \("run_id","name","step"\) ` +
		`DO UPDATE SET "value"="excluded"."value","wall_time"="excluded"."wall_time"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)).AddRow(int64(2)))
	mock.ExpectExec(`UPDATE "post_training_run" SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	now := time.Now()
	points := []*PostTrainingRunMetric{
		{Name: "train/loss", Step: 1, Value: 2.0, WallTime: now},
		{Name: "train/loss", Step: 1, Value: 1.5, WallTime: now},
		{Name: "eval/val/loss", Step: 1, Value: math.NaN(), WallTime: now},
	}
	err := c.SavePostTrainingRunMetrics(context.Background(), "r1", points,
		map[string]interface{}{"metric_cursor": `{"seq":3}`})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Metric points are rolled back when the run they update is missing.
func TestSavePostTrainingRunMetricsRollsBackWhenRunMissing(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "post_training_run_metric"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	mock.ExpectExec(`UPDATE "post_training_run" SET`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	points := []*PostTrainingRunMetric{{Name: "train/loss", Step: 1, Value: 1, WallTime: time.Now()}}
	err := c.SavePostTrainingRunMetrics(context.Background(), "missing", points,
		map[string]interface{}{"metric_gap": true})
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Rewinding a run deletes the points after a step but keeps the system series.
func TestDeletePostTrainingRunMetricsAfterStepKeepsSystemSeries(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectExec(`DELETE FROM "post_training_run_metric" WHERE run_id = \$1 AND step > \$2 AND name NOT LIKE \$3`).
		WithArgs("r1", int64(10), "system/%").
		WillReturnResult(sqlmock.NewResult(0, 3))

	require.NoError(t, c.DeletePostTrainingRunMetricsAfterStep(context.Background(), "r1", 10))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Of the points of one name and step the last is kept, and every point gets the run ID.
func TestDedupePostTrainingRunMetrics(t *testing.T) {
	first := &PostTrainingRunMetric{Name: "train/loss", Step: 1, Value: 1}
	other := &PostTrainingRunMetric{Name: "train/loss", Step: 2, Value: 2}
	last := &PostTrainingRunMetric{Name: "train/loss", Step: 1, Value: 3}

	out := dedupePostTrainingRunMetrics("r1", []*PostTrainingRunMetric{first, nil, other, last})
	require.Equal(t, []*PostTrainingRunMetric{last, other}, out)
	for _, p := range out {
		require.Equal(t, "r1", p.RunID)
	}
}

// A recipe's successful runs are counted on the platform, and the latest one is taken from the workspace only.
func TestListPostTrainingRecipeRecords(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(`SELECT recipe_hash, COUNT\(\*\) AS succeeded, COALESCE\(\(ARRAY_AGG\(run_id ORDER BY `+
		`end_time DESC NULLS LAST\) FILTER \(WHERE workspace = \$1\)\)\[1\], ''\) AS latest_run `+
		`FROM "post_training_run" WHERE .*recipe_hash IN .*GROUP BY "recipe_hash"`).
		WithArgs("ws1", "h1", "h2", postTrainingRunSucceeded, PostTrainingSourceSafe, false).
		WillReturnRows(sqlmock.NewRows([]string{"recipe_hash", "succeeded", "latest_run"}).
			AddRow("h1", 2, "pt-b").AddRow("h2", 5, ""))
	got, err := c.ListPostTrainingRecipeRecords(context.Background(), "ws1", []string{"h1", "h2"})
	require.NoError(t, err)
	require.Equal(t, map[string]RecipeRecord{"h1": {RecipeHash: "h1", Succeeded: 2, LatestRun: "pt-b"},
		"h2": {RecipeHash: "h2", Succeeded: 5}}, got, "h2 succeeded in other workspaces only")
	require.NoError(t, mock.ExpectationsWereMet())

	empty, err := (&Client{}).ListPostTrainingRecipeRecords(context.Background(), "ws1", nil)
	require.NoError(t, err, "no hashes need no query")
	require.Empty(t, empty)
}
