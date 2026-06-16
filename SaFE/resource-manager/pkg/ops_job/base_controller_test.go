/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func opsScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func newBaseReconciler(t *testing.T, objs ...client.Object) *OpsJobBaseReconciler {
	t.Helper()
	cl := ctrlfake.NewClientBuilder().
		WithScheme(opsScheme(t)).
		WithStatusSubresource(&v1.OpsJob{}).
		WithObjects(objs...).
		Build()
	return &OpsJobBaseReconciler{Client: cl}
}

type stubComponent struct {
	filterResult bool
	observeQuit  bool
	observeErr   error
	handleResult ctrlruntime.Result
	handleErr    error
}

func (s *stubComponent) observe(_ context.Context, _ *v1.OpsJob) (bool, error) {
	return s.observeQuit, s.observeErr
}
func (s *stubComponent) filter(_ context.Context, _ *v1.OpsJob) bool { return s.filterResult }
func (s *stubComponent) handle(_ context.Context, _ *v1.OpsJob) (ctrlruntime.Result, error) {
	return s.handleResult, s.handleErr
}

func newTestOpsJob(name string) *v1.OpsJob {
	return &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{Name: name, Finalizers: []string{v1.OpsJobFinalizer}},
		Spec:       v1.OpsJobSpec{Type: v1.OpsJobRebootType},
	}
}

func TestReconcileNotFound(t *testing.T) {
	r := newBaseReconciler(t)
	res, err := r.Reconcile(context.Background(),
		ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing"}}, &stubComponent{})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestReconcileFiltered(t *testing.T) {
	job := newTestOpsJob("j1")
	r := newBaseReconciler(t, job)
	_, err := r.Reconcile(context.Background(),
		ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}}, &stubComponent{filterResult: true})
	assert.NoError(t, err)
}

func TestReconcileObserveQuit(t *testing.T) {
	job := newTestOpsJob("j1")
	r := newBaseReconciler(t, job)
	_, err := r.Reconcile(context.Background(),
		ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}}, &stubComponent{observeQuit: true})
	assert.NoError(t, err)
}

func TestReconcileHandle(t *testing.T) {
	job := newTestOpsJob("j1")
	r := newBaseReconciler(t, job)
	res, err := r.Reconcile(context.Background(),
		ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "j1"}},
		&stubComponent{handleResult: ctrlruntime.Result{RequeueAfter: time.Second}})
	assert.NoError(t, err)
	assert.Equal(t, time.Second, res.RequeueAfter)
}

func TestSetJobCompleted(t *testing.T) {
	job := newTestOpsJob("j1")
	r := newBaseReconciler(t, job)
	err := r.setJobCompleted(context.Background(), job, v1.OpsJobSucceeded, "done", nil)
	assert.NoError(t, err)
	assert.Equal(t, v1.OpsJobSucceeded, job.Status.Phase)

	// Already in target phase -> no-op.
	assert.NoError(t, r.setJobCompleted(context.Background(), job, v1.OpsJobSucceeded, "again", nil))
}

func TestSetJobCompletedFailed(t *testing.T) {
	job := newTestOpsJob("j2")
	r := newBaseReconciler(t, job)
	err := r.setJobCompleted(context.Background(), job, v1.OpsJobFailed, "", nil)
	assert.NoError(t, err)
	assert.Equal(t, v1.OpsJobFailed, job.Status.Phase)
}

func TestSetJobPhase(t *testing.T) {
	job := newTestOpsJob("j1")
	r := newBaseReconciler(t, job)
	err := r.setJobPhase(context.Background(), job, v1.OpsJobRunning)
	assert.NoError(t, err)
	assert.Equal(t, v1.OpsJobRunning, job.Status.Phase)
	assert.NotNil(t, job.Status.StartedAt)
}

func TestTimeout(t *testing.T) {
	job := newTestOpsJob("j1")
	job.Spec.TimeoutSecond = 10
	r := newBaseReconciler(t, job)
	err := r.timeout(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, v1.OpsJobFailed, job.Status.Phase)
}

func TestGetAdminNodeNotFound(t *testing.T) {
	r := newBaseReconciler(t)
	_, err := r.getAdminNode(context.Background(), "missing")
	assert.Error(t, err)
}

func TestGetFaultNotFound(t *testing.T) {
	r := newBaseReconciler(t)
	_, err := r.getFault(context.Background(), "node1", "monitor1")
	assert.Error(t, err)
}

func TestBaseDeleteNotFinished(t *testing.T) {
	job := newTestOpsJob("j1")
	r := newBaseReconciler(t, job)
	called := false
	clear := func(_ context.Context, _ *v1.OpsJob) error { called = true; return nil }
	// Not finished -> setJobCompleted(Failed) then clearFuncs then RemoveFinalizer.
	err := r.delete(context.Background(), job, clear)
	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, v1.OpsJobFailed, job.Status.Phase)
}

func TestBaseDeleteFinished(t *testing.T) {
	job := newTestOpsJob("j2")
	job.Status.Phase = v1.OpsJobSucceeded
	job.Status.FinishedAt = &metav1.Time{Time: time.Now()}
	r := newBaseReconciler(t, job)
	err := r.delete(context.Background(), job)
	assert.NoError(t, err)
}

func TestDeleteFaultNoFault(t *testing.T) {
	r := newBaseReconciler(t)
	// No fault present -> no error.
	assert.NoError(t, r.deleteFault(context.Background(), "node1", "monitor1"))
}

func TestGetInputNodesNone(t *testing.T) {
	job := newTestOpsJob("j1")
	r := newBaseReconciler(t, job)
	_, err := r.getInputNodes(context.Background(), job)
	assert.Error(t, err)
}

func TestListJobs(t *testing.T) {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "j1",
			Labels: map[string]string{
				v1.ClusterIdLabel:  "c1",
				v1.OpsJobTypeLabel: "reboot",
			},
		},
	}
	r := newBaseReconciler(t, job)
	jobs, err := r.listJobs(context.Background(), "c1", "reboot")
	assert.NoError(t, err)
	assert.Len(t, jobs, 1)
}

func TestNewRequeueAfterResult(t *testing.T) {
	job := newTestOpsJob("j1")
	assert.Equal(t, ctrlruntime.Result{}, newRequeueAfterResult(job))
	job.Spec.TimeoutSecond = 30
	assert.Equal(t, time.Second*30, newRequeueAfterResult(job).RequeueAfter)
}

func TestOnFirstPhaseChangedPredicate(t *testing.T) {
	p := onFirstPhaseChangedPredicate()
	oldJob := &v1.OpsJob{}
	newJob := &v1.OpsJob{Status: v1.OpsJobStatus{Phase: v1.OpsJobRunning}}
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: oldJob, ObjectNew: newJob}))
	// No phase change.
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: oldJob, ObjectNew: oldJob}))
}

func TestGetPreflightMasterPodLog(t *testing.T) {
	wl := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "wl1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}},
		Spec:       v1.WorkloadSpec{Workspace: "ws1"},
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      "wl1-master-0",
		Namespace: "ws1",
		Labels: map[string]string{
			v1.WorkloadIdLabel:                    "wl1",
			"training.kubeflow.org/replica-type": "master",
		},
	}}
	cs := k8sfake.NewSimpleClientset(pod)
	r := newBaseWithObjs(t, wl)
	r.clientManager = commonutils.NewObjectManager()
	_ = r.clientManager.Add("c1", commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", cs))

	data, err := r.getPreflightMasterPodLog(context.Background(), wl)
	assert.NoError(t, err)
	assert.NotNil(t, data)
}

func TestGetWorkloadCompletionMessageStopped(t *testing.T) {
	r := newBaseReconciler(t)
	wl := &v1.Workload{Status: v1.WorkloadStatus{Phase: v1.WorkloadStopped}}
	assert.Equal(t, "workload is stopped", r.getWorkloadCompletionMessage(context.Background(), wl))
}

func TestUpdateCondition(t *testing.T) {
	job := newTestOpsJob("j1")
	r := newBaseReconciler(t, job)
	cond := &metav1.Condition{Type: "Test", Status: metav1.ConditionTrue, Reason: "ok", Message: "m"}
	assert.NoError(t, r.updateCondition(context.Background(), job, cond))
}
