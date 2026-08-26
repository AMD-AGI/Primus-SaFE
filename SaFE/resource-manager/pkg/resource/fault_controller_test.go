/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
)

func newFaultReconciler(t *testing.T, objs ...client.Object) *FaultReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.Fault{}).
		WithObjects(objs...).
		Build()
	return &FaultReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl},
		opt:                   &FaultReconcilerOption{processWait: time.Millisecond, maxRetryCount: 3},
	}
}

func TestGenerateTaintKeyAndVal(t *testing.T) {
	fault := &v1.Fault{Spec: v1.FaultSpec{MonitorId: "m1"}}
	key, val := generateTaintKeyAndVal(fault)
	assert.Equal(t, commonfaults.GenerateTaintKey("m1"), key)
	assert.Equal(t, "", val)
}

func TestFaultReconcileNotFound(t *testing.T) {
	r := newFaultReconciler(t)
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestFaultUpdatePhase(t *testing.T) {
	fault := &v1.Fault{ObjectMeta: metav1.ObjectMeta{Name: "f1"}}
	r := newFaultReconciler(t, fault)
	err := r.updatePhase(context.Background(), fault, v1.FaultPhaseSucceeded)
	assert.NoError(t, err)
	assert.Equal(t, v1.FaultPhaseSucceeded, fault.Status.Phase)
}

func TestFaultTaintAndRemoveNodeTaint(t *testing.T) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Spec:       v1.NodeSpec{Cluster: pointer.String("c1")},
	}
	fault := &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{Name: "f1"},
		Spec: v1.FaultSpec{
			MonitorId: "m1",
			Node:      &v1.FaultNode{AdminName: "n1", ClusterName: "c1"},
		},
	}
	r := newFaultReconciler(t, node, fault)

	// Add taint.
	assert.NoError(t, r.taintNode(context.Background(), fault))
	updated := &v1.Node{}
	assert.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "n1"}, updated))
	assert.NotEmpty(t, updated.Spec.Taints)

	// Tainting again is a no-op.
	assert.NoError(t, r.taintNode(context.Background(), fault))

	// Remove taint.
	assert.NoError(t, r.removeNodeTaint(context.Background(), fault))
}

func TestFaultTaintNodeNoNode(t *testing.T) {
	fault := &v1.Fault{Spec: v1.FaultSpec{MonitorId: "m1"}}
	r := newFaultReconciler(t)
	// No node spec -> nil.
	assert.NoError(t, r.taintNode(context.Background(), fault))
	assert.NoError(t, r.removeNodeTaint(context.Background(), fault))
}

func TestFaultProcessFaultNoAction(t *testing.T) {
	fault := &v1.Fault{ObjectMeta: metav1.ObjectMeta{Name: "f1"}}
	r := newFaultReconciler(t, fault)
	_, err := r.processFault(context.Background(), fault)
	assert.NoError(t, err)
	assert.Equal(t, v1.FaultPhaseSucceeded, fault.Status.Phase)
}

func TestFaultDeleteFaults(t *testing.T) {
	fault := &v1.Fault{ObjectMeta: metav1.ObjectMeta{Name: "f1", Labels: map[string]string{v1.NodeIdLabel: "n1"}}}
	r := newFaultReconciler(t, fault)
	sel := labels.SelectorFromSet(map[string]string{v1.NodeIdLabel: "n1"})
	assert.NoError(t, r.deleteFaults(context.Background(), sel))
	// Fault should be gone.
	assert.Error(t, r.Get(context.Background(), client.ObjectKey{Name: "f1"}, &v1.Fault{}))
}

func TestFaultDelete(t *testing.T) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Spec:       v1.NodeSpec{Cluster: pointer.String("c1")},
	}
	fault := &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{Name: "f1", Finalizers: []string{v1.FaultFinalizer}},
		Spec:       v1.FaultSpec{MonitorId: "m1", Node: &v1.FaultNode{AdminName: "n1", ClusterName: "c1"}},
	}
	r := newFaultReconciler(t, node, fault)
	res, err := r.delete(context.Background(), fault)
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestFaultReconcileProcess(t *testing.T) {
	fault := &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{Name: "f1"},
		Spec:       v1.FaultSpec{MonitorId: "m1"},
	}
	r := newFaultReconciler(t, fault)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "f1"}})
	assert.NoError(t, err)
}

func TestFaultRetry(t *testing.T) {
	fault := &v1.Fault{ObjectMeta: metav1.ObjectMeta{Name: "f1"}}
	r := newFaultReconciler(t, fault)
	res, err := r.retry(context.Background(), fault)
	assert.NoError(t, err)
	assert.True(t, res.RequeueAfter > 0)

	// nil fault -> no result.
	res, err = r.retry(context.Background(), nil)
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}