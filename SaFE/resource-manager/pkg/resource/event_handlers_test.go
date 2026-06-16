/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func resWorkQueue() v1.RequestWorkQueue {
	return workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
}

type genericEventHandler interface {
	Create(context.Context, event.CreateEvent, v1.RequestWorkQueue)
	Update(context.Context, event.UpdateEvent, v1.RequestWorkQueue)
}

func TestFaultHandleNodeEvent(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Spec.Cluster = pointerStr("c1")
	r := newFaultReconciler(t, node)
	h := r.handleNodeEvent().(genericEventHandler)
	q := resWorkQueue()
	defer q.ShutDown()
	h.Create(context.Background(), event.CreateEvent{Object: node}, q)
	// Update: node loses cluster -> delete faults path.
	newNode := node.DeepCopy()
	newNode.Spec.Cluster = nil
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: node, ObjectNew: newNode}, q)
}

func TestFaultHandleConfigmapEvent(t *testing.T) {
	r := newFaultReconciler(t)
	h := r.handleConfigmapEvent()
	q := resWorkQueue()
	defer q.ShutDown()
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: common.PrimusFault, Namespace: common.PrimusSafeNamespace}}
	gh := h.(interface {
		Create(context.Context, event.CreateEvent, v1.RequestWorkQueue)
		Update(context.Context, event.UpdateEvent, v1.RequestWorkQueue)
		Delete(context.Context, event.DeleteEvent, v1.RequestWorkQueue)
	})
	gh.Create(context.Background(), event.CreateEvent{Object: cm}, q)
	gh.Update(context.Background(), event.UpdateEvent{ObjectOld: cm, ObjectNew: cm.DeepCopy()}, q)
	gh.Delete(context.Background(), event.DeleteEvent{Object: cm}, q)
}

func TestWorkspaceHandleNodeEvent(t *testing.T) {
	r := newMockWorkspaceReconciler(nil)
	h := r.handleNodeEvent().(genericEventHandler)
	q := resWorkQueue()
	defer q.ShutDown()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.WorkspaceIdLabel: "ws1"},
	}}
	h.Create(context.Background(), event.CreateEvent{Object: node}, q)
	newNode := node.DeepCopy()
	newNode.Labels[v1.WorkspaceIdLabel] = "ws2"
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: node, ObjectNew: newNode}, q)
}

func TestClusterRelevantChangePredicate(t *testing.T) {
	r := newClusterReconciler(t)
	p := r.relevantChangePredicate()
	ready := readyCluster("c1")
	assert.True(t, p.Create(event.CreateEvent{Object: ready}))
	assert.False(t, p.Create(event.CreateEvent{Object: testCluster("c2")}))
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: testCluster("c1"), ObjectNew: ready}))
	assert.False(t, p.Delete(event.DeleteEvent{Object: ready}))
}

func TestClusterHandleNodeEvent(t *testing.T) {
	r := newClusterReconciler(t)
	h := r.handleNodeEvent().(genericEventHandler)
	q := resWorkQueue()
	defer q.ShutDown()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "n1",
		OwnerReferences: []metav1.OwnerReference{
			{APIVersion: v1.SchemeGroupVersion.String(), Kind: v1.ClusterKind, Name: "c1"},
		},
	}}
	h.Create(context.Background(), event.CreateEvent{Object: node}, q)
	assert.Equal(t, 1, q.Len())
}

func TestClusterEndpointsPredicate(t *testing.T) {
	r := newClusterReconciler(t)
	p := r.endpointsPredicate()
	ep := &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace}}
	assert.True(t, p.Create(event.CreateEvent{Object: ep}))
	assert.True(t, p.Delete(event.DeleteEvent{Object: ep}))
	// Update with no subset change -> false.
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: ep, ObjectNew: ep.DeepCopy()}))
}

func TestClusterHandleEndpointsEvent(t *testing.T) {
	r := newClusterReconciler(t)
	h := r.handleEndpointsEvent()
	ep := &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace}}
	reqs := h.(interface {
		Create(context.Context, event.CreateEvent, v1.RequestWorkQueue)
	})
	q := resWorkQueue()
	defer q.ShutDown()
	reqs.Create(context.Background(), event.CreateEvent{Object: ep}, q)
	assert.Equal(t, 1, q.Len())
}

func TestClusterHandlePodEvent(t *testing.T) {
	r := newClusterReconciler(t)
	h := r.handlePodEvent().(genericEventHandler)
	q := resWorkQueue()
	defer q.ShutDown()
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "p1",
		OwnerReferences: []metav1.OwnerReference{
			{APIVersion: v1.SchemeGroupVersion.String(), Kind: v1.ClusterKind, Name: "c1"},
		},
	}}
	h.Create(context.Background(), event.CreateEvent{Object: pod}, q)
	assert.Equal(t, 1, q.Len())
}

func pointerStr(s string) *string { return &s }
