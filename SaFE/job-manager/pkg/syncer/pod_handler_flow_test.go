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
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestRemoveWorkloadPodEmptyId(t *testing.T) {
	r := &SyncerReconciler{}
	err := r.removeWorkloadPod(context.Background(), &resourceMessage{})
	assert.NilError(t, err)
}

func TestRemoveWorkloadPodNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).Build()
	r := &SyncerReconciler{Client: cl}
	err := r.removeWorkloadPod(context.Background(), &resourceMessage{workloadId: "missing", name: "p"})
	assert.NilError(t, err)
}

func TestRemoveWorkloadPodEnded(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w"}}
	w.Status.Phase = v1.WorkloadFailed
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(w).Build()
	r := &SyncerReconciler{Client: cl}
	err := r.removeWorkloadPod(context.Background(), &resourceMessage{workloadId: "w", name: "p"})
	assert.NilError(t, err)
}

func TestRemoveWorkloadPodNotInList(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:        "w",
		Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: "true"},
	}}
	w.Spec.MaxRetry = 3
	w.Status.Pods = []v1.WorkloadPod{{PodId: "other"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).WithObjects(w).Build()
	r := &SyncerReconciler{Client: cl}
	err := r.removeWorkloadPod(context.Background(),
		&resourceMessage{workloadId: "w", name: "p", dispatchCount: 1})
	assert.NilError(t, err)
}

func TestRemoveWorkloadPodRemoves(t *testing.T) {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:        "w",
		Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: "true"},
	}}
	w.Spec.MaxRetry = 3
	w.Status.Pods = []v1.WorkloadPod{{PodId: "p1"}, {PodId: "p2"}}
	cl := ctrlfake.NewClientBuilder().
		WithScheme(syncerScheme(t)).
		WithObjects(w).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	r := &SyncerReconciler{Client: cl}
	err := r.removeWorkloadPod(context.Background(),
		&resourceMessage{workloadId: "w", name: "p1", dispatchCount: 1})
	assert.NilError(t, err)

	got := &v1.Workload{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "w"}, got))
	assert.Equal(t, len(got.Status.Pods), 1)
	assert.Equal(t, got.Status.Pods[0].PodId, "p2")
}
