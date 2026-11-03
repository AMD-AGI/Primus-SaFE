/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraining

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

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

func TestResultOfOlderAttemptIsIgnored(t *testing.T) {
	run := testRunRow("Running", "train")
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
}

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

func TestDisableFailoverOnNoRetry(t *testing.T) {
	w := endedWorkload(v1.WorkloadRunning)
	r, _, _ := newTestReconciler(t, testRunRow("Running", "train"), w)
	require.NoError(t, r.disableFailover(context.Background(), w))
	got := &v1.Workload{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: testRun}, got))
	require.True(t, v1.IsWorkloadDisableFailover(got))
}

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
