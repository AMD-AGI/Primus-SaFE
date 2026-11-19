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

func TestOnUpdateNotifiesLowUtilizationOnce(t *testing.T) {
	got := captureNotifications(t)
	w := newInformer(t, notifiedOwner(), lowUtilWorkspace(false))

	oldW := ownedTrainingWorkload(v1.WorkloadRunning)
	newW := oldW.DeepCopy()
	v1.SetAnnotation(newW, v1.WorkloadLowUtilDetectedAtAnnotation, "2026-01-02T12:00:00Z")
	w.OnUpdate(oldW, newW)
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
}

func TestOnUpdateSkipsLowUtilizationDryRun(t *testing.T) {
	got := captureNotifications(t)
	w := newInformer(t, notifiedOwner(), lowUtilWorkspace(true))

	oldW := ownedTrainingWorkload(v1.WorkloadRunning)
	newW := oldW.DeepCopy()
	v1.SetAnnotation(newW, v1.WorkloadLowUtilDetectedAtAnnotation, "2026-01-02T12:00:00Z")
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

func TestOnDelete(t *testing.T) {
	w := newInformer(t)
	w.OnDelete(&v1.Workload{})
	assert.NotNil(t, w)
}
