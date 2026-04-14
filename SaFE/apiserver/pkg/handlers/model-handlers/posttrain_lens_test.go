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

	"gotest.tools/assert"
	"github.com/lib/pq"

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

func TestFetchLatestLossSummary_UsesTrainingPerformancePoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.URL.Path, "/workloads/lens-uid-123/trainingPerformance")
		assert.Equal(t, r.URL.Query().Get("start"), "1000")
		assert.Equal(t, r.URL.Query().Get("end"), "2000")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"meta": map[string]interface{}{
				"code":    2000,
				"message": "OK",
			},
			"data": []map[string]interface{}{
				{
					"metric":    "tflops",
					"value":     100.0,
					"timestamp": 1100,
				},
				{
					"metric":    "lm_loss",
					"value":     2.3,
					"timestamp": 1200,
				},
				{
					"metric":    "lm_loss",
					"value":     1.7,
					"timestamp": 1300,
				},
			},
		})
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
