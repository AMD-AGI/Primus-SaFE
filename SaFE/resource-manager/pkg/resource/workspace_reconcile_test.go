/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
)

func TestWorkspaceRelevantChangePredicate(t *testing.T) {
	r := newMockWorkspaceReconciler(nil)
	p := r.relevantChangePredicate()

	old := &v1.Workspace{}
	upd := &v1.Workspace{}
	// Deletion timestamp set -> true.
	now := metav1.Now()
	upd.DeletionTimestamp = &now
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: upd}))

	// No change -> false.
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: old.DeepCopy()}))
}

func TestWorkspaceGetClientSetOfDataplaneEmpty(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	cs, err := r.getClientSetOfDataplane(context.Background(), "")
	assert.NoError(t, err)
	assert.Nil(t, cs)
}

func TestWorkspaceGetClientSetOfDataplaneClusterMissing(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	_, err := r.getClientSetOfDataplane(context.Background(), "missing")
	assert.Error(t, err)
}

func TestWorkspaceReconcileNotFound(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestWorkspaceReconcileNoCluster(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithStatusSubresource(&v1.Workspace{}).WithObjects(ws).Build()
	r := newMockWorkspaceReconciler(cl)
	// No cluster -> clientSet nil -> nil result.
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "ws1"}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestWorkspaceDelete(t *testing.T) {
	now := metav1.Now()
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{
		Name:              "ws1",
		DeletionTimestamp: &now,
		Finalizers:        []string{v1.WorkspaceFinalizer},
	}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithStatusSubresource(&v1.Workspace{}).WithObjects(ws).Build()
	r := newMockWorkspaceReconciler(cl)
	// No nodes bound, no cluster -> deletes resources + removes finalizer.
	err := r.delete(context.Background(), ws)
	assert.NoError(t, err)
}

func TestWorkspaceUpdatePhase(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithStatusSubresource(&v1.Workspace{}).WithObjects(ws).Build()
	r := newMockWorkspaceReconciler(cl)
	err := r.updatePhase(context.Background(), ws, v1.WorkspaceDeleting)
	assert.NoError(t, err)
	assert.Equal(t, v1.WorkspaceDeleting, ws.Status.Phase)
	// No change -> no-op.
	assert.NoError(t, r.updatePhase(context.Background(), ws, v1.WorkspaceDeleting))
}

func TestWorkspaceGuaranteeDataPlaneResources(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	mockScheme, err := genMockScheme()
	assert.NoError(t, err)
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(mockScheme).Build())
	err = r.guaranteeDataPlaneResources(context.Background(), ws, cs)
	assert.NoError(t, err)
	// Namespace should be created.
	_, err = cs.CoreV1().Namespaces().Get(context.Background(), "ws1", metav1.GetOptions{})
	assert.NoError(t, err)
}

func TestWorkspaceDeleteDataPlaneResourcesNoCluster(t *testing.T) {
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	// No cluster -> clientSet nil -> nil.
	err := r.deleteDataPlaneResources(context.Background(), ws)
	assert.NoError(t, err)
}
