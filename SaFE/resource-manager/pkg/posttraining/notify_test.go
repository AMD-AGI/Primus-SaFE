/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestRunInfoPrefersWrittenColumns(t *testing.T) {
	run := testRunRow("Running", "train")
	start := time.Date(2026, 11, 3, 8, 0, 0, 0, time.UTC)
	run.StartTime = pq.NullTime{Time: start, Valid: true}
	run.MetricSummary = `{"points":10,"main":{"name":"train/loss","step":340,"value":0.42137}}`
	info := runInfo(run, map[string]interface{}{"status": "Failed", "failed_stage": "train", "message": "OOM",
		"end_time": start.Add(62*time.Minute + 20*time.Second)})
	require.Equal(t, "Failed", info.Status)
	require.Equal(t, "train", info.FailedStage)
	require.Equal(t, "OOM", info.Message)
	require.Equal(t, "1h2m0s", info.Duration)
	require.Equal(t, "train/loss 0.4214 at step 340", info.MainMetric)
	require.Equal(t, "Qwen3-8B", info.BaseModel)
}

func TestNotifyWithoutManagerIsNoop(t *testing.T) {
	r, _, _ := newTestReconciler(t, testRunRow("Running", "train"))
	r.notifyChanges(context.Background(), testRunRow("Running", "train"),
		map[string]interface{}{"status": "Succeeded", "stalled_since": time.Now()})
}
