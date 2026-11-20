/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	commonopsjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

const testRunDir = "/wekafs/custom/models/" + testRun

// testRunRow returns a run of testRun at stage.
func testRunRow(status, stage string) *dbclient.PostTrainingRun {
	return &dbclient.PostTrainingRun{
		RunID: testRun, WorkloadID: testRun, WorkloadUID: "uid1", DisplayName: "qwen-sft", TrainType: "sft",
		Workspace: "ws1", UserID: "u1", BaseModelID: "qwen3-8b", BaseModelName: "Qwen3-8B", Status: status, Stage: stage,
		Recipe:    `{"output":{"base":"` + testRunDir + `"}}`,
		CreatedAt: pq.NullTime{Time: time.Now().Add(-time.Hour), Valid: true},
	}
}

// submitterPods returns the workload_pod row of a submitter whose container message is what job-manager keeps
// of log.
func submitterPods(log string) []*dbclient.WorkloadPod {
	return []*dbclient.WorkloadPod{dbclient.WorkloadPodFromV1(testRun, "uid1", 1, &v1.WorkloadPod{
		PodId: testRun + "-sub", Containers: []v1.Container{{Name: SubmitterContainer,
			Message: commonopsjob.FilterRunResultLog([]byte(log), testRun)}}})}
}

// newTestReconciler builds a reconciler over a fake API server holding objs and a mocked database whose run
// row is run; updates sends every UpdatePostTrainingRun column set.
func newTestReconciler(t *testing.T, run *dbclient.PostTrainingRun, objs ...client.Object) (
	*RunReconciler, *mockclient.MockInterface, *[]map[string]interface{}) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, v1.AddToScheme(scheme))
	cli := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).
		WithStatusSubresource(&v1.Model{}, &v1.Workload{}).Build()
	db := mockclient.NewMockInterface(gomock.NewController(t))
	db.EXPECT().GetPostTrainingRun(gomock.Any(), testRun).Return(run, nil).AnyTimes()
	db.EXPECT().SavePostTrainingRunMetrics(gomock.Any(), testRun, gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	updates := &[]map[string]interface{}{}
	db.EXPECT().UpdatePostTrainingRun(gomock.Any(), testRun, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fields map[string]interface{}) error {
			*updates = append(*updates, fields)
			return nil
		}).AnyTimes()
	r := &RunReconciler{Client: cli, reader: cli, db: db, clientManager: commonutils.NewObjectManager()}
	return r, db, updates
}

// A submitter log the controller cannot read does not mark the launcher lost.
func TestAnUnreadableLogDoesNotMarkTheLauncherLost(t *testing.T) {
	run := testRunRow("Running", "train")
	data, err := json.Marshal(&logCursor{Pod: "sub-1", LastStage: time.Now().Add(-11 * time.Minute).Unix()})
	require.NoError(t, err)
	run.MetricCursor = string(data)
	w := endedWorkload(v1.WorkloadRunning)
	w.Status.EndTime = nil
	db := mockclient.NewMockInterface(gomock.NewController(t))
	var saved map[string]interface{}
	db.EXPECT().SavePostTrainingRunMetrics(gomock.Any(), testRun, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, _ []*dbclient.PostTrainingRunMetric, fields map[string]interface{}) error {
			saved = fields
			return nil
		})
	r := &RunReconciler{reader: ctrlfake.NewClientBuilder().Build(), db: db,
		clientManager: commonutils.NewObjectManager()}
	_, err = r.readLog(context.Background(), w, newRunUpdate(run))
	require.NoError(t, err)
	require.NotContains(t, saved, "message", "a log the controller cannot read says nothing about the launcher")
	require.NotContains(t, saved, "stalled_since")
}

// endedWorkload returns the train workload of testRun in phase, ended a minute ago.
func endedWorkload(phase v1.WorkloadPhase) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: testRun, UID: "uid1",
		Labels: map[string]string{v1.PostTrainingRunIdLabel: testRun}}}
	w.Spec.Workspace = "ws1"
	w.Status.Phase = phase
	w.Status.EndTime = &metav1.Time{Time: time.Now().Add(-time.Minute)}
	return w
}

// reconcileRun runs one reconcile of testRun.
func reconcileRun(t *testing.T, r *RunReconciler) ctrlruntime.Result {
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: testRun}})
	require.NoError(t, err)
	return res
}

// A run's output is registered as a Model, and the run finishes once the Model is ready.
func TestRegisterCreatesModelAndFinishes(t *testing.T) {
	base := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: "qwen3-8b"}}
	base.Status.ModelConfig = &v1.ModelConfig{ModelType: "qwen3"}
	r, db, updates := newTestReconciler(t, testRunRow("Running", "export"), endedWorkload(v1.WorkloadSucceeded), base)
	success := `[SUCCESS] {"run":"` + testRun + `","attempt":"a1","pv":1,"stage":"export","output":"` +
		testRunDir + `/final_model"}`
	db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid1").Return(submitterPods("log\n"+success), nil).AnyTimes()

	res := reconcileRun(t, r)
	require.Equal(t, runningRequeue, res.RequeueAfter, "the run waits for the Model to become Ready")
	last := (*updates)[len(*updates)-1]
	require.Equal(t, "register", last["stage"])
	require.Equal(t, success, last["result_line"])

	models := &v1.ModelList{}
	require.NoError(t, r.List(context.Background(), models, client.MatchingLabels{v1.PostTrainingRunIdLabel: testRun}))
	require.Len(t, models.Items, 1)
	model := &models.Items[0]
	require.Equal(t, model.Name, last["output_model_id"])
	require.Equal(t, v1.AccessModeLocalPath, model.Spec.Source.AccessMode)
	require.Equal(t, testRunDir+"/final_model", model.Spec.Source.LocalPath)
	require.Equal(t, "fine_tuned", model.Spec.Origin)
	require.Equal(t, testRun, model.Spec.PostTrainingRunId)
	require.Equal(t, "Qwen3-8B", model.Spec.BaseModel)

	reconcileRun(t, r)
	require.NoError(t, r.Get(context.Background(), client.ObjectKeyFromObject(model), model))
	require.Equal(t, "qwen3", model.Status.ModelConfig.ModelType, "the structure summary comes from the base")
	require.NoError(t, r.List(context.Background(), models))
	require.Len(t, models.Items, 2, "a second pass reuses the Model")

	model.Status.Phase = v1.ModelPhaseReady
	require.NoError(t, r.Status().Update(context.Background(), model))
	require.Zero(t, reconcileRun(t, r).RequeueAfter)
	last = (*updates)[len(*updates)-1]
	require.Equal(t, "done", last["stage"])
	require.Equal(t, "Succeeded", last["status"])
}

// Registration records the run's PEFT adapter path.
func TestRegisterRecordsTheAdapter(t *testing.T) {
	for adapter, want := range map[string]string{testRunDir + "/final_adapter": "register", "/tmp/a": "Failed"} {
		r, db, updates := newTestReconciler(t, testRunRow("Running", "export"), endedWorkload(v1.WorkloadSucceeded))
		db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid1").Return(submitterPods(`[SUCCESS] {"run":"`+
			testRun+`","attempt":"a1","output":"`+testRunDir+`/final_model","adapter":"`+adapter+`"}`), nil).AnyTimes()
		reconcileRun(t, r)
		last := (*updates)[len(*updates)-1]
		if want == "Failed" {
			require.Equal(t, "Failed", last["status"])
			require.Contains(t, last["message"], "The adapter path")
			continue
		}
		require.Equal(t, want, last["stage"])
		require.Equal(t, adapter, last["output_adapter_path"])
	}
}

// A run without the HF export succeeds without registering a model.
func TestARunWithoutTheHFExportFinishesWithoutAModel(t *testing.T) {
	run := testRunRow("Running", "export")
	run.Recipe = `{"output":{"base":"` + testRunDir + `","exportHf":false}}`
	r, db, updates := newTestReconciler(t, run, endedWorkload(v1.WorkloadSucceeded))
	db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid1").Return(submitterPods(
		`[SUCCESS] {"run":"`+testRun+`","attempt":"a1","stage":"export","output":""}`), nil).AnyTimes()
	reconcileRun(t, r)
	last := (*updates)[len(*updates)-1]
	require.Equal(t, "Succeeded", last["status"])
	require.Equal(t, "done", last["stage"])
	models := &v1.ModelList{}
	require.NoError(t, r.List(context.Background(), models))
	require.Empty(t, models.Items, "there is no model to register")
}

// An output outside the run directory fails the registration.
func TestRegisterRejectsOutputOutsideRunDir(t *testing.T) {
	r, db, updates := newTestReconciler(t, testRunRow("Running", "export"), endedWorkload(v1.WorkloadSucceeded))
	db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid1").Return(submitterPods(
		`[SUCCESS] {"run":"`+testRun+`","attempt":"a1","output":"/wekafs/custom/models/other/final_model"}`), nil).AnyTimes()
	reconcileRun(t, r)
	last := (*updates)[len(*updates)-1]
	require.Equal(t, "Failed", last["status"])
	require.Equal(t, "register", last["failed_stage"])
	require.Contains(t, last["message"], "not under the run directory")
}

// A run whose workload is gone takes its result from the stored submitter pod.
func TestLostWorkloadUsesWorkloadPodResult(t *testing.T) {
	r, db, updates := newTestReconciler(t, testRunRow("Running", "train"))
	db.EXPECT().GetWorkload(gomock.Any(), testRun).Return(
		&dbclient.Workload{Phase: sql.NullString{String: "Failed", Valid: true}}, nil)
	db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid1").Return(submitterPods(
		`[ERROR] {"run":"`+testRun+`","attempt":"a1","stage":"check","reason":"tokenizer mismatch"}`), nil)
	reconcileRun(t, r)
	last := (*updates)[len(*updates)-1]
	require.Equal(t, "Failed", last["status"])
	require.Equal(t, "check", last["failed_stage"])
	require.Equal(t, "tokenizer mismatch", last["message"])
}

// A result line of an earlier attempt does not finish the run; its output is verified instead.
func TestResultOfOlderAttemptIsIgnored(t *testing.T) {
	run := testRunRow("Running", "train")
	run.OutputPath = testRunDir
	run.MetricCursor = `{"order":["a1","a2"],"attempts":{"a1":{"maxSeq":3},"a2":{"maxSeq":1}}}`
	w := endedWorkload(v1.WorkloadSucceeded)
	w.Status.EndTime = &metav1.Time{Time: time.Now().Add(-2 * resultWait)}
	r, db, updates := newTestReconciler(t, run, w)
	db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid1").Return(submitterPods(
		`[SUCCESS] {"run":"`+testRun+`","attempt":"a1","output":"`+testRunDir+`/final_model"}`), nil).AnyTimes()
	require.Equal(t, pendingRequeue, reconcileRun(t, r).RequeueAfter)
	require.Contains(t, (*updates)[len(*updates)-1]["stage_history"], `"event":"verify"`)
	job := &v1.OpsJob{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: verifyJobName(testRun)}, job))
	require.Equal(t, testRunDir, job.GetParameter(v1.ParameterDestPath).Value)
	require.Equal(t, testRun, job.Labels[v1.PostTrainingRunIdLabel])

	job.Status.Phase = v1.OpsJobFailed
	job.Status.Outputs = []v1.Parameter{{Name: "result", Value: `["[ERROR] VERIFY={\"disk\":{\"total\":5}}"]`}}
	require.NoError(t, r.Update(context.Background(), job))
	reconcileRun(t, r)
	last := (*updates)[len(*updates)-1]
	require.Equal(t, "Failed", last["status"])
	require.Equal(t, "export", last["failed_stage"])
	require.Equal(t, `{"total":5}`, last["disk_usage"])
}

// A complete output found by verification is registered for a run without a result line.
func TestVerifyRegistersCompleteOutputOfLostWorkload(t *testing.T) {
	run := testRunRow("Running", "export")
	run.OutputPath = testRunDir
	r, db, updates := newTestReconciler(t, run)
	db.EXPECT().GetWorkload(gomock.Any(), testRun).Return(nil, errors.New("not found")).AnyTimes()
	db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid1").Return(nil, nil).AnyTimes()
	reconcileRun(t, r)
	job := &v1.OpsJob{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: verifyJobName(testRun)}, job))
	job.Status.Phase = v1.OpsJobSucceeded
	job.Status.Outputs = []v1.Parameter{{Name: "result", Value: commonopsjob.FilterResultLog([]byte(
		`[SUCCESS] VERIFY={"stage":"export","output":"` + testRunDir + `/final_model","iteration":10,` +
			`"disk":{"finalModel":9,"total":9}}`))}}
	require.NoError(t, r.Update(context.Background(), job))
	reconcileRun(t, r)
	last := (*updates)[len(*updates)-1]
	require.Equal(t, "register", last["stage"])
	require.Equal(t, `{"finalModel":9,"total":9}`, last["disk_usage"])
	require.NotEmpty(t, last["output_model_id"])
	// The run keeps the last checkpoint the launcher wrote down, in a line that is no result marker of an attempt.
	line := `[SUCCESS] {"run":"` + testRun + `","stage":"export","output":"` + testRunDir + `/final_model","iteration":10}`
	require.Equal(t, line, last["result_line"])
	require.Nil(t, lastResult(line, testRun))
}

// A pending run whose workload was never created gets no event of a workload: the sweep of the unfinished runs
// queues it, and its reconcile ends it once nothing of a workload is found.
func TestAPendingRunWithoutAWorkloadIsQueuedAndEnded(t *testing.T) {
	run := testRunRow("Pending", "queued")
	run.OutputPath, run.WorkloadUID = testRunDir, ""
	r, db, updates := newTestReconciler(t, run)
	r.recovered = make(chan event.GenericEvent, 2)
	db.EXPECT().ListPostTrainingRuns(gomock.Any(), dbclient.PostTrainingRunFilter{
		Statuses: []string{string(v1.WorkloadPending), string(v1.WorkloadRunning)}, Limit: recoverPageSize}).Return(
		[]*dbclient.PostTrainingRun{run}, int64(1), nil)
	r.sweepUnfinished(context.Background())
	require.Len(t, r.recovered, 1)
	require.Equal(t, testRun, (<-r.recovered).Object.GetName())

	db.EXPECT().GetWorkload(gomock.Any(), testRun).Return(nil, errors.New("not found")).AnyTimes()
	db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "").Return(nil, nil).AnyTimes()
	reconcileRun(t, r)
	job := &v1.OpsJob{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: verifyJobName(testRun)}, job),
		"the run directory is checked for what a workload left")
	job.Status.Phase = v1.OpsJobFailed
	require.NoError(t, r.Update(context.Background(), job))
	reconcileRun(t, r)
	require.Equal(t, "Failed", (*updates)[len(*updates)-1]["status"])
	run.Status = "Failed"
	reconcileRun(t, r)
	require.Error(t, r.Get(context.Background(), client.ObjectKey{Name: backfillWorkloadName(testRun)}, &v1.Workload{}),
		"a run without a workload has no directory to go over")

	// A run that was just recorded is given the time its workload takes to show.
	fresh := testRunRow("Pending", "queued")
	fresh.CreatedAt.Time = time.Now()
	r, _, updates = newTestReconciler(t, fresh)
	require.Equal(t, runningRequeue, reconcileRun(t, r).RequeueAfter)
	require.Empty(t, *updates)
}

// A pending run whose workload the cache does not show is followed when the API server has the workload: a cache
// that lags does not make the run lost.
func TestAPendingRunIsNotLostWhileTheAPIServerHasItsWorkload(t *testing.T) {
	run := testRunRow("Pending", "queued")
	run.OutputPath = testRunDir
	w := endedWorkload(v1.WorkloadRunning)
	w.Status.EndTime = nil
	shown, _, _ := newTestReconciler(t, run, w)
	r, db, _ := newTestReconciler(t, run)
	r.reader = shown.Client
	db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid1").Return(submitterPods("log\n"), nil).AnyTimes()
	require.Equal(t, runningRequeue, reconcileRun(t, r).RequeueAfter, "the run is followed as one that runs")
	require.Error(t, r.Get(context.Background(), client.ObjectKey{Name: verifyJobName(testRun)}, &v1.OpsJob{}),
		"nothing is left to look for")

	// The run that was set running is followed the same way while the cache still lags.
	run.Status, run.Stage, run.WorkloadUID = "Running", "prepare", "uid1"
	require.Equal(t, runningRequeue, reconcileRun(t, r).RequeueAfter)
	require.Error(t, r.Get(context.Background(), client.ObjectKey{Name: verifyJobName(testRun)}, &v1.OpsJob{}),
		"a run whose workload runs is not taken for lost")
}

// The UID of a run's workload is recorded with the end of the run when the workload is first seen as it ends, and
// when a workload that ran unseen is known from the workload table: the run's directory is then gone over.
func TestAWorkloadSeenOnlyAtItsEndLeavesItsUID(t *testing.T) {
	seen := testRunRow("Pending", "queued")
	seen.OutputPath, seen.WorkloadUID = testRunDir, ""
	stopped := endedWorkload(v1.WorkloadStopped)
	started := metav1.NewTime(time.Now().Add(-time.Hour).Truncate(time.Second))
	stopped.Status.StartTime = &started
	r, db, updates := newTestReconciler(t, seen, stopped)
	db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, gomock.Any()).Return(nil, nil).AnyTimes()
	reconcileRun(t, r)
	last := (*updates)[len(*updates)-1]
	require.Equal(t, "Stopped", last["status"])
	require.Equal(t, "uid1", last["workload_uid"], "the read of the log may not have saved it")
	require.Equal(t, started.Time, last["start_time"], "nor when the workload started")

	unseen := testRunRow("Pending", "queued")
	unseen.OutputPath, unseen.WorkloadUID = testRunDir, ""
	r, db, updates = newTestReconciler(t, unseen)
	db.EXPECT().GetWorkload(gomock.Any(), testRun).Return(&dbclient.Workload{
		Phase:       sql.NullString{String: "Failed", Valid: true},
		WorkloadUId: sql.NullString{String: "uid9", Valid: true}}, nil)
	// The pod rows are kept by the UID of their workload: the same reconcile reads them by the row's.
	db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid9").Return(submitterPods(
		`[ERROR] {"run":"`+testRun+`","attempt":"a1","stage":"check","reason":"tokenizer mismatch"}`), nil).AnyTimes()
	reconcileRun(t, r)
	last = (*updates)[len(*updates)-1]
	require.Equal(t, "Failed", last["status"])
	require.Equal(t, "uid9", last["workload_uid"])
	require.Equal(t, "check", last["failed_stage"], "the run ends with what its workload reported")
	require.Equal(t, "tokenizer mismatch", last["message"])
	unseen.Status, unseen.WorkloadUID = "Failed", "uid9"
	require.True(t, backfillPending(unseen, nil), "the workload ran, so its directory is counted")
}

// A run that exports no model and lost its result line ends as succeeded once verification finds its launcher
// finished; the same finding fails a run that was to export one.
func TestVerifyEndsALostRunThatExportsNoModel(t *testing.T) {
	for exportHf, status := range map[string]string{"false": "Succeeded", "true": "Failed"} {
		run := testRunRow("Running", "export")
		run.OutputPath = testRunDir
		run.Recipe = `{"output":{"base":"` + testRunDir + `","exportHf":` + exportHf + `}}`
		r, db, updates := newTestReconciler(t, run)
		db.EXPECT().GetWorkload(gomock.Any(), testRun).Return(nil, errors.New("not found")).AnyTimes()
		db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid1").Return(nil, nil).AnyTimes()
		reconcileRun(t, r)
		job := &v1.OpsJob{}
		require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: verifyJobName(testRun)}, job))
		job.Status.Phase = v1.OpsJobSucceeded
		job.Status.Outputs = []v1.Parameter{{Name: "result", Value: commonopsjob.FilterResultLog([]byte(
			`[SUCCESS] VERIFY={"stage":"export","output":"","disk":{"checkpoints":9,"total":9}}`))}}
		require.NoError(t, r.Update(context.Background(), job))
		reconcileRun(t, r)
		last := (*updates)[len(*updates)-1]
		require.Equal(t, status, last["status"], "exportHf=%s", exportHf)
		require.NotContains(t, last, "result_line", "a result without the last checkpoint leaves the line as it is")
	}
}

// The last checkpoint a verification finds is kept when it is a whole number that is not negative, also for a run
// that exports no model; any other value is ignored and fails no run whose launcher finished.
func TestTheVerifiedCheckpointIsKeptOnlyAsAWholeNumber(t *testing.T) {
	line := func(iteration string) string {
		return `[SUCCESS] {"run":"` + testRun + `","stage":"export","output":"","iteration":` + iteration + `}`
	}
	for raw, want := range map[string]string{`5`: line("5"), `0`: line("0"), `10.0`: "", `"10"`: "", `-1`: "",
		`null`: ""} {
		run := testRunRow("Running", "export")
		run.OutputPath = testRunDir
		run.Recipe = `{"output":{"base":"` + testRunDir + `","exportHf":false}}`
		r, db, updates := newTestReconciler(t, run)
		db.EXPECT().GetWorkload(gomock.Any(), testRun).Return(nil, errors.New("not found")).AnyTimes()
		db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid1").Return(nil, nil).AnyTimes()
		reconcileRun(t, r)
		job := &v1.OpsJob{}
		require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: verifyJobName(testRun)}, job))
		job.Status.Phase = v1.OpsJobSucceeded
		job.Status.Outputs = []v1.Parameter{{Name: "result", Value: commonopsjob.FilterResultLog([]byte(
			`[SUCCESS] VERIFY={"stage":"export","output":"","iteration":` + raw + `,"disk":{"total":9}}`))}}
		require.NoError(t, r.Update(context.Background(), job))
		reconcileRun(t, r)
		last := (*updates)[len(*updates)-1]
		require.Equal(t, "Succeeded", last["status"], "iteration %s", raw)
		if want == "" {
			require.NotContains(t, last, "result_line", "iteration %s", raw)
		} else {
			require.Equal(t, want, last["result_line"], "iteration %s", raw)
		}
	}
}

// A preempted run stays running while it waits to be rescheduled.
func TestPreemptedRunKeepsRunning(t *testing.T) {
	w := endedWorkload(v1.WorkloadPending)
	w.Status.EndTime = nil
	w.Status.QueuePosition = 3
	w.Labels[v1.WorkloadDispatchCntLabel] = "1"
	w.Annotations = map[string]string{v1.WorkloadPreemptedAnnotation: "2026-11-03"}
	r, _, updates := newTestReconciler(t, testRunRow("Running", "train"), w)
	require.Equal(t, pendingRequeue, reconcileRun(t, r).RequeueAfter)
	last := (*updates)[len(*updates)-1]
	require.NotContains(t, last, "status")
	require.Equal(t, "Preempted, waiting to be rescheduled", last["message"])
	require.Equal(t, 3, last["queue_position"])
	require.Contains(t, last["stage_history"], `"event":"preempted"`)
}

// A stage that refuses retry turns failover off once and records the event once.
func TestDisableFailoverOnNoRetry(t *testing.T) {
	w := endedWorkload(v1.WorkloadRunning)
	r, _, _ := newTestReconciler(t, testRunRow("Running", "train"), w)
	require.NoError(t, r.disableFailover(context.Background(), w))
	got := &v1.Workload{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: testRun}, got))
	require.True(t, v1.IsWorkloadDisableFailover(got))

	u := newRunUpdate(testRunRow("Running", "train"))
	markNotRetried(u)
	markNotRetried(u)
	require.True(t, hasEvent(u.history, notRetriedEvent))
	require.Len(t, u.history, 1, "the event is recorded once")
}

// A registration whose Model is not ready in time fails the run at register.
func TestRegisterTimesOut(t *testing.T) {
	run := testRunRow("Running", "register")
	run.StageHistory = `[{"stage":"register","time":"` + time.Now().Add(-31*time.Minute).UTC().Format(time.RFC3339) + `"}]`
	model := outputModel(run, testRunDir+"/final_model")
	r, db, updates := newTestReconciler(t, run, endedWorkload(v1.WorkloadSucceeded), model)
	db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid1").Return(submitterPods(
		`[SUCCESS] {"run":"`+testRun+`","attempt":"a1","output":"`+testRunDir+`/final_model"}`), nil).AnyTimes()
	reconcileRun(t, r)
	last := (*updates)[len(*updates)-1]
	require.Equal(t, "Failed", last["status"])
	require.Equal(t, "register", last["failed_stage"])
}

// Samples past the run's cap stay in the run directory only.
func TestSaveSamplesStopsAtTheCap(t *testing.T) {
	run := testRunRow("Running", "train")
	run.Recipe = `{"output":{"base":"` + testRunDir + `"},"metrics":{"samples":{"maxRows":3}}}`
	r, db, _ := newTestReconciler(t, run)
	rows := func(n int) []*dbclient.PostTrainingRunSample {
		out := make([]*dbclient.PostTrainingRunSample, n)
		for i := range out {
			out[i] = &dbclient.PostTrainingRunSample{RunID: testRun, Kind: "rollout", Step: 1, SampleIndex: i}
		}
		return out
	}
	db.EXPECT().UpsertPostTrainingRunSamples(gomock.Any(), gomock.Len(2)).Return(nil)
	db.EXPECT().CountPostTrainingRunSamples(gomock.Any(), testRun).Return(int64(2), nil)
	in := &ingestion{sum: &metricSummary{}}
	require.NoError(t, r.saveSamples(context.Background(), run, in, rows(2)))
	require.EqualValues(t, 2, in.sum.Samples)
	require.False(t, in.sum.SamplesCapped)

	db.EXPECT().UpsertPostTrainingRunSamples(gomock.Any(), gomock.Len(1)).Return(nil)
	db.EXPECT().CountPostTrainingRunSamples(gomock.Any(), testRun).Return(int64(3), nil)
	require.NoError(t, r.saveSamples(context.Background(), run, in, rows(4)))
	require.True(t, in.sum.SamplesCapped, "the samples past the cap stay in the run directory")
	require.NoError(t, r.saveSamples(context.Background(), run, in, rows(1)), "a full run stores nothing more")
}

// Samples the database refuses are dropped, so they do not hold up what was read with them; a database that could
// not be asked fails the save, which the next reconcile tries again.
func TestSaveSamplesTellsRefusedDataFromAFailedDatabase(t *testing.T) {
	run := testRunRow("Running", "train")
	r, db, _ := newTestReconciler(t, run)
	rows := []*dbclient.PostTrainingRunSample{{RunID: testRun, Kind: "rollout", Step: 1}}
	in := &ingestion{sum: &metricSummary{}}
	// The samples are written through GORM, whose driver is pgx: its errors are not pq errors.
	refusals := []error{&pgconn.PgError{Code: "22001"}, fmt.Errorf("upsert: %w", &pgconn.PgError{Code: "23505"}),
		&pq.Error{Code: "22001"}}
	for _, refused := range refusals {
		db.EXPECT().UpsertPostTrainingRunSamples(gomock.Any(), gomock.Any()).Return(refused)
		require.NoError(t, r.saveSamples(context.Background(), run, in, rows), "a value too long stays too long")
	}
	db.EXPECT().UpsertPostTrainingRunSamples(gomock.Any(), gomock.Any()).Return(&pgconn.PgError{Code: "57P01"})
	require.Error(t, r.saveSamples(context.Background(), run, in, rows), "a database that shut down is asked again")
	db.EXPECT().UpsertPostTrainingRunSamples(gomock.Any(), gomock.Any()).Return(errors.New("connection refused"))
	require.Error(t, r.saveSamples(context.Background(), run, in, rows))
}

// A stopped run says who stopped it.
func TestStoppedRunSaysWhoStoppedIt(t *testing.T) {
	w := endedWorkload(v1.WorkloadStopped)
	w.Status.Conditions = []metav1.Condition{{Type: string(v1.AdminStopped), Status: metav1.ConditionTrue,
		Message: "Stopped by alice", Reason: "r", LastTransitionTime: metav1.Now()}}
	r, db, updates := newTestReconciler(t, testRunRow("Running", "train"), w)
	db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid1").Return(submitterPods("log\n"), nil).AnyTimes()
	reconcileRun(t, r)
	last := (*updates)[len(*updates)-1]
	require.Equal(t, "Stopped", last["status"])
	require.Equal(t, "Stopped by alice", last["message"], "the detail page says who stopped the run")
}

// The reconcile that ends a run queues the run once more: what follows the end, such as the backfill of its
// metrics, has no event of the run's objects to start it.
func TestTheReconcileThatEndsARunQueuesItAgain(t *testing.T) {
	r, db, updates := newTestReconciler(t, testRunRow("Running", "train"), endedWorkload(v1.WorkloadStopped))
	r.recovered = make(chan event.GenericEvent, 2)
	db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid1").Return(submitterPods("log\n"), nil).AnyTimes()
	reconcileRun(t, r)
	require.Equal(t, "Stopped", (*updates)[len(*updates)-1]["status"])
	require.Len(t, r.recovered, 1)
	require.Equal(t, testRun, (<-r.recovered).Object.GetName())

	// A run that goes on has its workload's events.
	w := endedWorkload(v1.WorkloadRunning)
	w.Status.EndTime = nil
	r, db, _ = newTestReconciler(t, testRunRow("Pending", "queued"), w)
	r.recovered = make(chan event.GenericEvent, 2)
	db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid1").Return(submitterPods("log\n"), nil).AnyTimes()
	reconcileRun(t, r)
	require.Empty(t, r.recovered)
}

// Imported runs get no backfill, verification or disk recount.
func TestImportedRunsAreLeftAlone(t *testing.T) {
	run := testRunRow("Stopped", "done")
	run.Source, run.OutputPath = dbclient.PostTrainingSourceSpur, "/shared_nfs/runs/j2"
	r, _, updates := newTestReconciler(t, run)
	require.Zero(t, reconcileRun(t, r))
	require.Empty(t, *updates, "no backfill, verification or disk recount for a Spur directory")
}
