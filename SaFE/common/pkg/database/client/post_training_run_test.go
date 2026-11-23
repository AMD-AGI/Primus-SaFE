/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"fmt"
	"math"
	"strings"
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
		Stage: "train", BaseModelID: "m", UserID: "u", Search: "x",
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
		`\(display_name ILIKE \$2 OR run_id = \$3\) ORDER BY created_at DESC, id DESC`).
		WillReturnRows(sqlmock.NewRows([]string{"run_id"}))
	_, _, err := c.ListPostTrainingRuns(context.Background(), PostTrainingRunFilter{Search: " pt-sft-ab12 "})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet(), "a pasted run ID finds its run")
}

// The run search takes the LIKE wildcards of the text literally.
func TestARunSearchEscapesWildcards(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(`SELECT count\(\*\) FROM "post_training_run" WHERE is_deleted = \$1 AND `+
		`\(display_name ILIKE \$2 OR run_id = \$3\)`).
		WithArgs(false, `%100\%\_sft\\a%`, `100%_sft\a`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery(`SELECT \* FROM "post_training_run"`).WillReturnRows(sqlmock.NewRows([]string{"run_id"}))
	_, _, err := c.ListPostTrainingRuns(context.Background(), PostTrainingRunFilter{Search: `100%_sft\a`})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// A curve read of a run within the limit is not cut; past it each series keeps its newest points, the short ones
// all of theirs, and the long ones share what is left.
func TestACurveRequestCutsOnlyARunPastTheLimit(t *testing.T) {
	c, mock := newMockClient(t)
	counts := `SELECT name, COUNT\(\*\) AS points FROM "post_training_run_metric" WHERE run_id = \$1 AND ` +
		`name IN \(\$2,\$3\) GROUP BY "name"`
	mock.ExpectQuery(counts).WithArgs("r1", "a", "b").
		WillReturnRows(sqlmock.NewRows([]string{"name", "points"}).AddRow("a", 2).AddRow("b", 1))
	mock.ExpectQuery(`SELECT \* FROM "post_training_run_metric" WHERE run_id = \$1 AND name IN \(\$2,\$3\) `+
		`ORDER BY name ASC, step ASC`).WithArgs("r1", "a", "b").
		WillReturnRows(sqlmock.NewRows([]string{"run_id", "name", "step", "value"}).
			AddRow("r1", "a", 8, 3.0).AddRow("r1", "a", 9, 2.0).AddRow("r1", "b", 9, 1.0))
	got, err := c.ListLatestPostTrainingRunMetrics(context.Background(), "r1", []string{"a", "b"}, 3)
	require.NoError(t, err)
	require.Len(t, got, 3, "three points fit a limit of three")

	mock.ExpectQuery(counts).WithArgs("r1", "a", "b").
		WillReturnRows(sqlmock.NewRows([]string{"name", "points"}).AddRow("a", 5).AddRow("b", 1))
	mock.ExpectQuery(`SELECT \* FROM \(SELECT \*, ROW_NUMBER\(\) OVER \(PARTITION BY name ORDER BY step DESC\) AS rn `+
		`FROM "post_training_run_metric" WHERE run_id = \$1 AND name IN \(\$2,\$3\)\) AS latest `+
		`WHERE rn <= \$4 ORDER BY name ASC, step ASC`).WithArgs("r1", "a", "b", 2).
		WillReturnRows(sqlmock.NewRows([]string{"run_id", "name", "step", "value", "rn"}).
			AddRow("r1", "a", 8, 3.0, 2).AddRow("r1", "a", 9, 2.0, 1).AddRow("r1", "b", 9, 1.0, 1))
	got, err = c.ListLatestPostTrainingRunMetrics(context.Background(), "r1", []string{"a", "b"}, 3)
	require.NoError(t, err)
	var order []string
	for _, p := range got {
		order = append(order, fmt.Sprintf("%s@%d", p.Name, p.Step))
	}
	require.Equal(t, []string{"a@8", "a@9", "b@9"}, order, "b keeps its one point and a the two that are left")
	require.NoError(t, mock.ExpectationsWereMet())
}

// The limit of a curve read is shared by filling the short series first.
func TestSeriesQuota(t *testing.T) {
	sixty := make([]int64, 60)
	for i := range sixty {
		sixty[i] = 500
	}
	sixty[0] = 10000
	_, cut := seriesQuota(sixty, 200000)
	require.False(t, cut, "a run of 39500 points is far within a limit of 200000")
	quota, cut := seriesQuota([]int64{1000, 10, 10}, 320)
	require.True(t, cut)
	require.EqualValues(t, 300, quota, "the two short series take 20 and leave 300 to the long one")
	quota, _ = seriesQuota([]int64{1000, 1000, 1000}, 300)
	require.EqualValues(t, 100, quota)
	quota, cut = seriesQuota([]int64{5, 5, 5}, 2)
	require.True(t, cut)
	require.EqualValues(t, 1, quota, "every series keeps at least its newest point")
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

// Rewinding a run deletes the points after a step and says how many it deleted.
func TestDeletePostTrainingRunMetricsAfterStep(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectExec(`DELETE FROM "post_training_run_metric" WHERE run_id = \$1 AND step > \$2`).
		WithArgs("r1", int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 3))

	deleted, err := c.DeletePostTrainingRunMetricsAfterStep(context.Background(), "r1", 10)
	require.NoError(t, err)
	require.Equal(t, int64(3), deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Of the points of one name and step the last is kept, and every point gets the run ID.
func TestDedupePostTrainingRunMetrics(t *testing.T) {
	first := &PostTrainingRunMetric{Name: "train/loss", Step: 1, Value: 1}
	other := &PostTrainingRunMetric{Name: "train/loss", Step: 2, Value: 2}
	last := &PostTrainingRunMetric{Name: "train/loss", Step: 1, Value: 3}

	long := &PostTrainingRunMetric{Name: "eval/" + strings.Repeat("x", maxPostTrainingMetricNameBytes), Step: 1}

	out := dedupePostTrainingRunMetrics("r1", []*PostTrainingRunMetric{first, nil, other, long, last, long})
	require.Equal(t, []*PostTrainingRunMetric{last, other}, out, "a name too long to index is dropped")
	for _, p := range out {
		require.Equal(t, "r1", p.RunID)
	}
}

// A run counts as deleted when its row is marked so, and not when it has no row.
func TestIsPostTrainingRunDeleted(t *testing.T) {
	c, mock := newMockClient(t)
	for _, rows := range []int{1, 0} {
		mock.ExpectQuery(`SELECT count\(\*\) FROM "post_training_run" WHERE run_id = \$1 AND is_deleted = \$2`).
			WithArgs("r1", true).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(rows))
		deleted, err := c.IsPostTrainingRunDeleted(context.Background(), "r1")
		require.NoError(t, err)
		require.Equal(t, rows == 1, deleted)
	}
	require.NoError(t, mock.ExpectationsWereMet())
	_, err := (&Client{}).IsPostTrainingRunDeleted(context.Background(), "r1")
	require.Error(t, err, "without a database it cannot tell")
}

// A recipe's successful runs are counted on the platform, and the latest one is taken from the workspace only.
func TestListPostTrainingRecipeRecords(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(`SELECT recipe_hash, COUNT\(\*\) AS succeeded, COALESCE\(\(ARRAY_AGG\(run_id ORDER BY `+
		`end_time DESC NULLS LAST\) FILTER \(WHERE workspace = \$1\)\)\[1\], ''\) AS latest_run `+
		`FROM "post_training_run" WHERE .*recipe_hash IN .*GROUP BY "recipe_hash"`).
		WithArgs("ws1", "h1", "h2", postTrainingRunSucceeded, false).
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
