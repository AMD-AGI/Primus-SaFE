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

func TestPickLossMetricName_PreferredOrder(t *testing.T) {
	tests := []struct {
		name     string
		metrics  []string
		expected string
	}{
		{"exact loss match", []string{"tflops", "loss"}, "loss"},
		{"lm_loss preferred", []string{"tflops", "lm_loss"}, "lm_loss"},
		{"contains loss", []string{"tflops", "my_training_loss"}, "my_training_loss"},
		{"no loss metric", []string{"tflops", "throughput"}, ""},
		{"empty", []string{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := pickLossMetricName(tc.metrics)
			assert.Equal(t, result, tc.expected)
		})
	}
}

func TestLatestLossSummaryFromPoints(t *testing.T) {
	points := []PosttrainMetricPoint{
		{MetricName: "lm_loss", Value: 2.3, Timestamp: 1200, DataSource: "log"},
		{MetricName: "lm_loss", Value: 1.7, Timestamp: 1300, DataSource: "log"},
		{MetricName: "tflops", Value: 100.0, Timestamp: 1100, DataSource: "log"},
	}
	metrics := []string{"lm_loss", "tflops"}

	summary := latestLossSummaryFromPoints(points, metrics)
	assert.Assert(t, summary != nil)
	assert.Equal(t, summary.MetricName, "lm_loss")
	assert.Equal(t, summary.Value, 1.7)
	assert.Equal(t, summary.DataSource, "log")
}

func TestFilterTrainingPerformancePoints(t *testing.T) {
	points := []PosttrainMetricPoint{
		{MetricName: "lm_loss", Value: 2.3},
		{MetricName: "tflops", Value: 100.0},
		{MetricName: "throughput", Value: 50.0},
	}

	filtered := filterTrainingPerformancePoints(points, "lm_loss,tflops")
	assert.Equal(t, len(filtered), 2)

	all := filterTrainingPerformancePoints(points, "all")
	assert.Equal(t, len(all), 3)
}

func TestPickWorkloadUID(t *testing.T) {
	items := []robustWorkloadListItem{
		{Name: "workload-1", Namespace: "ws1", Kind: "PyTorchJob", UID: "uid-1"},
		{Name: "workload-1", Namespace: "ws1", Kind: "RayJob", UID: "uid-2"},
	}

	uid := pickWorkloadUID(items, "workload-1", "ws1", "PyTorchJob")
	assert.Equal(t, uid, "uid-1")

	uid = pickWorkloadUID(items, "workload-1", "ws1", "")
	assert.Equal(t, uid, "uid-1")

	uid = pickWorkloadUID(items, "nonexistent", "ws1", "")
	assert.Equal(t, uid, "")
}
