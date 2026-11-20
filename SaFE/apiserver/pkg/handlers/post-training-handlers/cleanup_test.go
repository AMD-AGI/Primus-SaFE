/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
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

// A cleanup plan removes the older checkpoints of actor and critic and keeps the best on request.
func TestPlanCleanup(t *testing.T) {
	h := exportHandler(t)
	ctx := context.Background()

	plan, err := h.planCleanup(ctx, cleanupRun("Succeeded"), "best")
	require.NoError(t, err)
	var got checkpointCleanup
	require.NoError(t, json.Unmarshal([]byte(plan), &got))
	require.Equal(t, []string{"/w/pt-1/checkpoints/iter_0000009", "/w/pt-1/checkpoints/iter_0000029"}, got.Paths,
		"slime saves the checkpoint after step N as iter_{N-1}")
	require.Equal(t, 20, *got.KeptStep, "the lowest validation loss stays")
	require.Equal(t, []int{10, 30}, *got.CleanedSteps, "the plan names the steps it removes")

	single := cleanupRun("Succeeded")
	single.Recipe = `{"output":{"base":"/w/pt-1"},"choices":{"saveInterval":30}}`
	plan, err = h.planCleanup(ctx, single, "best")
	require.NoError(t, err)
	require.JSONEq(t, `{"keep":"best","paths":null,"keptStep":30,"cleanedSteps":[]}`, plan,
		"a plan with nothing to remove has its list, empty")

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

// Keeping the best checkpoint is refused once retention removed it, as the plan would delete the ones that are left.
func TestKeepBestIsRefusedWhenTheBestCheckpointIsGone(t *testing.T) {
	h := exportHandler(t)
	ctx := context.Background()
	run := cleanupRun("Succeeded")
	run.Recipe = `{"output":{"base":"/w/pt-1"},"choices":{"saveInterval":10},"checkpoint":{"keepLast":1}}`
	_, err := h.planCleanup(ctx, run, "best")
	require.Equal(t, http.StatusBadRequest, statusCode(err), "keepLast removed step 20, which has the lowest loss")

	run.Recipe = `{"output":{"base":"/w/pt-1"},"choices":{"saveInterval":10},"checkpoint":{"keepLast":1,"keepBest":true}}`
	plan, err := h.planCleanup(ctx, run, "best")
	require.NoError(t, err)
	var got checkpointCleanup
	require.NoError(t, json.Unmarshal([]byte(plan), &got))
	require.Equal(t, []string{"/w/pt-1/checkpoints/iter_0000029"}, got.Paths, "retention kept the best one")
	require.Equal(t, 20, *got.KeptStep)
}

// A cleanup request is refused while another one is queued.
func TestOnlyOneCleanupRequestIsTaken(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h := NewHandler(nil, db)
	db.EXPECT().RequestPostTrainingRunCleanup(gomock.Any(), "pt-1", `{"keep":"none"}`).Return(true, nil)
	require.NoError(t, h.requestCleanup(context.Background(), "pt-1", `{"keep":"none"}`))
	db.EXPECT().RequestPostTrainingRunCleanup(gomock.Any(), "pt-1", `{"keep":"best"}`).Return(false, nil)
	err := h.requestCleanup(context.Background(), "pt-1", `{"keep":"best"}`)
	require.Equal(t, http.StatusConflict, statusCode(err), "a request made meanwhile keeps the cleanup")
}

// The checkpoint markers show which checkpoints a cleanup removed and which it kept.
func TestMarkersShowCleanedCheckpoints(t *testing.T) {
	h := cleanupHandler(t)
	rows, err := h.db.ListPostTrainingRunMetrics(context.Background(), "pt-1", nil)
	require.NoError(t, err)
	points := map[string][]*dbclient.PostTrainingRunMetric{}
	for _, row := range rows {
		points[row.Name] = append(points[row.Name], row)
	}
	deleted := func(stage string, events ...string) map[int]bool {
		run := cleanupRun("Succeeded")
		for i, e := range events {
			events[i] = `{"stage":"done","event":"checkpoints-cleaned"` + e + `}`
		}
		run.Stage, run.StageHistory = stage, "["+strings.Join(events, ",")+"]"
		out := map[int]bool{}
		for _, cp := range buildMarkers(run, points).Checkpoints {
			out[cp.Step] = cp.Deleted
		}
		return out
	}
	require.Equal(t, map[int]bool{10: true, 20: false, 30: true}, deleted("", `,"keptStep":20`))
	// A run that is past training also lists its last save, after step 31.
	require.Equal(t, map[int]bool{10: true, 20: false, 30: true, 31: true},
		deleted("done", `,"keptStep":20,"cleanedSteps":[10,30,31]`))
	require.Equal(t, map[int]bool{10: true, 20: false, 30: false, 31: false},
		deleted("done", `,"keptStep":20,"cleanedSteps":[10]`), "only the checkpoints the event names are gone")
	require.Equal(t, map[int]bool{10: true, 20: false, 30: true, 31: false},
		deleted("done", `,"keptStep":20,"cleanedSteps":[10]`, `,"keptStep":20,"cleanedSteps":[30]`),
		"a later cleanup leaves what an earlier one removed as it is")
	require.Equal(t, map[int]bool{10: true, 20: false, 30: true, 31: false}, deleted("done", `,"keptStep":20`),
		"an event without the steps is of a cleanup made before the last save was listed, which it left")
	require.Equal(t, map[int]bool{10: false, 20: false, 30: false, 31: false},
		deleted("done", `,"keptStep":20,"cleanedSteps":[]`), "a cleanup that found none to remove marks none")
	require.Equal(t, map[int]bool{10: true, 20: true, 30: true, 31: true}, deleted("done", ``),
		"a cleanup that kept none removed the directory")
}

// An event without the steps leaves only a last save that the interval does not list: one on the interval was
// listed, and removed, before.
func TestAnEventWithoutStepsMarksALastSaveOnTheInterval(t *testing.T) {
	h := cleanupHandler(t)
	run := cleanupRun("Succeeded")
	run.Stage, run.ResultLine = "done", `[SUCCESS] {"run":"pt-1","stage":"export","output":"/w/pt-1/f","iteration":29}`
	run.StageHistory = `[{"stage":"done","event":"checkpoints-cleaned","keptStep":20}]`
	m, err := h.checkpointMarkers(context.Background(), run)
	require.NoError(t, err)
	require.Equal(t, []checkpointMark{{Step: 10, Deleted: true}, {Step: 20}, {Step: 30, Deleted: true}},
		stepsOnly(m.Checkpoints))
}

// stepsOnly returns the checkpoints without their validation losses.
func stepsOnly(cps []checkpointMark) []checkpointMark {
	out := make([]checkpointMark, 0, len(cps))
	for _, cp := range cps {
		out = append(out, checkpointMark{Step: cp.Step, Deleted: cp.Deleted})
	}
	return out
}

// A cleanup that an export got ahead of, between its plan and its request, is taken back.
func TestACleanupGivesWayToAnExportThatStartedMeanwhile(t *testing.T) {
	h := exportHandler(t)
	require.NoError(t, h.withdrawCleanupIfExporting(context.Background(), "pt-1"), "no export runs")

	h = exportHandler(t, exportJob("pt-1-x20-b", "Running", 2))
	h.db.(*mockclient.MockInterface).EXPECT().UpdatePostTrainingRun(gomock.Any(), "pt-1",
		map[string]interface{}{"cleanup_request": ""}).Return(nil)
	err := h.withdrawCleanupIfExporting(context.Background(), "pt-1")
	require.Equal(t, http.StatusConflict, statusCode(err), "the export reads a checkpoint the cleanup would delete")

	// The cache may not show a job made a moment ago: the exports are read from the API server.
	h = exportHandler(t)
	h.reader = exportHandler(t, exportJob("pt-1-x20-b", "Running", 2)).client
	h.db.(*mockclient.MockInterface).EXPECT().UpdatePostTrainingRun(gomock.Any(), "pt-1",
		map[string]interface{}{"cleanup_request": ""}).Return(errors.New("connection reset"))
	err = h.withdrawCleanupIfExporting(context.Background(), "pt-1")
	require.Equal(t, http.StatusInternalServerError, statusCode(err),
		"a request that could not be withdrawn is not answered as a mere conflict")
}

// cleanupEndpoint returns a handler that user u1 may clean up run pt-1 with, and its database: the API server
// shows a running export that the cache does not show yet.
func cleanupEndpoint(t *testing.T) (*Handler, *mockclient.MockInterface) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h := templateHandler(t, db)
	h.reader = exportHandler(t, exportJob("pt-1-x20-b", "Running", 2)).client
	run := cleanupRun("Succeeded")
	run.Workspace, run.UserID, run.WorkloadID = "ws1", "u1", "pt-1"
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil).AnyTimes()
	db.EXPECT().GetPostTrainingRun(gomock.Any(), "pt-1").Return(run, nil)
	return h, db
}

// The cleanup endpoint takes its request back when the API server shows an export that the cache does not show yet.
func TestTheCleanupEndpointWithdrawsForAnExportTheCacheMisses(t *testing.T) {
	h, db := cleanupEndpoint(t)
	asked := db.EXPECT().RequestPostTrainingRunCleanup(gomock.Any(), "pt-1", gomock.Any()).Return(true, nil)
	db.EXPECT().UpdatePostTrainingRun(gomock.Any(), "pt-1", map[string]interface{}{"cleanup_request": ""}).
		Return(nil).After(asked)

	code, body := runCall(context.Background(), h.CleanupCheckpoints, `{"keep":"none"}`)
	require.Equal(t, http.StatusConflict, code, body)
}

// The cleanup endpoint looks for an export although its caller left while the request was recorded: the database
// stored the request, and a call that ends with its caller would answer the cancellation and skip the look.
func TestTheCleanupEndpointChecksItsRequestAlthoughItsCallerLeft(t *testing.T) {
	h, db := cleanupEndpoint(t)
	ctx, leave := context.WithCancel(context.Background())
	asked := db.EXPECT().RequestPostTrainingRunCleanup(gomock.Any(), "pt-1", gomock.Any()).DoAndReturn(
		func(ctx context.Context, _, _ string) (bool, error) {
			leave()
			return true, ctx.Err()
		})
	db.EXPECT().UpdatePostTrainingRun(gomock.Any(), "pt-1", map[string]interface{}{"cleanup_request": ""}).
		Return(nil).After(asked)

	code, body := runCall(ctx, h.CleanupCheckpoints, `{"keep":"none"}`)
	require.Equal(t, http.StatusConflict, code, body)
}

// The look for an export reads the export jobs alone, and leaves the withdrawal time of its own.
func TestTheLookForAnExportIsApartFromItsWithdrawal(t *testing.T) {
	// Models that cannot be listed do not matter: only an export job reads a checkpoint.
	h := exportHandler(t)
	h.reader = intercepted(h.client, interceptor.Funcs{List: func(ctx context.Context, c ctrlclient.WithWatch,
		list ctrlclient.ObjectList, opts ...ctrlclient.ListOption) error {
		if _, ok := list.(*v1.ModelList); ok {
			return errors.New("the models are unavailable")
		}
		return c.List(ctx, list, opts...)
	}})
	require.NoError(t, h.withdrawCleanupIfExporting(context.Background(), "pt-1"))

	// Export jobs that cannot be listed may run: the cleanup is withdrawn.
	var lookBy, undoBy time.Time
	h.reader = intercepted(h.client, interceptor.Funcs{List: func(ctx context.Context, _ ctrlclient.WithWatch,
		_ ctrlclient.ObjectList, _ ...ctrlclient.ListOption) error {
		lookBy, _ = ctx.Deadline()
		return context.DeadlineExceeded
	}})
	h.db.(*mockclient.MockInterface).EXPECT().UpdatePostTrainingRun(gomock.Any(), "pt-1",
		map[string]interface{}{"cleanup_request": ""}).DoAndReturn(
		func(ctx context.Context, _ string, _ map[string]interface{}) error {
			undoBy, _ = ctx.Deadline()
			return nil
		})
	err := h.withdrawCleanupIfExporting(context.Background(), "pt-1")
	require.Equal(t, http.StatusInternalServerError, statusCode(err), "the cleanup cannot be told safe")
	require.False(t, lookBy.IsZero())
	require.True(t, undoBy.After(lookBy.Add(time.Second)), "a look that used up its time leaves the withdrawal its own")
}

// An export job that is being deleted does not hold a cleanup, as for the controller: its export was refused or
// dropped. One that runs without the label of its step is named by itself.
func TestAnExportJobBeingDeletedDoesNotHoldACleanup(t *testing.T) {
	gone := exportJob("pt-1-x20-b", "Running", 2)
	gone.Finalizers = []string{"primus-safe/workload"}
	gone.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	h := exportHandler(t, gone)
	require.NoError(t, h.withdrawCleanupIfExporting(context.Background(), "pt-1"))

	unlabelled := exportJob("pt-1-x20-c", "Running", 3)
	delete(unlabelled.Labels, v1.PostTrainingExportStepLabel)
	h = exportHandler(t, unlabelled)
	err := h.exportRunning(context.Background(), h.client, "pt-1")
	require.Equal(t, http.StatusConflict, statusCode(err))
	require.Contains(t, err.Error(), "export job pt-1-x20-c of run pt-1")
}
