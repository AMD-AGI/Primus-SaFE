/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

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

func TestUpdatePostTrainingRunNotFound(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectExec(`UPDATE "post_training_run" SET`).WillReturnResult(sqlmock.NewResult(0, 0))

	err := c.UpdatePostTrainingRun(context.Background(), "missing", map[string]interface{}{"stage": "train"})
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

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

func TestDeletePostTrainingRunMetricsAfterStepKeepsSystemSeries(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectExec(`DELETE FROM "post_training_run_metric" WHERE run_id = \$1 AND step > \$2 AND name NOT LIKE \$3`).
		WithArgs("r1", int64(10), "system/%").
		WillReturnResult(sqlmock.NewResult(0, 3))

	require.NoError(t, c.DeletePostTrainingRunMetricsAfterStep(context.Background(), "r1", 10))
	require.NoError(t, mock.ExpectationsWereMet())
}

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

func TestCountPostTrainingRunsByRecipe(t *testing.T) {
	c, mock := newMockClient(t)
	mock.ExpectQuery(`SELECT recipe_hash, COUNT\(\*\) AS succeeded, .*FROM "post_training_run" WHERE .*recipe_hash IN .*GROUP BY "recipe_hash"`).
		WithArgs("h1", "h2", postTrainingRunSucceeded, false).
		WillReturnRows(sqlmock.NewRows([]string{"recipe_hash", "succeeded", "latest_run"}).AddRow("h1", 2, "pt-b"))
	got, err := c.CountPostTrainingRunsByRecipe(context.Background(), []string{"h1", "h2"})
	require.NoError(t, err)
	require.Equal(t, map[string]RecipeRecord{"h1": {RecipeHash: "h1", Succeeded: 2, LatestRun: "pt-b"}}, got)
	require.NoError(t, mock.ExpectationsWereMet())

	empty, err := (&Client{}).CountPostTrainingRunsByRecipe(context.Background(), nil)
	require.NoError(t, err, "no hashes need no query")
	require.Empty(t, empty)
}
