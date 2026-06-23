/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestGetAdminWorkload(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(w).Build()
	r := &SyncerReconciler{Client: cl}

	got, err := r.getAdminWorkload(context.Background(), "w")
	assert.NilError(t, err)
	assert.Assert(t, got != nil)

	// Missing workload -> (nil, nil).
	got2, err := r.getAdminWorkload(context.Background(), "missing")
	assert.NilError(t, err)
	assert.Assert(t, got2 == nil)
}

func TestHandleJobWorkloadNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).Build()
	r := &SyncerReconciler{Client: cl}
	_, err := r.handleJob(context.Background(), &resourceMessage{workloadId: "missing"}, nil)
	assert.NilError(t, err)
}

func TestHandleJobNamespaceMismatch(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Workspace = "ws"
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(w).Build()
	r := &SyncerReconciler{Client: cl}
	res, err := r.handleJob(context.Background(), &resourceMessage{workloadId: "w", namespace: "other"}, nil)
	assert.NilError(t, err)
	assert.Equal(t, res.RequeueAfter.Nanoseconds(), int64(0))
}

func TestHandleJobNotDispatched(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Spec.Workspace = "ws"
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(w).Build()
	r := &SyncerReconciler{Client: cl}
	res, err := r.handleJob(context.Background(),
		&resourceMessage{workloadId: "w", namespace: "ws", gvk: schema.GroupVersionKind{Kind: "Job"}}, nil)
	assert.NilError(t, err)
	// Not dispatched -> requeue after a second.
	assert.Assert(t, res.RequeueAfter > 0)
}

func TestGetK8sObjectStatusDeleted(t *testing.T) {
	r := &SyncerReconciler{}
	msg := &resourceMessage{
		action: ResourceDel,
		name:   "obj",
		gvk:    schema.GroupVersionKind{Kind: "Job"},
	}
	status, err := r.getK8sObjectStatus(context.Background(), msg, nil, &v1.Workload{})
	assert.NilError(t, err)
	assert.Assert(t, status != nil)
	assert.Equal(t, status.Phase, string(v1.K8sDeleted))
}
