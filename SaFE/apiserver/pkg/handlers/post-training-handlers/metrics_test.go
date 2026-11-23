/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
)

// rows builds the points of one metric from (step, value) pairs.
func rows(name string, pairs ...float64) []*dbclient.PostTrainingRunMetric {
	var out []*dbclient.PostTrainingRunMetric
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, &dbclient.PostTrainingRunMetric{Name: name, Step: int(pairs[i]), Value: pairs[i+1]})
	}
	return out
}

// A name pattern matches whole path segments, with * inside one segment.
func TestMatchName(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"train/loss", "train/loss", true},
		{"eval/*", "eval/aime", true},
		{"eval/*", "eval/val/loss", false},
		{"eval/*/loss", "eval/val/loss", true},
		{"train/lr-pg_*", "train/lr-pg_0", true},
		{"eval/*-pass@*", "eval/aime-pass@4", true},
		{"eval/*-pass@*", "eval/aime", false},
		{"rollout/*/*", "rollout/response_len/max", true},
		{"", "train/loss", false},
	}
	for _, c := range cases {
		require.Equal(t, c.want, MatchName(c.pattern, c.name), "%s ~ %s", c.pattern, c.name)
	}
}

// Each metric gets the axis it is drawn on.
func TestAxisOf(t *testing.T) {
	require.Equal(t, AxisStep, axisOf("train/pg_loss", "rl"))
	require.Equal(t, AxisRollout, axisOf("rollout/raw_reward", "rl"))
	require.Equal(t, AxisStep, axisOf("eval/val/loss", "sft"))
}

// A series is downsampled with its spikes kept and its non-finite points listed.
func TestBuildSeries(t *testing.T) {
	s := buildSeries("train/loss", AxisStep, rows("train/loss", 1, 2.0, 2, math.NaN(), 3, math.Inf(1), 4, 1.5), 100)
	require.Equal(t, 4, s.Count)
	require.Equal(t, [][2]float64{{1, 2.0}, {4, 1.5}}, s.Points)
	require.Equal(t, [][2]interface{}{{2, "NaN"}, {3, "Inf"}}, s.NonFinite)
	require.Equal(t, [2]float64{4, 1.5}, *s.Last)
	require.False(t, s.Downsampled)

	var pairs []float64
	for i := 0; i < 1000; i++ {
		v := 1.0
		if i == 500 {
			v = 9.0
		}
		pairs = append(pairs, float64(i), v)
	}
	d := buildSeries("train/loss", AxisStep, rows("train/loss", pairs...), 100)
	require.True(t, d.Downsampled)
	require.LessOrEqual(t, len(d.Points), 100)
	peak := false
	for _, p := range d.Points {
		if p[1] == 9.0 {
			peak = true
		}
	}
	require.True(t, peak, "the spike survives downsampling")
	require.Equal(t, 1000, d.Count)
}

// The markers show where a run resumed and where it failed in training.
func TestBuildMarkers(t *testing.T) {
	run := &dbclient.PostTrainingRun{TrainType: "sft", Status: string(v1.WorkloadFailed), FailedStage: "train",
		Message:      "no progress for 60 minutes",
		StageHistory: `[{"stage":"queued"},{"stage":"train","resumeStep":10,"time":"t1"}]`}
	points := map[string][]*dbclient.PostTrainingRunMetric{
		"train/loss": rows("train/loss", 1, 3, 10, 2, 20, 1.5, 30, 1.2, 35, 1.1),
	}
	m := buildMarkers(run, points)
	require.Equal(t, []resumeMark{{Step: 10, Time: "t1"}}, m.Resumes)
	require.Equal(t, &failureMark{Step: 35, Reason: "no progress for 60 minutes"}, m.Failure)

	run.Status = string(v1.WorkloadRunning)
	require.Nil(t, buildMarkers(run, points).Failure)
}

// Stopping a workload records who stopped it and deletes it, and a missing one is fine.
func TestStopWorkload(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "pt-x"}, Status: v1.WorkloadStatus{Phase: v1.WorkloadRunning}}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(w).WithStatusSubresource(w).Build()
	h := NewHandler(cli, nil)
	require.NoError(t, h.stopWorkload(context.Background(), "pt-x", "stopped by alice"))
	err := cli.Get(context.Background(), ctrlclient.ObjectKey{Name: "pt-x"}, &v1.Workload{})
	require.Error(t, err, "the workload is deleted")
	require.NoError(t, h.stopWorkload(context.Background(), "missing", "x"), "a missing workload is fine")
}

// runChildren returns the training workload of run pt-x in phase.
func runChildren(phase v1.WorkloadPhase) []ctrlclient.Object {
	train := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "pt-x", Labels: map[string]string{
		v1.PostTrainingRunIdLabel: "pt-x", v1.PostTrainingRoleLabel: v1.PostTrainingRoleTrain}},
		Status: v1.WorkloadStatus{Phase: phase}}
	return []ctrlclient.Object{train}
}

// remaining lists the names of the workloads left in cli.
func remaining(t *testing.T, cli ctrlclient.Client) []string {
	names, workloads := []string{}, &v1.WorkloadList{}
	require.NoError(t, cli.List(context.Background(), workloads))
	for _, w := range workloads.Items {
		names = append(names, w.Name)
	}
	sort.Strings(names)
	return names
}

// Stopping a run stops its training workload only.
func TestStopRunStopsTheTrainingWorkload(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	objs := runChildren(v1.WorkloadRunning)
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithStatusSubresource(objs...).Build()
	h := NewHandler(cli, nil)
	run := &dbclient.PostTrainingRun{RunID: "pt-x", WorkloadID: "pt-x", Status: string(v1.WorkloadRunning), Stage: "train"}
	stopped, err := h.stopRun(context.Background(), run, "stopped by alice")
	require.NoError(t, err)
	require.True(t, stopped)
	require.Empty(t, remaining(t, cli))
}

// Deleting a run stops its training workload, which the controller no longer follows.
func TestDeleteRunEndsWhatTheControllerNoLongerFollows(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	objs := runChildren(v1.WorkloadRunning)
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithStatusSubresource(objs...).Build()
	db := mockclient.NewMockInterface(gomock.NewController(t))
	db.EXPECT().SetPostTrainingRunDeleted(gomock.Any(), "pt-x").Return(nil)
	db.EXPECT().DeletePostTrainingRunMetrics(gomock.Any(), "pt-x").Return(nil)
	h := NewHandler(cli, db)
	run := &dbclient.PostTrainingRun{RunID: "pt-x", WorkloadID: "pt-x", Status: string(v1.WorkloadRunning)}
	require.NoError(t, h.deleteRun(testContext(), run, false))
	require.Empty(t, remaining(t, cli), "a soft-deleted run's workload is not reconciled any more")
}

// Deleting a run with its model is refused while another workload runs on that model, as the model API refuses.
func TestDeleteRunKeepsAModelThatIsInUse(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	objs := append(runChildren(v1.WorkloadSucceeded), &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "infer-1"},
		Spec:   v1.WorkloadSpec{Env: map[string]string{sourceModelEnv: "m-out"}},
		Status: v1.WorkloadStatus{Phase: v1.WorkloadRunning}})
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithStatusSubresource(objs...).Build()
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h := NewHandler(cli, db)
	run := &dbclient.PostTrainingRun{RunID: "pt-x", WorkloadID: "pt-x", Status: string(v1.WorkloadSucceeded),
		OutputModelID: "m-out"}
	before := remaining(t, cli)

	based := db.EXPECT().ListPostTrainingRuns(gomock.Any(), dbclient.PostTrainingRunFilter{BaseModelID: "m-out",
		Statuses: []string{"Pending", "Running"}}).Return(nil, int64(0), nil)
	err := h.deleteRun(testContext(), run, true)
	require.Equal(t, http.StatusBadRequest, statusCode(err), "infer-1 still runs on the model")
	require.Contains(t, err.Error(), "infer-1")
	require.Equal(t, before, remaining(t, cli), "a refused delete stops nothing")

	require.NoError(t, cli.Delete(context.Background(), &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "infer-1"}}))
	tuned := db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return([]*dbclient.PostTrainingRun{
		{RunID: "pt-x"}, {RunID: "pt-next"}}, int64(2), nil).After(based)
	err = h.deleteRun(testContext(), run, true)
	require.Equal(t, http.StatusBadRequest, statusCode(err), "pt-next trains on the model as its base")
	require.Contains(t, err.Error(), "exist: [pt-next].", "the run does not hold its own model")
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil).After(tuned)
	db.EXPECT().SetPostTrainingRunDeleted(gomock.Any(), "pt-x").Return(nil)
	db.EXPECT().DeletePostTrainingRunMetrics(gomock.Any(), "pt-x").Return(nil)
	require.NoError(t, h.deleteRun(testContext(), run, true))
	require.NotContains(t, remaining(t, cli), "pt-x-job", "the run's unfinished job ends with the run")
}

// A run without a framework is a slime run.
func TestFrameworkOf(t *testing.T) {
	reg, err := registry.Parse([]byte(testRegistry))
	require.NoError(t, err)
	require.Equal(t, "slime", frameworkOf(reg, "").Name, "runs without a framework are slime runs")
	require.Nil(t, frameworkOf(reg, "verl"))
	require.Nil(t, frameworkOf(nil, "slime"))
}

// A curve request reads at most the newest points of the run.
func TestACurveRequestReadsAtMostTheNewestPoints(t *testing.T) {
	db := mockclient.NewMockInterface(gomock.NewController(t))
	h := memberHandler(t, db)
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil).AnyTimes()
	db.EXPECT().GetPostTrainingRun(gomock.Any(), "pt-1").Return(&dbclient.PostTrainingRun{RunID: "pt-1",
		Workspace: "ws1", UserID: "u1", TrainType: "sft"}, nil)
	db.EXPECT().ListLatestPostTrainingRunMetrics(gomock.Any(), "pt-1", gomock.Nil(), maxMetricRows).
		Return([]*dbclient.PostTrainingRunMetric{{RunID: "pt-1", Name: "train/loss", Step: 1, Value: 0.5}}, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "pt-1"}}
	c.Set(common.UserId, "u1")
	c.Set(common.UserName, "alice")
	h.GetMetrics(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"train/loss"`)
}
