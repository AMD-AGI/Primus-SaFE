/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"context"
	"fmt"
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
	require.Equal(t, AxisTime, axisOf("system/gpu_util_mean", "rl"))
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

// The checkpoint markers keep the most recent and the best checkpoints.
func TestBuildMarkers(t *testing.T) {
	run := &dbclient.PostTrainingRun{TrainType: "sft", Status: string(v1.WorkloadFailed), FailedStage: "train",
		Message:      "no progress for 60 minutes",
		Recipe:       `{"choices":{"saveInterval":10},"checkpoint":{"keepLast":1,"keepBest":true}}`,
		StageHistory: `[{"stage":"queued"},{"stage":"train","resumeStep":10,"time":"t1"}]`}
	points := map[string][]*dbclient.PostTrainingRunMetric{
		"train/loss":    rows("train/loss", 1, 3, 10, 2, 20, 1.5, 30, 1.2, 35, 1.1),
		"eval/val/loss": rows("eval/val/loss", 10, 2.1, 20, 1.9, 30, 2.0),
	}
	m := buildMarkers(run, points)
	require.Len(t, m.Checkpoints, 3)
	require.Equal(t, 20, m.BestCheckpoint.Step)
	require.True(t, m.Checkpoints[0].Deleted, "older than keepLast and not the best")
	require.False(t, m.Checkpoints[1].Deleted, "the best is kept")
	require.False(t, m.Checkpoints[2].Deleted, "the most recent is kept")
	require.Equal(t, []resumeMark{{Step: 10, Time: "t1"}}, m.Resumes)
	require.Equal(t, &failureMark{Step: 35, Reason: "no progress for 60 minutes"}, m.Failure)

	run.Status, run.Recipe = string(v1.WorkloadRunning), `{}`
	m = buildMarkers(run, points)
	require.Empty(t, m.Checkpoints)
	require.Nil(t, m.BestCheckpoint)
	require.Nil(t, m.Failure)

	run.Recipe = `{"choices":{"saveInterval":10}}`
	m = buildMarkers(run, map[string][]*dbclient.PostTrainingRunMetric{"train/loss": rows("train/loss", 0, 3, 19, 1)})
	require.Equal(t, []checkpointMark{{Step: 10}, {Step: 20}}, m.Checkpoints,
		"steps count from 0, so a last step of 19 means 20 steps done and the checkpoint saved after them")

	// slime saves once more when training ends, whatever the interval: a run that is past training has that
	// checkpoint, and the validation loss of the last step belongs to it.
	short := map[string][]*dbclient.PostTrainingRunMetric{
		"train/loss": rows("train/loss", 0, 3, 6, 1), "eval/val/loss": rows("eval/val/loss", 0, 2, 7, 0.7)}
	run.Recipe = `{"choices":{"saveInterval":200}}`
	require.Empty(t, buildMarkers(run, short).Checkpoints, "the run is still training")
	for _, stage := range []string{"export", "register", "eval", "done"} {
		run.Stage = stage
		m = buildMarkers(run, short)
		loss := 0.7
		require.Equal(t, []checkpointMark{{Step: 7, ValLoss: &loss}}, m.Checkpoints, stage)
		require.Equal(t, 7, m.BestCheckpoint.Step, stage)
	}
	run.Recipe = `{"choices":{"saveInterval":5},"checkpoint":{"keepLast":1}}`
	long := map[string][]*dbclient.PostTrainingRunMetric{"train/loss": rows("train/loss", 0, 3, 11, 1)}
	require.Equal(t, []checkpointMark{{Step: 5, Deleted: true}, {Step: 10, Deleted: true}, {Step: 12}},
		buildMarkers(run, long).Checkpoints, "the last save counts as the most recent one")
	run.Recipe = `{"choices":{"saveInterval":6}}`
	require.Equal(t, []checkpointMark{{Step: 6}, {Step: 12}}, buildMarkers(run, long).Checkpoints,
		"a last step on the interval is one checkpoint")

	require.Empty(t, buildMarkers(run, nil).Checkpoints, "a run without points has no step to list a last save at")

	// Stored points that stop short of the last step, with a gap left or at the cap on stored points, do not tell
	// where the training ended: a checkpoint after them would be one that is not there.
	run.Recipe = `{"choices":{"saveInterval":500}}`
	cut := map[string][]*dbclient.PostTrainingRunMetric{"train/loss": rows("train/loss", 0, 3, 990, 1)}
	require.Equal(t, []checkpointMark{{Step: 500}, {Step: 991}}, buildMarkers(run, cut).Checkpoints)
	run.MetricGap = true
	require.Equal(t, []checkpointMark{{Step: 500}}, buildMarkers(run, cut).Checkpoints, "a gap is left")
	run.MetricGap, run.MetricSummary = false, `{"points":500000,"capped":true}`
	require.Equal(t, []checkpointMark{{Step: 500}}, buildMarkers(run, cut).Checkpoints, "the cap was reached")
}

// The save made when training ends is listed where the launcher reported it with its result, whatever the stored
// points say. Without that report a run past training whose points stop short lists no such save, and still counts
// it among the ones its retention kept.
func TestTheLastSaveIsTheOneTheLauncherReported(t *testing.T) {
	cut := map[string][]*dbclient.PostTrainingRunMetric{"train/loss": rows("train/loss", 0, 3, 990, 1)}
	result := func(kind, iteration string) string {
		return kind + ` {"run":"pt-1","attempt":"a1","pv":1,"stage":"export","output":"/w/pt-1/final_model",` +
			`"iteration":` + iteration + `}`
	}
	listed := func(keepLast int, line string, change func(*dbclient.PostTrainingRun)) []checkpointMark {
		run := &dbclient.PostTrainingRun{TrainType: "sft", Status: "Succeeded", Stage: "done", ResultLine: line,
			Recipe: fmt.Sprintf(`{"choices":{"saveInterval":500},"checkpoint":{"keepLast":%d}}`, keepLast)}
		change(run)
		return buildMarkers(run, cut).Checkpoints
	}
	states := map[string]func(*dbclient.PostTrainingRun){
		"complete": func(*dbclient.PostTrainingRun) {},
		"a gap":    func(run *dbclient.PostTrainingRun) { run.MetricGap = true },
		"the cap":  func(run *dbclient.PostTrainingRun) { run.MetricSummary = `{"points":500000,"capped":true}` },
	}
	for name, state := range states {
		require.Equal(t, []checkpointMark{{Step: 500, Deleted: true}, {Step: 1000, Deleted: true},
			{Step: 1500, Deleted: true}, {Step: 2000}}, listed(1, result("[SUCCESS]", "1999"), state),
			"%s: the saves of the interval are listed up to the reported one", name)
		require.Equal(t, []checkpointMark{{Step: 500, Deleted: true}, {Step: 990}},
			listed(1, result("[SUCCESS]", "989"), state), "%s: a reported save off the interval", name)
	}
	gap := states["a gap"]
	for name, line := range map[string]string{"no result": "", "a failure": result("[ERROR]", "1999"),
		"no checkpoint reported": result("[SUCCESS]", "null")} {
		require.Equal(t, []checkpointMark{{Step: 500, Deleted: true}}, listed(1, line, gap),
			"%s: the save at the end, whose step is not known, is the one the retention kept", name)
		require.Equal(t, []checkpointMark{{Step: 500}}, listed(2, line, gap), name)
		require.Equal(t, []checkpointMark{{Step: 500, Deleted: true}, {Step: 991}}, listed(1, line, states["complete"]),
			"%s: complete points tell where the training ended", name)
	}

	// Points that end at a save of the interval end at the last save, gap or not: it takes no second place among
	// those the retention kept.
	whole := map[string][]*dbclient.PostTrainingRunMetric{"train/loss": rows("train/loss", 0, 3, 999, 1)}
	for keepLast, want := range map[int][]checkpointMark{
		1: {{Step: 500, Deleted: true}, {Step: 1000}}, 2: {{Step: 500}, {Step: 1000}}} {
		run := &dbclient.PostTrainingRun{TrainType: "sft", Status: "Failed", Stage: "export", MetricGap: true,
			Recipe: fmt.Sprintf(`{"choices":{"saveInterval":500},"checkpoint":{"keepLast":%d}}`, keepLast)}
		require.Equal(t, want, buildMarkers(run, whole).Checkpoints, "keepLast %d", keepLast)
	}
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

// runChildren returns the training workload of run pt-x in phase and, all running, an evaluation service, a kept
// service whose evaluation is over, a checkpoint export, a backfill and an evaluation OpsJob, plus a finished OpsJob.
func runChildren(phase v1.WorkloadPhase) []ctrlclient.Object {
	child := func(name, role string) *v1.Workload {
		return &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: name,
			Labels: map[string]string{v1.PostTrainingRunIdLabel: "pt-x", v1.PostTrainingRoleLabel: role}},
			Status: v1.WorkloadStatus{Phase: v1.WorkloadRunning}}
	}
	train := child("pt-x", v1.PostTrainingRoleTrain)
	train.Status.Phase = phase
	kept := child("pt-x-eval-kept", v1.PostTrainingRoleEval)
	kept.Annotations = map[string]string{v1.PostTrainingEvalDoneAnnotation: v1.TrueStr}
	job := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "pt-x-eval-job",
		Labels: map[string]string{v1.PostTrainingRunIdLabel: "pt-x"}},
		Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{{Name: v1.ParameterEvalTaskId, Value: "task-1"}}}}
	job.Status.Phase = v1.OpsJobRunning
	done := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "pt-x-verify",
		Labels: map[string]string{v1.PostTrainingRunIdLabel: "pt-x"}}}
	done.Status.Phase = v1.OpsJobSucceeded
	return []ctrlclient.Object{train, child("pt-x-eval", v1.PostTrainingRoleEval), kept,
		child("pt-x-x20", v1.PostTrainingRoleExport), child("pt-x-backfill", v1.PostTrainingRoleBackfill), job, done}
}

// remaining lists the names of the workloads and OpsJobs left in cli.
func remaining(t *testing.T, cli ctrlclient.Client) []string {
	ctx, names := context.Background(), []string{}
	workloads, jobs := &v1.WorkloadList{}, &v1.OpsJobList{}
	require.NoError(t, cli.List(ctx, workloads))
	require.NoError(t, cli.List(ctx, jobs))
	for _, w := range workloads.Items {
		names = append(names, w.Name)
	}
	for _, j := range jobs.Items {
		names = append(names, j.Name)
	}
	sort.Strings(names)
	return names
}

// Stopping a run also ends its evaluation and exports.
func TestStopRunEndsItsEvaluationAndExports(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	objs := runChildren(v1.WorkloadSucceeded)
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithStatusSubresource(objs...).Build()
	h := NewHandler(cli, nil)
	run := &dbclient.PostTrainingRun{RunID: "pt-x", WorkloadID: "pt-x", Status: string(v1.WorkloadSucceeded), Stage: "eval"}
	stopped, err := h.stopRun(context.Background(), run, "stopped by alice")
	require.NoError(t, err)
	require.True(t, stopped, "a run at its evaluation still has something to stop")
	require.Equal(t, []string{"pt-x", "pt-x-backfill", "pt-x-eval-job", "pt-x-eval-kept", "pt-x-verify"},
		remaining(t, cli), "the evaluation service and the export stop; the controller then cancels the job, a kept "+
			"service goes on serving and the backfill still fills the stopped run's metrics")
}

// Deleting a run ends its children, which the controller no longer follows.
func TestDeleteRunEndsWhatTheControllerNoLongerFollows(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	for _, deleteModel := range []bool{false, true} {
		objs := runChildren(v1.WorkloadRunning)
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithStatusSubresource(objs...).Build()
		db := mockclient.NewMockInterface(gomock.NewController(t))
		db.EXPECT().UpdateEvaluationTaskStatus(gomock.Any(), "task-1", dbclient.EvaluationTaskStatusCancelled).Return(nil)
		db.EXPECT().SetPostTrainingRunDeleted(gomock.Any(), "pt-x").Return(nil)
		db.EXPECT().DeletePostTrainingRunMetrics(gomock.Any(), "pt-x").Return(nil)
		db.EXPECT().DeletePostTrainingRunSamples(gomock.Any(), "pt-x", gomock.Any()).Return(nil)
		h := NewHandler(cli, db)
		run := &dbclient.PostTrainingRun{RunID: "pt-x", WorkloadID: "pt-x", Status: string(v1.WorkloadRunning)}
		require.NoError(t, h.deleteRun(testContext(), run, deleteModel))
		want := []string{"pt-x-eval-kept", "pt-x-verify"}
		if deleteModel {
			want = []string{"pt-x-verify"}
		}
		require.Equal(t, want, remaining(t, cli), "deleteModel=%v: a soft-deleted run's children are not "+
			"reconciled any more, and a kept service only outlives the run while its model does", deleteModel)
	}
}

// Deleting a run with its model is refused while another workload runs on that model, as the model API refuses.
func TestDeleteRunKeepsAModelThatIsInUse(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	objs := runChildren(v1.WorkloadSucceeded)
	for _, o := range objs {
		if w, ok := o.(*v1.Workload); ok && w.Name == "pt-x-eval-kept" {
			w.Spec.Env = map[string]string{sourceModelEnv: "m-out"}
		}
	}
	objs = append(objs, &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "infer-1"},
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
	require.NotContains(t, err.Error(), "pt-x-eval-kept", "the run's own service ends with the run")
	require.Equal(t, before, remaining(t, cli), "a refused delete stops nothing")

	require.NoError(t, cli.Delete(context.Background(), &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "infer-1"}}))
	tuned := db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return([]*dbclient.PostTrainingRun{
		{RunID: "pt-x"}, {RunID: "pt-next"}}, int64(2), nil).After(based)
	err = h.deleteRun(testContext(), run, true)
	require.Equal(t, http.StatusBadRequest, statusCode(err), "pt-next trains on the model as its base")
	require.Contains(t, err.Error(), "exist: [pt-next].", "the run does not hold its own model")
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil).After(tuned)
	db.EXPECT().UpdateEvaluationTaskStatus(gomock.Any(), "task-1", dbclient.EvaluationTaskStatusCancelled).Return(nil)
	db.EXPECT().SetPostTrainingRunDeleted(gomock.Any(), "pt-x").Return(nil)
	db.EXPECT().DeletePostTrainingRunMetrics(gomock.Any(), "pt-x").Return(nil)
	db.EXPECT().DeletePostTrainingRunSamples(gomock.Any(), "pt-x", gomock.Any()).Return(nil)
	require.NoError(t, h.deleteRun(testContext(), run, true))
	require.NotContains(t, remaining(t, cli), "pt-x-eval-kept")
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
