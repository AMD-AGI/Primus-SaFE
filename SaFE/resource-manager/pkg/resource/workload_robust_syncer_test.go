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
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
)

func TestBuildWorkloadSyncPayload(t *testing.T) {
	wl := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "wl1",
			Labels:            map[string]string{v1.ClusterIdLabel: "c1"},
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: v1.WorkloadSpec{
			Workspace: "ws1",
			Resources: []v1.WorkloadResource{{GPU: "4", Replica: 2}},
		},
		Status: v1.WorkloadStatus{
			Phase:   v1.WorkloadRunning,
			EndTime: &metav1.Time{Time: time.Now()},
		},
	}
	payload := buildWorkloadSyncPayload(wl)
	assert.Equal(t, "wl1", payload.Name)
	assert.Equal(t, "ws1", payload.Workspace)
	assert.Equal(t, 8, payload.GPURequest)
	assert.NotNil(t, payload.CreatedAt)
	assert.NotNil(t, payload.EndAt)
}

func newRobustSyncer(t *testing.T, objs ...client.Object) *WorkloadRobustSyncer {
	t.Helper()
	scheme, _ := genMockScheme()
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return &WorkloadRobustSyncer{Client: cl, robustClient: robustclient.NewClient(robustclient.ClientConfig{})}
}

func TestRobustReconcileNotFound(t *testing.T) {
	r := newRobustSyncer(t)
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestRobustSyncToRobustNoCluster(t *testing.T) {
	r := newRobustSyncer(t)
	// Workload with no cluster -> early return, no panic.
	r.syncToRobust(context.Background(), &v1.Workload{})
}

func TestRobustSyncToRobustClusterNotRegistered(t *testing.T) {
	r := newRobustSyncer(t)
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}}}
	// Cluster not registered in robust client -> ForCluster nil -> early return.
	r.syncToRobust(context.Background(), wl)
}

func TestRobustSyncDeleteToRobust(t *testing.T) {
	r := newRobustSyncer(t)
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}}}
	r.syncDeleteToRobust(context.Background(), wl)
	// No cluster on workload -> also covered.
	r.syncDeleteToRobust(context.Background(), &v1.Workload{})
}

func TestRobustReconcileDeletion(t *testing.T) {
	now := metav1.Now()
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{
		Name:              "wl1",
		DeletionTimestamp: &now,
		Finalizers:        []string{"f"},
		Labels:            map[string]string{v1.ClusterIdLabel: "c1"},
	}}
	r := newRobustSyncer(t, wl)
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "wl1"}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestRobustRunCatchUp(t *testing.T) {
	wl := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "wl1", Labels: map[string]string{v1.ClusterIdLabel: "c1"}},
		Status:     v1.WorkloadStatus{Phase: v1.WorkloadRunning},
	}
	r := newRobustSyncer(t, wl)
	// Lists workloads, batches per cluster, ForCluster nil -> continue. No panic.
	r.runCatchUp(context.Background())
}
