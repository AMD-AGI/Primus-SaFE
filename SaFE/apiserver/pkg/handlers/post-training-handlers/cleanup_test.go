/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// cleanupRun returns a finished SFT run that saved every 10 steps up to step 30.
func cleanupRun(status string) *dbclient.PostTrainingRun {
	return &dbclient.PostTrainingRun{RunID: "pt-1", Status: status, TrainType: "sft",
		Recipe: `{"output":{"base":"/w/pt-1"},"choices":{"saveInterval":10}}`}
}

// cleanupHandler returns a handler whose database holds a loss and a validation loss for the run.
func cleanupHandler(t *testing.T) *Handler {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	now := time.Now()
	var rows []*dbclient.PostTrainingRunMetric
	for step := 1; step <= 30; step++ {
		rows = append(rows, &dbclient.PostTrainingRunMetric{Name: "train/loss", Step: step, Value: 1, WallTime: now})
	}
	for step, v := range map[int]float64{10: 0.5, 20: 0.3, 30: 0.4} {
		rows = append(rows, &dbclient.PostTrainingRunMetric{Name: valLossMetric, Step: step, Value: v, WallTime: now})
	}
	db.EXPECT().ListPostTrainingRunMetrics(gomock.Any(), "pt-1", gomock.Any()).Return(rows, nil).AnyTimes()
	return NewHandler(nil, db)
}

func TestPlanCleanup(t *testing.T) {
	h := exportHandler(t)
	ctx := context.Background()

	plan, err := h.planCleanup(ctx, cleanupRun("Succeeded"), "best")
	require.NoError(t, err)
	var got checkpointCleanup
	require.NoError(t, json.Unmarshal([]byte(plan), &got))
	require.Equal(t, []string{"/w/pt-1/checkpoints/iter_0000010", "/w/pt-1/checkpoints/iter_0000030"}, got.Paths)
	require.Equal(t, 20, *got.KeptStep, "the lowest validation loss stays")

	plan, err = h.planCleanup(ctx, cleanupRun("Stopped"), "none")
	require.NoError(t, err)
	require.JSONEq(t, `{"keep":"none","paths":["/w/pt-1/checkpoints","/w/pt-1/critic_checkpoints"]}`, plan)

	_, err = h.planCleanup(ctx, cleanupRun("Running"), "none")
	require.Equal(t, http.StatusConflict, statusCode(err))
	pending := cleanupRun("Succeeded")
	pending.CleanupRequest = plan
	_, err = h.planCleanup(ctx, pending, "none")
	require.Equal(t, http.StatusConflict, statusCode(err))
	_, err = h.planCleanup(ctx, cleanupRun("Succeeded"), "latest")
	require.Equal(t, http.StatusBadRequest, statusCode(err))
	rl := cleanupRun("Succeeded")
	rl.TrainType = "rl"
	_, err = h.planCleanup(ctx, rl, "best")
	require.Equal(t, http.StatusBadRequest, statusCode(err), "RL has no validation loss to pick the best by")

	h = exportHandler(t, exportJob("pt-1-x20-b", "Running", 2))
	_, err = h.planCleanup(ctx, cleanupRun("Succeeded"), "none")
	require.Equal(t, http.StatusConflict, statusCode(err), "a running export still reads its checkpoint")
}

func TestMarkersShowCleanedCheckpoints(t *testing.T) {
	h := cleanupHandler(t)
	rows, err := h.db.ListPostTrainingRunMetrics(context.Background(), "pt-1", nil)
	require.NoError(t, err)
	points := map[string][]*dbclient.PostTrainingRunMetric{}
	for _, row := range rows {
		points[row.Name] = append(points[row.Name], row)
	}
	run := cleanupRun("Succeeded")
	run.StageHistory = `[{"stage":"done","event":"checkpoints-cleaned","keptStep":20}]`
	m := buildMarkers(run, points)
	deleted := map[int]bool{}
	for _, cp := range m.Checkpoints {
		deleted[cp.Step] = cp.Deleted
	}
	require.Equal(t, map[int]bool{10: true, 20: false, 30: true}, deleted)
}
