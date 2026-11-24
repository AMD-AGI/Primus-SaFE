/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/adopt"
)

func TestAdoptedClusterPredicateIgnoresUnrelatedUpdates(t *testing.T) {
	p := adoptedClusterPredicate()
	old := testCluster("c1")
	v1.SetAnnotation(old, v1.ClusterAdoptAnnotation, v1.TrueStr)
	old.Generation = 1
	old.Status.ControlPlaneStatus.Phase = v1.ReadyPhase

	touched := old.DeepCopy()
	v1.SetAnnotation(touched, "example", "1")
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: touched}))

	edited := old.DeepCopy()
	edited.Generation = 2
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: edited}))

	pending := old.DeepCopy()
	pending.Status.ControlPlaneStatus.Phase = v1.PendingPhase
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: pending, ObjectNew: old}))

	requested := old.DeepCopy()
	v1.SetAnnotation(requested, v1.ClusterAdoptWorkloadsAnnotation, v1.TrueStr)
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: requested}))
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: requested, ObjectNew: old}))
}

func TestClusterAdoptReconcileImportsWorkloadsOnlyWhenRequested(t *testing.T) {
	cluster := testCluster("c1")
	v1.SetAnnotation(cluster, v1.ClusterAdoptAnnotation, v1.TrueStr)
	r := &ClusterAdoptReconciler{ClusterReconciler: newPlaneReconciler(t, cluster)}
	imports := 0
	patches := gomonkey.ApplyPrivateMethod(reflect.TypeOf(&ClusterReconciler{}), "importAdoptedWorkloads",
		func(_ *ClusterReconciler, _ context.Context, _ *v1.Cluster) error { imports++; return nil })
	defer patches.Reset()
	req := ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "c1"}}

	_, err := r.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, 0, imports)

	got := &v1.Cluster{}
	assert.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "c1"}, got))
	v1.SetAnnotation(got, v1.ClusterAdoptWorkloadsAnnotation, v1.TrueStr)
	assert.NoError(t, r.Update(context.Background(), got))
	_, err = r.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, 1, imports)
}

func TestImportAdoptedWorkloadsRemovesRequest(t *testing.T) {
	cluster := testCluster("c1")
	v1.SetAnnotation(cluster, v1.ClusterAdoptWorkloadsAnnotation, v1.TrueStr)
	r := newClusterReconcilerFull(t, k8sfake.NewSimpleClientset(), cluster)

	assert.NoError(t, r.importAdoptedWorkloads(context.Background(), cluster))
	got := &v1.Cluster{}
	assert.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "c1"}, got))
	assert.False(t, v1.HasAnnotation(got, v1.ClusterAdoptWorkloadsAnnotation))
}

func TestImportPlannedWorkloadsSkipsNotImportable(t *testing.T) {
	r := newClusterReconcilerFull(t, k8sfake.NewSimpleClientset())
	user := &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "root-id"}}
	local := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w1"}, Spec: v1.WorkloadSpec{Workspace: "gone"}}
	plans := []adopt.Plan{
		{Workload: local, User: user, Action: adopt.ActionMatched},
		{Workload: &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w2"}}, Action: adopt.ActionUnscheduled},
	}
	assert.NoError(t, r.importPlannedWorkloads(context.Background(), "c1", plans))
}

func TestClusterAdoptReconcileRequeuesPeriodically(t *testing.T) {
	cluster := testCluster("c1")
	v1.SetAnnotation(cluster, v1.ClusterAdoptAnnotation, v1.TrueStr)
	r := &ClusterAdoptReconciler{ClusterReconciler: newPlaneReconciler(t, cluster)}
	result, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "c1"}})
	assert.NoError(t, err)
	assert.Positive(t, result.RequeueAfter)
}

// getClusterMessage reads the stored cluster status message.
func getClusterMessage(t *testing.T, r *ClusterReconciler, name string) string {
	t.Helper()
	got := &v1.Cluster{}
	assert.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: name}, got))
	return got.Status.ControlPlaneStatus.Message
}

func TestSetClusterMessageOwnsPrefix(t *testing.T) {
	cluster := testCluster("c1")
	r := newPlaneReconciler(t, cluster)
	ctx := context.Background()

	assert.NoError(t, r.setClusterMessage(ctx, cluster, dataPlaneUnreachableMessage, fmt.Errorf("no reachable apiserver endpoint")))
	assert.Equal(t, dataPlaneUnreachableMessage+"no reachable apiserver endpoint", getClusterMessage(t, r, "c1"))

	assert.NoError(t, r.setClusterMessage(ctx, cluster, adoptFailedMessage, nil))
	assert.Equal(t, dataPlaneUnreachableMessage+"no reachable apiserver endpoint", getClusterMessage(t, r, "c1"))

	assert.NoError(t, r.setClusterMessage(ctx, cluster, dataPlaneUnreachableMessage, nil))
	assert.Empty(t, getClusterMessage(t, r, "c1"))

	assert.NoError(t, r.setClusterMessage(ctx, cluster, adoptFailedMessage, fmt.Errorf("%s", strings.Repeat("x", 2*maxClusterMessageLength))))
	assert.Len(t, getClusterMessage(t, r, "c1"), maxClusterMessageLength)
}

func TestClusterReconcileRecordsUnreachableDataPlane(t *testing.T) {
	cluster := readyCluster("c1")
	r := newClusterReconcilerFull(t, k8sfake.NewSimpleClientset(), cluster)
	var probeErr error = fmt.Errorf("no reachable apiserver endpoint")
	patches := gomonkey.ApplyPrivateMethod(reflect.TypeOf(&ClusterReconciler{}), "guaranteeClientFactory",
		func(_ *ClusterReconciler, _ context.Context, _ *v1.Cluster) error { return probeErr })
	defer patches.Reset()
	req := ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "c1"}}

	_, err := r.Reconcile(context.Background(), req)
	assert.Error(t, err)
	assert.Equal(t, dataPlaneUnreachableMessage+"no reachable apiserver endpoint", getClusterMessage(t, r, "c1"))

	probeErr = nil
	_, _ = r.Reconcile(context.Background(), req)
	assert.Empty(t, getClusterMessage(t, r, "c1"))
}

func TestClusterAdoptReconcileRecordsFailure(t *testing.T) {
	cluster := testCluster("c1")
	v1.SetAnnotation(cluster, v1.ClusterAdoptAnnotation, v1.TrueStr)
	cluster.Status.ControlPlaneStatus.Message = dataPlaneUnreachableMessage + "timeout"
	r := &ClusterAdoptReconciler{ClusterReconciler: newPlaneReconciler(t, cluster)}
	var adoptErr error = fmt.Errorf("adopted cluster c1: %w", errAdoptedClientNotReady)
	patches := gomonkey.ApplyPrivateMethod(reflect.TypeOf(&ClusterReconciler{}), "adoptClusterContents",
		func(_ *ClusterReconciler, _ context.Context, _ *v1.Cluster) error { return adoptErr })
	defer patches.Reset()
	req := ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "c1"}}

	_, err := r.Reconcile(context.Background(), req)
	assert.Error(t, err)
	assert.Equal(t, dataPlaneUnreachableMessage+"timeout", getClusterMessage(t, r.ClusterReconciler, "c1"))

	adoptErr = fmt.Errorf("authorize workers failed: 10.0.0.2")
	_, err = r.Reconcile(context.Background(), req)
	assert.Error(t, err)
	assert.Equal(t, adoptFailedMessage+"authorize workers failed: 10.0.0.2", getClusterMessage(t, r.ClusterReconciler, "c1"))

	adoptErr = nil
	_, err = r.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	assert.Empty(t, getClusterMessage(t, r.ClusterReconciler, "c1"))
}
