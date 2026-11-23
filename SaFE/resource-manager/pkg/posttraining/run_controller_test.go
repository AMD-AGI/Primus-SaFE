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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/yaml"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonopsjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

const testRunDir = "/wekafs/custom/models/" + testRun

// loadChartRegistry makes the chart's default registry the reconciler's current one.
func loadChartRegistry(t *testing.T, r *RunReconciler) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "charts", "primus-safe", "values.yaml"))
	require.NoError(t, err)
	var values map[string]interface{}
	require.NoError(t, yaml.Unmarshal(data, &values))
	reg, err := yaml.Marshal(values["post_training"].(map[string]interface{})["registry"])
	require.NoError(t, err)
	require.NoError(t, r.registry.store.Load(reg, nil))
	r.registry.loadedAt = time.Now()
}

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

// A result line of an earlier attempt does not finish the run: once the result wait is over, it fails.
func TestResultOfOlderAttemptIsIgnored(t *testing.T) {
	run := testRunRow("Running", "train")
	run.OutputPath = testRunDir
	run.MetricCursor = `{"order":["a1","a2"],"attempts":{"a1":{"maxSeq":3},"a2":{"maxSeq":1}}}`
	w := endedWorkload(v1.WorkloadSucceeded)
	w.Status.EndTime = &metav1.Time{Time: time.Now().Add(-2 * resultWait)}
	r, db, updates := newTestReconciler(t, run, w)
	db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid1").Return(submitterPods(
		`[SUCCESS] {"run":"`+testRun+`","attempt":"a1","output":"`+testRunDir+`/final_model"}`), nil).AnyTimes()
	reconcileRun(t, r)
	last := (*updates)[len(*updates)-1]
	require.Equal(t, "Failed", last["status"])
	require.Equal(t, "export", last["failed_stage"])
	require.Equal(t, "The workload succeeded but reported no result", last["message"])
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
	last := (*updates)[len(*updates)-1]
	require.Equal(t, "Failed", last["status"])
	require.Equal(t, goneMessage, last["message"])

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
	r, db, updates := newTestReconciler(t, run)
	r.reader = shown.Client
	db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid1").Return(submitterPods("log\n"), nil).AnyTimes()
	require.Equal(t, runningRequeue, reconcileRun(t, r).RequeueAfter, "the run is followed as one that runs")
	requireNotFailed(t, *updates, "nothing is left to look for")

	// The run that was set running is followed the same way while the cache still lags.
	run.Status, run.Stage, run.WorkloadUID = "Running", "prepare", "uid1"
	require.Equal(t, runningRequeue, reconcileRun(t, r).RequeueAfter)
	requireNotFailed(t, *updates, "a run whose workload runs is not taken for lost")
}

// requireNotFailed checks that none of the updates failed the run.
func requireNotFailed(t *testing.T, updates []map[string]interface{}, msg string) {
	t.Helper()
	for _, u := range updates {
		require.NotEqual(t, "Failed", u["status"], msg)
	}
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

// The unfinished workloads of a deleted run are removed.
func TestTheChildrenOfADeletedRunAreRemoved(t *testing.T) {
	left := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "pt-gone", Labels: map[string]string{
		v1.PostTrainingRunIdLabel: "pt-gone", v1.PostTrainingRoleLabel: v1.PostTrainingRoleTrain}}}
	r, db, _ := newTestReconciler(t, testRunRow("Succeeded", "done"), left)
	db.EXPECT().GetPostTrainingRun(gomock.Any(), "pt-gone").Return(nil, gorm.ErrRecordNotFound).Times(2)
	ctx := context.Background()
	db.EXPECT().IsPostTrainingRunDeleted(gomock.Any(), "pt-gone").Return(false, nil)
	_, err := r.Reconcile(ctx, ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: "pt-gone"}})
	require.NoError(t, err)
	require.NoError(t, r.Get(ctx, client.ObjectKeyFromObject(left), &v1.Workload{}),
		"a run this database has no row of, as after a restore, is not a deleted one: what runs under it stays")

	db.EXPECT().IsPostTrainingRunDeleted(gomock.Any(), "pt-gone").Return(true, nil)
	_, err = r.Reconcile(ctx, ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: "pt-gone"}})
	require.NoError(t, err)
	require.Error(t, r.Get(ctx, client.ObjectKeyFromObject(left), &v1.Workload{}),
		"a workload of a deleted run holds GPUs nothing else frees")
}

// While Post Training is off the controller is not set up at all, even with the database on: nothing is watched
// and nothing is swept.
func TestTheControllerIsNotSetUpWhilePostTrainingIsOff(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(commonconfig.IsDBEnable, func() bool { return true })
	patches.ApplyFunc(commonconfig.IsPostTrainingEnable, func() bool { return false })
	// A nil manager fails any setup that reaches it.
	require.NoError(t, SetupPostTrainingController(context.Background(), nil))
}

// readTestLog reads the submitter log of a running testRun from a cluster whose only pod is the submitter pod
// named pod, with the run's cursor stored as cur, and returns the columns the read saved.
func readTestLog(t *testing.T, pod string, cur *logCursor) map[string]interface{} {
	run := testRunRow("Running", "train")
	data, err := json.Marshal(cur)
	require.NoError(t, err)
	run.MetricCursor = string(data)
	w := endedWorkload(v1.WorkloadRunning)
	w.Status.EndTime = nil
	w.Labels[v1.ClusterIdLabel] = "c1"
	submitter := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: pod, Namespace: "ws1", Labels: map[string]string{v1.WorkloadIdLabel: testRun}},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: SubmitterContainer}}},
	}
	clients := commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", k8sfake.NewSimpleClientset(submitter))
	manager := commonutils.NewObjectManager()
	manager.AddOrReplace("c1", clients)
	db := mockclient.NewMockInterface(gomock.NewController(t))
	var saved map[string]interface{}
	db.EXPECT().SavePostTrainingRunMetrics(gomock.Any(), testRun, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, _ []*dbclient.PostTrainingRunMetric, fields map[string]interface{}) error {
			saved = fields
			return nil
		})
	r := &RunReconciler{reader: ctrlfake.NewClientBuilder().Build(), db: db, clientManager: manager}
	_, err = r.readLog(context.Background(), w, newRunUpdate(run))
	require.NoError(t, err)
	return saved
}

// The last [STAGE] of a submitter pod judges only that pod: a retry or a preemption that kept the run queued for
// longer than lostAfter does not make the launcher of the next pod lost before it had the time to report.
func TestANewSubmitterPodIsNotTakenForALostLauncher(t *testing.T) {
	old := time.Now().Add(-30 * time.Minute)
	// The fake cluster serves a log without any marker, which is what a launcher that has not reported yet gives.
	saved := readTestLog(t, "submitter-a1", &logCursor{Pod: "submitter-a1", LastStage: old.Unix()})
	require.Contains(t, saved["message"], lostMessage, "the launcher of the pod read before is silent")

	saved = readTestLog(t, "submitter-a2", &logCursor{Pod: "submitter-a1", LastStage: old.Unix()})
	require.NotContains(t, saved, "message")
	require.NotContains(t, saved, "stalled_since")
	require.NotContains(t, saved, "stage_history", "no lost event is recorded")
	cur := loadCursor(saved["metric_cursor"].(string))
	require.Equal(t, "submitter-a2", cur.Pod)
	require.Zero(t, cur.LastStage)
}

// endedTestRun returns a reconciler of a run whose workload succeeded with a result in its run directory and
// whose output Model reconciles cannot create: create fails every Create of a Model.
func endedTestRun(t *testing.T, run *dbclient.PostTrainingRun, create error) (
	*RunReconciler, *[]map[string]interface{}) {
	r, db, updates := newTestReconciler(t, run, endedWorkload(v1.WorkloadSucceeded))
	db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid1").Return(submitterPods(
		`[SUCCESS] {"run":"`+testRun+`","attempt":"a1","output":"`+testRunDir+`/final_model"}`), nil).AnyTimes()
	r.Client = interceptor.NewClient(r.Client.(client.WithWatch), interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*v1.Model); ok {
				return create
			}
			return c.Create(ctx, obj, opts...)
		}})
	return r, updates
}

// A Model the API server rejects fails the run at once, and one that keeps failing otherwise fails it once the
// registration has taken longer than registerTimeout.
func TestARegistrationThatKeepsFailingEnds(t *testing.T) {
	invalid := apierrors.NewInvalid(schema.GroupKind{Group: "amd.com", Kind: "Model"}, testRun,
		field.ErrorList{field.Invalid(field.NewPath("spec"), "", "bad")})
	r, updates := endedTestRun(t, testRunRow("Running", "export"), invalid)
	reconcileRun(t, r)
	last := (*updates)[len(*updates)-1]
	require.Equal(t, "Failed", last["status"])
	require.Equal(t, "register", last["failed_stage"])
	require.Contains(t, last["message"], "The output model could not be registered")

	transient := apierrors.NewServiceUnavailable("try again")
	r, updates = endedTestRun(t, testRunRow("Running", "export"), transient)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: client.ObjectKey{Name: testRun}})
	require.Error(t, err, "an error a retry may not repeat is retried")
	last = (*updates)[len(*updates)-1]
	require.Equal(t, "register", last["stage"])
	require.NotContains(t, last, "status")

	late := testRunRow("Running", "register")
	late.StageHistory = `[{"stage":"register","time":"` + time.Now().Add(-31*time.Minute).UTC().Format(time.RFC3339) + `"}]`
	r, updates = endedTestRun(t, late, transient)
	reconcileRun(t, r)
	last = (*updates)[len(*updates)-1]
	require.Equal(t, "Failed", last["status"])
	require.Equal(t, "register", last["failed_stage"])
	require.Contains(t, last["message"], "not registered within 30 minutes")
}

// A reconcile that runs before the cache shows the Model a previous one created does not create a second one.
func TestTheOutputModelIsCreatedOnceWhileTheCacheLags(t *testing.T) {
	run := testRunRow("Running", "register")
	created := outputModel(run, testRunDir+"/final_model")
	r, db, updates := newTestReconciler(t, run, endedWorkload(v1.WorkloadSucceeded), created)
	db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid1").Return(submitterPods(
		`[SUCCESS] {"run":"`+testRun+`","attempt":"a1","output":"`+testRunDir+`/final_model"}`), nil).AnyTimes()
	api := r.Client
	r.reader = api
	r.Client = interceptor.NewClient(api.(client.WithWatch), interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*v1.Model); ok {
				return apierrors.NewNotFound(schema.GroupResource{Group: "amd.com", Resource: "models"}, key.Name)
			}
			return c.Get(ctx, key, obj, opts...)
		}})
	require.Equal(t, runningRequeue, reconcileRun(t, r).RequeueAfter)
	require.Equal(t, testRun, (*updates)[len(*updates)-1]["output_model_id"])
	models := &v1.ModelList{}
	require.NoError(t, api.List(context.Background(), models))
	require.Len(t, models.Items, 1)

	// A Model of the name that is not the run's is not taken for its output.
	foreign := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: testRun}}
	r, db, updates = newTestReconciler(t, run, endedWorkload(v1.WorkloadSucceeded), foreign)
	db.EXPECT().ListWorkloadPods(gomock.Any(), testRun, "uid1").Return(submitterPods(
		`[SUCCESS] {"run":"`+testRun+`","attempt":"a1","output":"`+testRunDir+`/final_model"}`), nil).AnyTimes()
	reconcileRun(t, r)
	last := (*updates)[len(*updates)-1]
	require.Equal(t, "Failed", last["status"])
	require.NotContains(t, last, "output_model_id")
}

// A run deleted while its last log read is saved ends the reconcile without an error.
func TestARunDeletedAtTheEndOfItsWorkloadIsNotRetried(t *testing.T) {
	run := testRunRow("Running", "train")
	r, _, _ := newTestReconciler(t, run, endedWorkload(v1.WorkloadSucceeded))
	db := mockclient.NewMockInterface(gomock.NewController(t))
	gomock.InOrder(
		db.EXPECT().GetPostTrainingRun(gomock.Any(), testRun).Return(run, nil),
		db.EXPECT().GetPostTrainingRun(gomock.Any(), testRun).Return(nil, gorm.ErrRecordNotFound),
	)
	db.EXPECT().SavePostTrainingRunMetrics(gomock.Any(), testRun, gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	r.db = db
	res := reconcileRun(t, r)
	require.Zero(t, res.RequeueAfter)
}
