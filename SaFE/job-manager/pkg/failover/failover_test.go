/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package failover

import (
	"context"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

func failoverScheme(t *testing.T) *runtime.Scheme {
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

// dispatchedWorkload returns a workload marked dispatched with a positive max retry.
func dispatchedWorkload(name string) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:        name,
		Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: "true"},
	}}
	w.Spec.MaxRetry = 3
	return w
}

func TestIsDisableFailover(t *testing.T) {
	// Disabled via annotation.
	w := dispatchedWorkload("w")
	w.Annotations[v1.WorkloadDisableFailoverAnnotation] = v1.TrueStr
	assert.Equal(t, isDisableFailover(w), true)

	// Not dispatched -> disabled.
	w2 := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w2"}}
	w2.Spec.MaxRetry = 3
	assert.Equal(t, isDisableFailover(w2), true)

	// Ended workload -> disabled.
	w3 := dispatchedWorkload("w3")
	w3.Status.Phase = v1.WorkloadFailed
	assert.Equal(t, isDisableFailover(w3), true)

	// MaxRetry <= 0 -> disabled.
	w4 := dispatchedWorkload("w4")
	w4.Spec.MaxRetry = 0
	assert.Equal(t, isDisableFailover(w4), true)

	// Preempted overrides retry limits -> not disabled.
	w5 := dispatchedWorkload("w5")
	w5.Spec.MaxRetry = 0
	w5.Annotations[v1.WorkloadPreemptedAnnotation] = "true"
	assert.Equal(t, isDisableFailover(w5), false)

	// Normal dispatched workload -> not disabled.
	w6 := dispatchedWorkload("w6")
	assert.Equal(t, isDisableFailover(w6), false)
}

func TestIsFailoverNeeded(t *testing.T) {
	// Disabled -> not needed.
	w := dispatchedWorkload("w")
	w.Annotations[v1.WorkloadDisableFailoverAnnotation] = v1.TrueStr
	assert.Equal(t, isFailoverNeeded(w), false)

	// Preempted -> needed.
	w2 := dispatchedWorkload("w2")
	w2.Annotations[v1.WorkloadPreemptedAnnotation] = "true"
	assert.Equal(t, isFailoverNeeded(w2), true)

	// Failed condition -> needed.
	w3 := dispatchedWorkload("w3")
	w3.Status.Conditions = []metav1.Condition{{
		Type:   string(v1.K8sFailed),
		Reason: commonworkload.GenerateDispatchReason(0),
	}}
	assert.Equal(t, isFailoverNeeded(w3), true)

	// Dispatched but nothing wrong -> not needed.
	w4 := dispatchedWorkload("w4")
	assert.Equal(t, isFailoverNeeded(w4), false)
}

func TestRelevantChangePredicate(t *testing.T) {
	p := relevantChangePredicate{}

	// Create with a non-workload object -> false.
	assert.Equal(t, p.Create(event.CreateEvent{Object: &corev1.Pod{}}), false)

	// Create with a workload needing failover -> true.
	needed := dispatchedWorkload("w")
	needed.Annotations[v1.WorkloadPreemptedAnnotation] = "true"
	assert.Equal(t, p.Create(event.CreateEvent{Object: needed}), true)

	// Create with a workload not needing failover -> false.
	assert.Equal(t, p.Create(event.CreateEvent{Object: dispatchedWorkload("w2")}), false)

	// Update transitioning into needing failover -> true.
	oldW := dispatchedWorkload("w3")
	newW := dispatchedWorkload("w3")
	newW.Annotations[v1.WorkloadPreemptedAnnotation] = "true"
	assert.Equal(t, p.Update(event.UpdateEvent{ObjectOld: oldW, ObjectNew: newW}), true)

	// Update with wrong types -> false.
	assert.Equal(t, p.Update(event.UpdateEvent{ObjectOld: &corev1.Pod{}, ObjectNew: &corev1.Pod{}}), false)
}

func TestAddFailoverConditionAlreadyExists(t *testing.T) {
	r := &FailoverReconciler{}
	w := dispatchedWorkload("w")
	// Pre-add the exact condition that addFailoverCondition would create.
	reason := commonworkload.GenerateDispatchReason(0)
	w.Status.Conditions = []metav1.Condition{{
		Type:   string(v1.AdminFailover),
		Reason: reason,
	}}
	err := r.addFailoverCondition(context.Background(), w, "msg")
	assert.NilError(t, err)
}

func TestAddFailoverConditionPatch(t *testing.T) {
	w := dispatchedWorkload("w")
	cl := ctrlfake.NewClientBuilder().
		WithScheme(failoverScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &FailoverReconciler{Client: cl}
	err := r.addFailoverCondition(context.Background(), w, "doing failover")
	assert.NilError(t, err)
}

func TestGetWorkloadsOnFaultNodeNodeNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(failoverScheme(t)).Build()
	r := &FailoverReconciler{Client: cl}
	fault := &v1.Fault{}
	fault.Spec.Node = &v1.FaultNode{AdminName: "missing-node"}
	_, err := r.getWorkloadsOnFaultNode(context.Background(), nil, fault)
	assert.Assert(t, err != nil)
}

func TestReconcileNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(failoverScheme(t)).Build()
	r := &FailoverReconciler{Client: cl}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{
		NamespacedName: ctrlclient.ObjectKey{Name: "missing"},
	})
	assert.NilError(t, err)
}

func TestReconcileDisabled(t *testing.T) {
	// Not-dispatched workload is disabled -> Reconcile returns without failover.
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(failoverScheme(t)).WithObjects(w).Build()
	r := &FailoverReconciler{Client: cl}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{
		NamespacedName: ctrlclient.ObjectKey{Name: "w"},
	})
	assert.NilError(t, err)
}

func TestHandleNoClusterClientSets(t *testing.T) {
	w := dispatchedWorkload("w")
	cl := ctrlfake.NewClientBuilder().WithScheme(failoverScheme(t)).WithObjects(w).Build()
	r := &FailoverReconciler{
		Client:            cl,
		clusterClientSets: commonutils.NewObjectManager(),
	}
	res, err := r.handle(context.Background(), w)
	assert.NilError(t, err)
	// No cluster client sets -> requeue requested.
	assert.Assert(t, res.RequeueAfter > 0)
}

func TestHandleConfigmapEvent(t *testing.T) {
	r := &FailoverReconciler{failoverConfigs: commonutils.NewObjectManager()}
	h := r.handleConfigmapEvent()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: common.PrimusFailover, Namespace: common.PrimusSafeNamespace},
		Data:       map[string]string{"k": `{"id":"mon1"}`},
	}
	// Create with the failover configmap registers the config.
	h.Create(context.Background(), event.CreateEvent{Object: cm}, nil)
	assert.Equal(t, isMonitorIdExists(r.failoverConfigs, "mon1"), true)

	// Create with an unrelated configmap is ignored.
	other := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "default"}}
	h.Create(context.Background(), event.CreateEvent{Object: other}, nil)

	// Delete clears the config.
	h.Delete(context.Background(), event.DeleteEvent{Object: cm}, nil)
	assert.Equal(t, isMonitorIdExists(r.failoverConfigs, "mon1"), false)
}

func TestHandleFaultEventIgnoresIrrelevant(t *testing.T) {
	r := &FailoverReconciler{failoverConfigs: commonutils.NewObjectManager()}
	h := r.handleFaultEvent()

	// Fault not succeeded -> check fails -> early return (no queue use).
	fault := &v1.Fault{}
	fault.Status.Phase = v1.FaultPhaseFailed
	h.Create(context.Background(), event.CreateEvent{Object: fault}, nil)

	// Wrong object type -> early return.
	h.Create(context.Background(), event.CreateEvent{Object: &corev1.Pod{}}, nil)
}
