/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"testing"
	"time"

	"github.com/lib/pq"
	"gotest.tools/assert"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// NOTE: The former TestResolveLensWorkloadUID_UnwrapsLensEnvelope and
// TestFetchLatestLossSummary_UsesMetricsData tests spun up an httptest server
// that emulated the primus-lens-api. After migrating posttrain_lens.go to
// use robustclient → data-plane robust-api, those handler paths are no
// longer HTTP-level but go through robustclient.Client, which requires a
// registered per-cluster endpoint. They are covered now by integration
// tests in resource-manager / robustclient packages.

func TestBuildPosttrainMetricSeries_AddsLossAlias(t *testing.T) {
	points := []PosttrainMetricPoint{
		{
			MetricName: "lm_loss",
			Value:      2.3,
			Timestamp:  1200,
			Iteration:  2,
			DataSource: "log",
		},
		{
			MetricName: "lm_loss",
			Value:      1.7,
			Timestamp:  1300,
			Iteration:  3,
			DataSource: "log",
		},
		{
			MetricName: "tflops",
			Value:      100,
			Timestamp:  1100,
			Iteration:  1,
			DataSource: "log",
		},
	}

	series := buildPosttrainMetricSeries(points, "lm_loss")
	assert.Assert(t, series != nil)
	assert.Equal(t, len(series["loss"]), 2)
	assert.Equal(t, series["loss"][0].Step, int32(2))
	assert.Equal(t, series["loss"][1].Value, 1.7)
	assert.Equal(t, len(series["lm_loss"]), 2)
}

func TestDefaultLensTimeRangeForRun_UsesStartAndEndTimes(t *testing.T) {
	start := time.Date(2026, 4, 12, 8, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 12, 8, 30, 0, 0, time.UTC)
	created := time.Date(2026, 4, 12, 7, 59, 0, 0, time.UTC)

	run := &dbclient.PosttrainRunView{
		StartTime: pq.NullTime{Time: start, Valid: true},
		EndTime:   pq.NullTime{Time: end, Valid: true},
		CreatedAt: pq.NullTime{Time: created, Valid: true},
	}

	startMs, endMs, err := defaultLensTimeRangeForRun(run)
	assert.NilError(t, err)
	assert.Equal(t, startMs, "1775980800000")
	assert.Equal(t, endMs, "1775982600000")
}
