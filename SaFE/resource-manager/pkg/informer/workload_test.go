/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package informer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	workloadtopic "github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/topic/workload"
)

func newInformer(t *testing.T, objs ...ctrlclient.Object) *WorkloadInformer {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
	return NewWorkloadInformer(cl)
}

type submittedNotification struct {
	uid  string
	data map[string]interface{}
}

func captureNotifications(t *testing.T) *[]submittedNotification {
	t.Helper()
	var got []submittedNotification
	orig := submitNotification
	submitNotification = func(_ context.Context, _, uid string, data map[string]interface{}) error {
		got = append(got, submittedNotification{uid: uid, data: data})
		return nil
	}
	t.Cleanup(func() { submitNotification = orig })
	return &got
}

func notifiedOwner() *v1.User {
	return &v1.User{ObjectMeta: metav1.ObjectMeta{
		Name:        "u1",
		Annotations: map[string]string{v1.UserEnableNotificationAnnotation: v1.TrueStr},
	}}
}

func lowUtilWorkspace(dryRun bool) *v1.Workspace {
	return &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws"},
		Spec: v1.WorkspaceSpec{LowUtilization: map[v1.WorkspaceScope]v1.LowUtilizationPolicy{
			v1.TrainScope: {Threshold: 5, Window: "1h", Mode: v1.LowUtilizationAverage, NotifyBefore: "30m", DryRun: dryRun},
		}},
	}
}

func ownedTrainingWorkload(phase v1.WorkloadPhase) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:   "wl1",
		Labels: map[string]string{v1.UserIdLabel: "u1"},
	}}
	w.Spec.Workspace = "ws"
	w.Spec.GroupVersionKind.Kind = common.PytorchJobKind
	w.Status.Phase = phase
	return w
}

// markNotified sets what job-manager writes once a stop countdown starts and the owner is told.
func markNotified(w *v1.Workload, enforcedAt, notifiedAt string) {
	v1.SetAnnotation(w, v1.WorkloadLowUtilDetectedAtAnnotation, "2026-01-02T11:00:00Z")
	v1.SetAnnotation(w, v1.WorkloadLowUtilEnforcedAtAnnotation, enforcedAt)
	v1.SetAnnotation(w, v1.WorkloadLowUtilNotifiedAtAnnotation, notifiedAt)
}

func TestOnUpdateNotifiesLowUtilizationOnce(t *testing.T) {
	got := captureNotifications(t)
	w := newInformer(t, notifiedOwner(), lowUtilWorkspace(false))

	oldW := ownedTrainingWorkload(v1.WorkloadRunning)
	// The detection mark alone, written before the Detected entries, does not notify.
	detected := oldW.DeepCopy()
	v1.SetAnnotation(detected, v1.WorkloadLowUtilDetectedAtAnnotation, "2026-01-02T11:00:00Z")
	w.OnUpdate(oldW, detected)
	assert.Empty(t, *got)

	newW := detected.DeepCopy()
	markNotified(newW, "2026-01-02T12:00:00Z", "2026-01-02T12:00:00Z")
	w.OnUpdate(detected, newW)
	// A later update keeping the mark does not notify again.
	w.OnUpdate(newW, newW.DeepCopy())

	assert.Len(t, *got, 1)
	n := (*got)[0]
	assert.Equal(t, "wl1-LowUtilization-2026-01-02T12:00:00Z", n.uid)
	assert.Equal(t, workloadtopic.ConditionLowUtilization, n.data["condition"])
	notice, ok := n.data["lowUtilization"].(*workloadtopic.LowUtilizationNotice)
	assert.True(t, ok)
	assert.Equal(t, workloadtopic.LowUtilizationNotice{
		Threshold: 5, Window: "1h", StopTime: "2026-01-02 12:30:00 UTC",
	}, *notice)

	// A countdown started again, for example after a dry-run policy is enforced, notifies again.
	again := newW.DeepCopy()
	markNotified(again, "2026-01-02T14:00:00Z", "2026-01-02T14:00:00Z")
	w.OnUpdate(newW, again)
	assert.Len(t, *got, 2)
	notice, ok = (*got)[1].data["lowUtilization"].(*workloadtopic.LowUtilizationNotice)
	assert.True(t, ok)
	assert.Equal(t, "2026-01-02 14:30:00 UTC", notice.StopTime)
}

func rulesWorkspace() *v1.Workspace {
	ws := lowUtilWorkspace(false)
	ws.Spec.LowUtilization[v1.TrainScope] = v1.LowUtilizationPolicy{
		Rules:        []v1.LowUtilizationRule{{Threshold: 11, Window: "30m"}, {Threshold: 30, Window: "1h"}},
		Mode:         v1.LowUtilizationAverage,
		NotifyBefore: "30m",
	}
	return ws
}

func lowUtilEntry(reason, message string) metav1.Condition {
	return metav1.Condition{Type: string(v1.AdminLowUtilization), Status: metav1.ConditionTrue, Reason: reason, Message: message}
}

// notifyLowUtilization runs the detection update for a workload carrying the given timeline.
func notifyLowUtilization(t *testing.T, ws *v1.Workspace, conds ...metav1.Condition) *workloadtopic.LowUtilizationNotice {
	t.Helper()
	got := captureNotifications(t)
	w := newInformer(t, notifiedOwner(), ws)
	oldW := ownedTrainingWorkload(v1.WorkloadRunning)
	oldW.Status.Conditions = conds
	newW := oldW.DeepCopy()
	markNotified(newW, "2026-01-02T12:00:00Z", "2026-01-02T12:00:00Z")
	w.OnUpdate(oldW, newW)
	assert.Len(t, *got, 1)
	notice, ok := (*got)[0].data["lowUtilization"].(*workloadtopic.LowUtilizationNotice)
	assert.True(t, ok)
	return notice
}

func TestLowUtilizationNoticeShowsMatchedRules(t *testing.T) {
	notice := notifyLowUtilization(t, rulesWorkspace(),
		lowUtilEntry("Detected_1_lt11_30m", "low GPU utilization below 11% for 30m: mode Average, observed max 9.0%; dry run"),
		lowUtilEntry("Recovered_1", "GPU utilization recovered: observed max 50.0%"),
		lowUtilEntry("Detected_2_lt11_30m", "low GPU utilization below 11% for 30m: mode Average, observed max 3.0%; "+
			"the workload will be stopped at 2026-01-02T12:30:00Z if it stays idle"),
		lowUtilEntry("Detected_2_lt30_1h", "low GPU utilization below 30% for 1h: mode Average, observed max 25.0%; "+
			"the workload will be stopped at 2026-01-02T12:30:00Z if it stays idle"))

	assert.Equal(t, workloadtopic.LowUtilizationNotice{
		StopTime: "2026-01-02 12:30:00 UTC",
		Rules: []string{
			"low GPU utilization below 11% for 30m: mode Average, observed max 3.0%",
			"low GPU utilization below 30% for 1h: mode Average, observed max 25.0%",
		},
	}, *notice)
}

func TestLowUtilizationNoticeLegacyPolicyEntry(t *testing.T) {
	notice := notifyLowUtilization(t, lowUtilWorkspace(false),
		lowUtilEntry("Detected_1", "low GPU utilization: mode Average, threshold 5%, window 1h, observed max 1.0%; "+
			"the workload will be stopped at 2026-01-02T12:30:00Z if it stays idle"))

	assert.Equal(t, workloadtopic.LowUtilizationNotice{
		Threshold: 5, Window: "1h", StopTime: "2026-01-02 12:30:00 UTC",
		Rules: []string{"low GPU utilization: mode Average, threshold 5%, window 1h, observed max 1.0%"},
	}, *notice)
}

func TestLowUtilizationDetections(t *testing.T) {
	w := ownedTrainingWorkload(v1.WorkloadRunning)
	assert.Nil(t, lowUtilizationDetections(w))

	w.Status.Conditions = []metav1.Condition{
		lowUtilEntry("Detected_1_lt11_30m", "a; x"),
		lowUtilEntry("Detected_x_lt5_1h", "bad index"),
		{Type: string(v1.AdminStopped), Reason: "Detected_9", Message: "other type"},
	}
	assert.Equal(t, []string{"a"}, lowUtilizationDetections(w))

	// The latest episode has recovered: nothing describes the current detection.
	w.Status.Conditions = append(w.Status.Conditions, lowUtilEntry("Recovered_1", "recovered"))
	assert.Nil(t, lowUtilizationDetections(w))
}

func TestOnUpdateSkipsLowUtilizationDryRun(t *testing.T) {
	got := captureNotifications(t)
	w := newInformer(t, notifiedOwner(), lowUtilWorkspace(true))

	oldW := ownedTrainingWorkload(v1.WorkloadRunning)
	newW := oldW.DeepCopy()
	markNotified(newW, "2026-01-02T12:00:00Z", "2026-01-02T12:00:00Z")
	w.OnUpdate(oldW, newW)

	assert.Empty(t, *got)
}

func TestOnUpdateStoppedCarriesStopMessage(t *testing.T) {
	got := captureNotifications(t)
	w := newInformer(t, notifiedOwner())

	oldW := ownedTrainingWorkload(v1.WorkloadRunning)
	newW := ownedTrainingWorkload(v1.WorkloadStopped)
	newW.Status.Conditions = []metav1.Condition{{
		Type: string(v1.AdminStopped), Status: metav1.ConditionTrue, Message: "stopped for low GPU utilization",
	}}
	w.OnUpdate(oldW, newW)

	assert.Len(t, *got, 1)
	assert.Equal(t, "wl1-Stopped", (*got)[0].uid)
	assert.Equal(t, "stopped for low GPU utilization", (*got)[0].data["message"])
}

func TestOnAdd(t *testing.T) {
	w := newInformer(t)
	// Wrong type -> no panic.
	w.OnAdd("not-a-workload", false)
	// In initial list -> skipped.
	w.OnAdd(&v1.Workload{Status: v1.WorkloadStatus{Phase: v1.WorkloadRunning}}, true)
	// Empty phase -> skipped.
	w.OnAdd(&v1.Workload{}, false)
	// Phase set, user not found -> handled gracefully.
	w.OnAdd(&v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1"}, Status: v1.WorkloadStatus{Phase: v1.WorkloadRunning}}, false)
}

func TestOnUpdate(t *testing.T) {
	w := newInformer(t)
	// Wrong new type.
	w.OnUpdate(&v1.Workload{}, "bad")
	// Wrong old type.
	w.OnUpdate("bad", &v1.Workload{Status: v1.WorkloadStatus{Phase: v1.WorkloadRunning}})
	// Empty phase.
	w.OnUpdate(&v1.Workload{}, &v1.Workload{})
	// Same phase -> skipped.
	same := &v1.Workload{Status: v1.WorkloadStatus{Phase: v1.WorkloadRunning}}
	w.OnUpdate(same, same.DeepCopy())
	// Phase changed -> reaches submit (user not found -> graceful).
	oldW := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1"}, Status: v1.WorkloadStatus{Phase: v1.WorkloadPending}}
	newW := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1"}, Status: v1.WorkloadStatus{Phase: v1.WorkloadRunning}}
	w.OnUpdate(oldW, newW)
}

// The cleanup of a conversion cache sends no workload mail and looks up no user.
func TestConversionCacheCleanupsGetNoNotification(t *testing.T) {
	s := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(s))
	lookups := 0
	w := NewWorkloadInformer(ctrlfake.NewClientBuilder().WithScheme(s).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c ctrlclient.WithWatch, key ctrlclient.ObjectKey, obj ctrlclient.Object,
			opts ...ctrlclient.GetOption) error {
			lookups++
			return c.Get(ctx, key, obj, opts...)
		},
	}).Build())
	running := v1.WorkloadStatus{Phase: v1.WorkloadRunning}
	w.submitWorkloadNotification(&v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1",
		Labels: map[string]string{v1.PostTrainingConvertCacheLabel: "c1"}}, Status: running})
	assert.Zero(t, lookups, "the cleanup stops before looking up whom to notify")
	w.submitWorkloadNotification(&v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1",
		Labels: map[string]string{v1.PostTrainingRunIdLabel: "pt-1"}}, Status: running})
	assert.Equal(t, 1, lookups, "other workloads, those of a Post Training run among them, look up their user")
}

func TestOnDelete(t *testing.T) {
	w := newInformer(t)
	w.OnDelete(&v1.Workload{})
	assert.NotNil(t, w)
}
