/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func ttlScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func intPtr(i int) *int { return &i }

func TestWorkloadTTLChangePredicateCreate(t *testing.T) {
	p := WorkloadTTLChangePredicate{}

	// Ended workload -> true.
	ended := &v1.Workload{}
	ended.Status.Phase = v1.WorkloadFailed
	assert.Equal(t, p.Create(event.CreateEvent{Object: ended}), true)

	// Timeout set -> true.
	tw := &v1.Workload{}
	tw.Spec.Timeout = intPtr(60)
	assert.Equal(t, p.Create(event.CreateEvent{Object: tw}), true)

	// Neither -> false.
	assert.Equal(t, p.Create(event.CreateEvent{Object: &v1.Workload{}}), false)

	// Wrong type -> false.
	assert.Equal(t, p.Create(event.CreateEvent{Object: &corev1.Pod{}}), false)
}

func TestWorkloadTTLChangePredicateUpdate(t *testing.T) {
	p := WorkloadTTLChangePredicate{}

	oldW := &v1.Workload{}
	newW := &v1.Workload{}
	newW.Status.Phase = v1.WorkloadFailed
	assert.Equal(t, p.Update(event.UpdateEvent{ObjectOld: oldW, ObjectNew: newW}), true)

	// Timeout value changed -> true.
	o2 := &v1.Workload{}
	n2 := &v1.Workload{}
	n2.Spec.Timeout = intPtr(30)
	assert.Equal(t, p.Update(event.UpdateEvent{ObjectOld: o2, ObjectNew: n2}), true)

	// No relevant change -> false.
	assert.Equal(t, p.Update(event.UpdateEvent{ObjectOld: &v1.Workload{}, ObjectNew: &v1.Workload{}}), false)

	// Wrong types -> false.
	assert.Equal(t, p.Update(event.UpdateEvent{ObjectOld: &corev1.Pod{}, ObjectNew: &corev1.Pod{}}), false)
}

func TestTTLReconcileNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &WorkloadTTLController{Client: cl}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{
		NamespacedName: ctrlclient.ObjectKey{Name: "missing"},
	})
	assert.NilError(t, err)
}

func TestTTLHandleEndedExpiredDeletes(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Status.Phase = v1.WorkloadFailed
	w.Status.EndTime = &metav1.Time{Time: time.Now().Add(-100 * time.Second)}
	w.Spec.TTLSecondsAfterFinished = intPtr(10)
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(w).Build()
	r := &WorkloadTTLController{Client: cl}

	res, err := r.handle(context.Background(), w)
	assert.NilError(t, err)
	assert.Equal(t, res.RequeueAfter, time.Duration(0))

	// Workload should be deleted.
	got := &v1.Workload{}
	gErr := cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, got)
	assert.Assert(t, gErr != nil)
}

func TestTTLHandleEndedNotExpiredRequeues(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Status.Phase = v1.WorkloadFailed
	w.Status.EndTime = &metav1.Time{Time: time.Now()}
	w.Spec.TTLSecondsAfterFinished = intPtr(3600)
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(w).Build()
	r := &WorkloadTTLController{Client: cl}

	res, err := r.handle(context.Background(), w)
	assert.NilError(t, err)
	assert.Assert(t, res.RequeueAfter > 0)
}

func TestTTLHandleTimeoutStopsAndDeletes(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Timeout = intPtr(1)
	w.Status.StartTime = &metav1.Time{Time: time.Now().Add(-10 * time.Second)}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(ttlScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &WorkloadTTLController{Client: cl}

	_, err := r.handle(context.Background(), w)
	assert.NilError(t, err)
}

func TestTTLHandleStartTimeNil(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Timeout = intPtr(60)
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(w).Build()
	r := &WorkloadTTLController{Client: cl}

	res, err := r.handle(context.Background(), w)
	assert.NilError(t, err)
	assert.Equal(t, res.RequeueAfter, time.Duration(0))
}

func TestTTLHandlePendingTimeoutRequeues(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Timeout = intPtr(3600)
	w.Status.StartTime = &metav1.Time{Time: time.Now()}
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).WithObjects(w).Build()
	r := &WorkloadTTLController{Client: cl}

	res, err := r.handle(context.Background(), w)
	assert.NilError(t, err)
	assert.Assert(t, res.RequeueAfter > 0)
}

func TestTTLDeleteWorkloadNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(ttlScheme(t)).Build()
	r := &WorkloadTTLController{Client: cl}
	// Deleting a non-existent workload is treated as success (IgnoreNotFound).
	err := r.deleteWorkload(context.Background(), &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "ghost"}})
	assert.NilError(t, err)
}
