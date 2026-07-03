/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"reflect"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func TestSyncerReconcileNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(syncerScheme(t)).Build()
	r := &SyncerReconciler{Client: cl, clusterClientSets: commonutils.NewObjectManager()}
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{
		NamespacedName: ctrlclient.ObjectKey{Name: "missing"},
	})
	assert.NilError(t, err)
}

func TestSyncerObserve(t *testing.T) {
	mgr := commonutils.NewObjectManager()
	r := &SyncerReconciler{clusterClientSets: mgr}

	// Not present -> false.
	assert.Equal(t, r.observe(&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}), false)

	// Present -> true.
	assert.NilError(t, mgr.Add("c1", newTestClientSets()))
	assert.Equal(t, r.observe(&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}), true)
}

func TestSyncerDeleteClusterClientSet(t *testing.T) {
	mgr := commonutils.NewObjectManager()
	assert.NilError(t, mgr.Add("c1", newTestClientSets()))
	r := &SyncerReconciler{clusterClientSets: mgr}
	r.deleteClusterClientSet("c1")
	_, ok := mgr.Get("c1")
	assert.Equal(t, ok, false)
}

func TestSyncerDoClusterNotFound(t *testing.T) {
	r := &SyncerReconciler{clusterClientSets: commonutils.NewObjectManager()}
	res, err := r.Do(context.Background(), &resourceMessage{cluster: "missing"})
	assert.NilError(t, err)
	// Unknown cluster -> requeue.
	assert.Assert(t, res.RequeueAfter > 0)
}

func TestSyncerDoUnknownKind(t *testing.T) {
	mgr := commonutils.NewObjectManager()
	assert.NilError(t, mgr.Add("c1", newTestClientSets()))
	r := &SyncerReconciler{clusterClientSets: mgr}
	// Cluster present but the gvk kind has no handler -> empty result, no error.
	res, err := r.Do(context.Background(), &resourceMessage{
		cluster: "c1",
		gvk:     schema.GroupVersionKind{Kind: "UnknownKind"},
	})
	assert.NilError(t, err)
	assert.Equal(t, res.RequeueAfter.Nanoseconds(), int64(0))
}

func TestSyncerResourceTemplateHandler(t *testing.T) {
	r := &SyncerReconciler{clusterClientSets: commonutils.NewObjectManager()}
	h := r.resourceTemplateHandler()
	// Wrong type -> no-op.
	h.Create(context.Background(), event.CreateEvent{Object: &corev1.Pod{}}, nil)
	h.Delete(context.Background(), event.DeleteEvent{Object: &corev1.Pod{}}, nil)
	// Valid ResourceTemplate with no cluster client sets -> no-op (GetAll empty).
	rt := &v1.ResourceTemplate{ObjectMeta: metav1.ObjectMeta{Name: "rt"}}
	h.Create(context.Background(), event.CreateEvent{Object: rt}, nil)
	h.Delete(context.Background(), event.DeleteEvent{Object: rt}, nil)
	_ = common.PodKind
}

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

// TestDoRoutesToHandlers patches GetClusterClientSets + handleJob/handlePod so Do's
// routing switch is exercised for both Job-like and Pod kinds.
func TestDoRoutesToHandlers(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	cs := monkeyClientSets()
	patches.ApplyFunc(GetClusterClientSets,
		func(*commonutils.ObjectManager, string) (*ClusterClientSets, error) { return cs, nil })
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "handleJob",
		func(_ *SyncerReconciler, _ context.Context, _ *resourceMessage, _ *ClusterClientSets) (ctrlruntime.Result, error) {
			return ctrlruntime.Result{}, nil
		})
	patches.ApplyPrivateMethod(reflect.TypeOf(&SyncerReconciler{}), "handlePod",
		func(_ *SyncerReconciler, _ context.Context, _ *resourceMessage, _ *ClusterClientSets) (ctrlruntime.Result, error) {
			return ctrlruntime.Result{}, nil
		})

	r := &SyncerReconciler{}
	_, err := r.Do(context.Background(), &resourceMessage{cluster: "c", gvk: schema.GroupVersionKind{Kind: "Job"}})
	assert.NilError(t, err)
	_, err = r.Do(context.Background(), &resourceMessage{cluster: "c", gvk: schema.GroupVersionKind{Kind: "Pod"}})
	assert.NilError(t, err)
}
