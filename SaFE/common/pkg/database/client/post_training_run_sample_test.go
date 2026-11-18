/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// Each run sample method runs its query.
func TestPostTrainingRunSampleCRUD(t *testing.T) {
	c, mock := newLooseMockClient(t)
	arm(mock, 20)
	ctx := context.Background()
	step := 3

	_ = c.UpsertPostTrainingRunSamples(ctx, []*PostTrainingRunSample{{RunID: "r1", Kind: "rollout"}})
	_, _ = c.CountPostTrainingRunSamples(ctx, "r1")
	_, _ = c.ListPostTrainingRunSamples(ctx, PostTrainingRunSampleFilter{RunID: "r1", Kind: "rollout", Step: 1})
	_, _ = c.ListPostTrainingRunSampleSteps(ctx, "r1", "rollout")
	_ = c.DeletePostTrainingRunSamples(ctx, "r1", &step)
	_ = c.DeletePostTrainingRunSamples(ctx, "r1", nil)
}

// Samples lose their NULs, fit their columns, and the last of a key is kept.
func TestSamplesAreCleanedForTheirColumns(t *testing.T) {
	c, mock := newMockClient(t)
	dataset := strings.Repeat("d", 130)
	mock.ExpectQuery(`INSERT INTO "post_training_run_sample" .* VALUES \(\$1,\$2,\$3,\$4,\$5,\$6,\$7,\$8,\$9,\$10,\$11,`+
		`\$12,\$13,\$14,\$15,\$16\) ON CONFLICT`).
		WithArgs("r1", "rollout", 1, dataset[:128], 0, 0, "p", "ab", nil, nil, nil, "ok", 0, 0, 0,
			sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	err := c.UpsertPostTrainingRunSamples(context.Background(), []*PostTrainingRunSample{
		{RunID: "r1", Kind: "rollout", Step: 1, Dataset: dataset, Prompt: "p", Response: "x", Status: "ok"},
		{RunID: "r1", Kind: "rollout", Step: 1, Dataset: dataset + "x", Prompt: "p", Response: "a\x00b", Status: "ok"},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet(),
		"a NUL is dropped, the dataset fits its column, and the sample that now shares its key replaces the other")
}

// Writing no samples needs no database, and reading them fails without one.
func TestPostTrainingRunSampleInputs(t *testing.T) {
	c := &Client{}
	require.NoError(t, c.UpsertPostTrainingRunSamples(context.Background(), nil), "no samples need no write")
	_, err := c.CountPostTrainingRunSamples(context.Background(), "r1")
	require.Error(t, err, "expected error when gorm not initialized")
}
