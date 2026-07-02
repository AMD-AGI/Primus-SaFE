/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"context"
	"errors"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

func utilsScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestIsUnrecoverableError(t *testing.T) {
	assert.Equal(t, IsUnrecoverableError(nil), false)
	assert.Equal(t, IsUnrecoverableError(commonerrors.NewBadRequest("bad")), true)
	assert.Equal(t, IsUnrecoverableError(commonerrors.NewInternalError("oops")), true)
	assert.Equal(t, IsUnrecoverableError(commonerrors.NewNotFound("kind", "name")), true)
	assert.Equal(t, IsUnrecoverableError(errors.New("transient")), false)
}

func TestFindConditionAndNewCondition(t *testing.T) {
	cond := NewCondition("TypeA", "msg", "reason1")
	assert.Equal(t, cond.Type, "TypeA")
	assert.Equal(t, cond.Reason, "reason1")
	assert.Equal(t, string(cond.Status), string(metav1.ConditionTrue))

	w := &v1.Workload{}
	assert.Assert(t, FindCondition(w, cond) == nil)

	w.Status.Conditions = []metav1.Condition{*cond}
	assert.Assert(t, FindCondition(w, cond) != nil)

	other := NewCondition("TypeB", "m", "r")
	assert.Assert(t, FindCondition(w, other) == nil)
}

func TestSetWorkloadFailed(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(utilsScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	err := SetWorkloadFailed(context.Background(), cl, w, "boom")
	assert.NilError(t, err)
	assert.Equal(t, w.Status.Phase, v1.WorkloadFailed)
	assert.Assert(t, w.Status.EndTime != nil)
	assert.Assert(t, len(w.Status.Conditions) > 0)
}

// TestUpdateWorkloadStatusWithRetryPreservesConcurrentConditions verifies that a
// status write does not drop a condition added concurrently by another
// controller (e.g. failover's AdminFailover), while still applying the caller's
// own status.
func TestUpdateWorkloadStatusWithRetryPreservesConcurrentConditions(t *testing.T) {
	stored := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	// A condition written concurrently by another controller, present in etcd but
	// not in the caller's in-memory object.
	stored.Status.Conditions = []metav1.Condition{*NewCondition("AdminFailover", "node failed", "r1")}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(utilsScheme(t)).
		WithObjects(stored).
		WithStatusSubresource(&v1.Workload{}).
		Build()

	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Status.Phase = v1.WorkloadRunning
	w.Status.Conditions = []metav1.Condition{*NewCondition("K8sRunning", "running", "dispatch-1")}

	err := UpdateWorkloadStatusWithRetry(context.Background(), cl, w)
	assert.NilError(t, err)

	got := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, got))
	// Syncer-owned status is applied.
	assert.Equal(t, got.Status.Phase, v1.WorkloadRunning)
	// Both the caller's condition and the concurrently-added one survive.
	assert.Assert(t, FindCondition(got, NewCondition("K8sRunning", "", "dispatch-1")) != nil)
	assert.Assert(t, FindCondition(got, NewCondition("AdminFailover", "", "r1")) != nil)
}

// TestUpdateWorkloadStatusWithRetryIdempotent verifies that applying the same
// computed status repeatedly does not duplicate or grow conditions (merge is by
// (Type, Reason)), and that a concurrently-added condition stays exactly once.
func TestUpdateWorkloadStatusWithRetryIdempotent(t *testing.T) {
	stored := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	stored.Status.Conditions = []metav1.Condition{*NewCondition("AdminFailover", "node failed", "r1")}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(utilsScheme(t)).
		WithObjects(stored).
		WithStatusSubresource(&v1.Workload{}).
		Build()

	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Status.Phase = v1.WorkloadRunning
	w.Status.Conditions = []metav1.Condition{*NewCondition("K8sRunning", "running", "dispatch-1")}

	assert.NilError(t, UpdateWorkloadStatusWithRetry(context.Background(), cl, w))
	first := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, first))
	assert.Equal(t, len(first.Status.Conditions), 2)

	// Re-apply the identical computed status; conditions must stay at 2.
	assert.NilError(t, UpdateWorkloadStatusWithRetry(context.Background(), cl, w))
	second := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, second))
	assert.Equal(t, len(second.Status.Conditions), 2)
	assert.Equal(t, second.Status.Phase, v1.WorkloadRunning)
}

func TestMarkWorkloadStopped(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(utilsScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	err := MarkWorkloadStopped(context.Background(), cl, w, StopReasonTimeout, "timed out")
	assert.NilError(t, err)
	assert.Equal(t, w.Status.Phase, v1.WorkloadStopped)
}

func TestMarkWorkloadStoppedAlreadyStopped(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Status.Phase = v1.WorkloadStopped
	// No client interaction expected since it is already stopped.
	err := MarkWorkloadStopped(context.Background(), nil, w, StopReasonManual, "noop")
	assert.NilError(t, err)
}

func TestSetWorkloadTimeout(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(utilsScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	err := SetWorkloadTimeout(context.Background(), cl, w, "timeout")
	assert.NilError(t, err)
	assert.Equal(t, w.Status.Phase, v1.WorkloadStopped)
}

func TestFindFailedCondition(t *testing.T) {
	w := &v1.Workload{}
	assert.Equal(t, FindFailedCondition(w), false)
}

func TestIsWorkloadOrPod(t *testing.T) {
	for _, kind := range []string{"Pod", "Deployment", "StatefulSet", "Job", "MonarchMesh"} {
		assert.Equal(t, isWorkloadOrPod(schema.GroupVersionKind{Kind: kind}), true)
	}
	assert.Equal(t, isWorkloadOrPod(schema.GroupVersionKind{Kind: "ConfigMap"}), false)
}

func TestK8sObjectStatusIsPending(t *testing.T) {
	assert.Equal(t, (&K8sObjectStatus{Phase: ""}).IsPending(), true)
	assert.Equal(t, (&K8sObjectStatus{Phase: string(v1.K8sPending)}).IsPending(), true)
	assert.Equal(t, (&K8sObjectStatus{Phase: "Running"}).IsPending(), false)
}

func TestNestedBool(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"enabled": true,
			"wrong":   "notbool",
		},
	}
	v, found, err := NestedBool(obj, []string{"spec", "enabled"})
	assert.NilError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, v, true)

	// Missing path -> not found.
	_, found, err = NestedBool(obj, []string{"spec", "missing"})
	assert.NilError(t, err)
	assert.Equal(t, found, false)

	// Wrong type -> error.
	_, _, err = NestedBool(obj, []string{"spec", "wrong"})
	assert.Assert(t, err != nil)
}

func TestGetLabels(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{"app": "x"},
		},
	}}
	labels, err := GetLabels(u, v1.ResourceSpec{})
	assert.NilError(t, err)
	assert.Equal(t, labels["app"], "x")

	// No labels present -> nil, no error.
	u2 := &unstructured.Unstructured{Object: map[string]interface{}{"metadata": map[string]interface{}{}}}
	labels2, err := GetLabels(u2, v1.ResourceSpec{})
	assert.NilError(t, err)
	assert.Assert(t, labels2 == nil)
}
