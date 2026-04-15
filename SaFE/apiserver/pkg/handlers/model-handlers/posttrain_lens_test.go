/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lib/pq"
	"gotest.tools/assert"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

func TestResolveLensWorkloadUID_UnwrapsLensEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.URL.Path, "/workloads")
		assert.Equal(t, r.URL.Query().Get("name"), "workload-123")
		assert.Equal(t, r.URL.Query().Get("namespace"), "ws1")
		assert.Equal(t, r.URL.Query().Get("kind"), "PyTorchJob")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"meta": map[string]interface{}{
				"code":    2000,
				"message": "OK",
			},
			"data": map[string]interface{}{
				"data": []map[string]interface{}{
					{
						"kind":      "PyTorchJob",
						"name":      "workload-123",
						"namespace": "ws1",
						"uid":       "lens-uid-123",
					},
				},
				"total": 1,
			},
		})
	}))
	defer server.Close()

	t.Setenv("POSTTRAIN_LENS_API_URL", server.URL)

	uid, err := resolveLensWorkloadUID(context.Background(), "workload-123", "ws1", "control-plane", "sft")
	assert.NilError(t, err)
	assert.Equal(t, uid, "lens-uid-123")
}

func TestFetchLatestLossSummary_UsesMetricsData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/workloads/lens-uid-123/metrics/available":
			assert.Equal(t, r.URL.Query().Get("start"), "")
			assert.Equal(t, r.URL.Query().Get("end"), "")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"meta": map[string]interface{}{
					"code":    2000,
					"message": "OK",
				},
				"data": map[string]interface{}{
					"workload_uid": "lens-uid-123",
					"metrics": []map[string]interface{}{
						{
							"name":        "tflops",
							"data_source": []string{"log"},
							"count":       1,
						},
						{
							"name":        "lm_loss",
							"data_source": []string{"log", "wandb"},
							"count":       2,
						},
					},
					"total_count": 2,
				},
			})
		case "/workloads/lens-uid-123/metrics/data":
			assert.Equal(t, r.URL.Query().Get("start"), "1000")
			assert.Equal(t, r.URL.Query().Get("end"), "2000")
			assert.Equal(t, r.URL.Query().Get("data_source"), "log")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"meta": map[string]interface{}{
					"code":    2000,
					"message": "OK",
				},
				"data": map[string]interface{}{
					"workload_uid": "lens-uid-123",
					"data_source":  "log",
					"data": []map[string]interface{}{
						{
							"metric_name": "tflops",
							"value":       100.0,
							"timestamp":   1100,
							"iteration":   1,
							"data_source": "log",
						},
						{
							"metric_name": "lm_loss",
							"value":       2.3,
							"timestamp":   1200,
							"iteration":   2,
							"data_source": "log",
						},
						{
							"metric_name": "lm_loss",
							"value":       1.7,
							"timestamp":   1300,
							"iteration":   3,
							"data_source": "log",
						},
					},
					"total_count": 3,
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv("POSTTRAIN_LENS_API_URL", server.URL)

	summary, metrics, err := fetchLatestLossSummary(context.Background(), "lens-uid-123", "1000", "2000")
	assert.NilError(t, err)
	assert.DeepEqual(t, metrics, []string{"lm_loss", "tflops"})
	assert.Assert(t, summary != nil)
	assert.Equal(t, summary.MetricName, "lm_loss")
	assert.Equal(t, summary.Value, 1.7)
	assert.Equal(t, summary.DataSource, "log")
}

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
