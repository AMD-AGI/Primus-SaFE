/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"testing"

	"github.com/stretchr/testify/require"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

func TestPickSampleStep(t *testing.T) {
	steps := []dbclient.PostTrainingRunSampleStep{
		{Step: 40, Dataset: "aime", Count: 8}, {Step: 40, Dataset: "math", Count: 8}, {Step: 20, Dataset: "aime", Count: 8},
	}
	step, dataset, ok := pickSampleStep(steps, "", "")
	require.True(t, ok)
	require.Equal(t, 40, step, "the latest step by default")
	require.Equal(t, "aime", dataset)
	step, dataset, ok = pickSampleStep(steps, "40", "math")
	require.True(t, ok)
	require.Equal(t, "math", dataset)
	require.Equal(t, 40, step)
	step, dataset, ok = pickSampleStep(steps, "20", "math")
	require.True(t, ok)
	require.Equal(t, "aime", dataset, "a step without the dataset shows its first")
	require.Equal(t, 20, step)
	_, _, ok = pickSampleStep(steps, "30", "")
	require.False(t, ok, "a step without samples")
	_, _, ok = pickSampleStep(nil, "", "")
	require.False(t, ok)
}

func TestSamplesCapped(t *testing.T) {
	require.True(t, samplesCapped(&dbclient.PostTrainingRun{MetricSummary: `{"samples":50000,"samplesCapped":true}`}))
	require.False(t, samplesCapped(&dbclient.PostTrainingRun{}))
}
