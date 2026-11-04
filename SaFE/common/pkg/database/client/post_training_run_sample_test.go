/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

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

func TestPostTrainingRunSampleInputs(t *testing.T) {
	c := &Client{}
	require.NoError(t, c.UpsertPostTrainingRunSamples(context.Background(), nil), "no samples need no write")
	_, err := c.CountPostTrainingRunSamples(context.Background(), "r1")
	require.Error(t, err, "expected error when gorm not initialized")
}
