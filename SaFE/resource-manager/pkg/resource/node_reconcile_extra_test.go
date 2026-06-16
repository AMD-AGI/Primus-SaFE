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
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
)

func TestNodeReconcileNotFound(t *testing.T) {
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	r := newMockNodeReconciler(cl)
	res, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "missing"}})
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}

func TestNodeReconcileNoCluster(t *testing.T) {
	// Node without cluster: getK8sNode returns nothing, reconcile completes with a requeue.
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(node).WithStatusSubresource(node).Build()
	r := newMockNodeReconciler(cl)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "n1"}})
	assert.NoError(t, err)
}

func TestNodeReconcileDelete(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:       "n1",
		Finalizers: []string{v1.NodeFinalizer},
	}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(node).WithStatusSubresource(node).Build()
	// Trigger deletion (object keeps existing because of finalizer).
	assert.NoError(t, cl.Delete(context.Background(), node))
	r := newMockNodeReconciler(cl)
	_, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: "n1"}})
	assert.NoError(t, err)
	// Finalizer removed -> node fully gone.
	err = cl.Get(context.Background(), client.ObjectKey{Name: "n1"}, &v1.Node{})
	assert.Error(t, err)
}

func TestNodeDeleteK8sNodeNoCluster(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	r := newMockNodeReconciler(cl)
	res, err := r.deleteK8sNode(context.Background(), node)
	assert.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
}
